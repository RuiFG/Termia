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

// AgentModel manages the agent interaction panel.
type AgentModel struct {
	viewport viewport.Model
	messages []string
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
func (m *AgentModel) AddMessage(msg string) {
	m.messages = append(m.messages, msg)
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
			emptyStyle.Render("Type a question in the input below, or use /help."),
			"",
			metaStyle.Render("  Supported providers: OpenAI, Anthropic, Ollama, DeepSeek"),
			metaStyle.Render("  Configure with: /models"),
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
	content := strings.Join(m.messages, "\n\n")
	m.viewport.SetContent(content)
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
