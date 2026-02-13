package main

import (
	"bufio"
	"io"
	"iter"
	"regexp"
	"strings"
	"unicode"
)

type Span struct {
	Line  int    // 0-based line index
	Start int    // byte offset in line (inclusive)
	End   int    // byte offset in line (exclusive)
	Text  string // cleaned match text
}

var (
	lineColSuffix = regexp.MustCompile(`^(.+?)(:\d+(?::\d+)?)$`)
	fileExtRe     = regexp.MustCompile(`\w\.[a-zA-Z]{1,10}$`)
	dotfileRe     = regexp.MustCompile(`^\.[a-zA-Z0-9_-]{3,}`)
	specialFileRe = regexp.MustCompile(`^[A-Z][a-zA-Z]{2,}file$`)
)

func looksLikePath(s string) bool {
	if strings.Contains(s, "/") && s != "/" {
		return true
	}
	if fileExtRe.MatchString(s) {
		return true
	}
	if dotfileRe.MatchString(s) {
		return true
	}
	return specialFileRe.MatchString(s)
}

func scanLines(r io.Reader) iter.Seq[string] {
	return func(yield func(string) bool) {
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			if !yield(scanner.Text()) {
				return
			}
		}
	}
}

func unwrap(lines iter.Seq[string], width int) iter.Seq[string] {
	return func(yield func(string) bool) {
		if width <= 0 {
			for line := range lines {
				if !yield(line) {
					return
				}
			}
			return
		}
		var buf string
		for line := range lines {
			buf += line
			if len(line) == width {
				continue
			}
			if !yield(buf) {
				return
			}
			buf = ""
		}
		if buf != "" {
			yield(buf)
		}
	}
}

func findPaths(lines []string) []Span {
	var spans []Span
	for i, line := range lines {
		spans = append(spans, tokenizeLine(i, line)...)
	}
	return spans
}

func tokenizeLine(lineIdx int, line string) []Span {
	var spans []Span
	off := 0
	for off < len(line) {
		if line[off] == ' ' || line[off] == '\t' {
			off++
			continue
		}

		end := off
		for end < len(line) && line[end] != ' ' && line[end] != '\t' {
			end++
		}

		tok := line[off:end]
		tokStart := off
		off = end

		if strings.Contains(tok, "://") {
			continue
		}

		tok, tokStart, tokEnd := stripWrapping(tok, tokStart)
		if tok == "" {
			continue
		}

		cleaned := tok
		if m := lineColSuffix.FindStringSubmatch(tok); m != nil {
			cleaned = m[1]
		}

		if (strings.HasPrefix(cleaned, "a/") || strings.HasPrefix(cleaned, "b/")) && len(cleaned) > 2 {
			cleaned = cleaned[2:]
		}

		if !looksLikePath(cleaned) {
			continue
		}

		spans = append(spans, Span{
			Line:  lineIdx,
			Start: tokStart,
			End:   tokEnd,
			Text:  cleaned,
		})
	}
	return spans
}

var wrapPairs = map[byte]byte{
	'"':  '"',
	'\'': '\'',
	'(':  ')',
	'[':  ']',
	'<':  '>',
}

func stripWrapping(tok string, start int) (string, int, int) {
	for len(tok) >= 2 {
		if close, ok := wrapPairs[tok[0]]; ok && tok[len(tok)-1] == close {
			tok = tok[1 : len(tok)-1]
			start++
		} else {
			break
		}
	}

	for len(tok) > 0 {
		last := tok[len(tok)-1]
		if last == ',' || last == ';' {
			tok = tok[:len(tok)-1]
		} else {
			break
		}
	}

	end := start + len(tok)

	// Strip leading/trailing whitespace (shouldn't happen, but be safe).
	tok = strings.TrimFunc(tok, unicode.IsSpace)

	return tok, start, end
}
