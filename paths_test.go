package main

import (
	"slices"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

func TestScanLines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "multiple lines",
			input: "hello\nworld\n",
			want:  []string{"hello", "world"},
		},
		{
			name:  "no trailing newline",
			input: "hello\nworld",
			want:  []string{"hello", "world"},
		},
		{
			name:  "single line",
			input: "hello\n",
			want:  []string{"hello"},
		},
		{
			name:  "empty input",
			input: "",
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := slices.Collect(scanLines(strings.NewReader(tt.input)))
			assert.DeepEqual(t, got, tt.want)
		})
	}
}

func TestUnwrap(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		width int
		want  []string
	}{
		{
			name:  "no wrapping, all lines shorter than width",
			lines: []string{"hello", "world"},
			width: 10,
			want:  []string{"hello", "world"},
		},
		{
			name:  "single wrapped line",
			lines: []string{"error in /very/long/path/to/some/deep/di", "r/file.go"},
			width: 40,
			want:  []string{"error in /very/long/path/to/some/deep/dir/file.go"},
		},
		{
			name:  "multiple consecutive wraps",
			lines: []string{"aaaaaaaaaa", "bbbbbbbbbb", "ccc"},
			width: 10,
			want:  []string{"aaaaaaaaaabbbbbbbbbbccc"},
		},
		{
			name:  "last line is exactly width",
			lines: []string{"short", "aaaaaaaaaa"},
			width: 10,
			want:  []string{"short", "aaaaaaaaaa"},
		},
		{
			name:  "empty input",
			lines: nil,
			width: 80,
			want:  nil,
		},
		{
			name:  "width zero is pass-through",
			lines: []string{"hello", "world"},
			width: 0,
			want:  []string{"hello", "world"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := slices.Collect(unwrap(slices.Values(tt.lines), tt.width))
			assert.DeepEqual(t, got, tt.want)
		})
	}
}

