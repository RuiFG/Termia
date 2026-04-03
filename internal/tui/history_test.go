package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/termia/termia/internal/config"
	"github.com/termia/termia/internal/db"
)

func TestHistoryModelRenderRowUsesCheckMarkerForCitedCommand(t *testing.T) {
	history := NewHistoryModel(DefaultKeyMap())
	history.SetSize(60, 6)
	history.SetCommands([]db.Command{{ID: "cmd-1", Command: "echo hello"}})
	history.SetCitedCommandIDs([]string{"cmd-1"})

	view := stripANSICodes(history.View())
	if !strings.Contains(view, "✓ echo hello") {
		t.Fatalf("expected cited command to render with check marker, got %q", view)
	}
}

func TestHistoryModelRenderRowFallsBackToEmptyForControlOnlyCommand(t *testing.T) {
	history := NewHistoryModel(DefaultKeyMap())
	history.SetSize(60, 6)
	history.SetCommands([]db.Command{{ID: "cmd-1", Command: "\x00\r\n\t"}})

	view := stripANSICodes(history.View())
	lines := strings.Split(view, "\n")
	if len(lines) == 0 {
		t.Fatalf("expected history view to contain at least one row, got %q", view)
	}
	if !strings.Contains(lines[0], "(empty)") {
		t.Fatalf("expected control-only command to render as (empty), got %q", lines[0])
	}
	if strings.ContainsAny(lines[0], "\x00\r\t") {
		t.Fatalf("expected control characters to be stripped from the history row, got %q", lines[0])
	}
}

func TestHistoryMouseWheelMovesSelection(t *testing.T) {
	app := New(nil, config.DefaultConfig(), nil)
	app.width = 120
	app.height = 40
	app.ready = true
	app.layoutPanels()

	commands := make([]db.Command, 6)
	for i := range commands {
		commands[i] = db.Command{ID: generateID(), Command: "cmd"}
	}
	app.history.SetCommands(commands)

	x := app.leftXStart + 1
	if app.rightWidth > 0 {
		x = app.rightXStart + 1
	}
	y := app.historyYStart + 1

	model, _ := app.handleMouse(tea.MouseMsg{
		X:      x,
		Y:      y,
		Button: tea.MouseButtonWheelDown,
		Action: tea.MouseActionPress,
	})
	updated := model.(App)
	if updated.history.SelectedIndex() == 0 {
		t.Fatalf("expected mouse wheel to move history selection")
	}
}

func TestContentMouseWheelScrollsAgentPanel(t *testing.T) {
	app := New(nil, config.DefaultConfig(), nil)
	app.width = 120
	app.height = 20
	app.ready = true
	app.layoutPanels()

	for i := 0; i < 40; i++ {
		app.agent.AddMessage("assistant", strings.Repeat("line ", 20))
	}
	before := app.agent.viewport.YOffset
	if before == 0 {
		t.Fatalf("expected agent viewport to overflow before scrolling")
	}

	innerX, innerY := panelInnerOrigin(contentPaneStyle, app.leftXStart, app.contentYStart)
	model, _ := app.handleMouse(tea.MouseMsg{
		X:      innerX,
		Y:      innerY,
		Button: tea.MouseButtonWheelUp,
		Action: tea.MouseActionPress,
	})
	updated := model.(App)
	if updated.agent.viewport.YOffset >= before {
		t.Fatalf("expected mouse wheel to scroll agent content up, got before=%d after=%d", before, updated.agent.viewport.YOffset)
	}
}
