package main

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"git.sr.ht/~rockorager/vaxis"
	"github.com/google/go-cmp/cmp"
	"gotest.tools/v3/assert"
)

func plainLines(ss []string) []styledLine {
	sls := make([]styledLine, len(ss))
	for i, s := range ss {
		sls[i] = styledLine{raw: s, text: s}
	}
	return sls
}

func TestColForBytePlainTabs(t *testing.T) {
	sl := styledLine{text: "\tfoo"}
	assert.Equal(t, colForByte(sl, 1), 8)

	sl = styledLine{text: "a\tfoo"}
	assert.Equal(t, colForByte(sl, 2), 8)

	sl = styledLine{text: "héllo"}
	assert.Equal(t, colForByte(sl, len("h")), 1)
	assert.Equal(t, colForByte(sl, len("hé")), 2)
}

func TestHintColForSpan(t *testing.T) {
	sl := styledLine{text: "\t\tchunknl/"}
	span := Span{Start: 2}
	assert.Equal(t, hintColForSpan(sl, span), 0)

	sl = styledLine{text: "Your branch is up to date with origin/main."}
	span = Span{Start: 30}
	assert.Equal(t, hintColForSpan(sl, span), colForByte(sl, span.Start))
}

func TestPickerNavigation(t *testing.T) {
	lines := []string{
		"error in ./src/main.go",
		"see also /tmp/log.txt",
	}
	spans := findPaths(lines)
	p := newPicker()
	p.appendData(plainLines(lines), spans)
	p.inputDone = true
	p.cols = 80
	p.rows = 10

	assert.Equal(t, p.cursor, 0)
	assert.Equal(t, p.spans[p.cursor].Text, "./src/main.go")

	p.moveDown()
	assert.Equal(t, p.cursor, 1)
	assert.Equal(t, p.spans[p.cursor].Text, "/tmp/log.txt")

	// Down at bottom stays at bottom.
	p.moveDown()
	assert.Equal(t, p.cursor, 1)

	p.moveUp()
	assert.Equal(t, p.cursor, 0)

	// Up at top stays at top.
	p.moveUp()
	assert.Equal(t, p.cursor, 0)
}

func TestPickerToggleSelection(t *testing.T) {
	lines := []string{
		"error in ./src/main.go",
		"see also /tmp/log.txt",
	}
	spans := findPaths(lines)
	p := newPicker()
	p.appendData(plainLines(lines), spans)
	p.inputDone = true
	p.cols = 80
	p.rows = 10

	p.toggleCurrent()
	assert.Equal(t, p.sel[keyForSpan(p.spans[0])], true)

	// Toggle again deselects.
	p.toggleCurrent()
	_, exists := p.sel[keyForSpan(p.spans[0])]
	assert.Assert(t, !exists)
}

func TestPickerSelectedDefault(t *testing.T) {
	lines := []string{
		"error in ./src/main.go",
		"see also /tmp/log.txt",
	}
	spans := findPaths(lines)
	p := newPicker()
	p.appendData(plainLines(lines), spans)
	p.inputDone = true

	got := p.selected()
	assert.DeepEqual(t, got, []string{"./src/main.go"})
}

func TestPickerSelectedExplicit(t *testing.T) {
	lines := []string{
		"error in ./src/main.go",
		"see also /tmp/log.txt",
	}
	spans := findPaths(lines)
	p := newPicker()
	p.appendData(plainLines(lines), spans)
	p.inputDone = true

	p.toggleCurrent()
	p.moveDown()
	p.toggleCurrent()

	got := p.selected()
	assert.DeepEqual(t, got, []string{"./src/main.go", "/tmp/log.txt"})
}

func TestPickerDuplicatePaths(t *testing.T) {
	lines := []string{
		"src/main.go src/main.go",
		"src/main.go again",
	}
	spans := findPaths(lines)
	p := newPicker()
	p.appendData(plainLines(lines), spans)
	p.inputDone = true

	assert.Equal(t, len(p.spans), 3)

	p.toggleCurrent()
	assert.Equal(t, p.sel[keyForSpan(p.spans[0])], true)
	assert.Assert(t, !p.sel[keyForSpan(p.spans[1])])
	assert.Assert(t, !p.sel[keyForSpan(p.spans[2])])

	got := p.selected()
	assert.DeepEqual(t, got, []string{"src/main.go"})
}

