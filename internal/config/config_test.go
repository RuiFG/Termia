package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProviderConfigPtrReturnsOpenAICompatibleConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LLM.OpenAICompatible.Model = "custom-model"

	providerCfg, ok := cfg.LLM.ProviderConfigPtr(ProviderOpenAICompatible)
	if !ok || providerCfg == nil {
		t.Fatalf("expected openai_compatible config pointer")
	}
	if providerCfg.Model != "custom-model" {
		t.Fatalf("expected openai_compatible model to round-trip, got %q", providerCfg.Model)
	}
}

func TestNormalizeProviderNameMapsClaudeToAnthropic(t *testing.T) {
	if got := NormalizeProviderName("claude"); got != ProviderAnthropic {
		t.Fatalf("expected claude alias to normalize to anthropic, got %q", got)
	}
}

func TestAddCustomProviderRegistersProviderMeta(t *testing.T) {
	cfg := DefaultConfig()
	custom, err := cfg.LLM.AddCustomProvider("Gateway", ProviderOpenAICompatible, LLMProviderConfig{
		APIKey:  "sk-test",
		BaseURL: "https://example.com/v1",
	})
	if err != nil {
		t.Fatalf("add custom provider: %v", err)
	}

	meta, ok := cfg.LLM.ProviderMeta(custom.ID)
	if !ok {
		t.Fatalf("expected custom provider meta for %q", custom.ID)
	}
	if meta.Kind != ProviderOpenAICompatible || meta.DisplayName != "Gateway" || !meta.Custom {
		t.Fatalf("unexpected custom provider meta: %#v", meta)
	}
}

func TestSaveDoesNotWriteLegacyAPIKeyEnv(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LLM.OpenAI.APIKey = "sk-test"

	path := filepath.Join(t.TempDir(), "config.toml")
	if err := Save(cfg, path); err != nil {
		t.Fatalf("save config: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	text := string(data)
	if strings.Contains(text, "api_key_env") {
		t.Fatalf("expected saved config to omit api_key_env, got:\n%s", text)
	}
	if !strings.Contains(text, `api_key = "sk-test"`) {
		t.Fatalf("expected saved config to include API key, got:\n%s", text)
	}
}

func TestProviderConfigPtrRoundTripsThinkingLevel(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LLM.OpenAI.ThinkingLevel = "high"

	providerCfg, ok := cfg.LLM.ProviderConfigPtr(ProviderOpenAI)
	if !ok || providerCfg == nil {
		t.Fatalf("expected openai config pointer")
	}
	if providerCfg.ThinkingLevel != "high" {
		t.Fatalf("expected thinking level to round-trip, got %q", providerCfg.ThinkingLevel)
	}
}
