package tui

import (
	"fmt"
	"strings"

	runtimeagent "github.com/termia/termia/internal/agent"
	"github.com/termia/termia/internal/agentapp"
	"github.com/termia/termia/internal/textutil"

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
	timeline := timelineFromAgentMessages(m.messages)
	timeline = agentapp.AppendTimelineText(timeline, role, content, false)
	m.messages = agentMessagesFromTimeline(timeline)
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
	timeline := timelineFromAgentMessages(m.messages)
	timeline = agentapp.AppendTimelineText(timeline, "assistant", chunk, true)
	m.messages = agentMessagesFromTimeline(timeline)
	m.refreshContent()
	if m.ready {
		m.viewport.GotoBottom()
	}
}

func (m *AgentModel) AppendReasoning(chunk string) {
	timeline := timelineFromAgentMessages(m.messages)
	timeline = agentapp.AppendTimelineText(timeline, "reasoning", chunk, true)
	m.messages = agentMessagesFromTimeline(timeline)
	m.refreshContent()
	if m.ready {
		m.viewport.GotoBottom()
	}
}

func (m *AgentModel) AppendToolCall(toolCall AgentToolCall) {
	timeline := timelineFromAgentMessages(m.messages)
	timeline = agentapp.UpsertTimelineToolCall(timeline, runtimeToolCallFromAgent(toolCall))
	m.messages = agentMessagesFromTimeline(timeline)
	m.refreshContent()
	if m.ready {
		m.viewport.GotoBottom()
	}
}

func (m *AgentModel) MarkLatestPendingToolFailed(reason string) {
	timeline := timelineFromAgentMessages(m.messages)
	timeline = agentapp.MarkLatestPendingToolFailed(timeline, reason)
	m.messages = agentMessagesFromTimeline(timeline)
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

func timelineFromAgentMessages(messages []AgentMessage) []agentapp.TimelineEntry {
	if len(messages) == 0 {
		return nil
	}
	timeline := make([]agentapp.TimelineEntry, 0, len(messages))
	for _, message := range messages {
		entry := agentapp.TimelineEntry{
			Role:              normalizeConversationRole(message.Role),
			Content:           textutil.NormalizeLineEndings(message.Content),
			CitedCommandCount: maxInt(0, message.CitedCommandCount),
		}
		if message.ToolCall != nil {
			call := runtimeToolCallFromAgent(*message.ToolCall)
			entry.ToolCall = &call
		}
		timeline = append(timeline, entry)
	}
	return timeline
}

func agentMessagesFromTimeline(entries []agentapp.TimelineEntry) []AgentMessage {
	if len(entries) == 0 {
		return nil
	}
	messages := make([]AgentMessage, 0, len(entries))
	for _, entry := range entries {
		message := AgentMessage{
			Role:              normalizeConversationRole(entry.Role),
			Content:           textutil.NormalizeLineEndings(entry.Content),
			CitedCommandCount: maxInt(0, entry.CitedCommandCount),
		}
		if entry.ToolCall != nil {
			call := agentToolCallFromRuntime(*entry.ToolCall)
			message.ToolCall = &call
			message.Role = "tool"
			message.Content = ""
		}
		messages = append(messages, message)
	}
	return messages
}

func runtimeToolCallFromAgent(toolCall AgentToolCall) runtimeagent.ToolCallEvent {
	return runtimeagent.ToolCallEvent{
		CallID:    strings.TrimSpace(toolCall.CallID),
		AgentName: textutil.NormalizeInlineText(toolCall.AgentName),
		ToolName:  textutil.NormalizeInlineText(toolCall.ToolName),
		Summary:   textutil.NormalizeInlineText(toolCall.Summary),
		Result:    textutil.NormalizeInlineText(toolCall.Result),
		State:     toolCall.State,
	}
}

func agentToolCallFromRuntime(toolCall runtimeagent.ToolCallEvent) AgentToolCall {
	return AgentToolCall{
		CallID:    strings.TrimSpace(toolCall.CallID),
		AgentName: textutil.NormalizeInlineText(toolCall.AgentName),
		ToolName:  textutil.NormalizeInlineText(toolCall.ToolName),
		Summary:   textutil.NormalizeInlineText(toolCall.Summary),
		Result:    textutil.NormalizeInlineText(toolCall.Result),
		State:     toolCall.State,
	}
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
	return textutil.NormalizeTrimmedText(message.Content) != ""
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
	case "reasoning":
		return "reasoning"
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
		return strings.Join(renderPrefixedMarkdown(textutil.NormalizeTrimmedText(message.Content), width, systemBulletPrefixStyle.Render("• "), "  ", systemBodyStyle), "\n")
	case "error":
		return strings.Join(renderPrefixedMarkdown(textutil.NormalizeTrimmedText(message.Content), width, errorBulletPrefixStyle.Render("• "), "  ", errorBodyStyle), "\n")
	case "reasoning":
		return strings.Join(renderPrefixedMarkdown(textutil.NormalizeTrimmedText(message.Content), width, reasoningBulletPrefixStyle.Render("… "), "  ", reasoningBodyStyle), "\n")
	default:
		return strings.Join(renderPrefixedMarkdown(textutil.NormalizeTrimmedText(message.Content), width, assistantBulletPrefixStyle.Render("• "), "  ", assistantBodyStyle), "\n")
	}
}

func renderUserMessage(message AgentMessage, width int) []string {
	lines := renderPrefixedMarkdown(textutil.NormalizeTrimmedText(message.Content), width, userPromptPrefixStyle.Render("> "), "  ", userBodyStyle)
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
	content = textutil.NormalizeTrimmedText(content)
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
	toolCall = agentToolCallFromRuntime(runtimeToolCallFromAgent(toolCall))
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