func TestPickerDuplicateSelectionOutputsEachOccurrence(t *testing.T) {
	lines := []string{
		"src/main.go src/main.go",
	}
	spans := findPaths(lines)
	p := newPicker()
	p.appendData(plainLines(lines), spans)
	p.inputDone = true

	p.toggleCurrent() // first occurrence
	p.moveDown()
	p.toggleCurrent() // second occurrence

	got := p.selected()
	assert.DeepEqual(t, got, []string{"src/main.go", "src/main.go"})
}

func TestPickerHandleKey(t *testing.T) {
	lines := []string{
		"error in ./src/main.go",
		"see also /tmp/log.txt",
	}
	spans := findPaths(lines)
	p := newPicker()
	p.appendData(plainLines(lines), spans)
	p.inputDone = true
	p.cols = 80
	p.rows = 10

	// Ctrl-C cancels (Kitty protocol: uppercase).
	action := p.handleKey(vaxis.Key{Keycode: 'C', Modifiers: vaxis.ModCtrl})
	assert.Equal(t, action, actionCancel)

	// Ctrl-C cancels (legacy: lowercase).
	action = p.handleKey(vaxis.Key{Keycode: 'c', Modifiers: vaxis.ModCtrl})
	assert.Equal(t, action, actionCancel)

	// Escape cancels.
	action = p.handleKey(vaxis.Key{Keycode: vaxis.KeyEsc})
	assert.Equal(t, action, actionCancel)

	// q cancels.
	action = p.handleKey(vaxis.Key{Keycode: 'q'})
	assert.Equal(t, action, actionCancel)

	// Enter confirms.
	action = p.handleKey(vaxis.Key{Keycode: vaxis.KeyEnter})
	assert.Equal(t, action, actionConfirm)

	// Tab toggles, advances cursor, and redraws.
	action = p.handleKey(vaxis.Key{Keycode: vaxis.KeyTab})
	assert.Equal(t, action, actionRedraw)
	assert.Equal(t, p.sel[keyForSpan(p.spans[0])], true)
	assert.Equal(t, p.cursor, 1)

	// Up arrow.
	action = p.handleKey(vaxis.Key{Keycode: vaxis.KeyUp})
	assert.Equal(t, action, actionRedraw)
	assert.Equal(t, p.cursor, 0)

	// j moves down.
	action = p.handleKey(vaxis.Key{Keycode: 'j'})
	assert.Equal(t, action, actionRedraw)
	assert.Equal(t, p.cursor, 1)

	// k moves up.
	action = p.handleKey(vaxis.Key{Keycode: 'k'})
	assert.Equal(t, action, actionRedraw)
	assert.Equal(t, p.cursor, 0)

	// G moves to last.
	action = p.handleKey(vaxis.Key{Keycode: 'G'})
	assert.Equal(t, action, actionRedraw)
	assert.Equal(t, p.cursor, 1)

	// gg moves to first (two separate inputs).
	action = p.handleKey(vaxis.Key{Keycode: 'g'})
	assert.Equal(t, action, actionNone)
	action = p.handleKey(vaxis.Key{Keycode: 'g'})
	assert.Equal(t, action, actionRedraw)
	assert.Equal(t, p.cursor, 0)
}

