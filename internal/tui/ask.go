package tui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/termia/termia/internal/agent"
)

type AskMode int

const (
	AskModeNone AskMode = iota
	AskModeSelect
	AskModeCustom
)

type AskInput struct {
	Request      agent.HITLRequest
	Questions    []agent.AskQuestion
	Answers      []agent.AskAnswer
	Selected     []map[int]bool
	CustomValues []string
	Index        int
	Cursor       int
	Mode         AskMode
	Custom       textarea.Model
}

func NewAskInput() AskInput {
	custom := textarea.New()
	custom.Placeholder = "Type your answer..."
	custom.Prompt = "> "
	custom.SetWidth(suggestedMinWidth)
	custom.SetHeight(1)
	custom.ShowLineNumbers = false
	custom.EndOfBufferCharacter = 0
	focusedStyle, blurredStyle := textarea.DefaultStyles()
	focusedStyle.CursorLine = lipgloss.NewStyle()
	focusedStyle.CursorLineNumber = lipgloss.NewStyle()
	focusedStyle.EndOfBuffer = lipgloss.NewStyle()
	focusedStyle.Text = lipgloss.NewStyle()
	focusedStyle.Placeholder = lipgloss.NewStyle().Foreground(colorMuted)
	focusedStyle.Prompt = inputPromptStyle
	blurredStyle = focusedStyle
	custom.FocusedStyle = focusedStyle
	custom.BlurredStyle = blurredStyle
	custom.Cursor.Style = inputCursorStyle
	custom.Cursor.Blink = false
	promptWidth := lipgloss.Width("> ")
	custom.SetPromptFunc(promptWidth, func(lineIdx int) string {
		if lineIdx == 0 {
			return "> "
		}
		return ""
	})
	return AskInput{Mode: AskModeNone, Custom: custom}
}

func (m *AskInput) SetRequest(request agent.HITLRequest) {
	m.Request = request
	m.SetQuestions(request.Questions)
}

func (m *AskInput) SetQuestions(questions []agent.AskQuestion) {
	normalized := make([]agent.AskQuestion, 0, len(questions))
	for _, question := range questions {
		norm, err := agent.NormalizeAskQuestion(question)
		if err != nil {
			normalized = append(normalized, question)
			continue
		}
		normalized = append(normalized, norm)
	}
	m.Questions = normalized
	m.Answers = make([]agent.AskAnswer, len(normalized))
	m.Selected = make([]map[int]bool, len(normalized))
	m.CustomValues = make([]string, len(normalized))
	m.Index = 0
	m.Cursor = m.defaultCursorForQuestion(0)
	m.Mode = AskModeSelect
	m.Custom.SetValue("")
	m.Custom.SetHeight(1)
	m.Custom.Blur()
}

func (m AskInput) Active() bool {
	return m.Mode != AskModeNone
}

func (m *AskInput) SetWidth(width int) {
	inputWidth := maxInt(width-lipgloss.Width(m.Custom.Prompt), suggestedMinWidth)
	m.Custom.SetWidth(inputWidth)
	m.Custom.SetHeight(1)
}

func (m AskInput) View(contentWidth int) string {
	width := maxInt(1, contentWidth)
	if len(m.Questions) == 0 || m.Index >= len(m.Questions) {
		return ""
	}
	question := m.Questions[m.Index]
	enterLabel := "next"
	if m.Index == len(m.Questions)-1 {
		enterLabel = "submit"
	}
	lines := make([]string, 0, len(question.Options)+8)
	if len(m.Questions) > 1 {
		lines = append(lines, m.renderQuestionTabs(width))
	}
	if header := strings.TrimSpace(question.Header); header != "" {
		lines = append(lines, hitlTitleStyle.Render(header))
	}
	if prompt := strings.TrimSpace(question.Question); prompt != "" {
		lines = append(lines, renderStyledParagraph(prompt, "", width, assistantBodyStyle)...)
	}
	for idx, option := range question.Options {
		marker := "( )"
		if question.Multiple {
			marker = "[ ]"
		}
		if m.isSelected(idx) {
			if question.Multiple {
				marker = "[x]"
			} else {
				marker = "(x)"
			}
		}
		lines = append(lines, renderAskOptionRow(width, marker, option, idx == m.Cursor && m.Mode == AskModeSelect, m.isSelected(idx))...)
	}
	if m.Mode == AskModeCustom {
		lines = append(lines, "")
		lines = append(lines, m.Custom.View())
		lines = append(lines, hitlHintStyle.Render("Enter save  Esc back"))
		return strings.Join(lines, "\n")
	}
	if question.Multiple {
		lines = append(lines, hitlHintStyle.Render("←/→ questions  ↑/↓ options  Space toggle  Enter "+enterLabel))
	} else {
		lines = append(lines, hitlHintStyle.Render("←/→ questions  ↑/↓ options  Space select  Enter "+enterLabel))
	}
	return strings.Join(lines, "\n")
}

