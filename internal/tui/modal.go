package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
	"github.com/muesli/reflow/wrap"
)

var modalStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(colorSubtle)

var modalSelectionStyle = lipgloss.NewStyle().Reverse(true)
var modalCursorStyle = lipgloss.NewStyle().Reverse(true)
var modalSelectionCursorStyle = modalSelectionStyle.Copy().Bold(true)

type ModalModel struct {
	open        bool
	commandID   string
	header      string
	content     string
	lines       []string
	width       int
	height      int
	totalWidth  int
	totalHeight int
	scroll      int
	cursor      modalPos
	selection   modalSelection
	dragging    bool
	lastDragPos modalPos
}

type modalPos struct {
	line int
	col  int
}

type modalSelection struct {
	active bool
	anchor modalPos
	head   modalPos
}

func NewModalModel() ModalModel {
	return ModalModel{}
}

func (m ModalModel) IsOpen() bool {
	return m.open
}

func (m ModalModel) ScrollOffset() int {
	return m.scroll
}

func (m ModalModel) CommandID() string {
	return m.commandID
}

func (m *ModalModel) Open(commandID string) {
	m.open = true
	m.commandID = commandID
	m.header = ""
	m.content = ""
	m.scroll = 0
	m.cursor = modalPos{}
	m.selection = modalSelection{}
	m.dragging = false
	m.lastDragPos = modalPos{}
}

func (m *ModalModel) Close() {
	m.open = false
	m.commandID = ""
	m.header = ""
	m.content = ""
	m.lines = nil
	m.scroll = 0
	m.cursor = modalPos{}
	m.selection = modalSelection{}
	m.dragging = false
	m.lastDragPos = modalPos{}
}

func (m *ModalModel) SetSize(totalW, totalH int) {
	m.totalWidth = totalW
	m.totalHeight = totalH
	frameW, frameH := modalStyle.GetFrameSize()
	m.width = totalW - frameW
	m.height = totalH - frameH
	if m.width < 1 {
		m.width = 1
	}
	if m.height < 1 {
		m.height = 1
	}
	m.rewrap()
}

func (m *ModalModel) SetHeader(header string) {
	m.header = strings.TrimSpace(header)
	m.rewrap()
	m.scroll = 0
	m.cursor = modalPos{}
	m.selection = modalSelection{}
}

func (m *ModalModel) SetContent(content string) {
	m.content = content
	m.rewrap()
	m.scroll = 0
	m.cursor = modalPos{}
	m.selection = modalSelection{}
}

func (m *ModalModel) ClearSelection() {
	m.selection = modalSelection{}
	m.dragging = false
	m.lastDragPos = modalPos{}
}

func (m ModalModel) View() string {
	if !m.open {
		return ""
	}
	content := m.renderContent()
	return modalStyle.
		Width(m.totalWidth).
		Height(m.totalHeight).
		Render(content)
}

func (m *ModalModel) HandleKey(msgType tea.KeyType) {
	switch msgType {
	case tea.KeyUp:
		m.Scroll(-1)
		m.selection = modalSelection{}
		if len(m.lines) > 0 {
			m.cursor.line = clamp(m.cursor.line-1, 0, len(m.lines)-1)
			m.cursor.col = clampCol(m.lines[m.cursor.line], m.cursor.col)
		}
	case tea.KeyDown:
		m.Scroll(1)
		m.selection = modalSelection{}
		if len(m.lines) > 0 {
			m.cursor.line = clamp(m.cursor.line+1, 0, len(m.lines)-1)
			m.cursor.col = clampCol(m.lines[m.cursor.line], m.cursor.col)
		}
	case tea.KeyShiftUp:
		m.moveCursor(0, -1, true)
	case tea.KeyShiftDown:
		m.moveCursor(0, 1, true)
	case tea.KeyShiftLeft:
		m.moveCursor(-1, 0, true)
	case tea.KeyShiftRight:
		m.moveCursor(1, 0, true)
	}
}

func (m *ModalModel) Scroll(delta int) {
	maxScroll := m.maxScroll()
	m.scroll += delta
	if m.scroll < 0 {
		m.scroll = 0
	}
	if m.scroll > maxScroll {
		m.scroll = maxScroll
	}
}

func (m *ModalModel) PageScroll(delta int) {
	page := m.height - 1
	if page < 1 {
		page = 1
	}
	m.Scroll(delta * page)
}

func (m *ModalModel) BeginSelection(line, col int) {
	if len(m.lines) == 0 {
		return
	}
	line = clamp(line, 0, len(m.lines)-1)
	col = clampCol(m.lines[line], col)
	pos := modalPos{line: line, col: col}
	m.cursor = pos
	m.selection = modalSelection{active: true, anchor: pos, head: pos}
	m.dragging = true
	m.ensureCursorVisible()
}

