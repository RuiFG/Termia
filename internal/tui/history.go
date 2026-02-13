package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/truncate"
	"github.com/termia/termia/internal/db"
)

// HistoryModel manages the command history list with viewport scrolling.
type HistoryModel struct {
	commands []db.Command
	selected int
	cited    map[string]bool // cited command IDs for reference feature
	viewport viewport.Model
	width    int
	height   int
	loading  bool
	ready    bool
	keys     KeyMap
}

// NewHistoryModel creates a new history panel.
func NewHistoryModel(keys KeyMap) HistoryModel {
	return HistoryModel{
		keys:    keys,
		cited:   make(map[string]bool),
		loading: true,
	}
}

// SetSize updates the dimensions of the history panel.
func (m *HistoryModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	if !m.ready {
		m.viewport = viewport.New(w, h)
		m.viewport.MouseWheelEnabled = true
		m.viewport.MouseWheelDelta = 3
		m.ready = true
	} else {
		m.viewport.Width = w
		m.viewport.Height = h
	}
	m.refreshContent()
}

// SetCommands updates the command list and refreshes the view.
func (m *HistoryModel) SetCommands(commands []db.Command) {
	m.commands = commands
	m.loading = false
	if m.selected >= len(m.commands) {
		m.selected = len(m.commands) - 1
		if m.selected < 0 {
			m.selected = 0
		}
	}
	m.refreshContent()
}

// SelectedCommand returns the currently selected command, if any.
func (m HistoryModel) SelectedCommand() *db.Command {
	if len(m.commands) == 0 || m.selected < 0 || m.selected >= len(m.commands) {
		return nil
	}
	cmd := m.commands[m.selected]
	return &cmd
}

// SelectedIndex returns the current selection index.
func (m HistoryModel) SelectedIndex() int {
	return m.selected
}

// Commands returns the current command list.
func (m HistoryModel) Commands() []db.Command {
	return m.commands
}

// ToggleCited toggles citation on the currently selected command.
func (m *HistoryModel) ToggleCited() {
	cmd := m.SelectedCommand()
	if cmd == nil {
		return
	}
	if m.cited[cmd.ID] {
		delete(m.cited, cmd.ID)
	} else {
		m.cited[cmd.ID] = true
	}
	m.refreshContent()
}

// CitedCount returns the number of cited commands.
func (m HistoryModel) CitedCount() int {
	return len(m.cited)
}

// CitedCommands returns the list of cited commands.
func (m HistoryModel) CitedCommands() []db.Command {
	var result []db.Command
	for _, cmd := range m.commands {
		if m.cited[cmd.ID] {
			result = append(result, cmd)
		}
	}
	return result
}

// IsCited returns whether a command ID is cited.
func (m HistoryModel) IsCited(id string) bool {
	return m.cited[id]
}

// RemoveCommand removes a command by ID and adjusts selection.
func (m *HistoryModel) RemoveCommand(id string) {
	for i, cmd := range m.commands {
		if cmd.ID == id {
			m.commands = append(m.commands[:i], m.commands[i+1:]...)
			if m.selected >= len(m.commands) {
				m.selected = len(m.commands) - 1
				if m.selected < 0 {
					m.selected = 0
				}
			}
			break
		}
	}
	m.refreshContent()
}

// UpdateCommand updates a command in the list (e.g., after toggling favorite).
func (m *HistoryModel) UpdateCommand(updated db.Command) {
	for i, cmd := range m.commands {
		if cmd.ID == updated.ID {
			m.commands[i] = updated
			break
		}
	}
	m.refreshContent()
}

