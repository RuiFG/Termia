package tui

import (
	"fmt"
	"strings"

	runtimeagent "github.com/termia/termia/internal/agent"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type AgentToolCall struct {
	CallID    string
	AgentName string
	ToolName  string
	Summary   string
	Result    string
	State     runtimeagent.ToolCallState
}

type AgentMessage struct {
	Role              string
	Content           string
	CitedCommandCount int
	ToolCall          *AgentToolCall
}

// AgentModel manages the agent interaction panel.
type AgentModel struct {
	viewport viewport.Model
	messages []AgentMessage
	width    int
	height   int
	ready    bool
	keys     KeyMap
}

// NewAgentModel creates a new agent panel.
func NewAgentModel(keys KeyMap) AgentModel {
	return AgentModel{keys: keys}
}

// SetSize updates the agent panel dimensions.
func (m *AgentModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	if !m.ready {
		m.viewport = viewport.New(w, h)
		m.viewport.MouseWheelEnabled = true
		m.ready = true
	} else {
		m.viewport.Width = w
		m.viewport.Height = h
	}
	m.refreshContent()
}

// AddMessage appends a new message block to the conversation timeline.
func (m *AgentModel) AddMessage(role, content string) {
	m.messages = appendTimelineText(m.messages, role, content, false)
	m.refreshContent()
	if m.ready {
		m.viewport.GotoBottom()
	}
}

func (m *AgentModel) AddTimelineMessage(message AgentMessage) {
	if !renderableTimelineMessage(message) {
		return
	}
	if message.ToolCall != nil {
		m.AppendToolCall(*message.ToolCall)
		return
	}
	m.messages = append(m.messages, AgentMessage{
		Role:              normalizeConversationRole(message.Role),
		Content:           message.Content,
		CitedCommandCount: maxInt(0, message.CitedCommandCount),
	})
	m.refreshContent()
	if m.ready {
		m.viewport.GotoBottom()
	}
}

func (m *AgentModel) SetMessages(messages []AgentMessage) {
	m.messages = nil
	if len(messages) > 0 {
		m.messages = append(m.messages, messages...)
	}
	m.refreshContent()
	if m.ready {
		m.viewport.GotoBottom()
	}
}

// AppendToLast appends assistant text to the latest assistant block, or starts one if needed.
func (m *AgentModel) AppendToLast(chunk string) {
	m.messages = appendTimelineText(m.messages, "assistant", chunk, true)
	m.refreshContent()
	if m.ready {
		m.viewport.GotoBottom()
	}
}

func (m *AgentModel) AppendToolCall(toolCall AgentToolCall) {
	m.messages = upsertTimelineToolCall(m.messages, toolCall)
	m.refreshContent()
	if m.ready {
		m.viewport.GotoBottom()
	}
}

func (m *AgentModel) MarkLatestPendingToolFailed(reason string) {
	m.messages = markLatestPendingToolFailed(m.messages, reason)
	m.refreshContent()
	if m.ready {
		m.viewport.GotoBottom()
	}
}

// Update handles events for the agent panel.
func (m AgentModel) Update(msg tea.Msg) (AgentModel, tea.Cmd) {
	if !m.ready {
		return m, nil
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m *AgentModel) Scroll(delta int) {
	if !m.ready || delta == 0 {
		return
	}
	if delta > 0 {
		m.viewport.ScrollDown(delta)
		return
	}
	m.viewport.ScrollUp(-delta)
}

// View renders the agent panel.
func (m AgentModel) View() string {
	if !m.ready {
		return ""
	}
	if len(m.messages) == 0 {
		welcome := lipgloss.JoinVertical(lipgloss.Left,
			panelTitleStyle.Render("Assistant Mode"),
			"",
			emptyStyle.Render("Ask AI about your commands and terminal history."),
			emptyStyle.Render("Type a question in the input below."),
			emptyStyle.Render("Use Ctrl+P for commands, Ctrl+X for modes."),
			"",
			"  "+metadataLabelStyle.Render("Providers:")+metaStyle.Render(" OpenAI, Anthropic, Ollama, DeepSeek"),
			"  "+metadataLabelStyle.Render("Models:")+metaStyle.Render(" Ctrl+P → Models"),
		)
		return lipgloss.NewStyle().Width(m.width).Height(m.height).Render(welcome)
	}
	content := m.viewport.View()
	return lipgloss.NewStyle().Width(m.width).Height(m.height).Render(content)
}

func (m *AgentModel) refreshContent() {
	if !m.ready {
		return
	}
	m.viewport.SetContent(renderConversationTimeline(m.messages, m.width))
}

func appendTimelineText(messages []AgentMessage, role, content string, appendToLast bool) []AgentMessage {
	if content == "" {
		return messages
	}
	role = normalizeConversationRole(role)
	if appendToLast && len(messages) > 0 {
		last := &messages[len(messages)-1]
		if normalizeConversationRole(last.Role) == role && last.ToolCall == nil {
			last.Content += content
			return messages
		}
	}
	return append(messages, AgentMessage{Role: role, Content: content})
}

func upsertTimelineToolCall(messages []AgentMessage, toolCall AgentToolCall) []AgentMessage {
	normalized := normalizeToolCall(toolCall)
	if normalized.ToolName == "" {
		return messages
	}
	if normalized.CallID != "" {
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].ToolCall == nil {
				continue
			}
			if strings.TrimSpace(messages[i].ToolCall.CallID) != normalized.CallID {
				continue
			}
			merged := mergeToolCall(*messages[i].ToolCall, normalized)
			messages[i].ToolCall = &merged
			return messages
		}
	}
	if normalized.State != "" && normalized.State != runtimeagent.ToolCallStatePending {
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].ToolCall == nil {
				continue
			}
			current := messages[i].ToolCall
			if current.State != runtimeagent.ToolCallStatePending {
				continue
			}
			if current.ToolName != normalized.ToolName {
				continue
			}
			if current.Summary != normalized.Summary {
				continue
			}
			merged := mergeToolCall(*current, normalized)
			messages[i].ToolCall = &merged
			return messages
		}
	}
	return append(messages, AgentMessage{Role: "tool", ToolCall: &normalized})
}