func (m *ModalModel) UpdateSelection(line, col int) {
	if !m.dragging || len(m.lines) == 0 {
		return
	}
	line = clamp(line, 0, len(m.lines)-1)
	col = clampCol(m.lines[line], col)
	pos := modalPos{line: line, col: col}
	if m.dragging && comparePos(pos, m.lastDragPos) == 0 {
		return
	}
	m.cursor = pos
	m.selection.head = pos
	m.ensureCursorVisible()
	m.lastDragPos = pos
}

func (m *ModalModel) EndSelection() {
	m.dragging = false
}

func (m ModalModel) SelectedText() string {
	start, end, ok := m.selectionRange()
	if !ok {
		return ""
	}
	if start.line == end.line {
		return sliceByCell(m.lines[start.line], start.col, end.col)
	}
	parts := make([]string, 0, end.line-start.line+1)
	for i := start.line; i <= end.line; i++ {
		line := m.lines[i]
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

func (m *ModalModel) rewrap() {
	if m.width < 1 {
		return
	}
	lines := wrapContent(m.combinedContent(), m.width)
	if len(lines) == 0 {
		lines = []string{""}
	}
	m.lines = lines
	if m.cursor.line >= len(m.lines) {
		m.cursor.line = len(m.lines) - 1
	}
	if m.cursor.line < 0 {
		m.cursor.line = 0
	}
	if len(m.lines) > 0 {
		m.cursor.col = clampCol(m.lines[m.cursor.line], m.cursor.col)
	}
	maxScroll := m.maxScroll()
	if m.scroll > maxScroll {
		m.scroll = maxScroll
	}
}

func (m ModalModel) renderContent() string {
	visible := m.visibleLines()
	for i := range visible {
		lineIndex := m.scroll + i
		startCol, endCol, ok := m.selectionForLine(lineIndex)
		cursorCol := -1
		if lineIndex == m.cursor.line {
			cursorCol = m.cursor.col
		}
		visible[i] = highlightLineWithCursor(visible[i], startCol, endCol, cursorCol, ok)
		visible[i] = padToWidth(visible[i], m.width)
	}
	return strings.Join(visible, "\n")
}

func (m ModalModel) visibleLines() []string {
	lines := make([]string, 0, m.height)
	for i := 0; i < m.height; i++ {
		idx := m.scroll + i
		if idx >= 0 && idx < len(m.lines) {
			lines = append(lines, m.lines[idx])
		} else {
			lines = append(lines, "")
		}
	}
	if len(lines) == 0 {
		lines = []string{""}
	}
	return lines
}

func (m ModalModel) selectionRange() (modalPos, modalPos, bool) {
	if !m.selection.active {
		return modalPos{}, modalPos{}, false
	}
	if comparePos(m.selection.anchor, m.selection.head) == 0 {
		return modalPos{}, modalPos{}, false
	}
	if comparePos(m.selection.anchor, m.selection.head) < 0 {
		return m.selection.anchor, m.selection.head, true
	}
	return m.selection.head, m.selection.anchor, true
}

func (m ModalModel) selectionForLine(line int) (int, int, bool) {
	start, end, ok := m.selectionRange()
	if !ok {
		return 0, 0, false
	}
	if line < start.line || line > end.line {
		return 0, 0, false
	}
	lineWidth := lineCellWidth(m.lines[line])
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

func (m *ModalModel) moveCursor(dx, dy int, extend bool) {
	if len(m.lines) == 0 {
		return
	}
	if !extend {
		m.selection = modalSelection{}
	}
	if extend && !m.selection.active {
		m.selection = modalSelection{active: true, anchor: m.cursor, head: m.cursor}
	}
	newLine := clamp(m.cursor.line+dy, 0, len(m.lines)-1)
	newCol := m.cursor.col
	if dy != 0 {
		newCol = clampCol(m.lines[newLine], newCol)
	} else {
		newCol = clampCol(m.lines[newLine], newCol+dx)
	}
	m.cursor = modalPos{line: newLine, col: newCol}
	if m.selection.active {
		m.selection.head = m.cursor
	}
	m.ensureCursorVisible()
}

func (m *ModalModel) ensureCursorVisible() {
	if m.height <= 0 {
		return
	}
	if m.cursor.line < m.scroll {
		m.scroll = m.cursor.line
		return
	}
	maxVisible := m.scroll + m.height - 1
	if m.cursor.line > maxVisible {
		m.scroll = m.cursor.line - m.height + 1
	}
	maxScroll := m.maxScroll()
	if m.scroll > maxScroll {
		m.scroll = maxScroll
	}
}

func (m ModalModel) maxScroll() int {
	max := len(m.lines) - m.height
	if max < 0 {
		max = 0
	}
	return max
}

func (m ModalModel) combinedContent() string {
	if m.header == "" {
		return m.content
	}
	if m.content == "" {
		return m.header
	}
	return m.header + "\n\n" + m.content
}

func wrapContent(content string, width int) []string {
	if width < 1 {
		return []string{""}
	}
	parts := strings.Split(content, "\n")
	var out []string
	for _, line := range parts {
		if line == "" {
			out = append(out, "")
			continue
		}
		if len(line) <= width && isASCII(line) {
			out = append(out, line)
			continue
		}
		if lipgloss.Width(line) <= width {
			out = append(out, line)
			continue
		}
		wrapped := wordwrap.String(line, width)
		wrapped = wrap.String(wrapped, width)
		out = append(out, strings.Split(wrapped, "\n")...)
	}
	return out
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > 127 {
			return false
		}
	}
	return true
}

func highlightLine(line string, startCol, endCol int) string {
	if startCol >= endCol {
		return line
	}
	var b strings.Builder
	cur := 0
	for _, r := range line {
		rw := lipgloss.Width(string(r))
		next := cur + rw
		selected := next > startCol && cur < endCol
		if selected {
			b.WriteString(modalSelectionStyle.Render(string(r)))
		} else {
			b.WriteRune(r)
		}
		cur = next
	}
	return b.String()
}

func highlightCursorLine(line string, col int) string {
	width := lineCellWidth(line)
	if width == 0 {
		return line
	}
	if col < 0 {
		col = 0
	}
	if col >= width {
		col = width - 1
	}
	var b strings.Builder
	cur := 0
	for _, r := range line {
		rw := lipgloss.Width(string(r))
		next := cur + rw
		if cur <= col && col < next {
			b.WriteString(modalCursorStyle.Render(string(r)))
		} else {
			b.WriteRune(r)
		}
		cur = next
	}
	return b.String()
}

func highlightLineWithCursor(line string, startCol, endCol, cursorCol int, hasSelection bool) string {
	if !hasSelection && cursorCol < 0 {
		return line
	}
	width := lineCellWidth(line)
	if width == 0 {
		return line
	}
	if cursorCol >= width {
		cursorCol = width - 1
	}
	if cursorCol < 0 {
		cursorCol = -1
	}
	if hasSelection && cursorCol < 0 && startCol <= 0 && endCol >= width {
		return modalSelectionStyle.Render(line)
	}
	var b strings.Builder
	cur := 0
	for _, r := range line {
		rw := lipgloss.Width(string(r))
		next := cur + rw
		inSelection := hasSelection && next > startCol && cur < endCol
		isCursor := cursorCol >= 0 && cur <= cursorCol && cursorCol < next
		cell := string(r)
		switch {
		case inSelection && isCursor:
			cell = modalSelectionCursorStyle.Render(cell)
		case inSelection:
			cell = modalSelectionStyle.Render(cell)
		case isCursor:
			cell = modalCursorStyle.Render(cell)
		}
		b.WriteString(cell)
		cur = next
	}
	return b.String()
}

func highlightLineRangeWithWidth(line string, lineWidth, startCol, endCol int, hasSelection bool) string {
	if !hasSelection {
		return line
	}
	if lineWidth <= 0 {
		lineWidth = lineCellWidth(line)
	}
	startCol = clamp(startCol, 0, lineWidth)
	endCol = clamp(endCol, 0, lineWidth)
	if startCol >= endCol {
		return line
	}
	if startCol == 0 && endCol >= lineWidth {
		return modalSelectionStyle.Render(line)
	}
	prefix, selected, suffix := splitByCellRange(line, startCol, endCol)
	if selected == "" {
		return line
	}
	return prefix + modalSelectionStyle.Render(selected) + suffix
}

func splitByCellRange(line string, startCol, endCol int) (string, string, string) {
	var prefix strings.Builder
	var selected strings.Builder
	var suffix strings.Builder
	cur := 0
	for _, r := range line {
		rw := lipgloss.Width(string(r))
		next := cur + rw
		if next <= startCol {
			prefix.WriteRune(r)
		} else if cur >= endCol {
			suffix.WriteRune(r)
		} else {
			selected.WriteRune(r)
		}
		cur = next
	}
	return prefix.String(), selected.String(), suffix.String()
}

func sliceByCell(line string, startCol, endCol int) string {
	if startCol >= endCol {
		return ""
	}
	startCol = clamp(startCol, 0, lineCellWidth(line))
	endCol = clamp(endCol, 0, lineCellWidth(line))
	if startCol >= endCol {
		return ""
	}
	var b strings.Builder
	cur := 0
	for _, r := range line {
		rw := lipgloss.Width(string(r))
		next := cur + rw
		if next <= startCol {
			cur = next
			continue
		}
		if cur >= endCol {
			break
		}
		b.WriteRune(r)
		cur = next
	}
	return b.String()
}

func lineCellWidth(line string) int {
	return lipgloss.Width(line)
}

func clampCol(line string, col int) int {
	width := lineCellWidth(line)
	return clamp(col, 0, width)
}

func comparePos(a, b modalPos) int {
	if a.line < b.line {
		return -1
	}
	if a.line > b.line {
		return 1
	}
	if a.col < b.col {
		return -1
	}
	if a.col > b.col {
		return 1
	}
	return 0
}

func clamp(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
