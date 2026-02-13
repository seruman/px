package main

import (
	"fmt"
	"regexp"
	"sort"
	"unicode/utf8"

	"git.sr.ht/~rockorager/vaxis"
)

type Picker struct {
	lines   []string
	spans   []Span
	unique  []string
	pathIdx map[string]int
	sel     map[int]bool

	cursor   int
	viewTop  int
	cols     int
	rows     int
	pendingG bool // waiting for second 'g' in gg sequence

	searching  bool           // in search input mode
	searchBuf  string         // current search input text
	searchRe   *regexp.Regexp // compiled regex (nil = no active search)
	searchHits []int          // indices into p.spans that match
	searchPos  int            // current position in searchHits for n/N cycling
}

func newPicker(lines []string, spans []Span) *Picker {
	p := &Picker{
		lines:   lines,
		spans:   spans,
		pathIdx: make(map[string]int),
		sel:     make(map[int]bool),
	}
	for _, s := range spans {
		if _, ok := p.pathIdx[s.Text]; !ok {
			p.pathIdx[s.Text] = len(p.unique)
			p.unique = append(p.unique, s.Text)
		}
	}
	return p
}

func (p *Picker) ensureVisible() {
	line := p.spans[p.cursor].Line
	contentRows := p.rows - 1
	if line < p.viewTop {
		p.viewTop = line
	}
	if line >= p.viewTop+contentRows {
		p.viewTop = line - contentRows + 1
	}
}

// If none were explicitly toggled, returns the text under the cursor.
func (p *Picker) selected() []string {
	if len(p.sel) == 0 {
		return []string{p.spans[p.cursor].Text}
	}
	var texts []string
	for i, text := range p.unique {
		if p.sel[i] {
			texts = append(texts, text)
		}
	}
	return texts
}

func (p *Picker) toggleCurrent() {
	idx := p.pathIdx[p.spans[p.cursor].Text]
	if p.sel[idx] {
		delete(p.sel, idx)
	} else {
		p.sel[idx] = true
	}
}

func (p *Picker) moveUp() {
	if p.cursor == 0 {
		return
	}
	p.cursor--
	p.ensureVisible()
}

func (p *Picker) moveDown() {
	if p.cursor >= len(p.spans)-1 {
		return
	}
	p.cursor++
	p.ensureVisible()
}

func (p *Picker) moveFirst() {
	p.cursor = 0
	p.ensureVisible()
}

func (p *Picker) moveLast() {
	p.cursor = len(p.spans) - 1
	p.ensureVisible()
}

func (p *Picker) moveBy(n int) {
	p.cursor += n
	if p.cursor < 0 {
		p.cursor = 0
	}
	if p.cursor >= len(p.spans) {
		p.cursor = len(p.spans) - 1
	}
	p.ensureVisible()
}

func (p *Picker) updateSearchHits() {
	p.searchHits = p.searchHits[:0]
	if p.searchRe == nil {
		return
	}
	for i, s := range p.spans {
		if p.searchRe.MatchString(s.Text) {
			p.searchHits = append(p.searchHits, i)
		}
	}
}

func (p *Picker) searchNext() {
	if len(p.searchHits) == 0 {
		return
	}
	// Find first hit after cursor.
	idx := sort.SearchInts(p.searchHits, p.cursor+1)
	if idx >= len(p.searchHits) {
		idx = 0
	}
	p.cursor = p.searchHits[idx]
	p.searchPos = idx
	p.ensureVisible()
}

func (p *Picker) searchPrev() {
	if len(p.searchHits) == 0 {
		return
	}
	// Find last hit before cursor.
	idx := sort.SearchInts(p.searchHits, p.cursor) - 1
	if idx < 0 {
		idx = len(p.searchHits) - 1
	}
	p.cursor = p.searchHits[idx]
	p.searchPos = idx
	p.ensureVisible()
}

func (p *Picker) searchJumpNearest() {
	if len(p.searchHits) == 0 {
		return
	}
	// Find first hit at or after cursor.
	idx := sort.SearchInts(p.searchHits, p.cursor)
	if idx >= len(p.searchHits) {
		idx = 0
	}
	p.cursor = p.searchHits[idx]
	p.searchPos = idx
	p.ensureVisible()
}

func (p *Picker) recompileSearch() {
	if p.searchBuf == "" {
		p.searchRe = nil
		p.updateSearchHits()
		return
	}
	re, err := regexp.Compile(p.searchBuf)
	if err != nil {
		// Invalid regex mid-typing: keep previous state.
		return
	}
	p.searchRe = re
	p.updateSearchHits()
	p.searchJumpNearest()
}

func (p *Picker) clearSearch() {
	p.searching = false
	p.searchBuf = ""
	p.searchRe = nil
	p.searchHits = p.searchHits[:0]
}

