package main

import (
	"bufio"
	"fmt"
	"io"
	"iter"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"git.sr.ht/~rockorager/vaxis"
)

func shellSplit(s string) ([]string, error) {
	var words []string
	var word strings.Builder
	inWord := false
	i := 0

	for i < len(s) {
		ch := s[i]

		switch {
		case ch == '\'':
			inWord = true
			i++
			for i < len(s) && s[i] != '\'' {
				word.WriteByte(s[i])
				i++
			}
			if i >= len(s) {
				return nil, fmt.Errorf("unterminated single quote")
			}
			i++

		case ch == '"':
			inWord = true
			i++
			for i < len(s) && s[i] != '"' {
				if s[i] == '\\' && i+1 < len(s) {
					next := s[i+1]
					if next == '"' || next == '\\' {
						word.WriteByte(next)
						i += 2
						continue
					}
				}
				word.WriteByte(s[i])
				i++
			}
			if i >= len(s) {
				return nil, fmt.Errorf("unterminated double quote")
			}
			i++

		case ch == ' ' || ch == '\t':
			if inWord {
				words = append(words, word.String())
				word.Reset()
				inWord = false
			}
			i++

		default:
			inWord = true
			word.WriteByte(ch)
			i++
		}
	}

	if inWord {
		words = append(words, word.String())
	}

	return words, nil
}

func parseSpanLine(s string, lines []string) (Span, error) {
	parts := strings.SplitN(s, ":", 3)
	if len(parts) != 3 {
		return Span{}, fmt.Errorf("invalid span format %q: expected LINE:START:END", s)
	}

	lineNum, err := strconv.Atoi(parts[0])
	if err != nil {
		return Span{}, fmt.Errorf("invalid line number %q: %w", parts[0], err)
	}
	if lineNum < 1 || lineNum > len(lines) {
		return Span{}, fmt.Errorf("line number %d out of range [1, %d]", lineNum, len(lines))
	}

	start, err := strconv.Atoi(parts[1])
	if err != nil {
		return Span{}, fmt.Errorf("invalid start offset %q: %w", parts[1], err)
	}

	end, err := strconv.Atoi(parts[2])
	if err != nil {
		return Span{}, fmt.Errorf("invalid end offset %q: %w", parts[2], err)
	}

	lineIdx := lineNum - 1
	line := lines[lineIdx]

	if start < 0 || start >= len(line) {
		return Span{}, fmt.Errorf("start offset %d out of range [0, %d) for line %d", start, len(line), lineNum)
	}
	if end <= start || end > len(line) {
		return Span{}, fmt.Errorf("end offset %d out of range (%d, %d] for line %d", end, start, len(line), lineNum)
	}

	return Span{
		Line:  lineIdx,
		Start: start,
		End:   end,
		Text:  line[start:end],
	}, nil
}

type extDef struct {
	bin  string
	args []string
}

type resolvedMatcher struct {
	name    string
	builtin MatchFunc
	ext     *extDef
}

type styledLine struct {
	text  string       // ANSI-stripped clean text
	cells []vaxis.Cell // styled cells (nil when input has no ANSI)
}

func newStyledLine(raw string) styledLine {
	if !strings.Contains(raw, "\x1b") {
		return styledLine{text: raw}
	}

	// ParseStyledString treats tabs as control characters and drops them.
	// Expand tabs to spaces at 8-column tab stops before parsing.
	sanitized := expandTabsANSI(raw)
	cells := vaxis.ParseStyledString(sanitized)

	var b strings.Builder
	for _, c := range cells {
		b.WriteString(c.Grapheme)
	}

	return styledLine{text: b.String(), cells: cells}
}

// expandTabsANSI expands tab characters to spaces at 8-column tab stops,
// skipping ANSI escape sequences when counting column positions.
func expandTabsANSI(s string) string {
	if !strings.Contains(s, "\t") {
		return s
	}

	var b strings.Builder
	col := 0
	i := 0

	for i < len(s) {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			// CSI sequence: skip parameter (0x30-0x3F) and intermediate
			// (0x20-0x2F) bytes, then consume the final byte (0x40-0x7E).
			// Control characters (< 0x20) are not part of the sequence.
			j := i + 2
			for j < len(s) && s[j] >= 0x20 && (s[j] < 0x40 || s[j] > 0x7E) {
				j++
			}
			if j < len(s) && s[j] >= 0x40 && s[j] <= 0x7E {
				j++ // include final byte
			}
			b.WriteString(s[i:j])
			i = j
			continue
		}

		if s[i] == '\t' {
			spaces := 8 - (col % 8)
			b.WriteString(strings.Repeat(" ", spaces))
			col += spaces
			i++
			continue
		}

		_, size := utf8.DecodeRuneInString(s[i:])
		b.WriteString(s[i : i+size])
		col++
		i += size
	}

	return b.String()
}

