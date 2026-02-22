package tui

import (
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
	Questions []agent.AskQuestion
	Answers   []agent.AskAnswer
	Selected  []map[int]bool
	Index     int
	Cursor    int
	Mode      AskMode
	Custom    textarea.Model
}

func NewAskInput() AskInput {
	custom := textarea.New()
	custom.Placeholder = ""
	custom.Prompt = "> "
	custom.SetWidth(suggestedMinWidth)
	custom.SetHeight(3)
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
	return AskInput{
		Mode:   AskModeNone,
		Custom: custom,
	}
}

func (m *AskInput) SetQuestions(questions []agent.AskQuestion) {
	m.Questions = questions
	m.Answers = make([]agent.AskAnswer, len(questions))
	m.Selected = make([]map[int]bool, len(questions))
	m.Index = 0
	m.Cursor = 0
	m.Mode = AskModeSelect
	m.Custom.SetValue("")
}

func (m AskInput) Active() bool {
	return m.Mode != AskModeNone
}

func (m *AskInput) SetWidth(width int) {
	inputWidth := maxInt(width-lipgloss.Width(m.Custom.Prompt), suggestedMinWidth)
	m.Custom.SetWidth(inputWidth)
}

func (m AskInput) View(contentWidth int) string {
	if len(m.Questions) == 0 || m.Index >= len(m.Questions) {
		return ""
	}
	question := m.Questions[m.Index]
	lines := []string{}
	header := strings.TrimSpace(question.Question)
	if header == "" {
		header = "Question"
	}
	lines = append(lines, header)
	lines = append(lines, "")
	for idx, option := range question.Options {
		prefix := "  "
		if idx == m.Cursor && m.Mode == AskModeSelect {
			prefix = "> "
		}
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
		title := truncateToWidth(strings.TrimSpace(option.Title), agent.AskOptionTitleMaxLen)
		description := truncateToWidth(strings.TrimSpace(option.Description), agent.AskOptionDescMaxLen)
		if description != "" {
			lines = append(lines, prefix+marker+" "+title+" - "+description)
			continue
		}
		lines = append(lines, prefix+marker+" "+title)
	}
	lines = append(lines, "")
	switch m.Mode {
	case AskModeCustom:
		lines = append(lines, "Type your answer:")
		lines = append(lines, m.Custom.View())
		lines = append(lines, "Ctrl+J=submit  Esc=cancel")
	default:
		if question.Multiple {
			lines = append(lines, "↑/↓ move  Space toggle  Enter confirm")
		} else {
			lines = append(lines, "↑/↓ move  Enter confirm")
		}
	}
	return strings.Join(lines, "\n")
}

func (m *AskInput) Update(msg tea.Msg) (*[]agent.AskAnswer, tea.Cmd) {
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

func (m *AskInput) handleSelectKey(msg tea.KeyMsg) (*[]agent.AskAnswer, tea.Cmd) {
	if len(m.Questions) == 0 || m.Index >= len(m.Questions) {
		return nil, nil
	}
	question := m.Questions[m.Index]
	switch msg.Type {
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
		if m.selectionEmpty() {
			m.toggleSelection(m.Cursor, question)
		}
		if m.containsTypeYourAnswer(question) {
			m.Mode = AskModeCustom
			m.Custom.SetValue("")
			return nil, m.Custom.Focus()
		}
		m.storeSelectionAnswer(question)
		return m.advanceOrFinish()
	default:
		if msg.String() == " " {
			m.toggleSelection(m.Cursor, question)
		}
	}
	return nil, nil
}

func (m *AskInput) handleCustomKey(msg tea.KeyMsg) (*[]agent.AskAnswer, tea.Cmd) {
	if msg.Type == tea.KeyEsc {
		m.Mode = AskModeSelect
		return nil, nil
	}
	if msg.Type == tea.KeyCtrlJ {
		value := strings.TrimSpace(m.Custom.Value())
		if value == "" {
			return nil, nil
		}
		question := m.Questions[m.Index]
		m.Answers[m.Index] = agent.AskAnswer{
			Question:     question.Question,
			Selected:     []string{value},
			CustomAnswer: value,
			UsedCustom:   true,
		}
		return m.advanceOrFinish()
	}
	updated, cmd := m.Custom.Update(msg)
	m.Custom = updated
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
		return
	}
	if m.Selected[m.Index][idx] {
		delete(m.Selected[m.Index], idx)
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

func (m *AskInput) containsTypeYourAnswer(question agent.AskQuestion) bool {
	for idx, option := range question.Options {
		if !m.isSelected(idx) {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(option.Title), agent.AskTypeYourAnswerTitle) {
			return true
		}
	}
	return false
}

func (m *AskInput) storeSelectionAnswer(question agent.AskQuestion) {
	selected := make([]string, 0, len(m.Selected[m.Index]))
	for idx, option := range question.Options {
		if !m.isSelected(idx) {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(option.Title), agent.AskTypeYourAnswerTitle) {
			continue
		}
		selected = append(selected, option.Title)
	}
	m.Answers[m.Index] = agent.AskAnswer{
		Question: question.Question,
		Selected: selected,
	}
}

func (m *AskInput) advanceOrFinish() (*[]agent.AskAnswer, tea.Cmd) {
	if m.Index+1 < len(m.Questions) {
		m.Index++
		m.Cursor = 0
		m.Mode = AskModeSelect
		m.Custom.SetValue("")
		return nil, nil
	}
	answers := m.Answers
	m.Mode = AskModeNone
	return &answers, nil
}
