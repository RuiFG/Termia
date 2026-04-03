package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/termia/termia/internal/agent"
)

func TestAskInputSelectionToggleAndConfirm(t *testing.T) {
	input := NewAskInput()
	input.SetRequest(agent.HITLRequest{
		Kind: agent.HITLKindInputForm,
		Questions: []agent.AskQuestion{{
			Question: "Pick options",
			Options: []agent.AskOption{
				{Title: "Alpha"},
				{Title: "Beta"},
				{Title: "Gamma"},
			},
			Multiple: true,
		}},
	})

	_, _ = input.Update(tea.KeyMsg{Type: tea.KeyDown})
	_, _ = input.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	if !input.isSelected(1) {
		t.Fatalf("expected option 1 selected")
	}
	_, _ = input.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	if input.isSelected(1) {
		t.Fatalf("expected option 1 deselected")
	}

	response, _ := input.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if response == nil {
		t.Fatalf("expected response after confirm")
	}
	if len(response.Answers) != 1 {
		t.Fatalf("expected 1 answer, got %d", len(response.Answers))
	}
	selected := response.Answers[0].SelectedOptions
	if len(selected) != 1 || selected[0] != "Beta" {
		t.Fatalf("expected selected [Beta], got %#v", selected)
	}
}

func TestAskInputTypeYourAnswerFlow(t *testing.T) {
	input := NewAskInput()
	input.SetRequest(agent.HITLRequest{
		Kind: agent.HITLKindInputForm,
		Questions: []agent.AskQuestion{{
			Question: "Provide details",
			Options: []agent.AskOption{
				{Title: "One"},
				{Title: agent.AskTypeYourAnswerTitle},
			},
		}},
	})

	_, _ = input.Update(tea.KeyMsg{Type: tea.KeyDown})
	_, _ = input.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if input.Mode != AskModeCustom {
		t.Fatalf("expected custom mode, got %v", input.Mode)
	}
	_, _ = input.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("C")})
	if got := input.Custom.Value(); got != "C" {
		t.Fatalf("expected custom input to accept typing, got %q", got)
	}
	_, _ = input.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ustom answer")})
	response, _ := input.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if response != nil {
		t.Fatalf("expected enter in custom mode to only save answer, got %#v", response)
	}
	if input.Mode != AskModeSelect {
		t.Fatalf("expected to return to select mode after saving custom answer, got %v", input.Mode)
	}
	response, _ = input.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if response == nil {
		t.Fatalf("expected response after confirming question")
	}
	answer := response.Answers[0]
	if len(answer.CustomTexts) != 1 || answer.CustomTexts[0] != "Custom answer" {
		t.Fatalf("expected custom answer, got %#v", answer)
	}
	if len(answer.SelectedOptions) != 0 {
		t.Fatalf("expected no predefined selections, got %#v", answer.SelectedOptions)
	}
}

func TestAskInputCustomModeDoesNotRepeatTypeYourAnswerHeading(t *testing.T) {
	input := NewAskInput()
	input.SetRequest(agent.HITLRequest{
		Kind: agent.HITLKindInputForm,
		Questions: []agent.AskQuestion{{
			Question: "Provide details",
			Options: []agent.AskOption{
				{Title: "One"},
				{Title: agent.AskTypeYourAnswerTitle},
			},
		}},
	})

	_, _ = input.Update(tea.KeyMsg{Type: tea.KeyDown})
	_, _ = input.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if input.Mode != AskModeCustom {
		t.Fatalf("expected custom mode, got %v", input.Mode)
	}

	view := stripANSICodes(input.View(80))
	if strings.Contains(view, "\nType your answer\n> ") {
		t.Fatalf("expected redundant custom-mode subtitle to be hidden, got %q", view)
	}
	if !strings.Contains(view, "> ") {
		t.Fatalf("expected custom input prompt in view, got %q", view)
	}
}

