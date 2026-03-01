package main

import (
	"cmp"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"git.sr.ht/~rockorager/vaxis"
)

var errNoMatches = errors.New("no matches found in input")

const hintAlphabet = "asdfghjkl;'"

type hintLabel struct {
	spanIdx int
	label   string
}

type indexedSpan struct {
	span Span
	idx  int
}

type spanKey struct {
	line  int
	start int
	end   int
	text  string
}

func keyForSpan(s Span) spanKey {
	return spanKey{line: s.Line, start: s.Start, end: s.End, text: s.Text}
}

func generateHintLabels(n int) []string {
	if n <= 0 {
		return nil
	}
	alpha := []rune(hintAlphabet)
	if n <= len(alpha) {
		labels := make([]string, n)
		for i := range n {
			labels[i] = string(alpha[i])
		}
		return labels
	}
	var labels []string
	for _, a := range alpha {
		for _, b := range alpha {
			labels = append(labels, string([]rune{a, b}))
			if len(labels) >= n {
				return labels
			}
		}
	}
	return labels
}

type Picker struct {
	lines []styledLine
	spans []Span
	sel   map[spanKey]bool

	cursor    int
	viewTop   int
	cols      int
	rows      int
	pendingG  bool // waiting for second 'g' in gg sequence
	inputDone bool // all input has been read

	searching  bool           // in search input mode
	searchBuf  string         // current search input text
	searchRe   *regexp.Regexp // compiled regex (nil = no active search)
	searchHits []int          // indices into p.spans that match
	searchPos  int            // current position in searchHits for n/N cycling

	hinting    bool        // in hint mode
	hintBuf    string      // chars typed so far
	hintLabels []hintLabel // computed on entering hint mode
}

func newPicker() *Picker {
	return &Picker{sel: make(map[spanKey]bool)}
}

func (p *Picker) appendData(lines []styledLine, spans []Span) {
	var curSpan Span
	if len(p.spans) > 0 {
		curSpan = p.spans[p.cursor]
	}

	p.lines = append(p.lines, lines...)
	p.spans = append(p.spans, spans...)

	slices.SortFunc(p.spans, func(a, b Span) int {
		if c := cmp.Compare(a.Line, b.Line); c != 0 {
			return c
		}
		return cmp.Compare(a.Start, b.Start)
	})

	if curSpan != (Span{}) {
		for i, s := range p.spans {
			if s.Line == curSpan.Line && s.Start == curSpan.Start {
				p.cursor = i
				break
			}
		}
	}

	if p.searchRe != nil {
		p.updateSearchHits()
	}
}

func (p *Picker) ensureVisible() {
	if len(p.spans) == 0 {
		return
	}

	line := p.spans[p.cursor].Line
	contentRows := p.rows - 1

	if line < p.viewTop {
		p.viewTop = line
	}
	if line >= p.viewTop+contentRows {
		p.viewTop = line - contentRows + 1
	}
}

func (p *Picker) selected() []string {
	if len(p.sel) == 0 {
		return []string{p.spans[p.cursor].Text}
	}

	var texts []string
	for _, s := range p.spans {
		if p.sel[keyForSpan(s)] {
			texts = append(texts, s.Text)
		}
	}

	return texts
}

