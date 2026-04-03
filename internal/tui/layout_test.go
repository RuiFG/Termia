package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/termia/termia/internal/agent"
	"github.com/termia/termia/internal/config"
)

func TestLayoutTwoColumnSizing(t *testing.T) {
	app := New(nil, config.DefaultConfig(), nil)
	app.width = 120
	app.height = 40
	app.ready = true
	app.layoutPanels()

	if !app.twoColumn {
		t.Fatalf("expected twoColumn layout")
	}
	containerFW, _ := containerStyle.GetFrameSize()
	innerW := app.width - containerFW
	leftExpected := innerW * 5 / 8
	rightExpected := innerW - leftExpected
	if app.leftWidth != leftExpected {
		t.Fatalf("expected left width %d, got %d", leftExpected, app.leftWidth)
	}
	if app.rightWidth != rightExpected {
		t.Fatalf("expected right width %d, got %d", rightExpected, app.rightWidth)
	}
}

func TestLayoutUsesFullScreenOriginWithoutOuterContainerBorder(t *testing.T) {
	app := New(nil, config.DefaultConfig(), nil)
	app.width = 120
	app.height = 40
	app.ready = true
	app.layoutPanels()

	x, y := containerInnerOrigin(containerStyle)
	if app.leftXStart != x {
		t.Fatalf("expected leftXStart %d, got %d", x, app.leftXStart)
	}
	if app.contentYStart != y {
		t.Fatalf("expected contentYStart %d, got %d", y, app.contentYStart)
	}
	if frameW, frameH := containerStyle.GetFrameSize(); frameW != 0 || frameH != 0 {
		t.Fatalf("expected outer container frame to be removed, got frame %dx%d", frameW, frameH)
	}
}

func TestInputHeightAlwaysBlankLine(t *testing.T) {
	app := New(nil, config.DefaultConfig(), nil)
	app.width = 120
	app.height = 40
	app.ready = true

	app.input.SetValue("hello")
	app.layoutPanels()
	inputLines := InputLineCount(app.input)
	expected := inputLines + 2 + app.inputCwdLineCount()
	if app.inputHeight != expected {
		t.Fatalf("expected input height %d, got %d", expected, app.inputHeight)
	}

	app.input.SetValue("line1\nline2")
	app.layoutPanels()
	inputLines = InputLineCount(app.input)
	expected = inputLines + 2 + app.inputCwdLineCount()
	if app.inputHeight != expected {
		t.Fatalf("expected input height %d, got %d", expected, app.inputHeight)
	}
}

func TestAskRequestRelayoutsImmediately(t *testing.T) {
	app := New(nil, config.DefaultConfig(), nil)
	app.width = 120
	app.height = 40
	app.ready = true
	app.layoutPanels()
	before := app.inputHeight

	model, _ := app.Update(askRequestMsg{
		request: askRequest{
			request: agent.HITLRequest{
				Kind: agent.HITLKindInputForm,
				Questions: []agent.AskQuestion{{
					Question: "This is a much longer question that should require more space in the input area immediately.",
					Options: []agent.AskOption{
						{Title: "One"},
						{Title: "Two"},
						{Title: agent.AskTypeYourAnswerTitle},
					},
				}},
			},
			response: make(chan agent.HITLResponse, 1),
		},
	})
	updated := model.(App)
	if updated.inputHeight <= before {
		t.Fatalf("expected input height to grow immediately, got before=%d after=%d", before, updated.inputHeight)
	}
}

