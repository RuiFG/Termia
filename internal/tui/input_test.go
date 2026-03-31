package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestInputHistoryNavigation(t *testing.T) {
	input := NewInputModel()
	input.AddHistory("first")
	input.AddHistory("second")
	input.SetValue("draft")

	input, _ = input.Update(tea.KeyMsg{Type: tea.KeyUp})
	if got := input.Value(); got != "second" {
		t.Fatalf("expected second, got %q", got)
	}

	input, _ = input.Update(tea.KeyMsg{Type: tea.KeyUp})
	if got := input.Value(); got != "first" {
		t.Fatalf("expected first, got %q", got)
	}

	input, _ = input.Update(tea.KeyMsg{Type: tea.KeyDown})
	if got := input.Value(); got != "second" {
		t.Fatalf("expected second, got %q", got)
	}

	input, _ = input.Update(tea.KeyMsg{Type: tea.KeyDown})
	if got := input.Value(); got != "draft" {
		t.Fatalf("expected draft, got %q", got)
	}
}

func TestInputSetHistory(t *testing.T) {
	input := NewInputModel()
	input.SetHistory([]string{"  one  ", "one", "", "two"})
	input.SetValue("draft")

	input, _ = input.Update(tea.KeyMsg{Type: tea.KeyUp})
	if got := input.Value(); got != "two" {
		t.Fatalf("expected two, got %q", got)
	}

	input, _ = input.Update(tea.KeyMsg{Type: tea.KeyUp})
	if got := input.Value(); got != "one" {
		t.Fatalf("expected one, got %q", got)
	}
}

func TestInputHistoryNavigationRestoresCitedCommands(t *testing.T) {
	input := NewInputModel()
	input.SetHistoryEntries([]InputHistoryEntry{
		{Value: "first", CitedCommandIDs: []string{"cmd-1"}},
		{Value: "second", CitedCommandIDs: []string{"cmd-2", "cmd-3"}},
	})
	input.SetValue("draft")
	input.SetHistoryDraftCitedCommandIDs([]string{"draft-cmd"})

	input, _ = input.Update(tea.KeyMsg{Type: tea.KeyUp})
	if got := input.Value(); got != "second" {
		t.Fatalf("expected second, got %q", got)
	}
	if got := input.CurrentHistoryCitedCommandIDs(); !sameStringSlice(got, []string{"cmd-2", "cmd-3"}) {
		t.Fatalf("expected cited commands for second history entry, got %v", got)
	}

	input, _ = input.Update(tea.KeyMsg{Type: tea.KeyDown})
	if got := input.Value(); got != "draft" {
		t.Fatalf("expected draft, got %q", got)
	}
	if got := input.CurrentHistoryCitedCommandIDs(); !sameStringSlice(got, []string{"draft-cmd"}) {
		t.Fatalf("expected draft cited commands to be restored, got %v", got)
	}
}
