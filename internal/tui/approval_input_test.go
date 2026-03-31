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

	input.SetRequest(request)
	response, _ = input.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if response == nil || response.Confirmed {
		t.Fatalf("expected rejected response on r, got %#v", response)
	}
}

func TestApprovalInputViewShowsVisibleActions(t *testing.T) {
	input := NewApprovalInput()
	input.SetRequest(agent.HITLRequest{
		Kind:    agent.HITLKindConfirm,
		Title:   "Allow command?",
		Command: "rm -rf tmp",
		Prompt:  "Please approve or reject the tool call command() by responding with the approval payload.",
	})

	view := input.View(80)
	normalized := strings.Join(strings.Fields(view), " ")
	if !strings.Contains(normalized, "Allow command") {
		t.Fatalf("expected compact command approval title, got %q", view)
	}
	if !strings.Contains(normalized, "Allow Enter / Y") {
		t.Fatalf("expected allow action rendered, got %q", view)
	}
	if !strings.Contains(normalized, "Reject Esc / N") {
		t.Fatalf("expected reject action rendered, got %q", view)
	}
	if strings.Contains(strings.ToLower(normalized), "approve or reject the tool call") {
		t.Fatalf("expected generic approval prompt to be hidden, got %q", view)
	}
}

func TestApprovalInputRejectShortcuts(t *testing.T) {
	input := NewApprovalInput()
	input.SetRequest(agent.HITLRequest{
		Kind:    agent.HITLKindConfirm,
		Command: "echo hi",
	})

	response, _ := input.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if response == nil || response.Confirmed {
		t.Fatalf("expected reject on n, got %#v", response)
	}
	payload, ok := response.Payload.(map[string]any)
	if !ok || payload["status"] != "rejected" {
		t.Fatalf("expected reject payload, got %#v", response)
	}

	input.SetRequest(agent.HITLRequest{
		Kind:    agent.HITLKindConfirm,
		Command: "echo hi",
	})
	response, _ = input.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if response == nil || !response.Confirmed {
		t.Fatalf("expected allow on y, got %#v", response)
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
