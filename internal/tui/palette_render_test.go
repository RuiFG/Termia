package tui

import (
	"strings"
	"testing"

	"github.com/termia/termia/internal/config"
)

func TestRenderCommandPaletteDoesNotInsertSpacerRows(t *testing.T) {
	app := New(nil, config.DefaultConfig(), nil)
	rendered := app.renderCommandPalette(80)
	lines := strings.Split(rendered, "\n")

	searchIndex := findPaletteContentLine(lines, "Search:")
	suggestedIndex := findPaletteContentLine(lines, "Suggested")
	modelsIndex := findPaletteContentLine(lines, "Models")
	newSessionIndex := findPaletteContentLine(lines, "New Session")
	modeIndex := findPaletteContentLine(lines, "Mode")
	assistantIndex := findPaletteContentLine(lines, "Assistant")

	if searchIndex < 0 || suggestedIndex < 0 || modelsIndex < 0 || newSessionIndex < 0 || modeIndex < 0 || assistantIndex < 0 {
		t.Fatalf("expected palette lines to exist, got %#v", lines)
	}
	if suggestedIndex != searchIndex+1 {
		t.Fatalf("expected Suggested immediately after Search, got search=%d suggested=%d", searchIndex, suggestedIndex)
	}
	if modelsIndex != suggestedIndex+1 {
		t.Fatalf("expected Models immediately after Suggested, got suggested=%d models=%d", suggestedIndex, modelsIndex)
	}
	if modeIndex != newSessionIndex+1 {
		t.Fatalf("expected Mode immediately after New Session, got newSession=%d mode=%d", newSessionIndex, modeIndex)
	}
	if assistantIndex != modeIndex+1 {
		t.Fatalf("expected Assistant immediately after Mode, got mode=%d assistant=%d", modeIndex, assistantIndex)
	}
}

func findPaletteContentLine(lines []string, want string) int {
	for i, line := range lines {
		content := paletteContentText(line)
		if content == want {
			return i
		}
	}
	return -1
}

func paletteContentText(line string) string {
	plain := stripANSICodes(line)
	plain = strings.Trim(plain, "│╭╮╰╯─ ")
	return strings.TrimSpace(plain)
}
