package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/termia/termia/internal/agent"
)

type ApprovalInput struct {
	Request agent.HITLRequest
	active  bool
	cursor  int
}

func NewApprovalInput() ApprovalInput {
	return ApprovalInput{}
}

func (m *ApprovalInput) SetRequest(request agent.HITLRequest) {
	m.Request = request
	m.active = true
	m.cursor = 0
}

func (m ApprovalInput) Active() bool {
	return m.active
}

func (m *ApprovalInput) SetWidth(width int) {
	_ = width
}

func (m ApprovalInput) View(contentWidth int) string {
	width := maxInt(1, contentWidth)
	lines := []string{hitlTitleStyle.Render(approvalTitle(m.Request))}
	if line := approvalCommandLine(m.Request); line != "" {
		lines = append(lines, renderCodeBlockLine(line, width)...)
	}
	meta := approvalMetaLine(m.Request)
	if meta != "" {
		lines = append(lines, hitlSubtitleStyle.Render(meta))
	}
	if prompt := approvalPrompt(m.Request); prompt != "" {
		lines = append(lines, renderStyledParagraph(prompt, "", width, hitlHintStyle)...)
	}
	lines = append(lines, "")
	lines = append(lines, renderApprovalActions(m.cursor))
	return strings.Join(lines, "\n")
}

func (m *ApprovalInput) Update(msg tea.Msg) (*agent.HITLResponse, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil, nil
	}
	switch {
	case keyMsg.Type == tea.KeyLeft || keyMsg.Type == tea.KeyShiftTab:
		m.cursor = 0
		return nil, nil
	case keyMsg.Type == tea.KeyRight || keyMsg.Type == tea.KeyTab:
		m.cursor = 1
		return nil, nil
	case keyMsg.Type == tea.KeyUp:
		m.cursor = 0
		return nil, nil
	case keyMsg.Type == tea.KeyDown:
		m.cursor = 1
		return nil, nil
	case keyMsg.Type == tea.KeyEnter:
		resp := approvalResponse(m.cursor == 0)
		m.active = false
		return resp, nil
	case keyMsg.Type == tea.KeyEsc:
		resp := approvalResponse(false)
		m.active = false
		return resp, nil
	default:
		return nil, nil
	}
}

func (m ApprovalInput) String() string {
	return fmt.Sprintf("active=%v tool=%s", m.active, m.Request.OriginalTool)
}

func approvalTitle(request agent.HITLRequest) string {
	if strings.TrimSpace(request.Command) != "" {
		return "Allow command"
	}
	if title := strings.TrimSpace(request.Title); title != "" && !strings.EqualFold(title, "Confirmation Required") {
		return title
	}
	return "Allow action"
}

func approvalMetaLine(request agent.HITLRequest) string {
	parts := make([]string, 0, 2)
	if tool := strings.TrimSpace(request.OriginalTool); tool != "" && !strings.EqualFold(tool, "command") {
		parts = append(parts, "tool "+tool)
	}
	return strings.Join(parts, "  ")
}

func approvalCommandLine(request agent.HITLRequest) string {
	command := strings.TrimSpace(request.Command)
	if command == "" {
		return ""
	}
	if cwd := strings.TrimSpace(request.Cwd); cwd != "" {
		return command + "  " + cwd
	}
	return command
}

func approvalPrompt(request agent.HITLRequest) string {
	if note := strings.TrimSpace(request.RiskNote); note != "" {
		return note
	}
	prompt := strings.TrimSpace(request.Prompt)
	if isGenericApprovalPrompt(prompt) {
		return ""
	}
	return prompt
}

func isGenericApprovalPrompt(prompt string) bool {
	if prompt == "" {
		return true
	}
	if strings.EqualFold(prompt, "Approval required.") {
		return true
	}
	lower := strings.ToLower(prompt)
	return strings.Contains(lower, "approve or reject the tool call")
}

func renderApprovalActions(cursor int) string {
	allow := renderApprovalActionChoice("Allow", "Enter", cursor == 0, false)
	reject := renderApprovalActionChoice("Reject", "Esc", cursor == 1, true)
	return allow + "  " + reject
}

func renderApprovalActionChoice(label, hint string, focused bool, danger bool) string {
	titleStyle := hitlChoiceTitleStyle
	hintStyle := hitlChoiceDescStyle
	if focused {
		titleStyle = hitlChoiceFocusStyle
	}
	if danger && focused {
		titleStyle = hitlChoiceFocusStyle
	}
	return titleStyle.Render(label) + " " + hintStyle.Render(hint)
}

func approvalResponse(confirmed bool) *agent.HITLResponse {
	if confirmed {
		return &agent.HITLResponse{Confirmed: true}
	}
	return &agent.HITLResponse{
		Confirmed: false,
		Payload: map[string]any{
			"status":  "rejected",
			"reason":  "User rejected this tool call.",
			"message": "Do not execute the tool. Ask the user for an alternative if needed.",
		},
	}
}
