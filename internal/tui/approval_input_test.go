package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/termia/termia/internal/agent"
)

func TestApprovalInputKeybindings(t *testing.T) {
	input := NewApprovalInput()
	prompt := agent.ApprovalPrompt{Command: "  echo hi  "}

	input.SetPrompt(prompt)
	decision, _ := input.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if decision == nil || decision.Type != agent.ApprovalDecisionApprove {
		t.Fatalf("expected approve decision, got %#v", decision)
	}

	input.SetPrompt(prompt)
	_, _ = input.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if input.Mode != ApprovalModeEdit {
		t.Fatalf("expected edit mode, got %v", input.Mode)
	}
	if got := input.editInput.Value(); got != "echo hi" {
		t.Fatalf("expected edit value %q, got %q", "echo hi", got)
	}
	input.editInput.SetValue("  edited  ")
	_, _ = input.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if input.Mode != ApprovalModeConfirmEdit {
		t.Fatalf("expected confirm edit mode, got %v", input.Mode)
	}
	decision, _ = input.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if decision == nil || decision.Type != agent.ApprovalDecisionEdit || decision.Command != "edited" {
		t.Fatalf("expected edit decision with command, got %#v", decision)
	}

	input.SetPrompt(prompt)
	decision, _ = input.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if decision == nil || decision.Type != agent.ApprovalDecisionReject {
		t.Fatalf("expected reject decision, got %#v", decision)
	}

	input.SetPrompt(prompt)
	_, _ = input.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if input.Mode != ApprovalModeRephrase {
		t.Fatalf("expected rephrase mode, got %v", input.Mode)
	}
	input.rephrase.SetValue("do it")
	decision, _ = input.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	if decision == nil || decision.Type != agent.ApprovalDecisionRephrase || decision.Rephrase != "do it" {
		t.Fatalf("expected rephrase decision, got %#v", decision)
	}
}
