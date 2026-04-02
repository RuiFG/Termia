package tui

import (
	"testing"

	"github.com/termia/termia/internal/config"
)

func TestProviderServiceSelectModelPersistsDefaultProvider(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.LLM.OpenAI.APIKey = "sk-openai"
	saved := 0
	svc := newProviderService(cfg, func(*config.Config) error {
		saved++
		return nil
	})

	if err := svc.SelectModel(config.ProviderOpenAI, "gpt-4o"); err != nil {
		t.Fatalf("select model: %v", err)
	}
	if cfg.LLM.DefaultProvider != config.ProviderOpenAI {
		t.Fatalf("expected default provider to stay openai, got %q", cfg.LLM.DefaultProvider)
	}
	if cfg.LLM.OpenAI.Model != "gpt-4o" {
		t.Fatalf("expected selected model to persist, got %q", cfg.LLM.OpenAI.Model)
	}
	if saved != 1 {
		t.Fatalf("expected config save once, got %d", saved)
	}
}

func TestProviderServiceProviderFieldDisplayValueFallsBackToDefaultBaseURL(t *testing.T) {
	cfg := config.DefaultConfig()
	svc := newProviderService(cfg, nil)

	got := svc.ProviderFieldDisplayValue(config.ProviderOpenAI, "base_url")
	if got != "https://api.openai.com/v1" {
		t.Fatalf("expected default openai base url, got %q", got)
	}
}
