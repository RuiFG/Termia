package agent

import (
	"bufio"
	"strings"
	"testing"
)

func TestNormalizeAskQuestionAlwaysAppendsCustomOptionLast(t *testing.T) {
	question, err := NormalizeAskQuestion(AskQuestion{
		Question: "Choose a path",
		Options: []AskOption{
			{Title: "Alpha", Description: "first"},
			{Title: "Other", Description: "should be removed"},
			{Title: AskTypeYourAnswerTitle},
			{Title: "Beta", Description: "second"},
		},
	})
	if err != nil {
		t.Fatalf("expected normalized question, got error: %v", err)
	}
	if got := len(question.Options); got != 3 {
		t.Fatalf("expected 3 options after normalization, got %d", got)
	}
	last := question.Options[len(question.Options)-1]
	if last.Title != AskTypeYourAnswerTitle {
		t.Fatalf("expected last option %q, got %q", AskTypeYourAnswerTitle, last.Title)
	}
	if last.Description != AskTypeYourAnswerDesc {
		t.Fatalf("expected custom description %q, got %q", AskTypeYourAnswerDesc, last.Description)
	}
	if question.Options[1].Title != "Beta" {
		t.Fatalf("expected non-custom options preserved before custom option, got %#v", question.Options)
	}
}

func TestBuildCLIConfirmTextUsesCompactCommandApprovalCopy(t *testing.T) {
	text := buildCLIConfirmText(HITLRequest{
		Title:        "Confirmation Required",
		Prompt:       "Please approve or reject the tool call command() by responding with a FunctionResponse with an expected ToolConfirmation payload.",
		OriginalTool: "command",
		Command:      "pwd",
		Cwd:          "/tmp/project",
	})
	if !strings.Contains(text, "Allow command") {
		t.Fatalf("expected compact approval title, got %q", text)
	}
	if !strings.Contains(text, "pwd") {
		t.Fatalf("expected command body, got %q", text)
	}
	if strings.Contains(strings.ToLower(text), "approve or reject the tool call") {
		t.Fatalf("expected generic prompt to be filtered, got %q", text)
	}
	if strings.Contains(text, "Tool: command") {
		t.Fatalf("expected command tool label to be omitted, got %q", text)
	}
	if strings.Contains(text, "/tmp/project") {
		t.Fatalf("expected command confirmation to stay inline, got %q", text)
	}
	if !strings.Contains(text, "(y/n): ") {
		t.Fatalf("expected inline y/n hint, got %q", text)
	}
}

func TestCLIAskStateBuildsSingleSelectCustomAnswer(t *testing.T) {
	state := newCLIAskState(AskQuestion{
		ID:       "q1",
		Question: "Choose one",
		Options: []AskOption{
			{Title: "Alpha"},
			{Title: AskTypeYourAnswerTitle},
		},
	})
	state.cursor = 1
	state.customSelected = true
	state.customTexts = []string{"custom"}

	answer := state.buildAnswer()
	if len(answer.SelectedOptions) != 0 {
		t.Fatalf("expected no predefined selections, got %#v", answer)
	}
	if len(answer.CustomTexts) != 1 || answer.CustomTexts[0] != "custom" {
		t.Fatalf("expected saved custom answer, got %#v", answer)
	}
}

func TestCLIAskStateBuildsMultiSelectAnswer(t *testing.T) {
	state := newCLIAskState(AskQuestion{
		ID:       "q2",
		Question: "Choose many",
		Multiple: true,
		Options: []AskOption{
			{Title: "Alpha"},
			{Title: "Beta"},
			{Title: AskTypeYourAnswerTitle},
		},
	})
	state.selected[0] = true
	state.selected[1] = true
	state.customSelected = true
	state.customTexts = []string{"gamma"}

	answer := state.buildAnswer()
	if got := strings.Join(answer.SelectedOptions, ","); got != "Alpha,Beta" {
		t.Fatalf("expected predefined selections, got %#v", answer)
	}
	if len(answer.CustomTexts) != 1 || answer.CustomTexts[0] != "gamma" {
		t.Fatalf("expected custom text in multi answer, got %#v", answer)
	}
}

func TestRenderCLIBlockForRawTerminalUsesCRLF(t *testing.T) {
	got := renderCLIBlockForRawTerminal("\nHeader\nLine")
	if strings.Contains(strings.ReplaceAll(got, "\r\n", ""), "\n") {
		t.Fatalf("expected raw terminal rendering to convert bare LF, got %q", got)
	}
	if !strings.Contains(got, "\r\nHeader\r\nLine") {
		t.Fatalf("expected CRLF rendering, got %q", got)
	}
}

