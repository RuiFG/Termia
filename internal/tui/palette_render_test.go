package tui

import (
	"strconv"
	"strings"
	"testing"

	"github.com/termia/termia/internal/config"
	"github.com/termia/termia/internal/llm"
)

func TestRenderCommandPaletteDoesNotInsertSpacerRows(t *testing.T) {
	app := New(nil, config.DefaultConfig(), nil)
	rendered := app.renderCommandPalette(80, 24)
	lines := strings.Split(rendered, "\n")

	searchIndex := findPaletteContentLine(lines, "Search:")
	suggestedIndex := findPaletteContentLine(lines, "Suggested")
	providersIndex := findPaletteContentLine(lines, "Providers")
	modelsIndex := findPaletteContentLine(lines, "Models")
	newSessionIndex := findPaletteContentLine(lines, "New Session")
	modeIndex := findPaletteContentLine(lines, "Mode")
	assistantIndex := findPaletteContentLine(lines, "Assistant")

	if searchIndex < 0 || suggestedIndex < 0 || providersIndex < 0 || modelsIndex < 0 || newSessionIndex < 0 || modeIndex < 0 || assistantIndex < 0 {
		t.Fatalf("expected palette lines to exist, got %#v", lines)
	}
	if suggestedIndex != searchIndex+1 {
		t.Fatalf("expected Suggested immediately after Search, got search=%d suggested=%d", searchIndex, suggestedIndex)
	}
	if providersIndex != suggestedIndex+1 {
		t.Fatalf("expected Providers immediately after Suggested, got suggested=%d providers=%d", suggestedIndex, providersIndex)
	}
	if modelsIndex != providersIndex+1 {
		t.Fatalf("expected Models immediately after Providers, got providers=%d models=%d", providersIndex, modelsIndex)
	}
	if modeIndex != newSessionIndex+1 {
		t.Fatalf("expected Mode immediately after New Session, got newSession=%d mode=%d", newSessionIndex, modeIndex)
	}
	if assistantIndex != modeIndex+1 {
		t.Fatalf("expected Assistant immediately after Mode, got mode=%d assistant=%d", modeIndex, assistantIndex)
	}
}

func TestPaletteSelectionScrollsWithModelSelection(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.LLM.OpenAI.APIKey = "sk-test"
	app := New(nil, cfg, nil)
	app.height = 14
	app.paletteOpen = true
	app.paletteStage = paletteStageModels
	models := make([]llm.ModelDescriptor, 0, 12)
	for i := 1; i <= 12; i++ {
		models = append(models, llm.ModelDescriptor{
			ID:          "model-" + strconv.Itoa(i),
			DisplayName: "model-" + strconv.Itoa(i),
			Provider:    config.ProviderOpenAI,
		})
	}
	app.providerModels[config.ProviderOpenAI] = models
	app.resetPaletteIndex()

	for i := 0; i < 9; i++ {
		app.movePaletteSelection(1)
	}

	if app.paletteScroll == 0 {
		t.Fatalf("expected palette scroll to advance, got %d", app.paletteScroll)
	}
	rendered := app.renderCommandPalette(80, 12)
	if !strings.Contains(stripANSICodes(rendered), "model-10") {
		t.Fatalf("expected scrolled palette to include selected model, got:\n%s", stripANSICodes(rendered))
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