func TestAskLayoutUsesPanelWidthAndCanGrowBeyondDefaultInputLimit(t *testing.T) {
	app := New(nil, config.DefaultConfig(), nil)
	app.width = 120
	app.height = 50
	app.ready = true
	app.layoutPanels()

	app.askInput.SetRequest(agent.HITLRequest{
		Kind: agent.HITLKindInputForm,
		Questions: []agent.AskQuestion{{
			Question: "Choose the next action for this session.",
			Options: []agent.AskOption{
				{Title: "Inspect logs", Description: "Read the recent service logs before changing anything."},
				{Title: "Restart service", Description: "Restart the service and watch the startup health checks."},
				{Title: "Collect metrics", Description: "Capture CPU, memory, and file descriptor usage first."},
				{Title: "Open config", Description: "Review the active configuration file before applying changes."},
			},
		}},
	})
	app.layoutPanels()

	expected := countLines(app.askInput.View(app.leftContentW)) + 2 + app.inputCwdLineCount()
	if app.inputHeight < expected {
		t.Fatalf("expected ask input height >= %d using left panel width, got %d", expected, app.inputHeight)
	}
	if app.inputHeight <= maxInputLines+2 {
		t.Fatalf("expected ask input to grow beyond normal input limit, got %d", app.inputHeight)
	}
}

func TestAskInputKeyRelayoutsWhenEnteringCustomMode(t *testing.T) {
	app := New(nil, config.DefaultConfig(), nil)
	app.width = 120
	app.height = 40
	app.ready = true
	app.layoutPanels()

	model, _ := app.Update(askRequestMsg{
		request: askRequest{
			request: agent.HITLRequest{
				Kind: agent.HITLKindInputForm,
				Questions: []agent.AskQuestion{{
					Question: "Choose one action",
					Options: []agent.AskOption{
						{Title: "Inspect logs", Description: "Read the most recent service logs."},
					},
				}},
			},
			response: make(chan agent.HITLResponse, 1),
		},
	})
	updated := model.(App)
	before := updated.inputHeight

	model, _ = updated.handleInputKey(tea.KeyMsg{Type: tea.KeyDown})
	updated = model.(App)
	model, _ = updated.handleInputKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	updated = model.(App)

	if updated.askInput.Mode != AskModeCustom {
		t.Fatalf("expected custom mode after selecting the last option, got %v", updated.askInput.Mode)
	}
	if updated.inputHeight <= before {
		t.Fatalf("expected relayout after entering custom mode, got before=%d after=%d", before, updated.inputHeight)
	}
}

func TestAskInputKeySpaceInMultiSelectKeepsSelectMode(t *testing.T) {
	app := New(nil, config.DefaultConfig(), nil)
	app.width = 120
	app.height = 40
	app.ready = true
	app.layoutPanels()

	model, _ := app.Update(askRequestMsg{
		request: askRequest{
			request: agent.HITLRequest{
				Kind: agent.HITLKindInputForm,
				Questions: []agent.AskQuestion{{
					Question: "Choose outputs",
					Options: []agent.AskOption{
						{Title: "Logs"},
					},
					Multiple: true,
				}},
			},
			response: make(chan agent.HITLResponse, 1),
		},
	})
	updated := model.(App)

	model, _ = updated.handleInputKey(tea.KeyMsg{Type: tea.KeyDown})
	updated = model.(App)
	model, _ = updated.handleInputKey(tea.KeyMsg{Type: tea.KeySpace})
	updated = model.(App)

	if updated.askInput.Mode != AskModeSelect {
		t.Fatalf("expected key space in multi-select to keep select mode, got %v", updated.askInput.Mode)
	}
	if !updated.askInput.isSelected(1) {
		t.Fatalf("expected key space to toggle the custom option on")
	}
}

func TestApprovalResponseDoesNotSetStatusMessage(t *testing.T) {
	app := New(nil, config.DefaultConfig(), nil)
	app.pendingPromptID = "prompt-1"
	app.pendingPromptSessionID = "session-1"
	app.approvalInput.SetRequest(agent.HITLRequest{
		Kind:    agent.HITLKindConfirm,
		Command: "rm -rf tmp",
	})

	model, _ := app.handleApprovalResponse(agent.HITLResponse{Confirmed: false})
	updated := model.(App)
	if updated.statusMsg != "" {
		t.Fatalf("expected approval response to leave status bar empty, got %q", updated.statusMsg)
	}
}
