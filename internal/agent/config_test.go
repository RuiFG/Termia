package agent

import (
	"testing"

	"github.com/termia/termia/internal/config"
)

func TestDefaultModelSpecFromConfigSupportsOpenAICompatible(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.LLM.DefaultProvider = config.ProviderOpenAICompatible
	cfg.LLM.OpenAICompatible.APIKey = "custom-key"
	cfg.LLM.OpenAICompatible.BaseURL = "https://example.com/v1"
	cfg.LLM.OpenAICompatible.Model = "custom-model"

	spec, err := DefaultModelSpecFromConfig(&cfg.LLM)
	if err != nil {
		t.Fatalf("expected openai compatible spec, got %v", err)
	}
	if spec.Provider != config.ProviderOpenAICompatible {
		t.Fatalf("expected provider %q, got %q", config.ProviderOpenAICompatible, spec.Provider)
	}
	if spec.APIKey != "custom-key" || spec.BaseURL != "https://example.com/v1" || spec.Model != "custom-model" {
		t.Fatalf("unexpected model spec: %#v", spec)
	}
}

func TestDefaultModelSpecFromConfigDefaultsThinkingLevelFromModel(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.LLM.DefaultProvider = config.ProviderOpenAI
	cfg.LLM.OpenAI.APIKey = "sk-openai"
	cfg.LLM.OpenAI.Model = "gpt-5"
	cfg.LLM.OpenAI.ThinkingLevel = ""

	spec, err := DefaultModelSpecFromConfig(&cfg.LLM)
	if err != nil {
		t.Fatalf("expected openai thinking default, got %v", err)
	}
	if spec.ThinkingLevel != "medium" {
		t.Fatalf("expected default thinking level medium, got %q", spec.ThinkingLevel)
	}
	if spec.BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("expected openai default base url, got %q", spec.BaseURL)
	}
}

func TestDefaultModelSpecFromConfigPreservesConfiguredThinkingLevel(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.LLM.DefaultProvider = config.ProviderAnthropic
	cfg.LLM.Anthropic.APIKey = "sk-anthropic"
	cfg.LLM.Anthropic.Model = "claude-3-7-sonnet-20250219"
	cfg.LLM.Anthropic.ThinkingLevel = "high"

	spec, err := DefaultModelSpecFromConfig(&cfg.LLM)
	if err != nil {
		t.Fatalf("expected anthropic thinking spec, got %v", err)
	}
	if spec.ThinkingLevel != "high" {
		t.Fatalf("expected configured thinking level high, got %q", spec.ThinkingLevel)
	}
}
func TestDefaultModelSpecFromConfigClampsChatLatestThinkingLevelToModelSupport(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.LLM.DefaultProvider = config.ProviderOpenAI
	cfg.LLM.OpenAI.APIKey = "sk-openai"
	cfg.LLM.OpenAI.Model = "gpt-5.1-chat-latest"
	cfg.LLM.OpenAI.ThinkingLevel = "high"

	spec, err := DefaultModelSpecFromConfig(&cfg.LLM)
	if err != nil {
		t.Fatalf("expected openai chat-latest spec, got %v", err)
	}
	if spec.ThinkingLevel != "medium" {
		t.Fatalf("expected chat-latest thinking level to clamp to medium, got %q", spec.ThinkingLevel)
	}
}
