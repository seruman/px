package main

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"gotest.tools/v3/assert"
)

func TestShellSplit(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr string
	}{
		{
			name:  "simple words",
			input: "url --strict",
			want:  []string{"url", "--strict"},
		},
		{
			name:  "single word",
			input: "url",
			want:  []string{"url"},
		},
		{
			name:  "single quoted arg",
			input: "grep 'hello world'",
			want:  []string{"grep", "hello world"},
		},
		{
			name:  "double quoted arg",
			input: `grep "hello world"`,
			want:  []string{"grep", "hello world"},
		},
		{
			name:  "escaped quote in double quotes",
			input: `grep "say \"hi\""`,
			want:  []string{"grep", `say "hi"`},
		},
		{
			name:  "escaped backslash in double quotes",
			input: `grep "a\\b"`,
			want:  []string{"grep", `a\b`},
		},
		{
			name:  "extra whitespace",
			input: "  url   --strict  ",
			want:  []string{"url", "--strict"},
		},
		{
			name:  "tabs as separators",
			input: "url\t--strict",
			want:  []string{"url", "--strict"},
		},
		{
			name:  "empty string",
			input: "",
			want:  []string(nil),
		},
		{
			name:  "only whitespace",
			input: "   ",
			want:  []string(nil),
		},
		{
			name:  "adjacent quotes",
			input: "'hello''world'",
			want:  []string{"helloworld"},
		},
		{
			name:    "unterminated single quote",
			input:   "url 'oops",
			wantErr: "unterminated single quote",
		},
		{
			name:    "unterminated double quote",
			input:   `url "oops`,
			wantErr: "unterminated double quote",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := shellSplit(tt.input)
			if tt.wantErr != "" {
				assert.Error(t, err, tt.wantErr)
				return
			}
			assert.NilError(t, err)
			assert.DeepEqual(t, got, tt.want)
		})
	}
}

func TestParseSpanLine(t *testing.T) {
	lines := []string{"hello world", "foo bar baz", ""}

	tests := []struct {
		name    string
		input   string
		want    Span
		wantErr string
	}{
		{
			name:  "valid span",
			input: "1:6:11",
			want:  Span{Line: 0, Start: 6, End: 11, Text: "world"},
		},
		{
			name:  "first character",
			input: "2:0:3",
			want:  Span{Line: 1, Start: 0, End: 3, Text: "foo"},
		},
		{
			name:    "line out of range high",
			input:   "99:0:5",
			wantErr: "line number 99 out of range [1, 3]",
		},
		{
			name:    "line zero",
			input:   "0:0:5",
			wantErr: "line number 0 out of range [1, 3]",
		},
		{
			name:    "start out of range",
			input:   "1:100:105",
			wantErr: "start offset 100 out of range [0, 11) for line 1",
		},
		{
			name:    "end out of range",
			input:   "1:0:100",
			wantErr: "end offset 100 out of range (0, 11] for line 1",
		},
		{
			name:    "end before start",
			input:   "1:5:3",
			wantErr: "end offset 3 out of range (5, 11] for line 1",
		},
		{
			name:    "end equals start",
			input:   "1:5:5",
			wantErr: "end offset 5 out of range (5, 11] for line 1",
		},
		{
			name:    "malformed too few parts",
			input:   "1:2",
			wantErr: `invalid span format "1:2": expected LINE:START:END`,
		},
		{
			name:    "malformed non-numeric line",
			input:   "abc:0:5",
			wantErr: `invalid line number "abc": strconv.Atoi: parsing "abc": invalid syntax`,
		},
		{
			name:    "malformed non-numeric start",
			input:   "1:abc:5",
			wantErr: `invalid start offset "abc": strconv.Atoi: parsing "abc": invalid syntax`,
		},
		{
			name:    "malformed non-numeric end",
			input:   "1:0:abc",
			wantErr: `invalid end offset "abc": strconv.Atoi: parsing "abc": invalid syntax`,
		},
		{
			name:    "empty line cannot have span",
			input:   "3:0:1",
			wantErr: "start offset 0 out of range [0, 0) for line 3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSpanLine(tt.input, lines)
			if tt.wantErr != "" {
				assert.Error(t, err, tt.wantErr)
				return
			}
			assert.NilError(t, err)
			assert.DeepEqual(t, got, tt.want)
		})
	}
}

