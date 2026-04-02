package modelspec

import (
	"fmt"
	"strings"

	"github.com/termia/termia/internal/config"
	"github.com/termia/termia/internal/providerpolicy"
)

type Spec struct {
	Provider      string   `toml:"provider" json:"provider"`
	Model         string   `toml:"model" json:"model"`
	APIKey        string   `toml:"api_key" json:"api_key,omitempty"`
	APIKeyEnv     string   `toml:"api_key_env" json:"api_key_env"`
	BaseURL       string   `toml:"base_url" json:"base_url"`
	ThinkingLevel string   `toml:"thinking_level" json:"thinking_level,omitempty"`
	MaxTokens     int      `toml:"max_tokens" json:"max_tokens"`
	Temperature   *float64 `toml:"temperature" json:"temperature,omitempty"`
}

func DefaultFromConfig(llmCfg *config.LLMConfig) (Spec, error) {
	if llmCfg == nil {
		return Spec{}, fmt.Errorf("llm config is nil")
	}

	provider := providerpolicy.NormalizeProviderName(llmCfg.DefaultProvider)
	meta, ok := llmCfg.ProviderMeta(provider)
	if !ok {
		return Spec{}, fmt.Errorf("unsupported default provider %q", provider)
	}
	return FromProviderMeta(meta)
}

func FromProviderMeta(meta config.ProviderMeta) (Spec, error) {
	return FromProviderConfig(meta.Kind, meta.Config)
}

func FromProviderConfig(provider string, cfg config.LLMProviderConfig) (Spec, error) {
	provider = providerpolicy.NormalizeProviderName(provider)
	if provider == "" {
		return Spec{}, fmt.Errorf("provider is empty")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return Spec{}, fmt.Errorf("model is required for provider %s", provider)
	}

	thinkingLevel := providerpolicy.NormalizeThinkingLevel(cfg.ThinkingLevel)
	if thinkingLevel != "" && !providerpolicy.SupportsThinkingLevel(provider, cfg.Model, thinkingLevel) {
		thinkingLevel = ""
	}
	if thinkingLevel == "" {
		thinkingLevel = providerpolicy.DefaultThinkingLevel(provider, cfg.Model)
	}

	return Spec{
		Provider:      provider,
		Model:         strings.TrimSpace(cfg.Model),
		APIKey:        strings.TrimSpace(cfg.APIKey),
		BaseURL:       strings.TrimSpace(providerpolicy.EffectiveBaseURL(provider, cfg.BaseURL)),
		ThinkingLevel: thinkingLevel,
		MaxTokens:     cfg.MaxTokens,
	}, nil
}