func (p *Picker) handleSearchKey(key vaxis.Key) inputAction {
	switch {
	case key.Matches(vaxis.KeyEsc):
		p.clearSearch()
		return actionRedraw

	case key.Matches(vaxis.KeyEnter):
		p.searching = false
		return actionRedraw

	case key.Matches(vaxis.KeyBackspace):
		if len(p.searchBuf) > 0 {
			_, sz := utf8.DecodeLastRuneInString(p.searchBuf)
			p.searchBuf = p.searchBuf[:len(p.searchBuf)-sz]
			p.recompileSearch()
		}
		return actionRedraw

	default:
		if key.Text != "" {
			p.searchBuf += key.Text
			p.recompileSearch()
			return actionRedraw
		}
		return actionNone
	}
}

func (p *Picker) isSearchHit(spanIdx int) bool {
	if len(p.searchHits) == 0 {
		return false
	}
	i := sort.SearchInts(p.searchHits, spanIdx)
	return i < len(p.searchHits) && p.searchHits[i] == spanIdx
}

func appendSearchHighlight(segs []vaxis.Segment, text string, baseStyle, matchStyle vaxis.Style, re *regexp.Regexp) []vaxis.Segment {
	matches := re.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return append(segs, vaxis.Segment{Text: text, Style: matchStyle})
	}
	pos := 0
	for _, m := range matches {
		if m[0] > pos {
			segs = append(segs, vaxis.Segment{Text: text[pos:m[0]], Style: baseStyle})
		}
		segs = append(segs, vaxis.Segment{Text: text[m[0]:m[1]], Style: matchStyle})
		pos = m[1]
	}
	if pos < len(text) {
		segs = append(segs, vaxis.Segment{Text: text[pos:], Style: baseStyle})
	}
	return segs
}

// Returns nil if cancelled.
func (p *Picker) run() ([]string, error) {
	vx, err := vaxis.New(vaxis.Options{})
	if err != nil {
		return nil, fmt.Errorf("init terminal: %w", err)
	}
	defer vx.Close()

	win := vx.Window()
	p.cols, p.rows = win.Size()

	if p.rows < 3 {
		return nil, fmt.Errorf("terminal too small (need at least 3 rows, got %d)", p.rows)
	}

	p.ensureVisible()
	p.draw(vx)

	for ev := range vx.Events() {
		switch ev := ev.(type) {
		case vaxis.Key:
			action := p.handleKey(ev)
			switch action {
			case actionConfirm:
				return p.selected(), nil
			case actionCancel:
				return nil, nil
			}
		case vaxis.Resize:
			win = vx.Window()
			p.cols, p.rows = win.Size()
			p.ensureVisible()
		}
		p.draw(vx)
	}
	return nil, nil
}

type inputAction int

const (
	actionNone    inputAction = iota
	actionRedraw              // state changed, needs re-render
	actionConfirm             // user pressed Enter
	actionCancel              // user pressed Esc or Ctrl-C
)

func (p *Picker) handleKey(key vaxis.Key) inputAction {
	if p.searching {
		return p.handleSearchKey(key)
	}

	if p.pendingG {
		p.pendingG = false
		if key.Matches('g') {
			p.moveFirst()
			return actionRedraw
		}
	}

	switch {
	case key.Matches(vaxis.KeyEsc):
		if p.searchRe != nil {
			p.clearSearch()
			return actionRedraw
		}
		return actionCancel

	case key.Matches('q'),
		key.Matches('c', vaxis.ModCtrl), key.Matches('C', vaxis.ModCtrl):
		return actionCancel

	case key.Matches(vaxis.KeyEnter):
		return actionConfirm

	case key.Matches(vaxis.KeyTab):
		p.toggleCurrent()
		p.moveDown()
		return actionRedraw

	case key.Matches('j'), key.Matches(vaxis.KeyDown):
		p.moveDown()
		return actionRedraw

	case key.Matches('k'), key.Matches(vaxis.KeyUp):
		p.moveUp()
		return actionRedraw

	case key.Matches('d', vaxis.ModCtrl), key.Matches('D', vaxis.ModCtrl):
		p.moveBy((p.rows - 1) / 2)
		return actionRedraw

	case key.Matches('u', vaxis.ModCtrl), key.Matches('U', vaxis.ModCtrl):
		p.moveBy(-((p.rows - 1) / 2))
		return actionRedraw

	case key.Matches('f', vaxis.ModCtrl), key.Matches('F', vaxis.ModCtrl):
		p.moveBy(p.rows - 1)
		return actionRedraw

	case key.Matches('b', vaxis.ModCtrl), key.Matches('B', vaxis.ModCtrl):
		p.moveBy(-(p.rows - 1))
		return actionRedraw

	case key.Matches('G'):
		p.moveLast()
		return actionRedraw

	case key.Matches('g'):
		p.pendingG = true
		return actionNone

	case key.Matches('/'):
		p.searching = true
		p.searchBuf = ""
		return actionRedraw

	case key.Matches('n'):
		p.searchNext()
		return actionRedraw

	case key.Matches('N'):
		p.searchPrev()
		return actionRedraw

	default:
		return actionNone
	}
}