func TestPickerPageMovement(t *testing.T) {
	var lineStrs []string
	for i := range 20 {
		lineStrs = append(lineStrs, "path/"+strings.Repeat("x", i)+"/file.go")
	}
	spans := findPaths(lineStrs)
	p := newPicker()
	p.appendData(plainLines(lineStrs), spans)
	p.inputDone = true
	p.cols = 80
	p.rows = 11 // contentRows = 10

	// Ctrl+D: half page down (5 spans, Kitty).
	action := p.handleKey(vaxis.Key{Keycode: 'D', Modifiers: vaxis.ModCtrl})
	assert.Equal(t, action, actionRedraw)
	assert.Equal(t, p.cursor, 5)

	// Ctrl+U: half page up (5 spans, Kitty).
	action = p.handleKey(vaxis.Key{Keycode: 'U', Modifiers: vaxis.ModCtrl})
	assert.Equal(t, action, actionRedraw)
	assert.Equal(t, p.cursor, 0)

	// Ctrl+F: full page down (10 spans, Kitty).
	action = p.handleKey(vaxis.Key{Keycode: 'F', Modifiers: vaxis.ModCtrl})
	assert.Equal(t, action, actionRedraw)
	assert.Equal(t, p.cursor, 10)

	// Ctrl+B: full page up (10 spans, Kitty).
	action = p.handleKey(vaxis.Key{Keycode: 'B', Modifiers: vaxis.ModCtrl})
	assert.Equal(t, action, actionRedraw)
	assert.Equal(t, p.cursor, 0)

	// Legacy lowercase variants.
	action = p.handleKey(vaxis.Key{Keycode: 'd', Modifiers: vaxis.ModCtrl})
	assert.Equal(t, action, actionRedraw)
	assert.Equal(t, p.cursor, 5)

	action = p.handleKey(vaxis.Key{Keycode: 'u', Modifiers: vaxis.ModCtrl})
	assert.Equal(t, action, actionRedraw)
	assert.Equal(t, p.cursor, 0)

	action = p.handleKey(vaxis.Key{Keycode: 'f', Modifiers: vaxis.ModCtrl})
	assert.Equal(t, action, actionRedraw)
	assert.Equal(t, p.cursor, 10)

	action = p.handleKey(vaxis.Key{Keycode: 'b', Modifiers: vaxis.ModCtrl})
	assert.Equal(t, action, actionRedraw)
	assert.Equal(t, p.cursor, 0)

	// Ctrl+U at top stays at 0.
	action = p.handleKey(vaxis.Key{Keycode: 'U', Modifiers: vaxis.ModCtrl})
	assert.Equal(t, action, actionRedraw)
	assert.Equal(t, p.cursor, 0)

	// Move to end, Ctrl+D clamps.
	p.moveLast()
	action = p.handleKey(vaxis.Key{Keycode: 'D', Modifiers: vaxis.ModCtrl})
	assert.Equal(t, action, actionRedraw)
	assert.Equal(t, p.cursor, 19)
}

func TestPickerGGAborted(t *testing.T) {
	lines := []string{
		"error in ./src/main.go",
		"see also /tmp/log.txt",
	}
	spans := findPaths(lines)
	p := newPicker()
	p.appendData(plainLines(lines), spans)
	p.inputDone = true
	p.cols = 80
	p.rows = 10

	p.moveLast()
	assert.Equal(t, p.cursor, 1)

	action := p.handleKey(vaxis.Key{Keycode: 'g'})
	assert.Equal(t, action, actionNone)
	assert.Equal(t, p.pendingG, true)

	// 'k' should cancel pending g and move up.
	action = p.handleKey(vaxis.Key{Keycode: 'k'})
	assert.Equal(t, action, actionRedraw)
	assert.Equal(t, p.pendingG, false)
	assert.Equal(t, p.cursor, 0)
}

func TestPickerEnsureVisible(t *testing.T) {
	var lineStrs []string
	for i := range 20 {
		lineStrs = append(lineStrs, "path/"+strings.Repeat("x", i)+"/file.go")
	}
	spans := findPaths(lineStrs)
	p := newPicker()
	p.appendData(plainLines(lineStrs), spans)
	p.inputDone = true
	p.cols = 80
	p.rows = 5

	assert.Equal(t, p.viewTop, 0)

	p.cursor = 10
	p.ensureVisible()
	assert.Equal(t, p.viewTop, 7)

	p.cursor = 0
	p.ensureVisible()
	assert.Equal(t, p.viewTop, 0)
}

func TestPickerSpanStyle(t *testing.T) {
	lines := []string{
		"src/main.go src/main.go",
		"other/file.go",
	}
	spans := findPaths(lines)
	p := newPicker()
	p.appendData(plainLines(lines), spans)
	p.inputDone = true

	// Cursor on first span (src/main.go at position 0).
	cursorText := p.spans[p.cursor].Text

	// Cursor span: reverse video.
	got := p.spanStyle(p.spans[0], true, cursorText)
	assert.Equal(t, got, styleCursor)

	// Same path, not cursor: plain underline.
	got = p.spanStyle(p.spans[1], false, cursorText)
	assert.Equal(t, got, stylePath)

	// Different path, not cursor: plain underline.
	got = p.spanStyle(p.spans[2], false, cursorText)
	assert.Equal(t, got, stylePath)

	// Selected path, not cursor, not same as cursor: green underline.
	p.moveDown()
	p.moveDown()
	p.toggleCurrent() // select other/file.go
	p.moveFirst()
	cursorText = p.spans[p.cursor].Text
	got = p.spanStyle(p.spans[2], false, cursorText)
	assert.Equal(t, got, styleSelected)

	// Cursor + selected.
	p.toggleCurrent() // select src/main.go
	got = p.spanStyle(p.spans[0], true, cursorText)
	assert.Equal(t, got, styleCursorSelected)
}

