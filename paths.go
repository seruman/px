package main

import (
	"regexp"
	"strings"
	"unicode"
)

type PathSpan struct {
	Line  int    // 0-based line index
	Start int    // byte offset in line (inclusive)
	End   int    // byte offset in line (exclusive)
	Path  string // cleaned path (no :line:col, no quotes)
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

func unwrapLines(lines []string, width int) []string {
	if width <= 0 {
		return lines
	}

	var out []string
	var buf string
	for _, line := range lines {
		buf += line
		if len(line) == width {
			continue
		}
		out = append(out, buf)
		buf = ""
	}
	if buf != "" {
		out = append(out, buf)
	}
	return out
}

func findPaths(lines []string) []PathSpan {
	var spans []PathSpan
	for i, line := range lines {
		spans = tokenizeLine(spans, i, line)
	}
	return spans
}

func tokenizeLine(spans []PathSpan, lineIdx int, line string) []PathSpan {
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

		spans = append(spans, PathSpan{
			Line:  lineIdx,
			Start: tokStart,
			End:   tokEnd,
			Path:  cleaned,
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