func TestRenderCLIInteractiveQuestionHidesSubmitHintOnCustomMultiSelect(t *testing.T) {
	state := newCLIAskState(AskQuestion{
		Header:   "Question",
		Question: "Choose many",
		Multiple: true,
		Options: []AskOption{
			{Title: "Alpha"},
			{Title: AskTypeYourAnswerTitle, Description: AskTypeYourAnswerDesc},
		},
	})
	state.cursor = 1

	rendered := renderCLIInteractiveQuestion(state)
	if !strings.Contains(rendered, "Enter submit") {
		t.Fatalf("expected custom-focused multi-select hint to keep submit on enter, got %q", rendered)
	}
	if !strings.Contains(rendered, "Tab edit custom") {
		t.Fatalf("expected custom-focused multi-select hint to show tab edit action, got %q", rendered)
	}
}

func TestReadCLIKeyDecodesUTF8Runes(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("你"))
	key, value, err := readCLIKey(reader)
	if err != nil {
		t.Fatalf("expected rune decode, got error: %v", err)
	}
	if key != cliKeyRune {
		t.Fatalf("expected rune key, got %v", key)
	}
	if value != '你' {
		t.Fatalf("expected UTF-8 rune, got %q", value)
	}
}

func TestRenderCLIInteractiveQuestionShowsSingleLineCustomEditor(t *testing.T) {
	state := newCLIAskState(AskQuestion{
		Header:   "Question",
		Question: "Choose many",
		Multiple: true,
		Options: []AskOption{
			{Title: "Alpha"},
			{Title: AskTypeYourAnswerTitle, Description: AskTypeYourAnswerDesc},
		},
	})
	state.cursor = 1
	state.customActive = true
	state.customBuffer = []rune("大象")

	rendered := renderCLIInteractiveQuestion(state)
	if strings.Contains(rendered, "Enter to save  Esc to cancel") {
		t.Fatalf("expected custom editor hint to stay inline, got %q", rendered)
	}
	if !strings.Contains(rendered, "Type your answer ") || !strings.Contains(rendered, "(Enter save, Esc cancel)") || !strings.Contains(rendered, ": 大象") {
		t.Fatalf("expected inline custom editor content, got %q", rendered)
	}
}

func TestToggleCurrentSelectionSupportsCustomOption(t *testing.T) {
	state := newCLIAskState(AskQuestion{
		Multiple: true,
		Options: []AskOption{
			{Title: "Alpha"},
			{Title: AskTypeYourAnswerTitle},
		},
	})
	state.cursor = 1
	state.customTexts = []string{"gamma"}

	state.toggleCurrentSelection()
	if !state.customSelected {
		t.Fatalf("expected custom option to become selected")
	}
	state.toggleCurrentSelection()
	if state.customSelected {
		t.Fatalf("expected custom option to toggle off")
	}
}

func TestRenderCLIInteractiveQuestionShowsSubmitHintForSelectedCustomMultiSelect(t *testing.T) {
	state := newCLIAskState(AskQuestion{
		Header:   "Question",
		Question: "Choose many",
		Multiple: true,
		Options: []AskOption{
			{Title: "Alpha"},
			{Title: AskTypeYourAnswerTitle, Description: AskTypeYourAnswerDesc},
		},
	})
	state.cursor = 1
	state.customTexts = []string{"gamma"}
	state.customSelected = true

	rendered := renderCLIInteractiveQuestion(state)
	if !strings.Contains(rendered, "Enter submit") {
		t.Fatalf("expected selected custom option to allow submit, got %q", rendered)
	}
	if !strings.Contains(rendered, "Tab edit custom") {
		t.Fatalf("expected selected custom option to keep edit hint on tab, got %q", rendered)
	}
}

func TestCLIAskStateBuildAnswerIgnoresUnselectedSingleCustom(t *testing.T) {
	state := newCLIAskState(AskQuestion{
		ID:       "q3",
		Question: "Choose one",
		Options: []AskOption{
			{Title: "Alpha"},
			{Title: AskTypeYourAnswerTitle},
		},
	})
	state.customTexts = []string{"custom"}

	answer := state.buildAnswer()
	if len(answer.CustomTexts) != 0 {
		t.Fatalf("expected unselected single custom answer to be ignored, got %#v", answer)
	}
}

func TestReadCLIKeyRecognizesTab(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("\t"))
	key, _, err := readCLIKey(reader)
	if err != nil {
		t.Fatalf("expected tab decode, got error: %v", err)
	}
	if key != cliKeyTab {
		t.Fatalf("expected tab key, got %v", key)
	}
}