func (m *AskInput) Update(msg tea.Msg) (*agent.HITLResponse, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, nil
	}
	switch m.Mode {
	case AskModeSelect:
		return m.handleSelectKey(keyMsg)
	case AskModeCustom:
		return m.handleCustomKey(keyMsg)
	default:
		return nil, nil
	}
}

func (m *AskInput) handleSelectKey(msg tea.KeyMsg) (*agent.HITLResponse, tea.Cmd) {
	if len(m.Questions) == 0 || m.Index >= len(m.Questions) {
		return nil, nil
	}
	question := m.Questions[m.Index]
	if msg.Type == tea.KeyRunes && len(msg.Runes) > 0 && msg.String() != " " && m.cursorIsCustom(question) {
		m.ensureSelection()
		if !m.isSelected(m.Cursor) {
			m.toggleSelection(m.Cursor, question)
		}
		focusCmd := m.enterCustomMode()
		updated, inputCmd := m.Custom.Update(msg)
		m.Custom = updated
		m.syncCustomDraft()
		return nil, tea.Batch(focusCmd, inputCmd)
	}
	switch msg.Type {
	case tea.KeyLeft:
		return nil, m.goToQuestion(m.Index - 1)
	case tea.KeyRight:
		return nil, m.goToQuestion(m.Index + 1)
	case tea.KeyUp:
		if m.Cursor > 0 {
			m.Cursor--
		}
	case tea.KeyDown:
		if m.Cursor < len(question.Options)-1 {
			m.Cursor++
		}
	case tea.KeyEnter:
		m.ensureSelection()
		if !question.Multiple {
			m.Selected[m.Index] = map[int]bool{m.Cursor: true}
			if !m.cursorIsCustom(question) {
				m.CustomValues[m.Index] = ""
			}
		} else if m.selectionEmpty() {
			m.toggleSelection(m.Cursor, question)
		}
		if m.selectionHasCustom(question) {
			if strings.TrimSpace(m.CustomValue(m.Index)) == "" {
				return nil, m.enterCustomMode()
			}
		}
		return m.advanceOrFinish()
	default:
		if msg.String() == " " {
			m.toggleSelection(m.Cursor, question)
			if !question.Multiple && m.cursorIsCustom(question) && m.isSelected(m.Cursor) {
				return nil, m.enterCustomMode()
			}
		}
	}
	return nil, nil
}

func (m *AskInput) handleCustomKey(msg tea.KeyMsg) (*agent.HITLResponse, tea.Cmd) {
	if !m.Custom.Focused() {
		_ = m.Custom.Focus()
	}
	if msg.Type == tea.KeyEsc {
		m.syncCustomDraft()
		m.Mode = AskModeSelect
		m.Custom.Blur()
		return nil, nil
	}
	if msg.Type == tea.KeyEnter {
		m.syncCustomDraft()
		value := strings.TrimSpace(m.Custom.Value())
		if value == "" {
			return nil, nil
		}
		m.Mode = AskModeSelect
		m.Custom.Blur()
		return nil, nil
	}
	if msg.Type == tea.KeyRunes && len(msg.Runes) > 0 {
		m.Custom.InsertString(string(msg.Runes))
		m.syncCustomDraft()
		return nil, nil
	}
	if msg.Type == tea.KeySpace {
		m.Custom.InsertString(" ")
		m.syncCustomDraft()
		return nil, nil
	}
	if msg.Type == tea.KeyBackspace {
		m.deleteLastCustomRune()
		m.syncCustomDraft()
		return nil, nil
	}
	updated, cmd := m.Custom.Update(msg)
	m.Custom = updated
	m.syncCustomDraft()
	return nil, cmd
}

