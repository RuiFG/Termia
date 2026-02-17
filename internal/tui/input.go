package tui

import (
	"fmt"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
	"github.com/muesli/reflow/wrap"
)

// SlashCommand represents a parsed slash command.
type SlashCommand struct {
	Name string
	Args string
}

// SlashSuggestion represents a slash command suggestion with description.
type SlashSuggestion struct {
	Name string
	Desc string
}

// InputModel is the bottom input bar component.
type InputModel struct {
	textInput        textinput.Model
	textarea         textarea.Model
	width            int
	focused          bool
	slashSuggestions []SlashSuggestion
	slashIndex       int
	useTextarea      bool
	pasteBlocks      map[rune]pasteBlock
	pasteSeq         int
	prompt           string
}

type pasteBlock struct {
	content string
	lines   int
}

const (
	inputPrompt       = "> "
	inputPlaceholder  = "Type / for chat commands..."
	suggestedMinWidth = 10
	maxInputLines     = 6
	pasteTokenBase    = 0xE000
)

// NewInputModel creates a new input bar.
func NewInputModel() InputModel {
	ti := textinput.New()
	ti.Placeholder = "" // We render placeholder manually in RenderInputSection
	ti.Prompt = inputPrompt
	ti.PromptStyle = inputPromptStyle
	ti.CharLimit = 500
	ti.Cursor.Style = inputCursorStyle
	ti.Cursor.SetMode(cursor.CursorStatic)
	ti.Cursor.Blink = false
	ti.Focus()

	ta := textarea.New()
	ta.Placeholder = ""
	ta.Prompt = inputPrompt
	ta.SetWidth(suggestedMinWidth)
	ta.SetHeight(1)
	ta.ShowLineNumbers = false
	ta.EndOfBufferCharacter = 0
	focusedStyle, blurredStyle := textarea.DefaultStyles()
	focusedStyle.CursorLine = lipgloss.NewStyle()
	focusedStyle.CursorLineNumber = lipgloss.NewStyle()
	focusedStyle.EndOfBuffer = lipgloss.NewStyle()
	focusedStyle.Text = lipgloss.NewStyle()
	focusedStyle.Placeholder = lipgloss.NewStyle().Foreground(colorMuted)
	focusedStyle.Prompt = inputPromptStyle
	blurredStyle = focusedStyle
	ta.FocusedStyle = focusedStyle
	ta.BlurredStyle = blurredStyle
	ta.Cursor.Style = inputCursorStyle
	ta.Cursor.SetMode(cursor.CursorStatic)
	ta.Cursor.Blink = false
	promptWidth := lipgloss.Width(inputPrompt)
	ta.SetPromptFunc(promptWidth, func(lineIdx int) string {
		if lineIdx == 0 {
			return inputPrompt
		}
		return ""
	})
	ta.Focus()

	suggestions := []SlashSuggestion{
		{Name: "ralph-loop", Desc: "start ralph loop"},
	}
	return InputModel{
		textInput:        ti,
		textarea:         ta,
		focused:          true,
		slashSuggestions: suggestions,
		pasteBlocks:      make(map[rune]pasteBlock),
		prompt:           inputPrompt,
	}
}

func (m *InputModel) SetPrompt(prompt string) {
	if strings.TrimSpace(prompt) == "" {
		prompt = inputPrompt
	}
	m.prompt = prompt
	m.textInput.Prompt = prompt
	m.textarea.Prompt = prompt
	promptWidth := lipgloss.Width(prompt)
	m.textarea.SetPromptFunc(promptWidth, func(lineIdx int) string {
		if lineIdx == 0 {
			return prompt
		}
		return ""
	})
	if m.width > 0 {
		m.SetWidth(m.width)
	}
}

// Focus sets focus on the input.
func (m *InputModel) Focus() tea.Cmd {
	m.focused = true
	if m.useTextarea {
		m.textInput.Blur()
		return m.textarea.Focus()
	}
	m.textarea.Blur()
	return m.textInput.Focus()
}