func TestRunExtension(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test requires unix")
	}

	dir := t.TempDir()
	bin := filepath.Join(dir, "px-test")

	script := `#!/bin/sh
awk '{
	idx = index($0, "hello")
	if (idx > 0) {
		printf "%d:%d:%d\n", NR, idx - 1, idx - 1 + 5
	}
}'
`
	err := os.WriteFile(bin, []byte(script), 0o755)
	assert.NilError(t, err)

	inputLines := []string{"say hello world", "no match here", "hello again"}
	ext := extDef{bin, nil}
	lines, spans, err := runMatchers(slices.Values(inputLines), []resolvedMatcher{
		{name: "test", ext: &ext},
	}, 0)
	assert.NilError(t, err)
	assert.DeepEqual(t, lines, inputLines)
	assert.DeepEqual(t, spans, []Span{
		{Line: 0, Start: 4, End: 9, Text: "hello"},
		{Line: 2, Start: 0, End: 5, Text: "hello"},
	})
}

func TestRunExtensionPXWidth(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test requires unix")
	}

	dir := t.TempDir()
	bin := filepath.Join(dir, "px-env")

	script := `#!/bin/sh
cat > /dev/null
if [ -n "$PX_WIDTH" ]; then
	echo "1:0:5"
fi
`
	err := os.WriteFile(bin, []byte(script), 0o755)
	assert.NilError(t, err)

	inputLines := []string{"hello world"}

	ext := extDef{bin, nil}

	t.Run("width zero omits PX_WIDTH", func(t *testing.T) {
		_, spans, err := runMatchers(slices.Values(inputLines), []resolvedMatcher{
			{name: "env", ext: &ext},
		}, 0)
		assert.NilError(t, err)
		var noSpans []Span
		assert.DeepEqual(t, spans, noSpans)
	})

	t.Run("width set exports PX_WIDTH", func(t *testing.T) {
		_, spans, err := runMatchers(slices.Values(inputLines), []resolvedMatcher{
			{name: "env", ext: &ext},
		}, 80)
		assert.NilError(t, err)
		assert.DeepEqual(t, spans, []Span{
			{Line: 0, Start: 0, End: 5, Text: "hello"},
		})
	})
}

func TestRunExtensionFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test requires unix")
	}

	dir := t.TempDir()
	bin := filepath.Join(dir, "px-fail")

	script := `#!/bin/sh
cat > /dev/null
exit 1
`
	err := os.WriteFile(bin, []byte(script), 0o755)
	assert.NilError(t, err)

	inputLines := []string{"hello"}
	ext := extDef{bin, nil}
	_, _, err = runMatchers(slices.Values(inputLines), []resolvedMatcher{
		{name: "fail", ext: &ext},
	}, 0)
	assert.Error(t, err, "extension failed: exit status 1")
}

func TestRunMatchersMixed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test requires unix")
	}

	dir := t.TempDir()
	bin := filepath.Join(dir, "px-test")

	script := `#!/bin/sh
awk '{
	idx = index($0, "hello")
	if (idx > 0) {
		printf "%d:%d:%d\n", NR, idx - 1, idx - 1 + 5
	}
}'
`
	err := os.WriteFile(bin, []byte(script), 0o755)
	assert.NilError(t, err)

	inputLines := []string{"hello https://example.com"}
	ext := extDef{bin, nil}
	lines, spans, err := runMatchers(slices.Values(inputLines), []resolvedMatcher{
		{name: "url", builtin: matchURLs},
		{name: "test", ext: &ext},
	}, 0)
	assert.NilError(t, err)
	assert.DeepEqual(t, lines, inputLines)
	assert.DeepEqual(t, spans, []Span{
		{Line: 0, Start: 6, End: 25, Text: "https://example.com"},
		{Line: 0, Start: 0, End: 5, Text: "hello"},
	})
}
