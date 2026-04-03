package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/termia/termia/internal/agent"
)

func TestApprovalInputKeybindings(t *testing.T) {
	input := NewApprovalInput()
	request := agent.HITLRequest{
		Kind:    agent.HITLKindConfirm,
		Command: "echo hi",
	}

	input.SetRequest(request)
	response, _ := input.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if response == nil || !response.Confirmed {
		t.Fatalf("expected approved response, got %#v", response)
	}

	input.SetRequest(request)
	response, _ = input.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if response == nil || response.Confirmed {
		t.Fatalf("expected rejected response on esc, got %#v", response)
	}
}

func TestApprovalInputViewShowsVisibleActions(t *testing.T) {
	input := NewApprovalInput()
	input.SetRequest(agent.HITLRequest{
		Kind:    agent.HITLKindConfirm,
		Title:   "Allow command?",
		Command: "rm -rf tmp",
		Cwd:     "/tmp/project",
		Prompt:  "Please approve or reject the tool call command() by responding with the approval payload.",
	})

	view := stripANSICodes(input.View(120))
	normalized := strings.Join(strings.Fields(view), " ")
	if !strings.Contains(normalized, "Allow command") {
		t.Fatalf("expected compact command approval title, got %q", view)
	}
	lines := strings.Split(view, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected title and command lines, got %q", view)
	}
	if !strings.Contains(lines[1], "rm -rf tmp") || !strings.Contains(lines[1], "/tmp/project") {
		t.Fatalf("expected command and cwd on the same line, got %q", lines[1])
	}
	for _, line := range lines[2:] {
		if strings.TrimSpace(line) == "/tmp/project" {
			t.Fatalf("expected cwd not to render on its own line, got %q", view)
		}
	}
	if !strings.Contains(normalized, "Allow Enter") {
		t.Fatalf("expected allow action rendered, got %q", view)
	}
	if !strings.Contains(normalized, "Reject Esc") {
		t.Fatalf("expected reject action rendered, got %q", view)
	}
	if strings.Contains(normalized, "/ Y") || strings.Contains(normalized, "/ N") {
		t.Fatalf("expected legacy Y/N hints to be removed, got %q", view)
	}
	if strings.Contains(strings.ToLower(normalized), "approve or reject the tool call") {
		t.Fatalf("expected generic approval prompt to be hidden, got %q", view)
	}
}

func TestApprovalInputIgnoresLegacyLetterShortcuts(t *testing.T) {
	input := NewApprovalInput()
	input.SetRequest(agent.HITLRequest{
		Kind:    agent.HITLKindConfirm,
		Command: "echo hi",
	})

	response, _ := input.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if response != nil {
		t.Fatalf("expected n to be ignored, got %#v", response)
	}
	if !input.Active() {
		t.Fatalf("expected approval input to remain active after ignored n shortcut")
	}

	for _, legacy := range []rune{'y', 'r'} {
		input.SetRequest(agent.HITLRequest{
			Kind:    agent.HITLKindConfirm,
			Command: "echo hi",
		})
		response, _ = input.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{legacy}})
		if response != nil {
			t.Fatalf("expected %q to be ignored, got %#v", string(legacy), response)
		}
		if !input.Active() {
			t.Fatalf("expected approval input to remain active after ignored %q shortcut", string(legacy))
		}
	}
}

func TestApprovalInputArrowSelectionControlsEnterResult(t *testing.T) {
	input := NewApprovalInput()
	input.SetRequest(agent.HITLRequest{
		Kind:    agent.HITLKindConfirm,
		Command: "echo hi",
	})

	_, _ = input.Update(tea.KeyMsg{Type: tea.KeyRight})
	response, _ := input.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if response == nil || response.Confirmed {
		t.Fatalf("expected reject when reject choice is focused, got %#v", response)
	}
}