func TestAskInputCustomOptionViaSpaceOnlyTogglesSelectionInMultiSelect(t *testing.T) {
	input := NewAskInput()
	input.SetRequest(agent.HITLRequest{
		Kind: agent.HITLKindInputForm,
		Questions: []agent.AskQuestion{{
			Question: "Select outputs",
			Options: []agent.AskOption{
				{Title: "Logs"},
				{Title: agent.AskTypeYourAnswerTitle},
			},
			Multiple: true,
		}},
	})

	_, _ = input.Update(tea.KeyMsg{Type: tea.KeyDown})
	_, _ = input.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	if input.Mode != AskModeSelect {
		t.Fatalf("expected space in multi-select to keep select mode, got %v", input.Mode)
	}
	if !input.isSelected(1) {
		t.Fatalf("expected custom option to be toggled on")
	}
	if got := input.Custom.Value(); got != "" {
		t.Fatalf("expected no custom input draft when only toggling selection, got %q", got)
	}

	_, _ = input.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	if input.isSelected(1) {
		t.Fatalf("expected second space to toggle custom option off")
	}
}

func TestAskInputCustomOptionViaKeySpaceOnlyTogglesSelectionInMultiSelect(t *testing.T) {
	input := NewAskInput()
	input.SetRequest(agent.HITLRequest{
		Kind: agent.HITLKindInputForm,
		Questions: []agent.AskQuestion{{
			Question: "Select outputs",
			Options: []agent.AskOption{
				{Title: "Logs"},
				{Title: agent.AskTypeYourAnswerTitle},
			},
			Multiple: true,
		}},
	})

	_, _ = input.Update(tea.KeyMsg{Type: tea.KeyDown})
	_, _ = input.Update(tea.KeyMsg{Type: tea.KeySpace})
	if input.Mode != AskModeSelect {
		t.Fatalf("expected key space in multi-select to keep select mode, got %v", input.Mode)
	}
	if !input.isSelected(1) {
		t.Fatalf("expected key space to toggle the custom option on")
	}

	_, _ = input.Update(tea.KeyMsg{Type: tea.KeySpace})
	if input.isSelected(1) {
		t.Fatalf("expected second key space to toggle the custom option off")
	}
}

func TestAskInputMultiSelectWithCustom(t *testing.T) {
	input := NewAskInput()
	input.SetRequest(agent.HITLRequest{
		Kind: agent.HITLKindInputForm,
		Questions: []agent.AskQuestion{{
			Question: "Select outputs",
			Options: []agent.AskOption{
				{Title: "Logs"},
				{Title: agent.AskTypeYourAnswerTitle},
			},
			Multiple: true,
		}},
	})

	_, _ = input.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	_, _ = input.Update(tea.KeyMsg{Type: tea.KeyDown})
	_, _ = input.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	if input.Mode != AskModeSelect {
		t.Fatalf("expected custom option selection to stay in select mode until enter, got %v", input.Mode)
	}
	_, _ = input.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if input.Mode != AskModeCustom {
		t.Fatalf("expected custom mode, got %v", input.Mode)
	}
	_, _ = input.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("alerts")})
	response, _ := input.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if response != nil {
		t.Fatalf("expected enter in custom mode to only save answer, got %#v", response)
	}
	if input.Mode != AskModeSelect {
		t.Fatalf("expected to return to select mode after saving custom answer, got %v", input.Mode)
	}
	response, _ = input.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if response == nil {
		t.Fatalf("expected response after confirming question")
	}
	answer := response.Answers[0]
	if len(answer.SelectedOptions) != 1 || answer.SelectedOptions[0] != "Logs" {
		t.Fatalf("expected predefined selection preserved, got %#v", answer.SelectedOptions)
	}
	if len(answer.CustomTexts) != 1 || answer.CustomTexts[0] != "alerts" {
		t.Fatalf("expected custom text recorded, got %#v", answer.CustomTexts)
	}
}