func TestPickerStatusLine(t *testing.T) {
	lines := []string{
		"error in ./src/main.go",
		"see also /tmp/log.txt",
	}
	spans := findPaths(lines)
	p := newPicker()
	p.appendData(plainLines(lines), spans)
	p.inputDone = true
	p.cols = 80
	p.rows = 5

	status := p.statusLine()
	assert.Equal(t, status, " 1/2 matches | 0 selected | Tab:select  f:hints  Enter:confirm  Esc/q:cancel")
}

func TestPickerSearch(t *testing.T) {
	lines := []string{
		"error in ./src/main.go",
		"see also /tmp/log.txt",
		"and ./src/main_test.go",
	}
	spans := findPaths(lines)
	p := newPicker()
	p.appendData(plainLines(lines), spans)
	p.inputDone = true
	p.cols = 80
	p.rows = 10

	// Enter search mode.
	action := p.handleKey(vaxis.Key{Keycode: '/'})
	assert.Equal(t, action, actionRedraw)
	assert.Equal(t, p.searching, true)
	assert.Equal(t, p.statusLine(), " /_")

	// Type "main" incrementally.
	action = p.handleKey(vaxis.Key{Text: "m"})
	assert.Equal(t, action, actionRedraw)
	action = p.handleKey(vaxis.Key{Text: "a"})
	assert.Equal(t, action, actionRedraw)
	action = p.handleKey(vaxis.Key{Text: "i"})
	assert.Equal(t, action, actionRedraw)
	action = p.handleKey(vaxis.Key{Text: "n"})
	assert.Equal(t, action, actionRedraw)

	assert.Equal(t, p.searchBuf, "main")
	assert.DeepEqual(t, p.searchHits, []int{0, 2})
	assert.Equal(t, p.cursor, 0)
	assert.Equal(t, p.statusLine(), " /main_")

	// Confirm search.
	action = p.handleKey(vaxis.Key{Keycode: vaxis.KeyEnter})
	assert.Equal(t, action, actionRedraw)
	assert.Equal(t, p.searching, false)
	assert.DeepEqual(t, p.searchHits, []int{0, 2})
	assert.Equal(t, p.cursor, 0)
	assert.Equal(t, p.statusLine(), " 1/3 matches | 0 selected | Tab:select  f:hints  Enter:confirm  Esc/q:cancel [/main: 1/2]")
}

func TestPickerSearchNext(t *testing.T) {
	lines := []string{
		"error in ./src/main.go",
		"see also /tmp/log.txt",
		"and ./src/main_test.go",
		"lib/other.go",
		"also ./src/main_util.go",
	}
	spans := findPaths(lines)
	p := newPicker()
	p.appendData(plainLines(lines), spans)
	p.inputDone = true
	p.cols = 80
	p.rows = 10

	// Set up search for "main".
	p.handleKey(vaxis.Key{Keycode: '/'})
	p.handleKey(vaxis.Key{Text: "main"})
	p.handleKey(vaxis.Key{Keycode: vaxis.KeyEnter})

	assert.DeepEqual(t, p.searchHits, []int{0, 2, 4})
	assert.Equal(t, p.cursor, 0)

	// n → next hit.
	p.handleKey(vaxis.Key{Keycode: 'n'})
	assert.Equal(t, p.cursor, 2)

	// n → next hit.
	p.handleKey(vaxis.Key{Keycode: 'n'})
	assert.Equal(t, p.cursor, 4)

	// n → wrap around to first.
	p.handleKey(vaxis.Key{Keycode: 'n'})
	assert.Equal(t, p.cursor, 0)
}

