package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/termia/termia/internal/db"
)

type HistoryDetailModel struct {
	command         *db.Command
	content         string
	lines           []string
	width           int
	height          int
	contentHeight   int
	ready           bool
	loading         bool
	keys            KeyMap
	scroll          int
	cursor          modalPos
	selection       modalSelection
	dragging        bool
	lastDragPos     modalPos
	scrollByCommand map[string]int
}

func NewHistoryDetailModel(keys KeyMap) HistoryDetailModel {
	return HistoryDetailModel{
		keys:            keys,
		scrollByCommand: make(map[string]int),
	}
}

func (m *HistoryDetailModel) SetSize(w, h int) {
	if w < 1 {
		w = 1
	}
	headerHeight := m.HeaderHeight()
	contentHeight := h - headerHeight
	if contentHeight < 1 {
		contentHeight = 1
	}
	widthChanged := w != m.width
	m.width = w
	m.height = h
	m.contentHeight = contentHeight
	if !m.ready {
		m.ready = true
	}
	if widthChanged {
		m.rewrap()
		return
	}
	m.clampScroll()
}

func (m *HistoryDetailModel) SetCommand(cmd *db.Command) {
	m.storeScrollPosition()
	m.command = cmd
	m.loading = true
	m.content = ""
	m.lines = nil
	m.scroll = 0
	m.cursor = modalPos{}
	m.selection = modalSelection{}
	m.dragging = false
	m.lastDragPos = modalPos{}
	if cmd != nil {
		m.scroll = m.scrollByCommand[cmd.ID]
	}
	if m.ready {
		m.rewrap()
	}
}

func (m HistoryDetailModel) CommandID() string {
	if m.command == nil {
		return ""
	}
	return m.command.ID
}

func (m HistoryDetailModel) ScrollOffset() int {
	return m.scroll
}

func (m HistoryDetailModel) HeaderHeight() int {
	_, frameH := previewHeaderStyle.GetFrameSize()
	return 1 + frameH
}

func (m HistoryDetailModel) ContentHeight() int {
	return m.contentHeight
}

func (m *HistoryDetailModel) SetContent(content string) {
	wasLoading := m.loading
	m.content = normalizeDetailContent(content)
	m.loading = false
	if wasLoading && m.command != nil {
		if saved, ok := m.scrollByCommand[m.command.ID]; ok {
			m.scroll = saved
		}
	}
	if m.ready {
		m.rewrap()
	}
}

func (m *HistoryDetailModel) ClearContent() {
	m.storeScrollPosition()
	m.content = ""
	m.command = nil
	m.loading = false
	m.lines = nil
	m.scroll = 0
	m.cursor = modalPos{}
	m.selection = modalSelection{}
	m.dragging = false
	m.lastDragPos = modalPos{}
	if m.ready {
		m.rewrap()
	}
}

func (m *HistoryDetailModel) ClearSelection() {
	m.selection = modalSelection{}
	m.dragging = false
	m.lastDragPos = modalPos{}
}

func (m HistoryDetailModel) Update(msg tea.Msg) (HistoryDetailModel, tea.Cmd) {
	if !m.ready {
		return m, nil
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.PageUp):
			m.PageScroll(-1)
			return m, nil
		case key.Matches(msg, m.keys.PageDown):
			m.PageScroll(1)
			return m, nil
		case key.Matches(msg, m.keys.HalfUp):
			m.HalfScroll(-1)
			return m, nil
		case key.Matches(msg, m.keys.HalfDown):
			m.HalfScroll(1)
			return m, nil
		case key.Matches(msg, m.keys.GotoTop):
			m.GotoTop()
			return m, nil
		case key.Matches(msg, m.keys.GotoEnd):
			m.GotoEnd()
			return m, nil
		}
		switch msg.Type {
		case tea.KeyUp:
			m.moveCursor(0, -1, false)
		case tea.KeyDown:
			m.moveCursor(0, 1, false)
		case tea.KeyLeft:
			m.moveCursor(-1, 0, false)
		case tea.KeyRight:
			m.moveCursor(1, 0, false)
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
	return m, nil
}

