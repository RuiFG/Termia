package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/termia/termia/internal/db"
)

// PreviewModel shows command output with scrollable viewport.
type PreviewModel struct {
	viewport viewport.Model
	command  *db.Command
	content  string
	width    int
	height   int
	ready    bool
	loading  bool
	keys     KeyMap
}

// NewPreviewModel creates a new preview panel.
func NewPreviewModel(keys KeyMap) PreviewModel {
	return PreviewModel{
		keys: keys,
	}
}

// SetSize updates the preview panel dimensions.
func (m *PreviewModel) SetSize(w, h int) {
	m.width = w
	contentHeight := h - 4
	if contentHeight < 1 {
		contentHeight = 1
	}
	m.height = h
	if !m.ready {
		m.viewport = viewport.New(w, contentHeight)
		m.viewport.MouseWheelEnabled = true
		m.viewport.MouseWheelDelta = 3
		m.ready = true
	} else {
		m.viewport.Width = w
		m.viewport.Height = contentHeight
	}
	if m.content != "" {
		m.viewport.SetContent(m.content)
	}
}

// SetCommand sets the command to preview.
func (m *PreviewModel) SetCommand(cmd *db.Command) {
	m.command = cmd
	m.loading = true
	m.content = ""
	if m.ready {
		m.viewport.SetContent("")
		m.viewport.GotoTop()
	}
}

// CommandID returns the ID of the currently previewed command, or empty string.
func (m PreviewModel) CommandID() string {
	if m.command == nil {
		return ""
	}
	return m.command.ID
}

// SetContent sets the output content for the preview.
func (m *PreviewModel) SetContent(content string) {
	m.content = content
	m.loading = false
	if m.ready {
		m.viewport.SetContent(content)
		m.viewport.GotoTop()
	}
}

// ClearContent resets the preview to empty state.
func (m *PreviewModel) ClearContent() {
	m.content = ""
	m.command = nil
	m.loading = false
	if m.ready {
		m.viewport.SetContent("")
		m.viewport.GotoTop()
	}
}

// Update handles key events for the preview.
func (m PreviewModel) Update(msg tea.Msg) (PreviewModel, tea.Cmd) {
	if !m.ready {
		return m, nil
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// View renders the preview panel.
func (m PreviewModel) View() string {
	if !m.ready {
		return ""
	}

	header := m.renderHeader()
	content := m.viewport.View()

	joined := lipgloss.JoinVertical(lipgloss.Left, header, content)
	// Parent renderContent() clamps to exact height; no need to double-clamp here.
	return lipgloss.NewStyle().Width(m.width).Height(m.height).Render(joined)
}

// renderHeader renders the command info header.
func (m PreviewModel) renderHeader() string {
	if m.command == nil {
		return previewHeaderStyle.Width(m.width).Render("No command selected")
	}

	cmd := m.command
	parts := []string{lipgloss.NewStyle().Bold(true).Foreground(colorOnSurface).Render(cmd.Command)}

	if cmd.Cwd != "" {
		parts = append(parts, metadataLabelStyle.Render("CWD:")+" "+cwdStyle.Render(cmd.Cwd))
	}

	exitStr := "?"
	if cmd.ExitCode != nil {
		if *cmd.ExitCode == 0 {
			exitStr = exitOKStyle.Render("0")
		} else {
			exitStr = exitErrStyle.Render(fmt.Sprintf("%d", *cmd.ExitCode))
		}
	}
	parts = append(parts, metadataLabelStyle.Render("EXIT:")+" "+exitStr)

	dur := formatDuration(cmd.DurationMs)
	if dur != "" {
		parts = append(parts, metadataLabelStyle.Render("DUR:")+" "+metaStyle.Render(dur))
	}

	if m.loading {
		parts = append(parts, loadingStyle.Render("loading..."))
	} else {
		scrollPct := fmt.Sprintf("%.0f%%", m.viewport.ScrollPercent()*100)
		parts = append(parts, metadataLabelStyle.Render("POS:")+" "+metaStyle.Render(scrollPct))
	}

	header := strings.Join(parts, "   ")
	// Truncate to fit width — prevent line wrapping that breaks layout
	if lipgloss.Width(header) > m.width-2 { // -2 for padding
		runes := []rune(header)
		maxW := m.width - 3 // -2 padding, -1 for ellipsis
		if maxW < 0 {
			maxW = 0
		}
		if len(runes) > maxW {
			header = string(runes[:maxW]) + "…"
		}
	}
	return previewHeaderStyle.Width(m.width).Render(header)
}

func exitCodeText(exitCode *int) string {
	if exitCode == nil {
		return "?"
	}
	return fmt.Sprintf("%d", *exitCode)
}