func TestPickerSearchPrev(t *testing.T) {
	lines := []string{
		"error in ./src/main.go",
		"see also /tmp/log.txt",
		"and ./src/main_test.go",
		"lib/other.go",
		"also ./src/main_util.go",
	}
	spans := findPaths(lines)
	p := newPicker()
	p.appendData(plainLines(lines), spans)
	p.inputDone = true
	p.cols = 80
	p.rows = 10

	// Set up search for "main".
	p.handleKey(vaxis.Key{Keycode: '/'})
	p.handleKey(vaxis.Key{Text: "main"})
	p.handleKey(vaxis.Key{Keycode: vaxis.KeyEnter})

	assert.DeepEqual(t, p.searchHits, []int{0, 2, 4})
	assert.Equal(t, p.cursor, 0)

	// N → wrap around to last.
	p.handleKey(vaxis.Key{Keycode: 'N'})
	assert.Equal(t, p.cursor, 4)

	// N → previous hit.
	p.handleKey(vaxis.Key{Keycode: 'N'})
	assert.Equal(t, p.cursor, 2)

	// N → previous hit.
	p.handleKey(vaxis.Key{Keycode: 'N'})
	assert.Equal(t, p.cursor, 0)

	// N → wrap around to last again.
	p.handleKey(vaxis.Key{Keycode: 'N'})
	assert.Equal(t, p.cursor, 4)
}

func TestPickerSearchCancel(t *testing.T) {
	lines := []string{
		"error in ./src/main.go",
		"see also /tmp/log.txt",
	}
	spans := findPaths(lines)
	p := newPicker()
	p.appendData(plainLines(lines), spans)
	p.inputDone = true
	p.cols = 80
	p.rows = 10

	// Enter search and type pattern.
	p.handleKey(vaxis.Key{Keycode: '/'})
	p.handleKey(vaxis.Key{Text: "main"})
	assert.DeepEqual(t, p.searchHits, []int{0})

	// Escape cancels and clears search state.
	action := p.handleKey(vaxis.Key{Keycode: vaxis.KeyEsc})
	assert.Equal(t, action, actionRedraw)
	assert.Equal(t, p.searching, false)
	assert.Equal(t, p.searchBuf, "")
	assert.Assert(t, p.searchRe == nil)
	assert.Equal(t, len(p.searchHits), 0)

	// n should be no-op with no search active.
	prevCursor := p.cursor
	p.handleKey(vaxis.Key{Keycode: 'n'})
	assert.Equal(t, p.cursor, prevCursor)
}

func TestPickerSearchRegex(t *testing.T) {
	lines := []string{
		"error in ./src/main.go",
		"see also /tmp/log.txt",
		"and ./src/main_test.go",
	}
	spans := findPaths(lines)
	p := newPicker()
	p.appendData(plainLines(lines), spans)
	p.inputDone = true
	p.cols = 80
	p.rows = 10

	// Search with regex: main\.go$
	p.handleKey(vaxis.Key{Keycode: '/'})
	for _, ch := range `main\.go` {
		p.handleKey(vaxis.Key{Text: string(ch)})
	}

	// Should match only the two .go paths ending in main.go, not main_test.go.
	assert.DeepEqual(t, p.searchHits, []int{0})
	assert.Equal(t, p.cursor, 0)
}

func TestPickerSearchNoMatch(t *testing.T) {
	lines := []string{
		"error in ./src/main.go",
		"see also /tmp/log.txt",
	}
	spans := findPaths(lines)
	p := newPicker()
	p.appendData(plainLines(lines), spans)
	p.inputDone = true
	p.cols = 80
	p.rows = 10

	p.moveDown()
	assert.Equal(t, p.cursor, 1)

	// Search for something that doesn't match.
	p.handleKey(vaxis.Key{Keycode: '/'})
	p.handleKey(vaxis.Key{Text: "zzz"})

	// Cursor should not move.
	assert.Equal(t, p.cursor, 1)
	assert.Equal(t, len(p.searchHits), 0)
}

func TestPickerSearchInvalidRegex(t *testing.T) {
	lines := []string{
		"error in ./src/main.go",
		"see also /tmp/log.txt",
	}
	spans := findPaths(lines)
	p := newPicker()
	p.appendData(plainLines(lines), spans)
	p.inputDone = true
	p.cols = 80
	p.rows = 10

	p.handleKey(vaxis.Key{Keycode: '/'})

	// Type valid pattern first.
	p.handleKey(vaxis.Key{Text: "main"})
	assert.DeepEqual(t, p.searchHits, []int{0})
	assert.Equal(t, p.cursor, 0)

	// Type incomplete regex — opening bracket with no close.
	p.handleKey(vaxis.Key{Text: "["})

	// Previous search hits should be preserved (invalid regex is ignored).
	assert.DeepEqual(t, p.searchHits, []int{0})
	assert.Equal(t, p.searchBuf, "main[")

	// Confirm even with invalid regex: should exit search mode.
	action := p.handleKey(vaxis.Key{Keycode: vaxis.KeyEnter})
	assert.Equal(t, action, actionRedraw)
	assert.Equal(t, p.searching, false)
}

