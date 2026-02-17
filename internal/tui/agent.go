package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// StepStatus represents the status of an agent step.
type StepStatus int

const (
	StepPending StepStatus = iota
	StepRunning
	StepCompleted
	StepSkipped
	StepFailed
)

// AgentStep represents a single step in an agent plan.
type AgentStep struct {
	Command     string
	Explanation string
	Status      StepStatus
	Output      string
	ExitCode    *int
}

type AgentMessage struct {
	Role    string
	Content string
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
	return AgentModel{
		keys: keys,
	}
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

// AddMessage adds a message to the agent log.
func (m *AgentModel) AddMessage(role, content string) {
	m.messages = append(m.messages, AgentMessage{Role: role, Content: content})
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

// AppendToLast appends content to the most recent message.
func (m *AgentModel) AppendToLast(chunk string) {
	if len(m.messages) == 0 {
		m.AddMessage("agent", chunk)
		return
	}
	last := len(m.messages) - 1
	m.messages[last].Content += chunk
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

// View renders the agent panel.
func (m AgentModel) View() string {
	if !m.ready {
		return ""
	}

	if len(m.messages) == 0 {
		welcome := lipgloss.JoinVertical(lipgloss.Left,
			panelTitleStyle.Render("Agent Mode"),
			"",
			emptyStyle.Render("Ask AI about your commands and terminal history."),
			emptyStyle.Render("Type a question in the input below."),
			emptyStyle.Render("Use Ctrl+P for commands, Ctrl+X for modes."),
			"",
			"  "+metadataLabelStyle.Render("Providers:")+metaStyle.Render(" OpenAI, Anthropic, Ollama, DeepSeek"),
			"  "+metadataLabelStyle.Render("Models:")+metaStyle.Render(" Ctrl+P → Models"),
		)
		// Parent renderContent() clamps to exact height; no need to double-clamp here.
		return lipgloss.NewStyle().Width(m.width).Height(m.height).Render(welcome)
	}

	content := m.viewport.View()
	return lipgloss.NewStyle().Width(m.width).Height(m.height).Render(content)
}

// refreshContent re-renders the message log into the viewport.
func (m *AgentModel) refreshContent() {
	if !m.ready || len(m.messages) == 0 {
		return
	}
	sections := make([]string, 0, len(m.messages))
	for _, msg := range m.messages {
		role := roleLabel(msg.Role)
		if role == "" {
			role = "Agent"
		}
		header := roleHeaderStyle.Render(role)
		separator := roleSeparatorStyle.Render(strings.Repeat("─", maxInt(1, m.width)))
		section := fmt.Sprintf("%s\n%s\n%s", header, separator, msg.Content)
		sections = append(sections, section)
	}
	content := strings.Join(sections, "\n\n")
	m.viewport.SetContent(content)
}

func roleLabel(role string) string {
	role = strings.TrimSpace(strings.ToLower(role))
	switch role {
	case "user":
		return "User"
	case "assistant", "agent":
		return "Agent"
	case "system":
		return "System"
	case "error":
		return "Error"
	default:
		if role == "" {
			return ""
		}
		runes := []rune(role)
		runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
		return string(runes)
	}
}

func statusIcon(status StepStatus) string {
	switch status {
	case StepCompleted:
		return "OK"
	case StepRunning:
		return ">"
	case StepSkipped:
		return "SKIP"
	case StepFailed:
		return "FAIL"
	default:
		return "WAIT"
	}
}

func formatAgentStep(step AgentStep, width int) string {
	line := fmt.Sprintf("[%s] %s", statusIcon(step.Status), step.Command)
	if len(line) > width {
		return line[:width-1]
	}
	return line
}
