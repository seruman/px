package main

import (
	"regexp"
	"strings"
	"testing"

	"git.sr.ht/~rockorager/vaxis"
	"gotest.tools/v3/assert"
)

func plainLines(ss []string) []styledLine {
	sls := make([]styledLine, len(ss))
	for i, s := range ss {
		sls[i] = styledLine{text: s}
	}
	return sls
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
	assert.Equal(t, p.sel[p.pathIdx["./src/main.go"]], true)

	// Toggle again deselects.
	p.toggleCurrent()
	_, exists := p.sel[p.pathIdx["./src/main.go"]]
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

	assert.Equal(t, len(p.unique), 1)
	assert.Equal(t, p.unique[0], "src/main.go")
	assert.Equal(t, len(p.spans), 3)

	p.toggleCurrent()
	assert.Equal(t, p.sel[0], true)

	got := p.selected()
	assert.DeepEqual(t, got, []string{"src/main.go"})
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
	assert.Equal(t, p.sel[p.pathIdx["./src/main.go"]], true)
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
	got := p.spanStyle("src/main.go", true, cursorText)
	assert.Equal(t, got, styleCursor)

	// Same path, not cursor: blue underline.
	got = p.spanStyle("src/main.go", false, cursorText)
	assert.Equal(t, got, styleSamePath)

	// Different path, not cursor: plain underline.
	got = p.spanStyle("other/file.go", false, cursorText)
	assert.Equal(t, got, stylePath)

	// Selected path, not cursor, not same as cursor: green underline.
	p.moveDown()
	p.moveDown()
	p.toggleCurrent() // select other/file.go
	p.moveFirst()
	cursorText = p.spans[p.cursor].Text
	got = p.spanStyle("other/file.go", false, cursorText)
	assert.Equal(t, got, styleSelected)

	// Cursor + selected.
	p.toggleCurrent() // select src/main.go
	got = p.spanStyle("src/main.go", true, cursorText)
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
	assert.Equal(t, status, " 1/2 matches | 0 selected | Tab:select  Enter:confirm  Esc/q:cancel")
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
	assert.Equal(t, p.statusLine(), " 1/3 matches | 0 selected | Tab:select  Enter:confirm  Esc/q:cancel [/main: 1/2]")
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
	samePathHL := styleSamePath
	samePathHL.Foreground = styleSearchMatch.Foreground
	samePathHL.Background = styleSearchMatch.Background

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

	// Multiple matches with samePath base style.
	re2 := regexp.MustCompile("a")
	segs = appendSearchHighlight(nil, "abc/abc", styleSamePath, samePathHL, re2)
	assert.DeepEqual(t, segs, []vaxis.Segment{
		{Text: "a", Style: samePathHL},
		{Text: "bc/", Style: styleSamePath},
		{Text: "a", Style: samePathHL},
		{Text: "bc", Style: styleSamePath},
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