func TestAskInputViewShowsDescriptionsWithoutArrow(t *testing.T) {
	input := NewAskInput()
	input.SetRequest(agent.HITLRequest{
		Kind: agent.HITLKindInputForm,
		Questions: []agent.AskQuestion{{
			Question: "Choose how to continue",
			Options: []agent.AskOption{
				{Title: "Alpha", Description: "first path"},
			},
		}},
	})

	view := input.View(80)
	normalized := strings.Join(strings.Fields(view), " ")
	if !strings.Contains(normalized, "first path") {
		t.Fatalf("expected option description in view, got %q", view)
	}
	if !strings.Contains(normalized, agent.AskTypeYourAnswerTitle) {
		t.Fatalf("expected custom option title in view, got %q", view)
	}
	if !strings.Contains(normalized, agent.AskTypeYourAnswerDesc) {
		t.Fatalf("expected custom option description in view, got %q", view)
	}
	if strings.Contains(view, "> ") {
		t.Fatalf("expected no arrow cursor in ask view, got %q", view)
	}
}

func TestRenderAskOptionRowSelectedTitleUsesDefaultStyle(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(previousProfile)

	lines := renderAskOptionRow(80, "[x]", agent.AskOption{Title: "Logs"}, false, true)
	if len(lines) == 0 {
		t.Fatalf("expected rendered lines")
	}
	if !strings.Contains(lines[0], hitlChoiceTitleStyle.Render("Logs")) {
		t.Fatalf("expected selected option title to keep default style, got %q", lines[0])
	}
	if strings.Contains(lines[0], hitlSelectedStyle.Render("Logs")) {
		t.Fatalf("expected selected option title to avoid selected highlight style, got %q", lines[0])
	}
}

func TestAskInputSupportsQuestionNavigationAndEdit(t *testing.T) {
	input := NewAskInput()
	input.SetRequest(agent.HITLRequest{
		Kind: agent.HITLKindInputForm,
		Questions: []agent.AskQuestion{
			{
				Question: "Pick the first option",
				Options: []agent.AskOption{
					{Title: "Alpha"},
					{Title: "Beta"},
				},
			},
			{
				Question: "Pick the second option",
				Options: []agent.AskOption{
					{Title: "One"},
					{Title: "Two"},
				},
			},
		},
	})

	_, _ = input.Update(tea.KeyMsg{Type: tea.KeyDown})
	_, _ = input.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if input.Index != 1 {
		t.Fatalf("expected to advance to second question, got %d", input.Index)
	}
	_, _ = input.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if input.Index != 0 {
		t.Fatalf("expected to navigate back to first question, got %d", input.Index)
	}
	if input.Cursor != 1 || !input.isSelected(1) {
		t.Fatalf("expected prior selection restored on first question")
	}
	_, _ = input.Update(tea.KeyMsg{Type: tea.KeyUp})
	_, _ = input.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if input.Index != 1 {
		t.Fatalf("expected to return to second question after editing first, got %d", input.Index)
	}
	response, _ := input.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if response == nil {
		t.Fatalf("expected final response on last question")
	}
	if got := response.Answers[0].SelectedOptions[0]; got != "Alpha" {
		t.Fatalf("expected edited answer preserved, got %q", got)
	}
}

func TestAskInputCustomSaveDoesNotAdvanceQuestion(t *testing.T) {
	input := NewAskInput()
	input.SetRequest(agent.HITLRequest{
		Kind: agent.HITLKindInputForm,
		Questions: []agent.AskQuestion{
			{
				Question: "Provide a first answer",
				Options: []agent.AskOption{
					{Title: "Alpha"},
				},
			},
			{
				Question: "Second question",
				Options: []agent.AskOption{
					{Title: "One"},
				},
			},
		},
	})

	_, _ = input.Update(tea.KeyMsg{Type: tea.KeyDown})
	_, _ = input.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_, _ = input.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("custom")})
	response, _ := input.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if response != nil {
		t.Fatalf("expected custom enter to save only, got %#v", response)
	}
	if input.Index != 0 {
		t.Fatalf("expected to stay on the same question after saving custom answer, got %d", input.Index)
	}
	if input.Mode != AskModeSelect {
		t.Fatalf("expected select mode after saving custom answer, got %v", input.Mode)
	}
	if got := input.CustomValue(0); got != "custom" {
		t.Fatalf("expected saved custom answer, got %q", got)
	}
}
