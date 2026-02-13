package main

import (
	"bufio"
	"bytes"
	"fmt"
	"iter"
	"os"
	"os/exec"
	"strconv"
	"strings"
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

func runExtension(bin string, lineSeq iter.Seq[string], args []string, width int) ([]string, []Span, error) {
	cmd := exec.Command(bin, args...)
	cmd.Stderr = os.Stderr

	if width > 0 {
		cmd.Env = append(os.Environ(), fmt.Sprintf("PX_WIDTH=%d", width))
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("stdin pipe: %w", err)
	}

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("start extension: %w", err)
	}

	var lines []string
	for line := range lineSeq {
		lines = append(lines, line)
		fmt.Fprintln(stdin, line)
	}
	stdin.Close()

	if err := cmd.Wait(); err != nil {
		return nil, nil, fmt.Errorf("extension failed: %w", err)
	}

	var spans []Span
	scanner := bufio.NewScanner(&stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		span, err := parseSpanLine(line, lines)
		if err != nil {
			return nil, nil, fmt.Errorf("parse extension output: %w", err)
		}
		spans = append(spans, span)
	}

	return lines, spans, nil
}
