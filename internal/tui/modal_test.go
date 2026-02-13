package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/termia/termia/internal/config"
)

func TestModalSelectionShiftArrows(t *testing.T) {
	m := NewModalModel()
	m.SetSize(20, 6)
	m.Open("cmd")
	m.SetContent("abcd")

	m.HandleKey(tea.KeyShiftRight)
	if got := m.SelectedText(); got != "a" {
		t.Fatalf("expected selection 'a', got %q", got)
	}

	m.HandleKey(tea.KeyShiftRight)
	if got := m.SelectedText(); got != "ab" {
		t.Fatalf("expected selection 'ab', got %q", got)
	}
}

func TestModalMouseSelection(t *testing.T) {
	m := NewModalModel()
	m.SetSize(20, 6)
	m.Open("cmd")
	m.SetContent("hello\nworld")

	m.BeginSelection(0, 0)
	m.UpdateSelection(1, 3)
	m.EndSelection()

	if got := m.SelectedText(); got != "hello\nwor" {
		t.Fatalf("expected selection 'hello\\nwor', got %q", got)
	}
}

func TestModalCtrlCCopiesNoQuit(t *testing.T) {
	app := New(nil, config.DefaultConfig(), nil)
	app.width = 80
	app.height = 24
	app.ready = true
	app.layoutPanels()
	app.modal.Open("cmd")
	app.modal.SetContent("abcd")
	app.modal.HandleKey(tea.KeyShiftRight)

	model, cmd := app.handleModalKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatalf("expected copy command")
	}
	appOut := model.(App)
	if !appOut.modal.IsOpen() {
		t.Fatalf("expected modal to remain open")
	}
	if appOut.modal.SelectedText() != "" {
		t.Fatalf("expected selection to be cleared after copy")
	}
}
