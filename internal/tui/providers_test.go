package tui

import (
	"testing"

	"github.com/termia/termia/internal/config"
	"github.com/termia/termia/internal/llm"
)

func TestProviderPaletteSectionsIncludeCustomProviderCreateAction(t *testing.T) {
	app := New(nil, config.DefaultConfig(), nil)
	sections := app.providerPaletteSections()

	createItems := sectionItemsByLabel(sections, "Create")
	if len(createItems) != 1 {
		t.Fatalf("expected create section to contain one item, got %#v", createItems)
	}
	if createItems[0].Action != paletteActionCreateProvider || createItems[0].Label != "OpenAI Compatible" {
		t.Fatalf("expected OpenAI Compatible create action, got %#v", createItems[0])
	}
}

func TestProviderDetailSectionsOnlyShowCredentialManagement(t *testing.T) {
	cfg := config.DefaultConfig()
	custom, err := cfg.LLM.AddCustomProvider("My Gateway", config.ProviderOpenAICompatible, config.LLMProviderConfig{
		APIKey:  "sk-test",
		BaseURL: "https://example.com/v1",
	})
	if err != nil {
		t.Fatalf("add custom provider: %v", err)
	}
	app := New(nil, cfg, nil)
	app.activePaletteProvider = custom.ID

	sections := app.providerDetailSections()
	if sectionItemsByLabel(sections, "Models") != nil {
		t.Fatalf("expected provider detail to exclude models section, got %#v", sections)
	}
	credentials := sectionItemsByLabel(sections, "Credentials")
	if len(credentials) != 2 {
		t.Fatalf("expected two credential items for custom provider, got %#v", credentials)
	}
	actions := sectionItemsByLabel(sections, "Actions")
	if len(actions) < 1 || actions[len(actions)-1].Action != paletteActionDeleteProvider {
		t.Fatalf("expected delete action for custom provider, got %#v", actions)
	}
}

func TestModelPaletteSectionsGroupModelsByConfiguredProvider(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.LLM.OpenAI.APIKey = "sk-openai"
	custom, err := cfg.LLM.AddCustomProvider("Gateway", config.ProviderOpenAICompatible, config.LLMProviderConfig{
		APIKey:  "sk-gateway",
		BaseURL: "https://example.com/v1",
	})
	if err != nil {
		t.Fatalf("add custom provider: %v", err)
	}
	app := New(nil, cfg, nil)
	app.providerModels[config.ProviderOpenAI] = []llm.ModelDescriptor{{ID: "gpt-4o", DisplayName: "gpt-4o"}}
	app.providerModels[custom.ID] = []llm.ModelDescriptor{{ID: "reasoner", DisplayName: "reasoner"}}

	sections := app.modelPaletteSections()
	if sectionItemsByLabel(sections, "OpenAI") == nil {
		t.Fatalf("expected OpenAI section, got %#v", sections)
	}
	if sectionItemsByLabel(sections, "Gateway") == nil {
		t.Fatalf("expected custom provider section, got %#v", sections)
	}
}

func TestCurrentThinkingLevelsFollowProviderMetadata(t *testing.T) {
	cfg := config.DefaultConfig()
	custom, err := cfg.LLM.AddCustomProvider("Gateway", config.ProviderOpenAICompatible, config.LLMProviderConfig{
		APIKey:  "sk-gateway",
		BaseURL: "https://example.com/v1",
		Model:   "reasoner",
	})
	if err != nil {
		t.Fatalf("add custom provider: %v", err)
	}
	cfg.LLM.DefaultProvider = custom.ID
	app := New(nil, cfg, nil)
	app.providerModels[custom.ID] = []llm.ModelDescriptor{
		{
			ID:              "reasoner",
			DisplayName:     "reasoner",
			Provider:        config.ProviderOpenAICompatible,
			ThinkingSupport: llm.ThinkingSupportSupported,
			ThinkingLevels:  []string{"low", "high"},
		},
	}

	levels := app.currentThinkingLevels()
	if len(levels) != 2 || levels[0] != ThinkLow || levels[1] != ThinkHigh {
		t.Fatalf("expected advertised thinking levels to be preserved, got %#v", levels)
	}
}

func TestCycleThinkLevelFallsBackToCurrentConfiguredModelMetadata(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.LLM.DefaultProvider = config.ProviderOpenAI
	cfg.LLM.OpenAI.Model = "gpt-5"
	cfg.LLM.OpenAI.ThinkingLevel = "medium"
	app := New(nil, cfg, nil)

	app.cycleThinkLevel()

	if app.statusMsg == "Current model does not advertise thinking levels." {
		t.Fatalf("expected current model fallback metadata to avoid no-thinking warning")
	}
	if app.thinkLevel != ThinkHigh {
		t.Fatalf("expected thinking level to advance to high, got %v", app.thinkLevel)
	}
}

func TestBeginModelsPaletteLoadDoesNotSetProviderLoadingState(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.LLM.OpenAI.APIKey = "sk-openai"
	app := New(nil, cfg, nil)

	_ = app.beginModelsPaletteLoad()

	if app.providerModelLoading[config.ProviderOpenAI] {
		t.Fatalf("expected model palette load not to set per-provider loading state")
	}
}

func sectionItemsByLabel(sections []paletteSection, label string) []paletteItem {
	for _, section := range sections {
		if section.Label == label {
			return section.Items
		}
	}
	return nil
}
