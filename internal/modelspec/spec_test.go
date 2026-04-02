package modelspec

import (
	"testing"

	"github.com/termia/termia/internal/config"
)

func TestDefaultFromConfigAppliesProviderDefaultsAndThinkingClamp(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.LLM.DefaultProvider = config.ProviderOpenAI
	cfg.LLM.OpenAI.APIKey = "sk-openai"
	cfg.LLM.OpenAI.Model = "gpt-5.1-chat-latest"
	cfg.LLM.OpenAI.ThinkingLevel = "high"

	spec, err := DefaultFromConfig(&cfg.LLM)
	if err != nil {
		t.Fatalf("default from config: %v", err)
	}
	if spec.BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("expected openai default base url, got %q", spec.BaseURL)
	}
	if spec.ThinkingLevel != "medium" {
		t.Fatalf("expected thinking level to clamp to medium, got %q", spec.ThinkingLevel)
	}
}