// Blur removes focus from the input.
func (m *InputModel) Blur() {
	m.focused = false
	if m.useTextarea {
		m.textarea.Blur()
	}
	m.textInput.Blur()
}

// Focused returns whether the input is focused.
func (m InputModel) Focused() bool {
	return m.focused
}

// Value returns the current input text.
func (m InputModel) Value() string {
	return m.ExpandedValue()
}

func (m InputModel) RawValue() string {
	if m.useTextarea {
		return m.textarea.Value()
	}
	return m.textInput.Value()
}

func (m InputModel) ExpandedValue() string {
	if len(m.pasteBlocks) == 0 {
		return m.RawValue()
	}
	return expandPasteBlocks(m.RawValue(), m.pasteBlocks)
}

// SetValue sets the input text.
func (m *InputModel) SetValue(s string) {
	m.resetPasteBlocks()
	m.textInput.SetValue(s)
	m.textarea.SetValue(s)
	m.slashIndex = 0
	if InputLineCount(*m) > 1 {
		m.useTextarea = true
		m.textarea.CursorEnd()
	} else {
		m.useTextarea = false
	}
}

// Reset clears the input.
func (m *InputModel) Reset() {
	m.textarea.Reset()
	m.textInput.Reset()
	m.useTextarea = false
	m.slashIndex = 0
	m.resetPasteBlocks()
}

// SetWidth updates the input width (content width).
func (m *InputModel) SetWidth(w int) {
	m.width = w
	// Width for text input is content width - prompt width
	// We assume w is the available content width (inside borders/padding)
	promptWidth := lipgloss.Width(m.textInput.Prompt)
	m.textInput.Width = w - promptWidth
	if m.textInput.Width < suggestedMinWidth {
		m.textInput.Width = suggestedMinWidth
	}

	// Textarea width matches content width; prompt handled internally.
	m.textarea.SetWidth(w)
}

// SetHeight updates the textarea height for multiline input.
func (m *InputModel) SetHeight(h int) {
	if h < 1 {
		h = 1
	}
	if h > maxInputLines {
		h = maxInputLines
	}
	m.textarea.SetHeight(h)
}

// IsSlashCommand checks if the current input is a slash command.
func (m InputModel) IsSlashCommand() bool {
	return strings.HasPrefix(m.Value(), "/")
}

// ParseSlashCommand parses the input as a slash command.
// Returns nil if not a slash command.
func (m InputModel) ParseSlashCommand() *SlashCommand {
	val := strings.TrimSpace(m.Value())
	if !strings.HasPrefix(val, "/") {
		return nil
	}

	// Remove the leading /
	val = val[1:]
	parts := strings.SplitN(val, " ", 2)
	name := strings.ToLower(parts[0])
	if name == "" {
		if selected := m.SelectedSlashSuggestion(); selected != "" {
			name = selected
		}
	}
	cmd := &SlashCommand{Name: name}
	if len(parts) > 1 {
		cmd.Args = strings.TrimSpace(parts[1])
	}
	return cmd
}

// Update handles input events.
func (m InputModel) Update(msg tea.Msg) (InputModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.handlePasteKey(msg) {
			m.cleanupPasteBlocks()
			m.syncTextareaMode()
			return m, nil
		}
		switch msg.Type {
		case tea.KeyUp:
			m.moveSlashSelection(-1)
			return m, nil
		case tea.KeyDown:
			m.moveSlashSelection(1)
			return m, nil
		case tea.KeyEnter:
			// Avoid inserting newlines on Enter; App handles submission.
			if m.useTextarea {
				return m, nil
			}
		}
	}

	var cmd tea.Cmd
	before := m.RawValue()
	if m.useTextarea {
		m.textarea, cmd = m.textarea.Update(msg)
	} else {
		m.textInput, cmd = m.textInput.Update(msg)
	}
	if before != m.RawValue() {
		m.slashIndex = 0
	}

	m.cleanupPasteBlocks()
	m.syncTextareaMode()

	return m, cmd
}

