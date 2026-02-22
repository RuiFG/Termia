package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/termia/termia/internal/agent"
)

type ApprovalMode int

const (
	ApprovalModeNone ApprovalMode = iota
	ApprovalModePrompt
	ApprovalModeEdit
	ApprovalModeConfirmEdit
	ApprovalModeRephrase
)

type ApprovalInput struct {
	Prompt      agent.ApprovalPrompt
	Mode        ApprovalMode
	editInput   textinput.Model
	rephrase    textarea.Model
	editedValue string
}

func NewApprovalInput() ApprovalInput {
	edit := textinput.New()
	edit.Placeholder = ""
	edit.Prompt = "> "
	edit.PromptStyle = inputPromptStyle
	edit.CharLimit = 500
	edit.Cursor.Style = inputCursorStyle
	edit.Cursor.Blink = false

	rp := textarea.New()
	rp.Placeholder = ""
	rp.Prompt = "> "
	rp.SetWidth(suggestedMinWidth)
	rp.SetHeight(3)
	rp.ShowLineNumbers = false
	rp.EndOfBufferCharacter = 0
	focusedStyle, blurredStyle := textarea.DefaultStyles()
	focusedStyle.CursorLine = lipgloss.NewStyle()
	focusedStyle.CursorLineNumber = lipgloss.NewStyle()
	focusedStyle.EndOfBuffer = lipgloss.NewStyle()
	focusedStyle.Text = lipgloss.NewStyle()
	focusedStyle.Placeholder = lipgloss.NewStyle().Foreground(colorMuted)
	focusedStyle.Prompt = inputPromptStyle
	blurredStyle = focusedStyle
	rp.FocusedStyle = focusedStyle
	rp.BlurredStyle = blurredStyle
	rp.Cursor.Style = inputCursorStyle
	rp.Cursor.Blink = false
	promptWidth := lipgloss.Width("> ")
	rp.SetPromptFunc(promptWidth, func(lineIdx int) string {
		if lineIdx == 0 {
			return "> "
		}
		return ""
	})

	return ApprovalInput{
		Mode:      ApprovalModeNone,
		editInput: edit,
		rephrase:  rp,
	}
}

func (m *ApprovalInput) SetPrompt(prompt agent.ApprovalPrompt) {
	m.Prompt = prompt
	m.Mode = ApprovalModePrompt
	m.editedValue = ""
	m.editInput.SetValue("")
	m.rephrase.SetValue("")
}

func (m ApprovalInput) Active() bool {
	return m.Mode != ApprovalModeNone
}

func (m *ApprovalInput) SetWidth(width int) {
	inputWidth := maxInt(width-lipgloss.Width(m.editInput.Prompt), suggestedMinWidth)
	m.editInput.Width = inputWidth
	m.rephrase.SetWidth(inputWidth)
}

func (m ApprovalInput) View(contentWidth int) string {
	lines := []string{}
	switch m.Mode {
	case ApprovalModeEdit:
		lines = append(lines, "Edit command:")
		lines = append(lines, m.editInput.View())
		lines = append(lines, "Enter=confirm  Esc=cancel")
	case ApprovalModeConfirmEdit:
		lines = append(lines, "Confirm edited command:")
		lines = append(lines, "  "+m.editedValue)
		lines = append(lines, "Enter=approve  E=edit  R=reject")
	case ApprovalModeRephrase:
		lines = append(lines, "Describe desired action:")
		lines = append(lines, m.rephrase.View())
		lines = append(lines, "Ctrl+J=submit  Esc=cancel")
	case ApprovalModePrompt:
		lines = append(lines, "Proposed command:")
		lines = append(lines, "  "+strings.TrimSpace(m.Prompt.Command))
		if strings.TrimSpace(m.Prompt.RiskNote) != "" {
			lines = append(lines, "")
			lines = append(lines, "Risk/Note:")
			lines = append(lines, "  "+strings.TrimSpace(m.Prompt.RiskNote))
		}
		lines = append(lines, "")
		lines = append(lines, "Enter=approve  E=edit  R=reject  N=natural language")
	}
	return strings.Join(lines, "\n")
}

func (m *ApprovalInput) Update(msg tea.Msg) (*agent.ApprovalDecision, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, nil
	}
	switch m.Mode {
	case ApprovalModePrompt:
		return m.handlePromptKey(keyMsg)
	case ApprovalModeEdit:
		return m.handleEditKey(keyMsg)
	case ApprovalModeConfirmEdit:
		return m.handleConfirmKey(keyMsg)
	case ApprovalModeRephrase:
		return m.handleRephraseKey(keyMsg)
	default:
		return nil, nil
	}
}

func (m *ApprovalInput) handlePromptKey(msg tea.KeyMsg) (*agent.ApprovalDecision, tea.Cmd) {
	key := strings.ToLower(msg.String())
	switch {
	case msg.Type == tea.KeyEnter:
		decision := agent.ApprovalDecision{Type: agent.ApprovalDecisionApprove}
		return &decision, nil
	case key == "e":
		m.Mode = ApprovalModeEdit
		m.editInput.SetValue(strings.TrimSpace(m.Prompt.Command))
		return nil, m.editInput.Focus()
	case key == "r":
		decision := agent.ApprovalDecision{Type: agent.ApprovalDecisionReject}
		return &decision, nil
	case key == "n":
		m.Mode = ApprovalModeRephrase
		m.rephrase.SetValue("")
		return nil, m.rephrase.Focus()
	}
	return nil, nil
}

func (m *ApprovalInput) handleEditKey(msg tea.KeyMsg) (*agent.ApprovalDecision, tea.Cmd) {
	if msg.Type == tea.KeyEsc {
		m.Mode = ApprovalModePrompt
		return nil, nil
	}
	if msg.Type == tea.KeyEnter {
		value := strings.TrimSpace(m.editInput.Value())
		if value == "" {
			return nil, nil
		}
		m.editedValue = value
		m.Mode = ApprovalModeConfirmEdit
		return nil, nil
	}
	updated, cmd := m.editInput.Update(msg)
	m.editInput = updated
	return nil, cmd
}

func (m *ApprovalInput) handleConfirmKey(msg tea.KeyMsg) (*agent.ApprovalDecision, tea.Cmd) {
	key := strings.ToLower(msg.String())
	switch {
	case msg.Type == tea.KeyEnter:
		decision := agent.ApprovalDecision{Type: agent.ApprovalDecisionEdit, Command: strings.TrimSpace(m.editedValue)}
		return &decision, nil
	case key == "e":
		m.Mode = ApprovalModeEdit
		return nil, m.editInput.Focus()
	case key == "r", msg.Type == tea.KeyEsc:
		decision := agent.ApprovalDecision{Type: agent.ApprovalDecisionReject}
		return &decision, nil
	}
	return nil, nil
}

func (m *ApprovalInput) handleRephraseKey(msg tea.KeyMsg) (*agent.ApprovalDecision, tea.Cmd) {
	if msg.Type == tea.KeyEsc {
		m.Mode = ApprovalModePrompt
		return nil, nil
	}
	if msg.Type == tea.KeyCtrlJ {
		value := strings.TrimSpace(m.rephrase.Value())
		if value == "" {
			return nil, nil
		}
		decision := agent.ApprovalDecision{Type: agent.ApprovalDecisionRephrase, Rephrase: value}
		return &decision, nil
	}
	updated, cmd := m.rephrase.Update(msg)
	m.rephrase = updated
	return nil, cmd
}
