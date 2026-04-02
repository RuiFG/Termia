package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/termia/termia/internal/config"
	"github.com/termia/termia/internal/providerpolicy"
)

type ThinkingSupport = providerpolicy.ThinkingSupport

const (
	ThinkingSupportUnknown     = providerpolicy.ThinkingSupportUnknown
	ThinkingSupportUnsupported = providerpolicy.ThinkingSupportUnsupported
	ThinkingSupportSupported   = providerpolicy.ThinkingSupportSupported
)

type ProviderConfigField = providerpolicy.ConfigField

const (
	ProviderFieldAPIKey  = providerpolicy.ConfigFieldAPIKey
	ProviderFieldBaseURL = providerpolicy.ConfigFieldBaseURL
)

type ProviderConfigFieldSpec = providerpolicy.ConfigFieldSpec

type ModelDescriptor struct {
	ID              string
	DisplayName     string
	Provider        string
	ThinkingSupport ThinkingSupport
	ThinkingLevels  []string
}

func ConfigFields(provider string) []ProviderConfigFieldSpec {
	return providerpolicy.ConfigFields(provider)
}

func RequiresAPIKey(provider string) bool {
	return providerpolicy.RequiresAPIKey(provider)
}

func RequiresExplicitBaseURL(provider string) bool {
	return providerpolicy.RequiresExplicitBaseURL(provider)
}

func DefaultBaseURL(provider string) string {
	return providerpolicy.DefaultBaseURL(provider)
}

func EffectiveBaseURL(provider string, providerCfg config.LLMProviderConfig) string {
	return providerpolicy.EffectiveBaseURL(provider, providerCfg.BaseURL)
}

func ValidateProviderConfig(provider string, providerCfg config.LLMProviderConfig) error {
	provider = providerpolicy.NormalizeProviderName(provider)
	if provider == "" {
		return fmt.Errorf("provider is empty")
	}
	if RequiresAPIKey(provider) && strings.TrimSpace(providerCfg.ResolvedAPIKey()) == "" {
		return fmt.Errorf("API key is required for %s", config.ProviderDisplayName(provider))
	}
	baseURL := EffectiveBaseURL(provider, providerCfg)
	if RequiresExplicitBaseURL(provider) && strings.TrimSpace(baseURL) == "" {
		return fmt.Errorf("base URL is required for %s", config.ProviderDisplayName(provider))
	}
	if strings.TrimSpace(baseURL) != "" {
		if _, err := url.ParseRequestURI(baseURL); err != nil {
			return fmt.Errorf("invalid base URL: %w", err)
		}
	}
	return nil
}

func ListModels(ctx context.Context, meta config.ProviderMeta) ([]ModelDescriptor, error) {
	provider := providerpolicy.NormalizeProviderName(meta.Kind)
	if provider == "" {
		return nil, fmt.Errorf("provider is empty")
	}
	if err := ValidateModelCatalogConfig(meta); err != nil {
		return nil, err
	}
	return listModelsFromModelsDev(ctx, meta)
}

func ValidateModelCatalogConfig(meta config.ProviderMeta) error {
	provider := providerpolicy.NormalizeProviderName(meta.Kind)
	if provider == "" {
		return fmt.Errorf("provider is empty")
	}

	baseURL := strings.TrimSpace(providerpolicy.EffectiveBaseURL(provider, meta.Config.BaseURL))
	if provider == config.ProviderOpenAICompatible && baseURL == "" {
		return fmt.Errorf("base URL is required for %s", config.ProviderDisplayName(provider))
	}
	if baseURL != "" {
		if _, err := url.ParseRequestURI(baseURL); err != nil {
			return fmt.Errorf("invalid base URL: %w", err)
		}
	}
	return nil
}

func newDescriptor(provider, id, label, raw string) ModelDescriptor {
	support, levels := parseThinkingMetadata(raw)
	inferredSupport, inferredLevels := providerpolicy.InferThinkingMetadata(provider, id)
	if support == ThinkingSupportUnknown && inferredSupport != ThinkingSupportUnknown {
		support = inferredSupport
		levels = inferredLevels
	}
	if support == ThinkingSupportSupported && len(levels) == 0 && inferredSupport == ThinkingSupportSupported && len(inferredLevels) > 0 {
		levels = inferredLevels
	}
	return ModelDescriptor{
		ID:              strings.TrimSpace(id),
		DisplayName:     strings.TrimSpace(label),
		Provider:        providerpolicy.NormalizeProviderName(provider),
		ThinkingSupport: support,
		ThinkingLevels:  levels,
	}
}

func parseThinkingMetadata(raw string) (ThinkingSupport, []string) {
	if strings.TrimSpace(raw) == "" {
		return ThinkingSupportUnknown, nil
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return ThinkingSupportUnknown, nil
	}

	for _, key := range []string{
		"thinking_levels",
		"supported_thinking_levels",
		"reasoning_efforts",
		"supported_reasoning_efforts",
	} {
		levels := normalizeThinkingLevelList(anyToStringSlice(payload[key]))
		if len(levels) > 0 {
			return ThinkingSupportSupported, levels
		}
	}

	for _, key := range []string{
		"supports_thinking",
		"thinking_supported",
		"supports_reasoning",
		"reasoning_supported",
		"reasoning",
	} {
		if value, ok := payload[key].(bool); ok {
			if value {
				return ThinkingSupportSupported, nil
			}
			return ThinkingSupportUnsupported, nil
		}
	}

	return ThinkingSupportUnknown, nil
}

func ThinkingLevelsForModel(provider, modelID string) []string {
	return providerpolicy.ThinkingLevelsForModel(provider, modelID)
}

func SupportsThinkingLevel(provider, modelID, level string) bool {
	return providerpolicy.SupportsThinkingLevel(provider, modelID, level)
}

func DefaultThinkingLevel(provider, modelID string) string {
	return providerpolicy.DefaultThinkingLevel(provider, modelID)
}

func NormalizeThinkingLevel(value string) string {
	return providerpolicy.NormalizeThinkingLevel(value)
}

func IsOpenAIResponsesOnlyModel(modelID string) bool {
	return providerpolicy.IsOpenAIResponsesOnlyModel(modelID)
}

func anyToStringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		values := make([]string, 0, len(typed))
		for _, entry := range typed {
			if str, ok := entry.(string); ok {
				values = append(values, str)
			}
		}
		return values
	default:
		return nil
	}
}

func normalizeThinkingLevelList(values []string) []string {
	return providerpolicy.NormalizeThinkingLevelList(values)
}

func sortDescriptors(models []ModelDescriptor) {
	sort.Slice(models, func(i, j int) bool {
		left := strings.ToLower(models[i].DisplayName)
		right := strings.ToLower(models[j].DisplayName)
		if left == right {
			return strings.ToLower(models[i].ID) < strings.ToLower(models[j].ID)
		}
		return left < right
	})
}