func (m HistoryDetailModel) View() string {
	if !m.ready {
		return ""
	}

	header := m.renderHeader()
	content := m.renderContent()
	joined := lipgloss.JoinVertical(lipgloss.Left, header, content)
	return lipgloss.NewStyle().Width(m.width).Height(m.height).Render(joined)
}

func (m HistoryDetailModel) renderHeader() string {
	if m.command == nil {
		return previewHeaderStyle.Width(m.width).Render("No command selected")
	}

	cmd := m.command
	innerWidth := m.width - previewHeaderStyle.GetHorizontalPadding()
	if innerWidth < 1 {
		innerWidth = 1
	}
	rightParts := []string{}
	if m.loading {
		rightParts = append(rightParts, loadingStyle.Render("loading..."))
	} else {
		scrollPct := fmt.Sprintf("%.0f%%", m.scrollPercent()*100)
		rightParts = append(rightParts, metaStyle.Render(scrollPct))
	}
	rightParts = append(rightParts, metaStyle.Render("Esc"))
	right := strings.Join(rightParts, " | ")
	cmdText := strings.ReplaceAll(cmd.Command, "\n", " ")
	cmdText = strings.ReplaceAll(cmdText, "\r", "")
	cwdText := strings.TrimSpace(cmd.Cwd)
	if cwdText != "" {
		cwdText = strings.ReplaceAll(cwdText, "\n", " ")
		cwdText = strings.ReplaceAll(cwdText, "\r", "")
	}
	leftParts := []string{}
	if cwdText != "" {
		leftParts = append(leftParts, cwdStyle.Render(cwdText))
	}
	if cmdText != "" {
		leftParts = append(leftParts, cmdText)
	}
	left := strings.Join(leftParts, " | ")
	header := buildStatusLine(left, right, innerWidth)
	return previewHeaderStyle.Width(m.width).Render(header)
}