type newDataEvent struct {
	lines []styledLine
	spans []Span
}

type inputDoneEvent struct{}

type inputErrorEvent struct {
	err error
}

func startMatchers(
	lineSeq iter.Seq[string],
	matchers []resolvedMatcher,
	width int,
	postEvent func(any),
) (cancel func()) {
	done := make(chan struct{})
	cancel = sync.OnceFunc(func() { close(done) })

	type proc struct {
		cmd   *exec.Cmd
		stdin io.WriteCloser
	}

	var procs []*proc
	extSpansCh := make(chan string, 256)
	var readersWg sync.WaitGroup

	setupFailed := func(err error) func() {
		for _, p := range procs {
			p.stdin.Close()
		}
		postEvent(inputErrorEvent{err: err})
		return cancel
	}

	for _, m := range matchers {
		if m.ext == nil {
			continue
		}

		cmd := exec.Command(m.ext.bin, m.ext.args...)
		cmd.Stderr = os.Stderr
		if width > 0 {
			cmd.Env = append(os.Environ(), fmt.Sprintf("PX_WIDTH=%d", width))
		}

		stdin, err := cmd.StdinPipe()
		if err != nil {
			return setupFailed(fmt.Errorf("stdin pipe for %s: %w", m.ext.bin, err))
		}

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return setupFailed(fmt.Errorf("stdout pipe for %s: %w", m.ext.bin, err))
		}

		if err := cmd.Start(); err != nil {
			return setupFailed(fmt.Errorf("start %s: %w", m.ext.bin, err))
		}

		procs = append(procs, &proc{cmd: cmd, stdin: stdin})

		readersWg.Add(1)
		go func(r io.Reader) {
			defer readersWg.Done()
			scanner := bufio.NewScanner(r)
			for scanner.Scan() {
				extSpansCh <- scanner.Text()
			}
		}(stdout)
	}

	go func() {
		readersWg.Wait()
		close(extSpansCh)
	}()

	go func() {
		const batchSize = 100
		const flushInterval = 50 * time.Millisecond

		cancelled := func() bool {
			select {
			case <-done:
				return true
			default:
				return false
			}
		}

		var lines []string
		var batchLines []styledLine
		var batchSpans []Span
		lastFlush := time.Now()

		flush := func() {
			if len(batchLines) == 0 && len(batchSpans) == 0 {
				return
			}

			postEvent(newDataEvent{lines: batchLines, spans: batchSpans})
			batchLines = nil
			batchSpans = nil
			lastFlush = time.Now()
		}

		for line := range lineSeq {
			if cancelled() {
				break
			}

			sl := newStyledLine(line)
			lineIdx := len(lines)
			lines = append(lines, sl.text)
			batchLines = append(batchLines, sl)

			for _, m := range matchers {
				if m.builtin != nil {
					batchSpans = append(batchSpans, m.builtin(lineIdx, sl.text)...)
				}
			}

			for _, p := range procs {
				fmt.Fprintln(p.stdin, sl.text)
			}

			if len(batchLines) >= batchSize || time.Since(lastFlush) >= flushInterval {
				flush()
			}
		}

		if !cancelled() {
			flush()
		}

		for _, p := range procs {
			p.stdin.Close()
		}

		var extLines []string
		for raw := range extSpansCh {
			extLines = append(extLines, raw)
		}

		for _, p := range procs {
			if err := p.cmd.Wait(); err != nil {
				if !cancelled() {
					postEvent(inputErrorEvent{err: fmt.Errorf("extension failed: %w", err)})
				}
				return
			}
		}

		if cancelled() {
			return
		}

		var extSpans []Span
		for _, raw := range extLines {
			if raw == "" {
				continue
			}

			span, err := parseSpanLine(raw, lines)
			if err != nil {
				postEvent(inputErrorEvent{err: fmt.Errorf("parse extension output: %w", err)})
				return
			}

			extSpans = append(extSpans, span)
		}

		if len(extSpans) > 0 {
			postEvent(newDataEvent{spans: extSpans})
		}

		postEvent(inputDoneEvent{})
	}()

	return cancel
}
