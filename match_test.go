package main

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestMatchURLs(t *testing.T) {
	tests := []struct {
		name string
		line string
		want []Span
	}{
		{
			name: "simple http",
			line: "visit http://example.com today",
			want: []Span{
				{Line: 0, Start: 6, End: 24, Text: "http://example.com"},
			},
		},
		{
			name: "simple https",
			line: "visit https://example.com today",
			want: []Span{
				{Line: 0, Start: 6, End: 25, Text: "https://example.com"},
			},
		},
		{
			name: "url with path",
			line: "see https://example.com/foo/bar",
			want: []Span{
				{Line: 0, Start: 4, End: 31, Text: "https://example.com/foo/bar"},
			},
		},
		{
			name: "url with query",
			line: "https://example.com/search?q=test&page=1",
			want: []Span{
				{Line: 0, Start: 0, End: 40, Text: "https://example.com/search?q=test&page=1"},
			},
		},
		{
			name: "multiple urls",
			line: "http://a.com and https://b.com",
			want: []Span{
				{Line: 0, Start: 0, End: 12, Text: "http://a.com"},
				{Line: 0, Start: 17, End: 30, Text: "https://b.com"},
			},
		},
		{
			name: "trailing period stripped",
			line: "Visit https://example.com.",
			want: []Span{
				{Line: 0, Start: 6, End: 25, Text: "https://example.com"},
			},
		},
		{
			name: "trailing comma stripped",
			line: "https://example.com, and more",
			want: []Span{
				{Line: 0, Start: 0, End: 19, Text: "https://example.com"},
			},
		},
		{
			name: "trailing semicolon stripped",
			line: "https://example.com;",
			want: []Span{
				{Line: 0, Start: 0, End: 19, Text: "https://example.com"},
			},
		},
		{
			name: "unbalanced trailing paren stripped",
			line: "(see https://example.com/path)",
			want: []Span{
				{Line: 0, Start: 5, End: 29, Text: "https://example.com/path"},
			},
		},
		{
			name: "balanced parens in url kept",
			line: "https://en.wikipedia.org/wiki/Fish_(animal)",
			want: []Span{
				{Line: 0, Start: 0, End: 43, Text: "https://en.wikipedia.org/wiki/Fish_(animal)"},
			},
		},
		{
			name: "unbalanced trailing bracket stripped",
			line: "[https://example.com]",
			want: []Span{
				{Line: 0, Start: 1, End: 20, Text: "https://example.com"},
			},
		},
		{
			name: "no urls",
			line: "no urls here at all",
			want: nil,
		},
		{
			name: "empty line",
			line: "",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchURLs(0, tt.line)
			assert.DeepEqual(t, got, tt.want)
		})
	}
}

func TestMatchIPs(t *testing.T) {
	tests := []struct {
		name string
		line string
		want []Span
	}{
		{
			name: "simple ipv4",
			line: "server 10.0.0.1 running",
			want: []Span{
				{Line: 0, Start: 7, End: 15, Text: "10.0.0.1"},
			},
		},
		{
			name: "multiple ipv4",
			line: "from 192.168.1.1 to 10.0.0.1",
			want: []Span{
				{Line: 0, Start: 5, End: 16, Text: "192.168.1.1"},
				{Line: 0, Start: 20, End: 28, Text: "10.0.0.1"},
			},
		},
		{
			name: "max octets",
			line: "addr 255.255.255.255",
			want: []Span{
				{Line: 0, Start: 5, End: 20, Text: "255.255.255.255"},
			},
		},
		{
			name: "invalid octet over 255",
			line: "addr 999.0.0.1",
			want: nil,
		},
		{
			name: "leading zeros rejected",
			line: "addr 010.0.0.1",
			want: nil,
		},
		{
			name: "ipv6 full",
			line: "addr 2001:0db8:85a3:0000:0000:8a2e:0370:7334 ok",
			want: []Span{
				{Line: 0, Start: 5, End: 44, Text: "2001:0db8:85a3:0000:0000:8a2e:0370:7334"},
			},
		},
		{
			name: "ipv6 loopback",
			line: "listen on ::1 port",
			want: []Span{
				{Line: 0, Start: 10, End: 13, Text: "::1"},
			},
		},
		{
			name: "no ips",
			line: "nothing here",
			want: nil,
		},
		{
			name: "empty line",
			line: "",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchIPs(0, tt.line)
			assert.DeepEqual(t, got, tt.want)
		})
	}
}

func TestMatchSHAs(t *testing.T) {
	tests := []struct {
		name string
		line string
		want []Span
	}{
		{
			name: "7 char sha",
			line: "commit abc1234 merged",
			want: []Span{
				{Line: 0, Start: 7, End: 14, Text: "abc1234"},
			},
		},
		{
			name: "full 40 char sha",
			line: "da39a3ee5e6b4b0d3255bfef95601890afd80709",
			want: []Span{
				{Line: 0, Start: 0, End: 40, Text: "da39a3ee5e6b4b0d3255bfef95601890afd80709"},
			},
		},
		{
			name: "multiple shas",
			line: "abc1234 def5678",
			want: []Span{
				{Line: 0, Start: 0, End: 7, Text: "abc1234"},
				{Line: 0, Start: 8, End: 15, Text: "def5678"},
			},
		},
		{
			name: "pure decimal rejected",
			line: "code 1234567 done",
			want: nil,
		},
		{
			name: "too short rejected",
			line: "abc123",
			want: nil,
		},
		{
			name: "no shas",
			line: "hello world",
			want: nil,
		},
		{
			name: "empty line",
			line: "",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchSHAs(0, tt.line)
			assert.DeepEqual(t, got, tt.want)
		})
	}
}

func TestMatchEmails(t *testing.T) {
	tests := []struct {
		name string
		line string
		want []Span
	}{
		{
			name: "simple email",
			line: "contact user@example.com please",
			want: []Span{
				{Line: 0, Start: 8, End: 24, Text: "user@example.com"},
			},
		},
		{
			name: "email with plus",
			line: "user+tag@example.com",
			want: []Span{
				{Line: 0, Start: 0, End: 20, Text: "user+tag@example.com"},
			},
		},
		{
			name: "email with dots",
			line: "first.last@sub.example.co.uk",
			want: []Span{
				{Line: 0, Start: 0, End: 28, Text: "first.last@sub.example.co.uk"},
			},
		},
		{
			name: "multiple emails",
			line: "a@b.com and c@d.org",
			want: []Span{
				{Line: 0, Start: 0, End: 7, Text: "a@b.com"},
				{Line: 0, Start: 12, End: 19, Text: "c@d.org"},
			},
		},
		{
			name: "no emails",
			line: "no emails here",
			want: nil,
		},
		{
			name: "empty line",
			line: "",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchEmails(0, tt.line)
			assert.DeepEqual(t, got, tt.want)
		})
	}
}
