package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/termia/termia/internal/agent"
)

func TestAskInputSelectionToggleAndConfirm(t *testing.T) {
	input := NewAskInput()
	questions := []agent.AskQuestion{
		{
			Question: "Pick options",
			Options: []agent.AskOption{
				{Title: "Alpha"},
				{Title: "Beta"},
				{Title: "Gamma"},
			},
			Multiple: true,
		},
	}
	input.SetQuestions(questions)

	_, _ = input.Update(tea.KeyMsg{Type: tea.KeyDown})
	_, _ = input.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	if !input.isSelected(1) {
		t.Fatalf("expected option 1 selected")
	}
	_, _ = input.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	if input.isSelected(1) {
		t.Fatalf("expected option 1 deselected")
	}

	answers, _ := input.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if answers == nil {
		t.Fatalf("expected answers after confirm")
	}
	if len(*answers) != 1 {
		t.Fatalf("expected 1 answer, got %d", len(*answers))
	}
	selected := (*answers)[0].Selected
	if len(selected) != 1 || selected[0] != "Beta" {
		t.Fatalf("expected selected [Beta], got %#v", selected)
	}
}

func TestAskInputTypeYourAnswerFlow(t *testing.T) {
	input := NewAskInput()
	questions := []agent.AskQuestion{
		{
			Question: "Provide details",
			Options: []agent.AskOption{
				{Title: "One"},
				{Title: agent.AskTypeYourAnswerTitle},
				{Title: "Two"},
			},
			Multiple: false,
		},
	}
	input.SetQuestions(questions)

	_, _ = input.Update(tea.KeyMsg{Type: tea.KeyDown})
	_, _ = input.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if input.Mode != AskModeCustom {
		t.Fatalf("expected custom mode, got %v", input.Mode)
	}
	input.Custom.SetValue("Custom answer")
	answers, _ := input.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	if answers == nil {
		t.Fatalf("expected answers after custom submit")
	}
	answer := (*answers)[0]
	if !answer.UsedCustom || answer.CustomAnswer != "Custom answer" {
		t.Fatalf("expected custom answer, got %#v", answer)
	}
	if len(answer.Selected) != 1 || answer.Selected[0] != "Custom answer" {
		t.Fatalf("expected selected to contain custom answer, got %#v", answer.Selected)
	}
}