func TestPickerSearchHighlight(t *testing.T) {
	// appendSearchHighlight splits span text into matching/non-matching segments.
	// Highlighted segments get the base style + search match background.
	re := regexp.MustCompile("main")

	pathHL := stylePath
	pathHL.Foreground = styleSearchMatch.Foreground
	pathHL.Background = styleSearchMatch.Background

	altBase := vaxis.Style{Foreground: vaxis.IndexColor(4), UnderlineStyle: vaxis.UnderlineSingle}
	altHL := altBase
	altHL.Foreground = styleSearchMatch.Foreground
	altHL.Background = styleSearchMatch.Background

	// Single match in middle.
	segs := appendSearchHighlight(nil, "src/main.go", stylePath, pathHL, re)
	assert.DeepEqual(t, segs, []vaxis.Segment{
		{Text: "src/", Style: stylePath},
		{Text: "main", Style: pathHL},
		{Text: ".go", Style: stylePath},
	})

	// No match in displayed text — whole span gets highlight.
	segs = appendSearchHighlight(nil, "other/file.go", stylePath, pathHL, re)
	assert.DeepEqual(t, segs, []vaxis.Segment{
		{Text: "other/file.go", Style: pathHL},
	})

	// Multiple matches with a non-default base style.
	re2 := regexp.MustCompile("a")
	segs = appendSearchHighlight(nil, "abc/abc", altBase, altHL, re2)
	assert.DeepEqual(t, segs, []vaxis.Segment{
		{Text: "a", Style: altHL},
		{Text: "bc/", Style: altBase},
		{Text: "a", Style: altHL},
		{Text: "bc", Style: altBase},
	})

	// Match at start and end.
	re3 := regexp.MustCompile("^src|go$")
	segs = appendSearchHighlight(nil, "src/main.go", stylePath, pathHL, re3)
	assert.DeepEqual(t, segs, []vaxis.Segment{
		{Text: "src", Style: pathHL},
		{Text: "/main.", Style: stylePath},
		{Text: "go", Style: pathHL},
	})

	// Cursor item: matched portion gets search match colors, dropping reverse.
	cursorHL := vaxis.Style{
		Foreground: styleSearchMatch.Foreground,
		Background: styleSearchMatch.Background,
	}
	segs = appendSearchHighlight(nil, "src/main.go", styleCursor, cursorHL, re)
	assert.DeepEqual(t, segs, []vaxis.Segment{
		{Text: "src/", Style: styleCursor},
		{Text: "main", Style: cursorHL},
		{Text: ".go", Style: styleCursor},
	})
}

func TestPickerSearchClearWithEscape(t *testing.T) {
	lines := []string{
		"error in ./src/main.go",
		"see also /tmp/log.txt",
	}
	spans := findPaths(lines)
	p := newPicker()
	p.appendData(plainLines(lines), spans)
	p.inputDone = true
	p.cols = 80
	p.rows = 10

	// Search and confirm.
	p.handleKey(vaxis.Key{Keycode: '/'})
	p.handleKey(vaxis.Key{Text: "main"})
	p.handleKey(vaxis.Key{Keycode: vaxis.KeyEnter})
	assert.DeepEqual(t, p.searchHits, []int{0})

	// First Escape clears search, does NOT cancel picker.
	action := p.handleKey(vaxis.Key{Keycode: vaxis.KeyEsc})
	assert.Equal(t, action, actionRedraw)
	assert.Assert(t, p.searchRe == nil)
	assert.Equal(t, len(p.searchHits), 0)

	// Second Escape cancels picker.
	action = p.handleKey(vaxis.Key{Keycode: vaxis.KeyEsc})
	assert.Equal(t, action, actionCancel)
}