func markLatestPendingToolFailed(messages []AgentMessage, reason string) []AgentMessage {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].ToolCall == nil {
			continue
		}
		if messages[i].ToolCall.State != runtimeagent.ToolCallStatePending {
			continue
		}
		call := *messages[i].ToolCall
		call.State = runtimeagent.ToolCallStateError
		if strings.TrimSpace(call.Result) == "" {
			call.Result = strings.TrimSpace(reason)
		}
		messages[i].ToolCall = &call
		return messages
	}
	return messages
}

func normalizeToolCall(toolCall AgentToolCall) AgentToolCall {
	return AgentToolCall{
		CallID:    strings.TrimSpace(toolCall.CallID),
		AgentName: strings.TrimSpace(toolCall.AgentName),
		ToolName:  strings.TrimSpace(toolCall.ToolName),
		Summary:   strings.TrimSpace(toolCall.Summary),
		Result:    strings.TrimSpace(toolCall.Result),
		State:     toolCall.State,
	}
}

func mergeToolCall(existing, incoming AgentToolCall) AgentToolCall {
	merged := existing
	if merged.CallID == "" {
		merged.CallID = incoming.CallID
	}
	if merged.AgentName == "" {
		merged.AgentName = incoming.AgentName
	}
	if merged.ToolName == "" {
		merged.ToolName = incoming.ToolName
	}
	if strings.TrimSpace(incoming.Summary) != "" {
		merged.Summary = incoming.Summary
	}
	if strings.TrimSpace(incoming.Result) != "" {
		merged.Result = incoming.Result
	}
	if incoming.State != "" {
		merged.State = incoming.State
	}
	return merged
}

func renderConversationTimeline(messages []AgentMessage, width int) string {
	if len(messages) == 0 {
		return ""
	}
	width = maxInt(1, width)
	sections := make([]string, 0, len(messages)*2)
	first := true
	for _, message := range messages {
		if !renderableTimelineMessage(message) {
			continue
		}
		if !first && normalizeConversationRole(message.Role) == "user" {
			sections = append(sections, conversationDividerStyle.Render(strings.Repeat("─", width)))
		}
		first = false
		sections = append(sections, renderTimelineMessage(message, width))
	}
	return strings.Join(sections, "\n")
}

func renderableTimelineMessage(message AgentMessage) bool {
	if message.ToolCall != nil {
		return strings.TrimSpace(message.ToolCall.ToolName) != ""
	}
	return strings.TrimSpace(message.Content) != ""
}

func normalizeConversationRole(role string) string {
	role = strings.TrimSpace(strings.ToLower(role))
	switch role {
	case "user":
		return "user"
	case "assistant", "agent":
		return "assistant"
	case "tool":
		return "tool"
	case "system":
		return "system"
	case "error":
		return "error"
	default:
		if role == "" {
			return "assistant"
		}
		return role
	}
}

