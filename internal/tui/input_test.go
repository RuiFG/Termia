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