func (p *Picker) toggleCurrent() {
	key := keyForSpan(p.spans[p.cursor])
	if p.sel[key] {
		delete(p.sel, key)
	} else {
		p.sel[key] = true
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
	if len(p.spans) == 0 {
		return
	}
	p.cursor = len(p.spans) - 1
	p.ensureVisible()
}

func (p *Picker) moveBy(n int) {
	if len(p.spans) == 0 {
		return
	}

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

func (p *Picker) buildHintLabels() {
	p.hintLabels = p.hintLabels[:0]
	contentRows := p.rows - 1

	var visible []int
	for i, s := range p.spans {
		if s.Line >= p.viewTop && s.Line < p.viewTop+contentRows {
			visible = append(visible, i)
		}
	}

	labels := generateHintLabels(len(visible))
	for i, idx := range visible {
		p.hintLabels = append(p.hintLabels, hintLabel{spanIdx: idx, label: labels[i]})
	}
}

func (p *Picker) exitHintMode() {
	p.hinting = false
	p.hintBuf = ""
	p.hintLabels = p.hintLabels[:0]
}

func (p *Picker) handleHintKey(key vaxis.Key) inputAction {
	if key.Matches(vaxis.KeyEsc) {
		p.exitHintMode()
		return actionRedraw
	}

	ch := key.Text
	if ch == "" || !strings.Contains(hintAlphabet, ch) {
		p.exitHintMode()
		return actionRedraw
	}

	p.hintBuf += ch

	for _, hl := range p.hintLabels {
		if hl.label == p.hintBuf {
			p.sel = make(map[spanKey]bool)
			p.cursor = hl.spanIdx
			p.exitHintMode()
			return actionConfirm
		}
	}

	hasPrefix := false
	for _, hl := range p.hintLabels {
		if strings.HasPrefix(hl.label, p.hintBuf) {
			hasPrefix = true
			break
		}
	}

	if !hasPrefix {
		p.exitHintMode()
		return actionRedraw
	}

	return actionRedraw
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
func (p *Picker) run(onStart func(postEvent func(any))) ([]string, error) {
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

	onStart(func(ev any) { vx.PostEvent(ev) })
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

		case newDataEvent:
			if p.hinting {
				p.exitHintMode()
			}
			p.appendData(ev.lines, ev.spans)

		case inputDoneEvent:
			p.inputDone = true
			if len(p.spans) == 0 {
				return nil, errNoMatches
			}

		case inputErrorEvent:
			return nil, ev.err
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
	if p.hinting {
		return p.handleHintKey(key)
	}

	if p.searching {
		return p.handleSearchKey(key)
	}

	if p.pendingG {
		p.pendingG = false
		if key.Matches('g') {
			if len(p.spans) == 0 {
				return actionNone
			}
			p.moveFirst()
			return actionRedraw
		}
	}

	// Actions that don't require spans.
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

	case key.Matches('/'):
		p.searching = true
		p.searchBuf = ""
		return actionRedraw
	}

	if len(p.spans) == 0 {
		return actionNone
	}

	switch {
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

	case key.Matches('n'):
		p.searchNext()
		return actionRedraw

	case key.Matches('N'):
		p.searchPrev()
		return actionRedraw

	case key.Matches('f'):
		p.hinting = true
		p.hintBuf = ""
		p.buildHintLabels()
		if len(p.hintLabels) == 0 {
			p.hinting = false
			return actionNone
		}
		return actionRedraw

	default:
		return actionNone
	}
}

func (p *Picker) draw(vx *vaxis.Vaxis) {
	win := vx.Window()
	win.Clear()
	contentRows := p.rows - 1

	var curSpan Span
	if len(p.spans) > 0 {
		curSpan = p.spans[p.cursor]
	}

	for row := range contentRows {
		lineIdx := p.viewTop + row
		if lineIdx >= len(p.lines) {
			break
		}

		p.drawLine(win, row, lineIdx, curSpan)
		if p.hinting {
			p.drawHintOverlay(win, row, lineIdx)
		}
	}

	p.drawStatus(win)
	vx.Render()
}

var (
	stylePath     = vaxis.Style{UnderlineStyle: vaxis.UnderlineSingle}
	styleCursor   = vaxis.Style{Attribute: vaxis.AttrReverse}
	styleSelected = vaxis.Style{
		Foreground:     vaxis.IndexColor(15),
		Background:     vaxis.IndexColor(2),
		UnderlineStyle: vaxis.UnderlineSingle,
	}
	styleCursorSelected = vaxis.Style{
		Foreground: vaxis.IndexColor(15),
		Background: vaxis.IndexColor(2),
		Attribute:  vaxis.AttrReverse,
	}
	styleSearchMatch = vaxis.Style{
		Foreground: vaxis.IndexColor(15),
		Background: vaxis.IndexColor(3),
	}
	styleStatusBar = vaxis.Style{Attribute: vaxis.AttrReverse}
	styleHintLabel = vaxis.Style{
		Foreground: vaxis.IndexColor(15),
		Background: vaxis.IndexColor(3),
	}
	styleHintTyped = vaxis.Style{
		Foreground: vaxis.IndexColor(15),
		Background: vaxis.IndexColor(8),
	}
)

func (p *Picker) spanStyle(span Span, isCursor bool, _ string) vaxis.Style {
	selected := p.sel[keyForSpan(span)]

	switch {
	case isCursor && selected:
		return styleCursorSelected
	case isCursor:
		return styleCursor
	case selected:
		return styleSelected
	default:
		return stylePath
	}
}

func (p *Picker) drawLine(win vaxis.Window, row, lineIdx int, curSpan Span) {
	sl := p.lines[lineIdx]
	if sl.cells != nil {
		p.drawStyledLine(win, row, sl, lineIdx, curSpan)
		return
	}
	p.drawPlainLine(win, row, sl.text, lineIdx, curSpan)
}

func (p *Picker) drawPlainLine(win vaxis.Window, row int, line string, lineIdx int, curSpan Span) {
	var lineSpans []indexedSpan
	start := sort.Search(len(p.spans), func(i int) bool {
		return p.spans[i].Line >= lineIdx
	})
	for i := start; i < len(p.spans) && p.spans[i].Line == lineIdx; i++ {
		lineSpans = append(lineSpans, indexedSpan{span: p.spans[i], idx: i})
	}

	var segs []vaxis.Segment
	pos := 0

	for _, is := range lineSpans {
		s := is.span

		if s.Start > pos {
			segs = append(segs, vaxis.Segment{Text: line[pos:s.Start]})
		}

		isCursor := s.Line == curSpan.Line && s.Start == curSpan.Start
		style := p.spanStyle(s, isCursor, curSpan.Text)
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

func (p *Picker) drawStyledLine(win vaxis.Window, row int, sl styledLine, lineIdx int, curSpan Span) {
	var lineSpans []indexedSpan

	start := sort.Search(len(p.spans), func(i int) bool {
		return p.spans[i].Line >= lineIdx
	})
	for i := start; i < len(p.spans) && p.spans[i].Line == lineIdx; i++ {
		lineSpans = append(lineSpans, indexedSpan{span: p.spans[i], idx: i})
	}

	var searchMatches [][2]int
	if p.searchRe != nil {
		for _, m := range p.searchRe.FindAllStringIndex(sl.text, -1) {
			searchMatches = append(searchMatches, [2]int{m[0], m[1]})
		}
	}

	var segs []vaxis.Segment
	byteOff := 0
	si := 0

	for _, cell := range sl.cells {
		graphLen := len(cell.Grapheme)

		for si < len(lineSpans) && lineSpans[si].span.End <= byteOff {
			si++
		}

		style := cell.Style
		inSpan := si < len(lineSpans) && byteOff >= lineSpans[si].span.Start && byteOff < lineSpans[si].span.End

		if inSpan {
			is := lineSpans[si]
			s := is.span
			isCursor := s.Line == curSpan.Line && s.Start == curSpan.Start
			style = p.mergeSpanStyle(cell.Style, s, isCursor, curSpan.Text)

			if p.searchRe != nil && p.isSearchHit(is.idx) {
				for _, m := range searchMatches {
					if byteOff >= m[0] && byteOff < m[1] {
						style.Foreground = styleSearchMatch.Foreground
						style.Background = styleSearchMatch.Background
						if isCursor {
							style.Attribute = 0
						}
						break
					}
				}
			}
		}

		if len(segs) > 0 && segs[len(segs)-1].Style == style {
			segs[len(segs)-1].Text += cell.Grapheme
		} else {
			segs = append(segs, vaxis.Segment{Text: cell.Grapheme, Style: style})
		}

		byteOff += graphLen
	}

	win.Println(row, segs...)
}

func (p *Picker) drawHintOverlay(win vaxis.Window, row, lineIdx int) {
	hintBySpan := make(map[int]*hintLabel)
	for i := range p.hintLabels {
		hintBySpan[p.hintLabels[i].spanIdx] = &p.hintLabels[i]
	}

	sl := p.lines[lineIdx]

	start := sort.Search(len(p.spans), func(i int) bool {
		return p.spans[i].Line >= lineIdx
	})
	for i := start; i < len(p.spans) && p.spans[i].Line == lineIdx; i++ {
		hl := hintBySpan[i]
		if hl == nil || !strings.HasPrefix(hl.label, p.hintBuf) {
			continue
		}

		col := hintColForSpan(sl, p.spans[i])
		typedLen := utf8.RuneCountInString(p.hintBuf)

		for j, r := range []rune(hl.label) {
			style := styleHintLabel
			if j < typedLen {
				style = styleHintTyped
			}

			win.SetCell(col+j, row, vaxis.Cell{
				Character: vaxis.Character{Grapheme: string(r), Width: 1},
				Style:     style,
			})
		}
	}
}

func colForByte(sl styledLine, bytePos int) int {
	if sl.cells == nil {
		return textColForByte(sl.text, bytePos)
	}

	col := 0
	off := 0

	for _, c := range sl.cells {
		if off >= bytePos {
			return col
		}
		off += len(c.Grapheme)
		col += c.Width
	}

	return col
}

func hintColForSpan(sl styledLine, span Span) int {
	if span.Start <= len(sl.text) {
		prefix := sl.text[:span.Start]
		if strings.TrimSpace(prefix) == "" {
			return 0
		}
	}
	return colForByte(sl, span.Start)
}

func (p *Picker) mergeSpanStyle(base vaxis.Style, span Span, isCursor bool, _ string) vaxis.Style {
	selected := p.sel[keyForSpan(span)]
	s := base

	switch {
	case isCursor && selected:
		s.Foreground = vaxis.IndexColor(15)
		s.Background = vaxis.IndexColor(2)
		s.Attribute |= vaxis.AttrReverse
	case isCursor:
		s.Attribute |= vaxis.AttrReverse
	case selected:
		s.Foreground = vaxis.IndexColor(15)
		s.Background = vaxis.IndexColor(2)
		s.UnderlineStyle = vaxis.UnderlineSingle
	default:
		s.UnderlineStyle = vaxis.UnderlineSingle
	}
	return s
}

func (p *Picker) drawStatus(win vaxis.Window) {
	status := p.statusLine()
	statusRow := p.rows - 1
	statusWin := win.New(0, statusRow, p.cols, 1)
	statusWin.Fill(vaxis.Cell{Style: styleStatusBar})
	statusWin.Println(0, vaxis.Segment{Text: status, Style: styleStatusBar})
}

func (p *Picker) statusLine() string {
	if p.hinting {
		if p.hintBuf == "" {
			return " HINTS: type a label to select"
		}
		return fmt.Sprintf(" HINTS: %s_", p.hintBuf)
	}

	if p.searching {
		return fmt.Sprintf(" /%s_", p.searchBuf)
	}

	if !p.inputDone {
		if len(p.spans) == 0 {
			return " Reading input..."
		}
		return fmt.Sprintf(" Reading... %d matches so far", len(p.spans))
	}

	status := fmt.Sprintf(" %d/%d matches | %d selected | Tab:select  f:hints  Enter:confirm  Esc/q:cancel",
		p.cursor+1, len(p.spans), len(p.sel))
	if p.searchRe == nil {
		return status
	}

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

	return status
}