func TestFindPaths(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []Span
	}{
		{
			name:  "simple relative path",
			input: []string{"error in ./src/main.go"},
			want: []Span{
				{Line: 0, Start: 9, End: 22, Text: "./src/main.go"},
			},
		},
		{
			name:  "absolute path",
			input: []string{"see /tmp/log.txt"},
			want: []Span{
				{Line: 0, Start: 4, End: 16, Text: "/tmp/log.txt"},
			},
		},
		{
			name:  "path with line and column",
			input: []string{"./src/main.go:42:10 something"},
			want: []Span{
				{Line: 0, Start: 0, End: 19, Text: "./src/main.go"},
			},
		},
		{
			name:  "path with line only",
			input: []string{"./src/main.go:42"},
			want: []Span{
				{Line: 0, Start: 0, End: 16, Text: "./src/main.go"},
			},
		},
		{
			name:  "quoted path",
			input: []string{`open "src/main.go" failed`},
			want: []Span{
				{Line: 0, Start: 6, End: 17, Text: "src/main.go"},
			},
		},
		{
			name:  "parenthesized path",
			input: []string{"(src/main.go)"},
			want: []Span{
				{Line: 0, Start: 1, End: 12, Text: "src/main.go"},
			},
		},
		{
			name:  "bracketed path",
			input: []string{"[src/main.go]"},
			want: []Span{
				{Line: 0, Start: 1, End: 12, Text: "src/main.go"},
			},
		},
		{
			name:  "angle bracketed path",
			input: []string{"<src/main.go>"},
			want: []Span{
				{Line: 0, Start: 1, End: 12, Text: "src/main.go"},
			},
		},
		{
			name:  "trailing comma",
			input: []string{"src/main.go, src/util.go"},
			want: []Span{
				{Line: 0, Start: 0, End: 11, Text: "src/main.go"},
				{Line: 0, Start: 13, End: 24, Text: "src/util.go"},
			},
		},
		{
			name:  "trailing semicolon",
			input: []string{"src/main.go;"},
			want: []Span{
				{Line: 0, Start: 0, End: 11, Text: "src/main.go"},
			},
		},
		{
			name:  "URL skipped",
			input: []string{"see https://example.com/path/file.go for details"},
			want:  nil,
		},
		{
			name:  "git diff prefix a/",
			input: []string{"diff a/src/main.go b/src/main.go"},
			want: []Span{
				{Line: 0, Start: 5, End: 18, Text: "src/main.go"},
				{Line: 0, Start: 19, End: 32, Text: "src/main.go"},
			},
		},
		{
			name:  "multiple lines",
			input: []string{"error in ./src/main.go:42", "see also /tmp/log.txt"},
			want: []Span{
				{Line: 0, Start: 9, End: 25, Text: "./src/main.go"},
				{Line: 1, Start: 9, End: 21, Text: "/tmp/log.txt"},
			},
		},
		{
			name:  "no paths",
			input: []string{"hello world", "no paths here"},
			want:  nil,
		},
		{
			name:  "bare slash ignored",
			input: []string{"/"},
			want:  nil,
		},
		{
			name:  "empty input",
			input: nil,
			want:  nil,
		},
		{
			name:  "empty lines",
			input: []string{"", "", ""},
			want:  nil,
		},
		{
			name:  "bare filename with extension",
			input: []string{"main.go"},
			want: []Span{
				{Line: 0, Start: 0, End: 7, Text: "main.go"},
			},
		},
		{
			name:  "duplicate paths on same line",
			input: []string{"src/main.go src/main.go"},
			want: []Span{
				{Line: 0, Start: 0, End: 11, Text: "src/main.go"},
				{Line: 0, Start: 12, End: 23, Text: "src/main.go"},
			},
		},
		{
			name:  "single quoted path",
			input: []string{"'src/main.go'"},
			want: []Span{
				{Line: 0, Start: 1, End: 12, Text: "src/main.go"},
			},
		},
		{
			name:  "nested wrapping stripped",
			input: []string{`("src/main.go")`},
			want: []Span{
				{Line: 0, Start: 2, End: 13, Text: "src/main.go"},
			},
		},
		{
			name:  "path with spaces via tabs",
			input: []string{"/tmp/log.txt\t/var/data/out.csv"},
			want: []Span{
				{Line: 0, Start: 0, End: 12, Text: "/tmp/log.txt"},
				{Line: 0, Start: 13, End: 30, Text: "/var/data/out.csv"},
			},
		},
		{
			name:  "bare go.mod and go.sum",
			input: []string{"go.mod go.sum"},
			want: []Span{
				{Line: 0, Start: 0, End: 6, Text: "go.mod"},
				{Line: 0, Start: 7, End: 13, Text: "go.sum"},
			},
		},
		{
			name:  "bare filename with line suffix",
			input: []string{"go.mod:42"},
			want: []Span{
				{Line: 0, Start: 0, End: 9, Text: "go.mod"},
			},
		},
		{
			name:  "dotfile",
			input: []string{".gitignore"},
			want: []Span{
				{Line: 0, Start: 0, End: 10, Text: ".gitignore"},
			},
		},
		{
			name:  "dotfile with dots",
			input: []string{".env.local"},
			want: []Span{
				{Line: 0, Start: 0, End: 10, Text: ".env.local"},
			},
		},
		{
			name:  "too-short dotfile rejected",
			input: []string{".a"},
			want:  nil,
		},
		{
			name:  "Makefile special name",
			input: []string{"Makefile"},
			want: []Span{
				{Line: 0, Start: 0, End: 8, Text: "Makefile"},
			},
		},
		{
			name:  "Dockerfile special name",
			input: []string{"Dockerfile"},
			want: []Span{
				{Line: 0, Start: 0, End: 10, Text: "Dockerfile"},
			},
		},
		{
			name:  "not a special name",
			input: []string{"Gemfilenope"},
			want:  nil,
		},
		{
			name:  "non-path tokens rejected",
			input: []string{"error | 42 +-"},
			want:  nil,
		},
		{
			name:  "git diff stat bare filename",
			input: []string{" go.mod    |  2 +-"},
			want: []Span{
				{Line: 0, Start: 1, End: 7, Text: "go.mod"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findPaths(tt.input)
			assert.DeepEqual(t, got, tt.want)
		})
	}
}