func renderTimelineMessage(message AgentMessage, width int) string {
	if message.ToolCall != nil {
		return strings.Join(renderToolMessage(*message.ToolCall, width), "\n")
	}
	role := normalizeConversationRole(message.Role)
	switch role {
	case "user":
		return strings.Join(renderUserMessage(message, width), "\n")
	case "system":
		return strings.Join(renderPrefixedMarkdown(strings.TrimSpace(message.Content), width, systemBulletPrefixStyle.Render("• "), "  ", systemBodyStyle), "\n")
	case "error":
		return strings.Join(renderPrefixedMarkdown(strings.TrimSpace(message.Content), width, errorBulletPrefixStyle.Render("• "), "  ", errorBodyStyle), "\n")
	default:
		return strings.Join(renderPrefixedMarkdown(strings.TrimSpace(message.Content), width, assistantBulletPrefixStyle.Render("• "), "  ", assistantBodyStyle), "\n")
	}
}

func renderUserMessage(message AgentMessage, width int) []string {
	lines := renderPrefixedMarkdown(strings.TrimSpace(message.Content), width, userPromptPrefixStyle.Render("> "), "  ", userBodyStyle)
	if message.CitedCommandCount <= 0 {
		return lines
	}
	summary := formatUserCommandSummary(message.CitedCommandCount)
	if strings.TrimSpace(summary) == "" {
		return lines
	}
	rendered := renderMarkdown(summary, maxInt(1, width-2), metaStyle)
	for _, line := range strings.Split(rendered, "\n") {
		if strings.TrimSpace(stripANSICodes(line)) == "" {
			continue
		}
		lines = append(lines, "  "+line)
	}
	return lines
}

func formatUserCommandSummary(count int) string {
	if count <= 0 {
		return ""
	}
	if count == 1 {
		return "referenced 1 command"
	}
	return fmt.Sprintf("referenced %d commands", count)
}

func renderPrefixedMarkdown(content string, width int, prefix, continuation string, bodyStyle lipgloss.Style) []string {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	prefixWidth := lipgloss.Width(prefix)
	bodyWidth := maxInt(1, width-prefixWidth)
	rendered := renderMarkdown(content, bodyWidth, bodyStyle)
	if strings.TrimSpace(rendered) == "" {
		return nil
	}
	lines := strings.Split(rendered, "\n")
	output := make([]string, 0, len(lines))
	for idx, line := range lines {
		if idx == 0 {
			output = append(output, prefix+line)
			continue
		}
		if strings.TrimSpace(line) == "" {
			output = append(output, "")
			continue
		}
		output = append(output, continuation+line)
	}
	return output
}

func renderToolMessage(toolCall AgentToolCall, width int) []string {
	toolCall = normalizeToolCall(toolCall)
	if toolCall.ToolName == "" {
		return nil
	}
	prefixStyle, bodyStyle := toolStyles(toolCall.State)
	prefix := prefixStyle.Render("• ")
	bodyWidth := maxInt(1, width-lipgloss.Width(prefix))
	lines := renderStyledParagraph(renderToolLine(toolCall), "", bodyWidth, bodyStyle)
	if len(lines) == 0 {
		return nil
	}
	output := make([]string, 0, len(lines))
	for idx, line := range lines {
		if idx == 0 {
			output = append(output, prefix+line)
			continue
		}
		output = append(output, "  "+line)
	}
	return output
}

func renderToolLine(toolCall AgentToolCall) string {
	parts := make([]string, 0, 3)
	if agentName := strings.TrimSpace(toolCall.AgentName); agentName != "" && !strings.EqualFold(agentName, "assistant") {
		parts = append(parts, agentName)
	}
	head := strings.TrimSpace(toolCall.ToolName)
	if summary := strings.TrimSpace(toolCall.Summary); summary != "" {
		head = strings.TrimSpace(head + " " + summary)
	}
	if head != "" {
		parts = append(parts, head)
	}
	result := strings.TrimSpace(toolCall.Result)
	if result != "" && !strings.EqualFold(result, "ok") && !strings.EqualFold(result, "done") {
		parts = append(parts, "· "+result)
	}
	return strings.Join(parts, " ")
}

func toolStyles(state runtimeagent.ToolCallState) (lipgloss.Style, lipgloss.Style) {
	switch state {
	case runtimeagent.ToolCallStateSuccess:
		return toolSuccessStyle.Copy().Bold(true), toolSuccessStyle
	case runtimeagent.ToolCallStateError:
		return toolErrorStyle.Copy().Bold(true), toolErrorStyle
	default:
		return toolPendingStyle.Copy().Bold(true), toolPendingStyle
	}
}

func (m AgentMessage) String() string {
	if m.ToolCall != nil {
		return fmt.Sprintf("tool: %s", renderToolLine(*m.ToolCall))
	}
	return fmt.Sprintf("%s: %s", m.Role, m.Content)
}
