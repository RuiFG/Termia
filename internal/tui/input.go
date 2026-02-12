package tui

import (
	"strings"

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
}

const (
	inputPrompt       = "> "
	inputPlaceholder  = "Type / for commands..."
	suggestedMinWidth = 10
	maxInputLines     = 6
)

// NewInputModel creates a new input bar.
func NewInputModel() InputModel {
	ti := textinput.New()
	ti.Placeholder = "" // We render placeholder manually in RenderInputSection
	ti.Prompt = inputPrompt
	ti.PromptStyle = inputPromptStyle
	ti.CharLimit = 500
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
	ta.Focus()

	suggestions := []SlashSuggestion{
		{Name: "help", Desc: "show help"},
		{Name: "search", Desc: "search history"},
		{Name: "models", Desc: "show model config"},
		{Name: "team", Desc: "switch to team mode"},
		{Name: "copilt", Desc: "switch to copilt mode"},
		{Name: "clear", Desc: "clear view"},
		{Name: "exit", Desc: "exit TUI"},
	}
	return InputModel{
		textInput:        ti,
		textarea:         ta,
		focused:          true,
		slashSuggestions: suggestions,
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
	if m.useTextarea {
		return m.textarea.Value()
	}
	return m.textInput.Value()
}

// SetValue sets the input text.
func (m *InputModel) SetValue(s string) {
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
	before := m.Value()
	if m.useTextarea {
		m.textarea, cmd = m.textarea.Update(msg)
	} else {
		m.textInput, cmd = m.textInput.Update(msg)
	}
	if before != m.Value() {
		m.slashIndex = 0
	}

	// Switch to textarea when content spans multiple lines.
	if !m.useTextarea {
		lineCount := InputLineCount(m)
		if lineCount > 1 {
			m.useTextarea = true
			m.textarea.SetValue(m.textInput.Value())
			m.textarea.CursorEnd()
			m.textInput.Blur()
			if m.focused {
				_ = m.textarea.Focus()
			}
		}
	} else if InputLineCount(m) <= 1 {
		m.useTextarea = false
		m.textInput.SetValue(m.textarea.Value())
		m.textInput.CursorEnd()
		m.textarea.Blur()
		if m.focused {
			_ = m.textInput.Focus()
		}
	}

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
	if m.Value() == "" && !m.IsSlashCommand() {
		prompt := inputPromptStyle.Render(inputPrompt)
		placeholder := lipgloss.NewStyle().Foreground(colorMuted).Render(inputPlaceholder)
		return prompt + placeholder
	}

	if m.useTextarea {
		lines := renderTextareaLines(m)
		if len(lines) == 0 {
			return ""
		}
		inputPromptW := InputPromptWidth(m)
		rows := make([]string, 0, len(lines))
		for i, line := range lines {
			if i == 0 {
				rows = append(rows, inputPromptStyle.Render(inputPrompt)+line)
				continue
			}
			rows = append(rows, strings.Repeat(" ", inputPromptW)+line)
		}
		return strings.Join(rows, "\n")
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

	val := m.Value()
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
	if m.Value() == "" && !m.IsSlashCommand() {
		return []string{RenderInputSection(m)}
	}

	if m.useTextarea {
		return renderTextareaLines(m)
	}

	width := m.textInput.Width
	if width < suggestedMinWidth {
		width = suggestedMinWidth
	}

	wrapped := wordwrap.String(m.Value(), width)
	wrapped = wrap.String(wrapped, width)
	lines := strings.Split(wrapped, "\n")
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

func renderTextareaLines(m InputModel) []string {
	lines := strings.Split(m.textarea.Value(), "\n")
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
