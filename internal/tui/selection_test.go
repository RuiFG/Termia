package tui

import "testing"

func TestTextSelectionClearAllowsSameLinesToBeReset(t *testing.T) {
	var selection textSelection
	lines := []string{"alpha", "beta"}

	selection.SetLines(lines)
	selection.BeginSelection(0, 0)
	selection.UpdateSelection(1, 2)
	selection.Clear()

	selection.SetLines(lines)
	selection.BeginSelection(0, 0)
	selection.UpdateSelection(1, 2)

	highlighted := selection.HighlightLines(10)
	if len(highlighted) != len(lines) {
		t.Fatalf("expected %d highlighted lines, got %d", len(lines), len(highlighted))
	}
}
