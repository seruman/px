package main

import (
	"iter"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"gotest.tools/v3/assert"
)

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

func collectMatchers(t *testing.T, lineSeq iter.Seq[string], matchers []resolvedMatcher, width int) ([]string, []Span, error) {
	t.Helper()
	events := make(chan any, 256)
	cancel := startMatchers(lineSeq, matchers, width, func(ev any) {
		events <- ev
	})
	defer cancel()

	var lines []string
	var spans []Span
	for ev := range events {
		switch e := ev.(type) {
		case newDataEvent:
			for _, sl := range e.lines {
				lines = append(lines, sl.text)
			}
			spans = append(spans, e.spans...)
		case inputDoneEvent:
			return lines, spans, nil
		case inputErrorEvent:
			return lines, spans, e.err
		}
	}
	t.Fatal("event channel closed without terminal event")
	return nil, nil, nil
}

func TestStartMatchersExtension(t *testing.T) {
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
	lines, spans, err := collectMatchers(t, slices.Values(inputLines), []resolvedMatcher{
		{name: "test", ext: &ext},
	}, 0)
	assert.NilError(t, err)
	assert.DeepEqual(t, lines, inputLines)
	assert.DeepEqual(t, spans, []Span{
		{Line: 0, Start: 4, End: 9, Text: "hello"},
		{Line: 2, Start: 0, End: 5, Text: "hello"},
	})
}

func TestStartMatchersPXWidth(t *testing.T) {
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
		_, spans, err := collectMatchers(t, slices.Values(inputLines), []resolvedMatcher{
			{name: "env", ext: &ext},
		}, 0)
		assert.NilError(t, err)
		var noSpans []Span
		assert.DeepEqual(t, spans, noSpans)
	})

	t.Run("width set exports PX_WIDTH", func(t *testing.T) {
		_, spans, err := collectMatchers(t, slices.Values(inputLines), []resolvedMatcher{
			{name: "env", ext: &ext},
		}, 80)
		assert.NilError(t, err)
		assert.DeepEqual(t, spans, []Span{
			{Line: 0, Start: 0, End: 5, Text: "hello"},
		})
	})
}

func TestStartMatchersFailure(t *testing.T) {
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
	_, _, err = collectMatchers(t, slices.Values(inputLines), []resolvedMatcher{
		{name: "fail", ext: &ext},
	}, 0)
	assert.Error(t, err, "extension failed: exit status 1")
}

func TestStartMatchersMixed(t *testing.T) {
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
	lines, spans, err := collectMatchers(t, slices.Values(inputLines), []resolvedMatcher{
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

func TestStartMatchersANSI(t *testing.T) {
	inputLines := []string{
		"\x1b[33mbc1d9e7\x1b[m remove runMatchers",
		"\x1b[33m0e585a9\x1b[m stream lines",
	}
	lines, spans, err := collectMatchers(t, slices.Values(inputLines), []resolvedMatcher{
		{name: "sha", builtin: matchSHAs},
	}, 0)
	assert.NilError(t, err)
	assert.DeepEqual(t, lines, []string{
		"bc1d9e7 remove runMatchers",
		"0e585a9 stream lines",
	})
	assert.DeepEqual(t, spans, []Span{
		{Line: 0, Start: 0, End: 7, Text: "bc1d9e7"},
		{Line: 1, Start: 0, End: 7, Text: "0e585a9"},
	})
}

func TestStartMatchersANSITabs(t *testing.T) {
	inputLines := []string{
		"\x1b[32m+\tfmt.Println(\"hello\")\x1b[m",
	}
	lines, _, err := collectMatchers(t, slices.Values(inputLines), []resolvedMatcher{
		{name: "paths", builtin: matchPaths},
	}, 0)
	assert.NilError(t, err)
	// Tab after "+" (column 1) expands to 7 spaces (next tab stop at column 8).
	assert.DeepEqual(t, lines, []string{
		"+       fmt.Println(\"hello\")",
	})
}

func TestNewStyledLinePreservesTabs(t *testing.T) {
	sl := newStyledLine("\x1b[32m+\tcode\x1b[m")
	assert.Equal(t, sl.text, "+       code")
	assert.Assert(t, sl.cells != nil)

	// Without ANSI, tabs pass through as-is.
	sl = newStyledLine("+\tcode")
	assert.Equal(t, sl.text, "+\tcode")
	assert.Assert(t, sl.cells == nil)
}

func TestExpandTabsANSI(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no tabs",
			input: "\x1b[32m+hello\x1b[m",
			want:  "\x1b[32m+hello\x1b[m",
		},
		{
			name:  "tab at column 0",
			input: "\x1b[32m\tfoo\x1b[m",
			want:  "\x1b[32m        foo\x1b[m",
		},
		{
			name:  "tab after one char",
			input: "\x1b[32m+\tfoo\x1b[m",
			want:  "\x1b[32m+       foo\x1b[m",
		},
		{
			name:  "two tabs",
			input: "\x1b[32m\t\tfoo\x1b[m",
			want:  "\x1b[32m                foo\x1b[m",
		},
		{
			name:  "tab without ANSI passthrough",
			input: "no-escape",
			want:  "no-escape",
		},
		{
			name:  "truncated CSI before tab",
			input: "\x1b[\t",
			want:  "\x1b[        ",
		},
		{
			name:  "multi-byte char before tab",
			input: "\x1b[32mhé\tfoo\x1b[m",
			want:  "\x1b[32mhé      foo\x1b[m",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandTabsANSI(tt.input)
			assert.Equal(t, got, tt.want)
		})
	}
}