func (m *HistoryDetailModel) BeginSelection(line, col int) {
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

func (m *HistoryDetailModel) UpdateSelection(line, col int) {
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

func (m *HistoryDetailModel) EndSelection() {
	m.dragging = false
}

func (m HistoryDetailModel) SelectedText() string {
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

func (m *HistoryDetailModel) Scroll(delta int) {
	m.scroll += delta
	m.clampScroll()
}

func (m *HistoryDetailModel) PageScroll(delta int) {
	page := m.contentHeight - 1
	if page < 1 {
		page = 1
	}
	m.Scroll(delta * page)
}

func (m *HistoryDetailModel) HalfScroll(delta int) {
	page := m.contentHeight / 2
	if page < 1 {
		page = 1
	}
	m.Scroll(delta * page)
}

func (m *HistoryDetailModel) GotoTop() {
	m.scroll = 0
}

func (m *HistoryDetailModel) GotoEnd() {
	m.scroll = m.maxScroll()
}

func (m *HistoryDetailModel) clampScroll() {
	maxScroll := m.maxScroll()
	if m.scroll < 0 {
		m.scroll = 0
	}
	if m.scroll > maxScroll {
		m.scroll = maxScroll
	}
}

func (m *HistoryDetailModel) moveCursor(dx, dy int, extend bool) {
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

func (m *HistoryDetailModel) ensureCursorVisible() {
	if m.contentHeight <= 0 {
		return
	}
	if m.cursor.line < m.scroll {
		m.scroll = m.cursor.line
		return
	}
	maxVisible := m.scroll + m.contentHeight - 1
	if m.cursor.line > maxVisible {
		m.scroll = m.cursor.line - m.contentHeight + 1
	}
	m.clampScroll()
}

func (m HistoryDetailModel) renderContent() string {
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

func (m HistoryDetailModel) visibleLines() []string {
	lines := make([]string, 0, m.contentHeight)
	for i := 0; i < m.contentHeight; i++ {
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

func (m HistoryDetailModel) selectionRange() (modalPos, modalPos, bool) {
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

func (m HistoryDetailModel) selectionForLine(line int) (int, int, bool) {
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

func (m *HistoryDetailModel) rewrap() {
	lines := wrapContent(m.content, m.width)
	infoLines := m.detailInfoLines()
	if len(infoLines) > 0 {
		lines = append(infoLines, lines...)
	}
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
	m.clampScroll()
}

func (m HistoryDetailModel) detailInfoLines() []string {
	if m.command == nil {
		return nil
	}
	cmd := m.command
	parts := []string{fmt.Sprintf("Command: %s", cmd.Command)}
	if cmd.Cwd != "" {
		parts = append(parts, fmt.Sprintf("CWD: %s", cmd.Cwd))
	}
	if relative := formatRelativeTime(cmd.TsStart); relative != "" {
		parts = append(parts, fmt.Sprintf("Time: %s", relative))
	}
	exitStr := "?"
	if cmd.ExitCode != nil {
		exitStr = fmt.Sprintf("%d", *cmd.ExitCode)
	}
	parts = append(parts, fmt.Sprintf("Exit: %s", exitStr))
	if dur := formatDuration(cmd.DurationMs); dur != "" {
		parts = append(parts, fmt.Sprintf("Duration: %s", dur))
	}
	info := strings.Join(parts, "\n")
	lines := wrapContent(info, m.width)
	if len(lines) > 0 {
		lines = append(lines, "")
	}
	return lines
}

func (m HistoryDetailModel) maxScroll() int {
	max := len(m.lines) - m.contentHeight
	if max < 0 {
		max = 0
	}
	return max
}

func (m HistoryDetailModel) scrollPercent() float64 {
	if m.contentHeight >= len(m.lines) {
		return 1.0
	}
	y := float64(m.scroll)
	h := float64(m.contentHeight)
	t := float64(len(m.lines))
	v := y / (t - h)
	if v < 0.0 {
		return 0.0
	}
	if v > 1.0 {
		return 1.0
	}
	return v
}

func (m *HistoryDetailModel) storeScrollPosition() {
	if m.command == nil {
		return
	}
	if m.scrollByCommand == nil {
		m.scrollByCommand = make(map[string]int)
	}
	m.scrollByCommand[m.command.ID] = m.scroll
}

func normalizeDetailContent(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "")
	return stripLineNumbersIfDetected(content)
}

func stripLineNumbersIfDetected(content string) string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return content
	}
	prefixLens := make([]int, len(lines))
	var numbers []int
	nonEmpty := 0
	matched := 0
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		nonEmpty++
		num, end, ok := parseLineNumberPrefix(line)
		if !ok {
			continue
		}
		matched++
		prefixLens[i] = end
		numbers = append(numbers, num)
	}
	if nonEmpty == 0 || matched < 3 {
		return content
	}
	if float64(matched)/float64(nonEmpty) < 0.8 {
		return content
	}
	if !lineNumbersSequential(numbers) {
		return content
	}
	for i, line := range lines {
		if prefixLens[i] > 0 {
			lines[i] = line[prefixLens[i]:]
		}
	}
	return strings.Join(lines, "\n")
}

func parseLineNumberPrefix(line string) (int, int, bool) {
	i := 0
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	start := i
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	if start == i {
		return 0, 0, false
	}
	numStr := line[start:i]
	num, err := strconv.Atoi(numStr)
	if err != nil {
		return 0, 0, false
	}
	if i < len(line) && (line[i] == ':' || line[i] == '|' || line[i] == '.' || line[i] == ')') {
		i++
	}
	if i >= len(line) || (line[i] != ' ' && line[i] != '\t') {
		return 0, 0, false
	}
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	return num, i, true
}

func lineNumbersSequential(nums []int) bool {
	if len(nums) < 2 {
		return false
	}
	prev := nums[0]
	for i := 1; i < len(nums); i++ {
		if nums[i] <= prev {
			return false
		}
		if nums[i]-prev > 2 {
			return false
		}
		prev = nums[i]
	}
	return true
}
