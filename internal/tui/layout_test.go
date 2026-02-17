package tui

import (
	"testing"

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