func TestPickerSearchBackspace(t *testing.T) {
	lines := []string{
		"error in ./src/main.go",
		"see also /tmp/log.txt",
		"and ./src/main_test.go",
	}
	spans := findPaths(lines)
	p := newPicker()
	p.appendData(plainLines(lines), spans)
	p.inputDone = true
	p.cols = 80
	p.rows = 10

	p.handleKey(vaxis.Key{Keycode: '/'})
	p.handleKey(vaxis.Key{Text: "log"})
	assert.DeepEqual(t, p.searchHits, []int{1})
	assert.Equal(t, p.cursor, 1)

	// Backspace one character: "lo" — still matches log.txt.
	p.handleKey(vaxis.Key{Keycode: vaxis.KeyBackspace})
	assert.Equal(t, p.searchBuf, "lo")
	assert.DeepEqual(t, p.searchHits, []int{1})

	// Backspace twice more: empty pattern.
	p.handleKey(vaxis.Key{Keycode: vaxis.KeyBackspace})
	p.handleKey(vaxis.Key{Keycode: vaxis.KeyBackspace})
	assert.Equal(t, p.searchBuf, "")
	assert.Equal(t, len(p.searchHits), 0)
}

func TestGenerateHintLabels(t *testing.T) {
	t.Run("zero", func(t *testing.T) {
		got := generateHintLabels(0)
		assert.Assert(t, got == nil)
	})

	t.Run("single char", func(t *testing.T) {
		got := generateHintLabels(3)
		assert.DeepEqual(t, got, []string{"a", "s", "d"})
	})

	t.Run("boundary single char", func(t *testing.T) {
		got := generateHintLabels(11)
		assert.DeepEqual(t, got, []string{"a", "s", "d", "f", "g", "h", "j", "k", "l", ";", "'"})
	})

	t.Run("two char", func(t *testing.T) {
		got := generateHintLabels(12)
		assert.DeepEqual(t, got, []string{
			"aa", "as", "ad", "af", "ag", "ah", "aj", "ak", "al", "a;", "a'",
			"sa",
		})
	})
}

func TestPickerHintModeEnterExit(t *testing.T) {
	lines := []string{
		"error in ./src/main.go",
		"see also /tmp/log.txt",
	}
	spans := findPaths(lines)
	p := newPicker()
	p.appendData(plainLines(lines), spans)
	p.inputDone = true
	p.cols = 80
	p.rows = 10

	action := p.handleKey(vaxis.Key{Keycode: 'f'})
	assert.Equal(t, action, actionRedraw)
	assert.Equal(t, p.hinting, true)
	assert.DeepEqual(t, p.hintLabels, []hintLabel{
		{spanIdx: 0, label: "a"},
		{spanIdx: 1, label: "s"},
	}, cmp.AllowUnexported(hintLabel{}))
	assert.Equal(t, p.statusLine(), " HINTS: type a label to select")

	action = p.handleKey(vaxis.Key{Keycode: vaxis.KeyEsc})
	assert.Equal(t, action, actionRedraw)
	assert.Equal(t, p.hinting, false)
}

func TestPickerHintSelect(t *testing.T) {
	lines := []string{
		"error in ./src/main.go",
		"see also /tmp/log.txt",
	}
	spans := findPaths(lines)
	p := newPicker()
	p.appendData(plainLines(lines), spans)
	p.inputDone = true
	p.cols = 80
	p.rows = 10

	p.handleKey(vaxis.Key{Keycode: 'f'})

	// Type 's' to select the second span (/tmp/log.txt).
	action := p.handleKey(vaxis.Key{Keycode: 's', Text: "s"})
	assert.Equal(t, action, actionConfirm)
	assert.Equal(t, p.hinting, false)
	assert.Equal(t, p.cursor, 1)
	assert.DeepEqual(t, p.selected(), []string{"/tmp/log.txt"})
}

func TestPickerHintNonAlphabetExits(t *testing.T) {
	lines := []string{
		"error in ./src/main.go",
		"see also /tmp/log.txt",
	}
	spans := findPaths(lines)
	p := newPicker()
	p.appendData(plainLines(lines), spans)
	p.inputDone = true
	p.cols = 80
	p.rows = 10

	p.handleKey(vaxis.Key{Keycode: 'f'})
	assert.Equal(t, p.hinting, true)

	action := p.handleKey(vaxis.Key{Keycode: 'z', Text: "z"})
	assert.Equal(t, action, actionRedraw)
	assert.Equal(t, p.hinting, false)
}

