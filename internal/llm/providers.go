package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/termia/termia/internal/config"
)

type ThinkingSupport int

const (
	ThinkingSupportUnknown ThinkingSupport = iota
	ThinkingSupportUnsupported
	ThinkingSupportSupported
)

type ProviderConfigField string

const (
	ProviderFieldAPIKey  ProviderConfigField = "api_key"
	ProviderFieldBaseURL ProviderConfigField = "base_url"
)

type ProviderConfigFieldSpec struct {
	Field  ProviderConfigField
	Label  string
	Secret bool
}

type ModelDescriptor struct {
	ID              string
	DisplayName     string
	Provider        string
	ThinkingSupport ThinkingSupport
	ThinkingLevels  []string
}

func ConfigFields(provider string) []ProviderConfigFieldSpec {
	switch config.NormalizeProviderName(provider) {
	case config.ProviderOpenAI, config.ProviderAnthropic, config.ProviderDeepSeek:
		return []ProviderConfigFieldSpec{
			{Field: ProviderFieldAPIKey, Label: "API Key", Secret: true},
		}
	case config.ProviderOllama:
		return []ProviderConfigFieldSpec{
			{Field: ProviderFieldBaseURL, Label: "Base URL", Secret: false},
		}
	case config.ProviderOpenAICompatible:
		return []ProviderConfigFieldSpec{
			{Field: ProviderFieldAPIKey, Label: "API Key", Secret: true},
			{Field: ProviderFieldBaseURL, Label: "Base URL", Secret: false},
		}
	default:
		return nil
	}
}

func RequiresAPIKey(provider string) bool {
	switch config.NormalizeProviderName(provider) {
	case config.ProviderOpenAI, config.ProviderAnthropic, config.ProviderDeepSeek, config.ProviderOpenAICompatible:
		return true
	default:
		return false
	}
}

func RequiresExplicitBaseURL(provider string) bool {
	switch config.NormalizeProviderName(provider) {
	case config.ProviderOpenAICompatible:
		return true
	default:
		return false
	}
}

func DefaultBaseURL(provider string) string {
	switch config.NormalizeProviderName(provider) {
	case config.ProviderOpenAI:
		return "https://api.openai.com/v1"
	case config.ProviderAnthropic:
		return "https://api.anthropic.com"
	case config.ProviderDeepSeek:
		return "https://api.deepseek.com"
	case config.ProviderOllama:
		return "http://localhost:11434"
	default:
		return ""
	}
}

func EffectiveBaseURL(provider string, providerCfg config.LLMProviderConfig) string {
	baseURL := strings.TrimSpace(providerCfg.BaseURL)
	if baseURL != "" {
		return baseURL
	}
	return DefaultBaseURL(provider)
}

func ValidateProviderConfig(provider string, providerCfg config.LLMProviderConfig) error {
	provider = config.NormalizeProviderName(provider)
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
	provider := config.NormalizeProviderName(meta.Kind)
	if provider == "" {
		return nil, fmt.Errorf("provider is empty")
	}
	if err := ValidateModelCatalogConfig(meta); err != nil {
		return nil, err
	}
	return listModelsFromModelsDev(ctx, meta)
}

func ValidateModelCatalogConfig(meta config.ProviderMeta) error {
	provider := config.NormalizeProviderName(meta.Kind)
	if provider == "" {
		return fmt.Errorf("provider is empty")
	}

	baseURL := strings.TrimSpace(EffectiveBaseURL(provider, meta.Config))
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
	inferredSupport, inferredLevels := inferThinkingMetadata(provider, id)
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
		Provider:        config.NormalizeProviderName(provider),
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
	_, levels := inferThinkingMetadata(provider, modelID)
	if len(levels) == 0 {
		return nil
	}
	return append([]string(nil), levels...)
}

func SupportsThinkingLevel(provider, modelID, level string) bool {
	level = NormalizeThinkingLevel(level)
	if level == "" {
		return false
	}
	for _, candidate := range ThinkingLevelsForModel(provider, modelID) {
		if candidate == level {
			return true
		}
	}
	return false
}

func DefaultThinkingLevel(provider, modelID string) string {
	levels := ThinkingLevelsForModel(provider, modelID)
	if len(levels) == 0 {
		return ""
	}
	for _, preferred := range []string{"medium", "low", "high"} {
		for _, level := range levels {
			if level == preferred {
				return level
			}
		}
	}
	return levels[0]
}

func NormalizeThinkingLevel(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "minimal":
		return "low"
	case "standard":
		return "medium"
	case "max":
		return "high"
	default:
		return value
	}
}

func inferThinkingMetadata(provider, modelID string) (ThinkingSupport, []string) {
	id := normalizedInferenceModelID(modelID)
	if id == "" {
		return ThinkingSupportUnknown, nil
	}

	switch config.NormalizeProviderName(provider) {
	case config.ProviderOpenAI, config.ProviderOpenAICompatible:
		if isOpenAIChatLatestModel(id) {
			return ThinkingSupportSupported, []string{"medium"}
		}
		if isOpenAIReasoningModel(id) {
			return ThinkingSupportSupported, []string{"low", "medium", "high"}
		}
	case config.ProviderAnthropic:
		if isAnthropicThinkingModel(id) {
			return ThinkingSupportSupported, []string{"low", "medium", "high"}
		}
	}

	return ThinkingSupportUnknown, nil
}

func normalizedInferenceModelID(modelID string) string {
	id := strings.ToLower(strings.TrimSpace(modelID))
	if id == "" {
		return ""
	}
	parts := strings.FieldsFunc(id, func(r rune) bool {
		return r == '/' || r == ':'
	})
	if len(parts) == 0 {
		return id
	}
	return parts[len(parts)-1]
}

func isOpenAIReasoningModel(id string) bool {
	switch {
	case strings.HasPrefix(id, "gpt-5"):
		return true
	case strings.HasPrefix(id, "o1"):
		return true
	case strings.HasPrefix(id, "o3"):
		return true
	case strings.HasPrefix(id, "o4-mini"):
		return true
	default:
		return false
	}
}

func isOpenAIChatLatestModel(id string) bool {
	return strings.HasPrefix(id, "gpt-5") && strings.HasSuffix(id, "-chat-latest")
}

func IsOpenAIResponsesOnlyModel(modelID string) bool {
	id := normalizedInferenceModelID(modelID)
	return strings.Contains(id, "codex")
}

func isAnthropicThinkingModel(id string) bool {
	if !strings.Contains(id, "claude") {
		return false
	}
	return strings.Contains(id, "3-7") ||
		strings.Contains(id, "claude-4") ||
		strings.Contains(id, "sonnet-4") ||
		strings.Contains(id, "opus-4")
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
	seen := make(map[string]struct{}, len(values))
	levels := make([]string, 0, len(values))
	for _, value := range values {
		value = NormalizeThinkingLevel(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		levels = append(levels, value)
	}
	return levels
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