// Update handles key events for the history panel.
func (m HistoryModel) Update(msg tea.Msg) (HistoryModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Up):
			if m.selected > 0 {
				m.selected--
				m.refreshContent()
				m.ensureVisible()
			}
			return m, nil
		case key.Matches(msg, m.keys.Down):
			if m.selected < len(m.commands)-1 {
				m.selected++
				m.refreshContent()
				m.ensureVisible()
			}
			return m, nil
		case key.Matches(msg, m.keys.GotoTop):
			m.selected = 0
			m.refreshContent()
			m.viewport.GotoTop()
			return m, nil
		case key.Matches(msg, m.keys.GotoEnd):
			if len(m.commands) > 0 {
				m.selected = len(m.commands) - 1
				m.refreshContent()
				m.viewport.GotoBottom()
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// View renders the history panel.
func (m HistoryModel) View() string {
	if !m.ready {
		return ""
	}

	if m.loading {
		return loadingStyle.Render("  Loading commands...")
	}

	if len(m.commands) == 0 {
		return emptyStyle.Render("No commands recorded yet.\nRun some commands in your terminal, then come back!")
	}

	return m.viewport.View()
}

// refreshContent re-renders the full command list into the viewport.
func (m *HistoryModel) refreshContent() {
	if !m.ready || m.loading {
		return
	}

	var lines []string
	for i, cmd := range m.commands {
		line := m.renderRow(cmd, i == m.selected)
		if line != "" {
			lines = append(lines, line)
		}
	}

	content := strings.Join(lines, "\n")
	m.viewport.SetContent(content)
}

// ensureVisible scrolls the viewport to keep the selected item visible.
func (m *HistoryModel) ensureVisible() {
	if !m.ready {
		return
	}
	top := m.viewport.YOffset
	bottom := top + m.viewport.Height - 1

	if m.selected < top {
		m.viewport.SetYOffset(m.selected)
	} else if m.selected > bottom {
		m.viewport.SetYOffset(m.selected - m.viewport.Height + 1)
	}
}

// renderRow renders a single command row.
func (m HistoryModel) renderRow(cmd db.Command, selected bool) string {
	w := m.width - 2 // padding

	// Build the command text — ensure single line (no newlines, no empty)
	cmdText := strings.TrimSpace(cmd.Command)
	if cmdText == "" {
		cmdText = "(empty)"
	}
	// Replace any newlines with spaces to keep on one line
	cmdText = strings.ReplaceAll(cmdText, "\n", " ")
	cmdText = strings.ReplaceAll(cmdText, "\r", "")

	// Citation marker
	citeMarker := ""
	if m.cited[cmd.ID] {
		citeMarker = citedMarkerStyle.Render("📎 ")
	}
	citeWidth := lipgloss.Width(citeMarker)

	// Exit code badge
	exitBadge := ""
	if cmd.ExitCode != nil {
		if *cmd.ExitCode == 0 {
			exitBadge = exitOKStyle.Render("0")
		} else {
			exitBadge = exitErrStyle.Render(fmt.Sprintf("%d", *cmd.ExitCode))
		}
	}

	// Favorite
	fav := ""
	if cmd.Favorite {
		fav = favoriteStyle.Render("*")
	}

	// Time
	timeStr := metaStyle.Render(formatRelativeTime(cmd.TsStart))

	// Duration
	durStr := ""
	if cmd.DurationMs != nil && *cmd.DurationMs > 0 {
		durStr = metaStyle.Render(formatDuration(cmd.DurationMs))
	}

	// Build right-side metadata
	var rightParts []string
	if exitBadge != "" {
		rightParts = append(rightParts, exitBadge)
	}
	if durStr != "" {
		rightParts = append(rightParts, durStr)
	}
	if timeStr != "" {
		rightParts = append(rightParts, timeStr)
	}
	if fav != "" {
		rightParts = append(rightParts, fav)
	}
	rightSide := strings.Join(rightParts, " ")
	rightWidth := lipgloss.Width(rightSide)

	available := w - rightWidth - citeWidth - 3
	if available < 10 {
		available = 10
	}
	if lipgloss.Width(cmdText) > available {
		cmdText = truncate.StringWithTail(cmdText, uint(available), "…")
	}
	left := cmdText

	// Pad command to fill space
	leftWidth := lipgloss.Width(left)
	padding := w - citeWidth - leftWidth - rightWidth
	if padding < 1 {
		padding = 1
	}
	padStr := strings.Repeat(" ", padding)

	line := citeMarker + left + padStr + rightSide

	if selected {
		return selectedRowStyle.Width(w).Render(line)
	}
	return normalRowStyle.Width(w).Render(line)
}