func (m *AskInput) ensureSelection() {
	if m.Selected[m.Index] == nil {
		m.Selected[m.Index] = make(map[int]bool)
	}
}

func (m *AskInput) toggleSelection(idx int, question agent.AskQuestion) {
	m.ensureSelection()
	if !question.Multiple {
		m.Selected[m.Index] = map[int]bool{idx: true}
		if !m.cursorIsCustom(question) {
			m.CustomValues[m.Index] = ""
		}
		return
	}
	if m.Selected[m.Index][idx] {
		delete(m.Selected[m.Index], idx)
		if idx < len(question.Options) && strings.EqualFold(question.Options[idx].Title, agent.AskTypeYourAnswerTitle) {
			m.CustomValues[m.Index] = ""
		}
		return
	}
	m.Selected[m.Index][idx] = true
}

func (m *AskInput) selectionEmpty() bool {
	return m.Selected[m.Index] == nil || len(m.Selected[m.Index]) == 0
}

func (m *AskInput) isSelected(idx int) bool {
	if m.Selected[m.Index] == nil {
		return false
	}
	return m.Selected[m.Index][idx]
}

func (m *AskInput) selectionHasCustom(question agent.AskQuestion) bool {
	for idx, option := range question.Options {
		if m.isSelected(idx) && strings.EqualFold(option.Title, agent.AskTypeYourAnswerTitle) {
			return true
		}
	}
	return false
}

func (m *AskInput) cursorIsCustom(question agent.AskQuestion) bool {
	if m.Cursor < 0 || m.Cursor >= len(question.Options) {
		return false
	}
	return strings.EqualFold(question.Options[m.Cursor].Title, agent.AskTypeYourAnswerTitle)
}

func (m *AskInput) enterCustomMode() tea.Cmd {
	m.Mode = AskModeCustom
	m.Custom.SetValue(m.CustomValues[m.Index])
	m.Custom.SetHeight(1)
	m.Custom.CursorEnd()
	return m.Custom.Focus()
}

func (m *AskInput) deleteLastCustomRune() {
	value := []rune(m.Custom.Value())
	if len(value) == 0 {
		return
	}
	m.Custom.SetValue(string(value[:len(value)-1]))
	m.Custom.CursorEnd()
}

func (m *AskInput) advanceOrFinish() (*agent.HITLResponse, tea.Cmd) {
	if m.Index+1 < len(m.Questions) {
		return nil, m.goToQuestion(m.Index + 1)
	}
	m.Mode = AskModeNone
	m.Custom.Blur()
	m.Answers = m.buildAnswers()
	resp := &agent.HITLResponse{
		Confirmed: true,
		Answers:   m.Answers,
		Payload: map[string]any{
			"answers": m.Answers,
		},
	}
	return resp, nil
}

func (m AskInput) renderQuestionTabs(width int) string {
	parts := []string{hitlSubtitleStyle.Render("Questions")}
	for idx := range m.Questions {
		label := lipgloss.NewStyle().Render(strconv.Itoa(idx + 1))
		switch {
		case idx == m.Index:
			label = hitlSelectedStyle.Render(strconv.Itoa(idx + 1))
		case m.questionAnswered(idx):
			label = hitlChoiceFocusStyle.Render(strconv.Itoa(idx + 1))
		default:
			label = hitlHintStyle.Render(strconv.Itoa(idx + 1))
		}
		parts = append(parts, "["+label+"]")
	}
	parts = append(parts, hitlHintStyle.Render(strconv.Itoa(m.Index+1)+"/"+strconv.Itoa(len(m.Questions))))
	return lipgloss.NewStyle().Width(width).Render(strings.Join(parts, " "))
}

