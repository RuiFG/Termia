package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/termia/termia/internal/config"
)

func TestTextSelectionHighlightKeepsANSIAndAddsVisibleFeedback(t *testing.T) {
	var selection textSelection
	plain := []string{"HELLO", "WORLD"}
	rendered := []string{"\x1b[31mHELLO\x1b[0m", "\x1b[34mWORLD\x1b[0m"}

	selection.SetLines(plain)
	selection.SetRenderLines(rendered)
	selection.BeginSelection(0, 0)
	selection.UpdateSelection(1, 3)

	highlighted := selection.HighlightLines(16)
	if len(highlighted) != 2 {
		t.Fatalf("expected 2 highlighted lines, got %d", len(highlighted))
	}
	if strings.Contains(highlighted[0], "\x1b[7m\x1b[31m") {
		t.Fatalf("expected selected text to avoid reusing its original foreground color inside the selection, got %q", highlighted[0])
	}
	if !strings.Contains(highlighted[1], "\x1b[34m") {
		t.Fatalf("expected unselected suffix to keep its original foreground color, got %q", highlighted[1])
	}
	if highlighted[0] == padToWidth(rendered[0], 16) && highlighted[1] == padToWidth(rendered[1], 16) {
		t.Fatalf("expected highlighted output to differ from original render")
	}
}

func TestMouseReleaseKeepsContentSelectionActive(t *testing.T) {
	app := New(nil, config.DefaultConfig(), nil)
	app.width = 120
	app.height = 30
	app.ready = true
	app.layoutPanels()
	app.agent.AddMessage("assistant", "alpha beta gamma")

	innerX, innerY := panelInnerOrigin(contentPaneStyle, app.leftXStart, app.contentYStart)
	model, _ := app.handleMouse(tea.MouseMsg{
		X:      innerX,
		Y:      innerY,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})
	app = model.(App)
	model, cmd := app.handleMouse(tea.MouseMsg{
		X:      innerX + 5,
		Y:      innerY,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionRelease,
	})
	app = model.(App)

	if !app.contentSelection.HasSelection() {
		t.Fatalf("expected content selection to persist after mouse release")
	}
	if cmd != nil {
		t.Fatalf("expected mouse release to keep selection without auto-copy command")
	}
}