// SlashSuggestions returns available slash command suggestions.
func (m InputModel) SlashSuggestions() []SlashSuggestion {
	val := strings.TrimSpace(m.Value())
	if !strings.HasPrefix(val, "/") {
		return nil
	}

	if val == "/" {
		return m.slashSuggestions
	}

	partial := strings.ToLower(strings.TrimSpace(val[1:]))
	if partial == "" {
		return m.slashSuggestions
	}

	// If partial contains a space, it's a fully typed command (e.g. "help " or "search foo")
	// — hide the menu
	if strings.Contains(partial, " ") {
		return nil
	}

	var matches []SlashSuggestion
	for _, cmd := range m.slashSuggestions {
		if strings.HasPrefix(cmd.Name, partial) {
			matches = append(matches, cmd)
		}
	}

	// If no matches, hide the menu (unknown command prefix)
	if len(matches) == 0 {
		return nil
	}

	// If exactly one match and it's the same as partial, command is fully typed — hide menu
	if len(matches) == 1 && matches[0].Name == partial {
		return nil
	}

	return matches
}

// SelectedSlashSuggestion returns the current suggestion selection.
func (m InputModel) SelectedSlashSuggestion() string {
	suggestions := m.SlashSuggestions()
	if len(suggestions) == 0 {
		return ""
	}
	if m.slashIndex < 0 || m.slashIndex >= len(suggestions) {
		return suggestions[0].Name
	}
	return suggestions[m.slashIndex].Name
}

func (m *InputModel) moveSlashSelection(delta int) {
	suggestions := m.SlashSuggestions()
	if len(suggestions) == 0 {
		return
	}
	m.slashIndex += delta
	if m.slashIndex < 0 {
		m.slashIndex = len(suggestions) - 1
	} else if m.slashIndex >= len(suggestions) {
		m.slashIndex = 0
	}
}

// SelectSlashSuggestion applies the selected suggestion into the input.
func (m *InputModel) SelectSlashSuggestion() bool {
	suggestions := m.SlashSuggestions()
	if len(suggestions) == 0 {
		return false
	}
	selected := m.SelectedSlashSuggestion()
	if selected == "" {
		return false
	}
	m.SetValue("/" + selected + " ")
	m.slashIndex = 0
	if m.useTextarea {
		m.textarea.CursorEnd()
		if m.focused {
			_ = m.textarea.Focus()
		}
	} else {
		m.textInput.CursorEnd()
		if m.focused {
			_ = m.textInput.Focus()
		}
	}
	return true
}

// View renders the input bar content (no border).
func (m InputModel) View() string {
	if m.useTextarea {
		return m.textarea.View()
	}
	return m.textInput.View()
}

// RenderInputSection renders the input bar content (no suggestions).
func RenderInputSection(m InputModel) string {
	// Manually render placeholder when empty — avoids textinput dual-line issues
	if m.DisplayValue() == "" && !m.IsSlashCommand() {
		return renderEmptyInputWithCursor(m)
	}

	if len(m.pasteBlocks) > 0 && !m.useTextarea {
		return renderTextInputWithPaste(m)
	}

	if m.useTextarea {
		return m.textarea.View()
	}

	return m.textInput.View()
}

// InputLineCount returns the number of visible lines in the input, based on
// soft-wrapping the value to the current width.
func InputLineCount(m InputModel) int {
	width := m.textInput.Width
	if m.useTextarea {
		width = m.textarea.Width()
	}
	if width < suggestedMinWidth {
		width = suggestedMinWidth
	}

	val := m.DisplayValue()
	if val == "" {
		return 1
	}

	// Preserve explicit newlines in the input.
	wrapped := wordwrap.String(val, width)
	wrapped = wrap.String(wrapped, width)
	lines := strings.Split(wrapped, "\n")
	count := 0
	for _, line := range lines {
		if line == "" {
			count++
			continue
		}
		count += (lipgloss.Width(line)-1)/width + 1
	}

	if count < 1 {
		return 1
	}
	if count > maxInputLines {
		return maxInputLines
	}
	return count
}