func (m *AskInput) goToQuestion(idx int) tea.Cmd {
	if idx < 0 || idx >= len(m.Questions) {
		return nil
	}
	m.syncCustomDraft()
	m.Index = idx
	m.Cursor = m.defaultCursorForQuestion(idx)
	m.Mode = AskModeSelect
	m.Custom.SetValue(m.CustomValues[idx])
	m.Custom.SetHeight(1)
	m.Custom.Blur()
	return nil
}

func (m AskInput) defaultCursorForQuestion(idx int) int {
	if idx < 0 || idx >= len(m.Questions) {
		return 0
	}
	if m.Selected[idx] != nil {
		for optionIdx := range m.Questions[idx].Options {
			if m.Selected[idx][optionIdx] {
				return optionIdx
			}
		}
	}
	return 0
}

func (m *AskInput) syncCustomDraft() {
	if m.Index < 0 || m.Index >= len(m.CustomValues) {
		return
	}
	m.CustomValues[m.Index] = m.Custom.Value()
}

func (m AskInput) questionAnswered(idx int) bool {
	if idx < 0 || idx >= len(m.Questions) {
		return false
	}
	if m.Selected[idx] != nil && len(m.Selected[idx]) > 0 {
		return true
	}
	return strings.TrimSpace(m.CustomValue(idx)) != ""
}

func (m AskInput) CustomValue(idx int) string {
	if idx < 0 || idx >= len(m.CustomValues) {
		return ""
	}
	return m.CustomValues[idx]
}

func (m AskInput) buildAnswers() []agent.AskAnswer {
	answers := make([]agent.AskAnswer, 0, len(m.Questions))
	for questionIdx, question := range m.Questions {
		selected := make([]string, 0)
		for optionIdx, option := range question.Options {
			if m.Selected[questionIdx] == nil || !m.Selected[questionIdx][optionIdx] {
				continue
			}
			if strings.EqualFold(option.Title, agent.AskTypeYourAnswerTitle) {
				continue
			}
			selected = append(selected, option.Title)
		}
		answer := agent.AskAnswer{
			ID:              question.ID,
			Question:        question.Question,
			SelectedOptions: selected,
		}
		if custom := strings.TrimSpace(m.CustomValue(questionIdx)); custom != "" {
			answer.CustomTexts = []string{custom}
		}
		answers = append(answers, answer)
	}
	return answers
}

func renderAskOptionRow(width int, marker string, option agent.AskOption, focused, selected bool) []string {
	width = maxInt(1, width)
	markerText := strings.TrimSpace(marker)
	prefixPlain := markerText + " "

	markerStyle := hitlChoiceMarkerStyle
	switch {
	case selected:
		markerStyle = hitlChoiceSelectedMarkerStyle
	case focused:
		markerStyle = hitlChoiceFocusMarkerStyle
	}

	titleStyle := hitlChoiceTitleStyle
	if focused {
		titleStyle = hitlChoiceFocusStyle
	}

	title := strings.TrimSpace(option.Title)
	titleLines := wrapStyledSpans([]styledSpan{{Text: title, Style: titleStyle}}, maxInt(1, width-lipgloss.Width(prefixPlain)))
	if len(titleLines) == 0 {
		titleLines = []string{titleStyle.Render(title)}
	}

	lines := make([]string, 0, len(titleLines)+2)
	lines = append(lines, markerStyle.Render(markerText)+" "+titleLines[0])
	continuation := strings.Repeat(" ", lipgloss.Width(prefixPlain))
	for _, line := range titleLines[1:] {
		lines = append(lines, continuation+line)
	}

	desc := strings.TrimSpace(option.Description)
	if desc == "" {
		return lines
	}

	if len(titleLines) == 1 {
		firstLinePlain := prefixPlain + title
		availableDescWidth := width - lipgloss.Width(firstLinePlain) - 2
		if availableDescWidth > 8 {
			descLines := wrapStyledSpans([]styledSpan{{Text: desc, Style: hitlChoiceDescStyle}}, availableDescWidth)
			if len(descLines) == 1 {
				lines[0] += "  " + descLines[0]
				return lines
			}
		}
	}

	descPrefix := continuation + "  "
	lines = append(lines, renderStyledParagraph(desc, descPrefix, width, hitlChoiceDescStyle)...)
	return lines
}
