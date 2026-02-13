package tui

import "strings"

type textSelection struct {
	lines       []string
	selection   modalSelection
	dragging    bool
	lastDragPos modalPos
	cached      []string
	cachedWidth int
	cachedStart modalPos
	cachedEnd   modalPos
	cachedOk    bool
}

func (s *textSelection) SetLines(lines []string) {
	if linesEqual(s.lines, lines) {
		return
	}
	s.lines = lines
	s.clampSelection()
	s.cached = nil
	s.cachedWidth = 0
	s.cachedOk = false
}

func (s *textSelection) Clear() {
	s.selection = modalSelection{}
	s.dragging = false
	s.lastDragPos = modalPos{}
	s.cached = nil
	s.cachedWidth = 0
	s.cachedOk = false
}

func (s *textSelection) BeginSelection(line, col int) {
	if len(s.lines) == 0 {
		return
	}
	line = clamp(line, 0, len(s.lines)-1)
	col = clampCol(s.lines[line], col)
	pos := modalPos{line: line, col: col}
	s.selection = modalSelection{active: true, anchor: pos, head: pos}
	s.dragging = true
	s.lastDragPos = pos
}

func (s *textSelection) UpdateSelection(line, col int) {
	if !s.dragging || len(s.lines) == 0 {
		return
	}
	line = clamp(line, 0, len(s.lines)-1)
	col = clampCol(s.lines[line], col)
	pos := modalPos{line: line, col: col}
	if comparePos(pos, s.lastDragPos) == 0 {
		return
	}
	s.selection.head = pos
	s.lastDragPos = pos
}

func (s *textSelection) EndSelection() {
	s.dragging = false
}

func (s textSelection) HasSelection() bool {
	_, _, ok := s.selectionRange()
	return ok
}

func (s textSelection) SelectedText() string {
	start, end, ok := s.selectionRange()
	if !ok {
		return ""
	}
	if start.line == end.line {
		return sliceByCell(s.lines[start.line], start.col, end.col)
	}
	parts := make([]string, 0, end.line-start.line+1)
	for i := start.line; i <= end.line; i++ {
		line := s.lines[i]
		lineWidth := lineCellWidth(line)
		switch {
		case i == start.line:
			parts = append(parts, sliceByCell(line, start.col, lineWidth))
		case i == end.line:
			parts = append(parts, sliceByCell(line, 0, end.col))
		default:
			parts = append(parts, line)
		}
	}
	return strings.Join(parts, "\n")
}

func (s textSelection) HighlightLines(width int) []string {
	if len(s.lines) == 0 {
		return []string{""}
	}
	if len(s.cached) != len(s.lines) || s.cachedWidth != width {
		s.cached = make([]string, len(s.lines))
		for i := range s.lines {
			s.cached[i] = padToWidth(s.lines[i], width)
		}
		s.cachedWidth = width
		s.cachedOk = false
	}
	if !s.HasSelection() {
		if s.cachedOk {
			for i := range s.lines {
				s.cached[i] = padToWidth(s.lines[i], width)
			}
			s.cachedOk = false
		}
		return s.cached
	}
	start, end, ok := s.selectionRange()
	if !ok {
		return s.cached
	}
	fromLine := start.line
	toLine := end.line
	if s.cachedOk {
		fromLine = minInt(fromLine, s.cachedStart.line)
		toLine = maxInt(toLine, s.cachedEnd.line)
	}
	fromLine = clamp(fromLine, 0, len(s.lines)-1)
	toLine = clamp(toLine, 0, len(s.lines)-1)
	for i := fromLine; i <= toLine; i++ {
		startCol, endCol, has := s.selectionForLine(i)
		s.cached[i] = highlightLineWithCursor(s.lines[i], startCol, endCol, -1, has)
		s.cached[i] = padToWidth(s.cached[i], width)
	}
	s.cachedStart = start
	s.cachedEnd = end
	s.cachedOk = true
	return s.cached
}

func (s textSelection) selectionRange() (modalPos, modalPos, bool) {
	if !s.selection.active {
		return modalPos{}, modalPos{}, false
	}
	if comparePos(s.selection.anchor, s.selection.head) == 0 {
		return modalPos{}, modalPos{}, false
	}
	if comparePos(s.selection.anchor, s.selection.head) < 0 {
		return s.selection.anchor, s.selection.head, true
	}
	return s.selection.head, s.selection.anchor, true
}

func (s textSelection) selectionForLine(line int) (int, int, bool) {
	start, end, ok := s.selectionRange()
	if !ok {
		return 0, 0, false
	}
	if line < start.line || line > end.line {
		return 0, 0, false
	}
	lineWidth := lineCellWidth(s.lines[line])
	if start.line == end.line {
		return clamp(start.col, 0, lineWidth), clamp(end.col, 0, lineWidth), true
	}
	if line == start.line {
		return clamp(start.col, 0, lineWidth), lineWidth, true
	}
	if line == end.line {
		return 0, clamp(end.col, 0, lineWidth), true
	}
	return 0, lineWidth, true
}

func (s *textSelection) clampSelection() {
	if !s.selection.active || len(s.lines) == 0 {
		return
	}
	maxLine := len(s.lines) - 1
	s.selection.anchor.line = clamp(s.selection.anchor.line, 0, maxLine)
	s.selection.head.line = clamp(s.selection.head.line, 0, maxLine)
	s.selection.anchor.col = clampCol(s.lines[s.selection.anchor.line], s.selection.anchor.col)
	s.selection.head.col = clampCol(s.lines[s.selection.head.line], s.selection.head.col)
}

func linesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