func (p *Picker) draw(vx *vaxis.Vaxis) {
	win := vx.Window()
	win.Clear()

	contentRows := p.rows - 1
	curSpan := p.spans[p.cursor]

	for row := range contentRows {
		lineIdx := p.viewTop + row
		if lineIdx >= len(p.lines) {
			break
		}
		p.drawLine(win, row, lineIdx, curSpan)
	}

	p.drawStatus(win)
	vx.Render()
}

var (
	stylePath     = vaxis.Style{UnderlineStyle: vaxis.UnderlineSingle}
	styleSamePath = vaxis.Style{
		Foreground:     vaxis.IndexColor(4),
		UnderlineStyle: vaxis.UnderlineSingle,
	}
	styleCursor   = vaxis.Style{Attribute: vaxis.AttrReverse}
	styleSelected = vaxis.Style{
		Foreground:     vaxis.IndexColor(2),
		UnderlineStyle: vaxis.UnderlineSingle,
	}
	styleCursorSelected = vaxis.Style{
		Foreground: vaxis.IndexColor(2),
		Attribute:  vaxis.AttrReverse,
	}
	styleSearchMatch = vaxis.Style{
		Foreground: vaxis.IndexColor(15),
		Background: vaxis.IndexColor(3),
	}
	styleStatusBar = vaxis.Style{Attribute: vaxis.AttrReverse}
)

func (p *Picker) spanStyle(text string, isCursor bool, cursorText string) vaxis.Style {
	idx := p.pathIdx[text]
	selected := p.sel[idx]

	switch {
	case isCursor && selected:
		return styleCursorSelected
	case isCursor:
		return styleCursor
	case selected:
		return styleSelected
	case text == cursorText:
		return styleSamePath
	default:
		return stylePath
	}
}

func (p *Picker) drawLine(win vaxis.Window, row, lineIdx int, curSpan Span) {
	line := p.lines[lineIdx]

	type indexedSpan struct {
		span Span
		idx  int // index in p.spans
	}

	var lineSpans []indexedSpan
	for i, s := range p.spans {
		if s.Line == lineIdx {
			lineSpans = append(lineSpans, indexedSpan{span: s, idx: i})
		}
		if s.Line > lineIdx {
			break
		}
	}

	var segs []vaxis.Segment
	pos := 0

	for _, is := range lineSpans {
		s := is.span
		if s.Start > pos {
			segs = append(segs, vaxis.Segment{Text: line[pos:s.Start]})
		}
		isCursor := s.Line == curSpan.Line && s.Start == curSpan.Start
		style := p.spanStyle(s.Text, isCursor, curSpan.Text)
		spanText := line[s.Start:s.End]
		if p.searchRe != nil && p.isSearchHit(is.idx) {
			matchStyle := style
			matchStyle.Foreground = styleSearchMatch.Foreground
			matchStyle.Background = styleSearchMatch.Background
			if isCursor {
				matchStyle.Attribute = 0
			}
			segs = appendSearchHighlight(segs, spanText, style, matchStyle, p.searchRe)
		} else {
			segs = append(segs, vaxis.Segment{Text: spanText, Style: style})
		}
		pos = s.End
	}

	if pos < len(line) {
		segs = append(segs, vaxis.Segment{Text: line[pos:]})
	}

	win.Println(row, segs...)
}

func (p *Picker) drawStatus(win vaxis.Window) {
	status := p.statusLine()
	statusRow := p.rows - 1
	statusWin := win.New(0, statusRow, p.cols, 1)
	statusWin.Fill(vaxis.Cell{Style: styleStatusBar})
	statusWin.Println(0, vaxis.Segment{Text: status, Style: styleStatusBar})
}

func (p *Picker) statusLine() string {
	if p.searching {
		return fmt.Sprintf(" /%s_", p.searchBuf)
	}

	status := fmt.Sprintf(" %d/%d matches | %d selected | Tab:select  Enter:confirm  Esc/q:cancel",
		p.cursor+1, len(p.spans), len(p.sel))

	if p.searchRe != nil {
		hitPos := -1
		for i, idx := range p.searchHits {
			if idx == p.cursor {
				hitPos = i + 1
				break
			}
		}
		if hitPos > 0 {
			status += fmt.Sprintf(" [/%s: %d/%d]", p.searchRe.String(), hitPos, len(p.searchHits))
		} else {
			status += fmt.Sprintf(" [/%s: %d matches]", p.searchRe.String(), len(p.searchHits))
		}
	}

	return status
}