func InputVisibleLines(m InputModel) []string {
	if m.DisplayValue() == "" && !m.IsSlashCommand() {
		return []string{RenderInputSection(m)}
	}

	if m.useTextarea {
		return renderTextareaLines(m)
	}

	width := m.textInput.Width
	if width < suggestedMinWidth {
		width = suggestedMinWidth
	}

	wrapped := wordwrap.String(m.DisplayValue(), width)
	wrapped = wrap.String(wrapped, width)
	lines := strings.Split(wrapped, "\n")
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

func InputSelectionLines(m InputModel, maxLines int) []string {
	plain := stripANSICodes(RenderInputSection(m))
	lines := strings.Split(plain, "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	for len(lines) < maxLines {
		lines = append(lines, "")
	}
	return lines
}

func renderTextareaLines(m InputModel) []string {
	lines := strings.Split(m.DisplayValue(), "\n")
	if len(lines) == 0 {
		return []string{""}
	}

	width := m.textarea.Width()
	if width < suggestedMinWidth {
		width = suggestedMinWidth
	}

	var wrappedLines []string
	for _, line := range lines {
		if line == "" {
			wrappedLines = append(wrappedLines, "")
			continue
		}
		wrapped := wordwrap.String(line, width)
		wrapped = wrap.String(wrapped, width)
		wrappedLines = append(wrappedLines, strings.Split(wrapped, "\n")...)
	}

	if len(wrappedLines) == 0 {
		wrappedLines = []string{""}
	}

	return wrappedLines
}

func (m InputModel) DisplayValue() string {
	if len(m.pasteBlocks) == 0 {
		return m.RawValue()
	}
	return renderPasteBlocks(m.RawValue(), m.pasteBlocks)
}

func (m *InputModel) handlePasteKey(msg tea.KeyMsg) bool {
	if msg.Type == tea.KeyCtrlV {
		pasted, err := clipboard.ReadAll()
		if err == nil && pasted != "" {
			return m.insertPasteBlock(pasted)
		}
		return false
	}
	if msg.Type == tea.KeyRunes {
		pasted := string(msg.Runes)
		if msg.Paste || strings.Contains(pasted, "\n") || strings.Contains(pasted, "\r") {
			return m.insertPasteBlock(pasted)
		}
	}
	return false
}

func (m *InputModel) insertPasteBlock(pasted string) bool {
	content := normalizePasteContent(pasted)
	lineCount := countLines(content)
	if lineCount <= 2 {
		return false
	}
	m.ensurePasteBlocks()
	token := rune(pasteTokenBase + m.pasteSeq)
	m.pasteSeq++
	m.pasteBlocks[token] = pasteBlock{content: content, lines: lineCount}
	m.insertToken(token)
	m.slashIndex = 0
	return true
}

func (m *InputModel) insertToken(token rune) {
	if m.useTextarea {
		m.textarea.InsertRune(token)
		return
	}
	pos := m.textInput.Position()
	value := []rune(m.textInput.Value())
	if pos < 0 {
		pos = 0
	}
	if pos > len(value) {
		pos = len(value)
	}
	value = append(value[:pos], append([]rune{token}, value[pos:]...)...)
	m.textInput.SetValue(string(value))
	m.textInput.SetCursor(pos + 1)
}

func (m *InputModel) syncTextareaMode() {
	if len(m.pasteBlocks) > 0 {
		if m.useTextarea {
			m.useTextarea = false
			m.textInput.SetValue(m.textarea.Value())
			m.textInput.CursorEnd()
			m.textarea.Blur()
			if m.focused {
				_ = m.textInput.Focus()
			}
		}
		return
	}
	if !m.useTextarea {
		lineCount := InputLineCount(*m)
		if lineCount > 1 {
			m.useTextarea = true
			m.textarea.SetValue(m.textInput.Value())
			m.textarea.CursorEnd()
			m.textInput.Blur()
			if m.focused {
				_ = m.textarea.Focus()
			}
		}
		return
	}
	if InputLineCount(*m) <= 1 {
		m.useTextarea = false
		m.textInput.SetValue(m.textarea.Value())
		m.textInput.CursorEnd()
		m.textarea.Blur()
		if m.focused {
			_ = m.textInput.Focus()
		}
	}
}

func (m *InputModel) cleanupPasteBlocks() {
	if len(m.pasteBlocks) == 0 {
		return
	}
	active := make(map[rune]struct{})
	for _, r := range m.RawValue() {
		if _, ok := m.pasteBlocks[r]; ok {
			active[r] = struct{}{}
		}
	}
	for token := range m.pasteBlocks {
		if _, ok := active[token]; !ok {
			delete(m.pasteBlocks, token)
		}
	}
}

func (m *InputModel) ensurePasteBlocks() {
	if m.pasteBlocks == nil {
		m.pasteBlocks = make(map[rune]pasteBlock)
	}
}

func (m *InputModel) resetPasteBlocks() {
	if m.pasteBlocks == nil {
		return
	}
	for k := range m.pasteBlocks {
		delete(m.pasteBlocks, k)
	}
	m.pasteSeq = 0
}

func normalizePasteContent(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	return content
}

func countLines(content string) int {
	if content == "" {
		return 0
	}
	return strings.Count(content, "\n") + 1
}

func renderPasteBlocks(value string, blocks map[rune]pasteBlock) string {
	var b strings.Builder
	for _, r := range value {
		if block, ok := blocks[r]; ok {
			b.WriteString(renderPastePlaceholder(block))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func expandPasteBlocks(value string, blocks map[rune]pasteBlock) string {
	var b strings.Builder
	for _, r := range value {
		if block, ok := blocks[r]; ok {
			b.WriteString(block.content)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func renderPastePlaceholder(block pasteBlock) string {
	label := fmt.Sprintf("[Pause %d lines]", block.lines)
	return pastePlaceholderStyle.Render(label)
}

func renderTextInputWithPaste(m InputModel) string {
	raw := []rune(m.RawValue())
	pos := m.textInput.Position()
	if pos < 0 {
		pos = 0
	}
	if pos > len(raw) {
		pos = len(raw)
	}
	width := m.textInput.Width
	if width < 1 {
		width = suggestedMinWidth
	}

	segments := make([]pasteSegment, 0, len(raw))
	for _, r := range raw {
		if block, ok := m.pasteBlocks[r]; ok {
			label := fmt.Sprintf("[Pause %d lines]", block.lines)
			segments = append(segments, pasteSegment{text: label, placeholder: true})
			continue
		}
		segments = append(segments, pasteSegment{text: string(r)})
	}

	segWidths := make([]int, len(segments))
	totalWidth := 0
	for i, seg := range segments {
		segWidths[i] = lipgloss.Width(seg.text)
		totalWidth += segWidths[i]
	}

	cursorCell := 0
	for i := 0; i < pos && i < len(segWidths); i++ {
		cursorCell += segWidths[i]
	}

	offset := 0
	if width > 0 {
		if cursorCell < offset {
			offset = cursorCell
		}
		if cursorCell >= offset+width {
			offset = cursorCell - width + 1
		}
		if offset < 0 {
			offset = 0
		}
	}
	end := offset + width
	if width <= 0 {
		end = totalWidth
	}

	var b strings.Builder
	cursorModel := m.textInput.Cursor
	cur := 0
	for i, seg := range segments {
		segStart := cur
		segEnd := cur + segWidths[i]
		if segEnd <= offset {
			cur = segEnd
			continue
		}
		if segStart >= end {
			break
		}
		visibleStart := maxInt(offset, segStart) - segStart
		visibleEnd := minInt(end, segEnd) - segStart
		if visibleEnd > visibleStart {
			cursorRel := -1
			if i == pos {
				cursorRel = cursorCell - segStart
			}
			b.WriteString(renderPasteSegment(seg, visibleStart, visibleEnd, cursorRel, &cursorModel))
		}
		cur = segEnd
	}

	if pos == len(raw) && cursorCell >= offset && cursorCell < end {
		cursorModel.SetChar(" ")
		b.WriteString(cursorModel.View())
	}

	content := b.String()
	contentWidth := lipgloss.Width(content)
	if width > 0 && contentWidth < width {
		content += strings.Repeat(" ", width-contentWidth)
	}

	return m.textInput.PromptStyle.Render(m.textInput.Prompt) + content
}

type pasteSegment struct {
	text        string
	placeholder bool
}

func renderPasteSegment(seg pasteSegment, startCell, endCell int, cursorRel int, cursorModel *cursor.Model) string {
	if startCell >= endCell || seg.text == "" {
		return ""
	}
	startCell = maxInt(0, startCell)
	endCell = maxInt(startCell, endCell)
	var b strings.Builder
	cur := 0
	for _, r := range seg.text {
		rw := lipgloss.Width(string(r))
		next := cur + rw
		if next <= startCell {
			cur = next
			continue
		}
		if cur >= endCell {
			break
		}
		cell := string(r)
		if cursorRel >= 0 && cursorRel >= cur && cursorRel < next {
			cursorModel.SetChar(cell)
			b.WriteString(cursorModel.View())
		} else if seg.placeholder {
			b.WriteString(pastePlaceholderStyle.Render(cell))
		} else {
			b.WriteString(cell)
		}
		cur = next
	}
	return b.String()
}

func renderEmptyInputWithCursor(m InputModel) string {
	promptValue := m.prompt
	if strings.TrimSpace(promptValue) == "" {
		promptValue = inputPrompt
	}
	prompt := inputPromptStyle.Render(promptValue)
	placeholder := inputPlaceholder
	if placeholder == "" {
		cursorModel := m.textInput.Cursor
		cursorModel.SetChar(" ")
		return prompt + cursorModel.View()
	}
	placeholderStyle := lipgloss.NewStyle().Foreground(colorMuted)
	first := string([]rune(placeholder)[0])
	rest := ""
	if len([]rune(placeholder)) > 1 {
		rest = string([]rune(placeholder)[1:])
	}
	cursorModel := m.textInput.Cursor
	cursorModel.TextStyle = placeholderStyle
	cursorModel.SetChar(first)
	return prompt + cursorModel.View() + placeholderStyle.Render(rest)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// InputPromptWidth returns the visible width of the input prompt.
func InputPromptWidth(m InputModel) int {
	if m.useTextarea {
		return lipgloss.Width(m.textarea.Prompt)
	}
	return lipgloss.Width(m.textInput.Prompt)
}

// RenderSlashMenu renders a selectable slash command menu.
func RenderSlashMenu(m InputModel, width int) string {
	suggestions := m.SlashSuggestions()
	if len(suggestions) == 0 {
		return ""
	}

	var lines []string
	for i, cmd := range suggestions {
		left := "/" + cmd.Name
		desc := cmd.Desc
		line := left
		if strings.TrimSpace(desc) != "" {
			leftWidth := lipgloss.Width(left)
			maxDesc := width - leftWidth - 1
			if maxDesc < 0 {
				maxDesc = 0
			}
			if lipgloss.Width(desc) > maxDesc {
				desc = truncateToWidth(desc, maxDesc)
			}
			descWidth := lipgloss.Width(desc)
			spaces := width - leftWidth - descWidth
			if spaces < 1 {
				spaces = 1
			}
			line = left + strings.Repeat(" ", spaces) + metaStyle.Render(desc)
		}
		style := normalRowStyle
		if i == m.slashIndex {
			style = selectedSlashRowStyle
		}
		lines = append(lines, style.Width(width).Inline(true).Render(line))
	}

	return strings.Join(lines, "\n")
}

func truncateToWidth(input string, maxWidth int) string {
	if maxWidth <= 0 || input == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range input {
		candidate := b.String() + string(r)
		if lipgloss.Width(candidate) > maxWidth {
			break
		}
		b.WriteRune(r)
	}
	return b.String()
}
