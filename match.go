package main

import (
	"net"
	"regexp"
	"strings"
)

type MatchFunc func(lineIdx int, line string) []Span

var builtinMatchers = map[string]MatchFunc{
	"paths": matchPaths,
	"url":   matchURLs,
	"ip":    matchIPs,
	"sha":   matchSHAs,
	"email": matchEmails,
}

var urlRe = regexp.MustCompile(`https?://[^\s<>"']+`)

func matchURLs(lineIdx int, line string) []Span {
	var spans []Span
	for _, loc := range urlRe.FindAllStringIndex(line, -1) {
		start, end := loc[0], loc[1]
		text := line[start:end]

		// Strip trailing punctuation that's likely not part of the URL.
		for len(text) > 0 {
			last := text[len(text)-1]
			if last == '.' || last == ',' || last == ';' {
				text = text[:len(text)-1]
				end--
				continue
			}
			if last == ')' && strings.Count(text, ")") > strings.Count(text, "(") {
				text = text[:len(text)-1]
				end--
				continue
			}
			if last == ']' && strings.Count(text, "]") > strings.Count(text, "[") {
				text = text[:len(text)-1]
				end--
				continue
			}
			break
		}

		if len(text) == 0 {
			continue
		}

		spans = append(spans, Span{
			Line:  lineIdx,
			Start: start,
			End:   end,
			Text:  text,
		})
	}
	return spans
}

var ipv4Re = regexp.MustCompile(`\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}\b`)

func validIPv4(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		if len(p) > 1 && p[0] == '0' {
			return false
		}
		n := 0
		for _, c := range []byte(p) {
			n = n*10 + int(c-'0')
		}
		if n > 255 {
			return false
		}
	}
	return true
}

var ipv6Re = regexp.MustCompile(`(?i)(?:^|[\s,;()\[\]])([0-9a-f:]{2,39})(?:[\s,;()\[\]]|$)`)

func matchIPs(lineIdx int, line string) []Span {
	var spans []Span
	for _, loc := range ipv4Re.FindAllStringIndex(line, -1) {
		text := line[loc[0]:loc[1]]
		if !validIPv4(text) {
			continue
		}
		spans = append(spans, Span{
			Line:  lineIdx,
			Start: loc[0],
			End:   loc[1],
			Text:  text,
		})
	}

	for _, match := range ipv6Re.FindAllStringSubmatchIndex(line, -1) {
		start, end := match[2], match[3]
		text := line[start:end]
		if strings.Count(text, ":") < 2 {
			continue
		}
		if net.ParseIP(text) == nil {
			continue
		}
		spans = append(spans, Span{
			Line:  lineIdx,
			Start: start,
			End:   end,
			Text:  text,
		})
	}

	return spans
}

var shaRe = regexp.MustCompile(`\b[0-9a-f]{7,40}\b`)

func matchSHAs(lineIdx int, line string) []Span {
	var spans []Span
	for _, loc := range shaRe.FindAllStringIndex(line, -1) {
		text := line[loc[0]:loc[1]]
		hasAlpha := false
		for _, c := range []byte(text) {
			if c >= 'a' && c <= 'f' {
				hasAlpha = true
				break
			}
		}
		if !hasAlpha {
			continue
		}
		spans = append(spans, Span{
			Line:  lineIdx,
			Start: loc[0],
			End:   loc[1],
			Text:  text,
		})
	}
	return spans
}

var emailRe = regexp.MustCompile(`\b[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}\b`)

func matchEmails(lineIdx int, line string) []Span {
	var spans []Span
	for _, loc := range emailRe.FindAllStringIndex(line, -1) {
		spans = append(spans, Span{
			Line:  lineIdx,
			Start: loc[0],
			End:   loc[1],
			Text:  line[loc[0]:loc[1]],
		})
	}
	return spans
}