func TestPickerHintOnlyVisibleSpans(t *testing.T) {
	var lineStrs []string
	for i := range 20 {
		lineStrs = append(lineStrs, fmt.Sprintf("path/%d/file.go", i))
	}
	spans := findPaths(lineStrs)
	p := newPicker()
	p.appendData(plainLines(lineStrs), spans)
	p.inputDone = true
	p.cols = 80
	p.rows = 6 // contentRows = 5, so only lines 0-4 visible

	p.handleKey(vaxis.Key{Keycode: 'f'})
	assert.Equal(t, p.hinting, true)
	assert.DeepEqual(t, p.hintLabels, []hintLabel{
		{spanIdx: 0, label: "a"},
		{spanIdx: 1, label: "s"},
		{spanIdx: 2, label: "d"},
		{spanIdx: 3, label: "f"},
		{spanIdx: 4, label: "g"},
	}, cmp.AllowUnexported(hintLabel{}))
	p.exitHintMode()
}

func TestPickerHintTwoChar(t *testing.T) {
	var lineStrs []string
	for i := range 15 {
		lineStrs = append(lineStrs, fmt.Sprintf("path/%d/file.go", i))
	}
	spans := findPaths(lineStrs)
	p := newPicker()
	p.appendData(plainLines(lineStrs), spans)
	p.inputDone = true
	p.cols = 80
	p.rows = 20 // all 15 visible

	p.handleKey(vaxis.Key{Keycode: 'f'})
	assert.Equal(t, p.hinting, true)
	assert.DeepEqual(t, p.hintLabels, []hintLabel{
		{spanIdx: 0, label: "aa"},
		{spanIdx: 1, label: "as"},
		{spanIdx: 2, label: "ad"},
		{spanIdx: 3, label: "af"},
		{spanIdx: 4, label: "ag"},
		{spanIdx: 5, label: "ah"},
		{spanIdx: 6, label: "aj"},
		{spanIdx: 7, label: "ak"},
		{spanIdx: 8, label: "al"},
		{spanIdx: 9, label: "a;"},
		{spanIdx: 10, label: "a'"},
		{spanIdx: 11, label: "sa"},
		{spanIdx: 12, label: "ss"},
		{spanIdx: 13, label: "sd"},
		{spanIdx: 14, label: "sf"},
	}, cmp.AllowUnexported(hintLabel{}))

	// Type first char: should redraw (prefix matches exist).
	action := p.handleKey(vaxis.Key{Keycode: 'a', Text: "a"})
	assert.Equal(t, action, actionRedraw)
	assert.Equal(t, p.hinting, true)
	assert.Equal(t, p.hintBuf, "a")
	assert.Equal(t, p.statusLine(), " HINTS: a_")

	// Type second char: should confirm.
	action = p.handleKey(vaxis.Key{Keycode: 's', Text: "s"})
	assert.Equal(t, action, actionConfirm)
	assert.Equal(t, p.hinting, false)
	assert.Equal(t, p.cursor, 1)
	assert.DeepEqual(t, p.selected(), []string{p.spans[1].Text})
}

func TestPickerHintNoVisibleMatches(t *testing.T) {
	var lineStrs []string
	for i := range 20 {
		lineStrs = append(lineStrs, fmt.Sprintf("path/%d/file.go", i))
	}
	spans := findPaths(lineStrs)
	p := newPicker()
	p.appendData(plainLines(lineStrs), spans)
	p.inputDone = true
	p.cols = 80
	p.rows = 4 // contentRows = 3

	// Scroll viewport past all spans.
	p.viewTop = 20

	action := p.handleKey(vaxis.Key{Keycode: 'f'})
	assert.Equal(t, action, actionNone)
	assert.Equal(t, p.hinting, false)
}

func TestPickerHintClearsSelection(t *testing.T) {
	lines := []string{
		"error in ./src/main.go",
		"see also /tmp/log.txt",
	}
	spans := findPaths(lines)
	p := newPicker()
	p.appendData(plainLines(lines), spans)
	p.inputDone = true
	p.cols = 80
	p.rows = 10

	// Toggle first span selected.
	p.toggleCurrent()
	assert.Equal(t, len(p.sel), 1)

	// Enter hint mode and select second via hint.
	p.handleKey(vaxis.Key{Keycode: 'f'})
	action := p.handleKey(vaxis.Key{Keycode: 's', Text: "s"})
	assert.Equal(t, action, actionConfirm)
	assert.Equal(t, len(p.sel), 0)
	assert.DeepEqual(t, p.selected(), []string{"/tmp/log.txt"})
}
