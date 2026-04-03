package providerpolicy

import "strings"

type ThinkingSupport int

const (
	ThinkingSupportUnknown ThinkingSupport = iota
	ThinkingSupportUnsupported
	ThinkingSupportSupported
)

type ConfigField string

const (
	ConfigFieldAPIKey  ConfigField = "api_key"
	ConfigFieldBaseURL ConfigField = "base_url"
)

type ConfigFieldSpec struct {
	Field  ConfigField
	Label  string
	Secret bool
}

const (
	ProviderOpenAI           = "openai"
	ProviderAnthropic        = "anthropic"
	ProviderDeepSeek         = "deepseek"
	ProviderOllama           = "ollama"
	ProviderOpenAICompatible = "openai_compatible"
)

func NormalizeProviderName(provider string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	switch provider {
	case "claude":
		return ProviderAnthropic
	default:
		return provider
	}
}

func ProviderDisplayName(provider string) string {
	switch NormalizeProviderName(provider) {
	case ProviderOpenAI:
		return "OpenAI"
	case ProviderAnthropic:
		return "Anthropic"
	case ProviderDeepSeek:
		return "DeepSeek"
	case ProviderOllama:
		return "Ollama"
	case ProviderOpenAICompatible:
		return "OpenAI Compatible"
	default:
		return strings.TrimSpace(provider)
	}
}

func ProviderOrder() []string {
	return []string{
		ProviderOpenAI,
		ProviderAnthropic,
		ProviderDeepSeek,
		ProviderOllama,
		ProviderOpenAICompatible,
	}
}

func BuiltinProviderOrder() []string {
	return []string{
		ProviderOpenAI,
		ProviderAnthropic,
		ProviderDeepSeek,
		ProviderOllama,
	}
}

func ConfigFields(provider string) []ConfigFieldSpec {
	switch NormalizeProviderName(provider) {
	case ProviderOpenAI, ProviderAnthropic, ProviderDeepSeek:
		return []ConfigFieldSpec{
			{Field: ConfigFieldAPIKey, Label: "API Key", Secret: true},
		}
	case ProviderOllama:
		return []ConfigFieldSpec{
			{Field: ConfigFieldBaseURL, Label: "Base URL", Secret: false},
		}
	case ProviderOpenAICompatible:
		return []ConfigFieldSpec{
			{Field: ConfigFieldAPIKey, Label: "API Key", Secret: true},
			{Field: ConfigFieldBaseURL, Label: "Base URL", Secret: false},
		}
	default:
		return nil
	}
}

func RequiresAPIKey(provider string) bool {
	switch NormalizeProviderName(provider) {
	case ProviderOpenAI, ProviderAnthropic, ProviderDeepSeek, ProviderOpenAICompatible:
		return true
	default:
		return false
	}
}

func RequiresExplicitBaseURL(provider string) bool {
	switch NormalizeProviderName(provider) {
	case ProviderOpenAICompatible:
		return true
	default:
		return false
	}
}

func DefaultBaseURL(provider string) string {
	switch NormalizeProviderName(provider) {
	case ProviderOpenAI:
		return "https://api.openai.com/v1"
	case ProviderAnthropic:
		return "https://api.anthropic.com"
	case ProviderDeepSeek:
		return "https://api.deepseek.com"
	case ProviderOllama:
		return "http://localhost:11434"
	default:
		return ""
	}
}

func EffectiveBaseURL(provider, configuredBaseURL string) string {
	if baseURL := strings.TrimSpace(configuredBaseURL); baseURL != "" {
		return baseURL
	}
	return DefaultBaseURL(provider)
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

func NormalizeThinkingLevelList(values []string) []string {
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

func ThinkingLevelsForModel(provider, modelID string) []string {
	_, levels := InferThinkingMetadata(provider, modelID)
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

func InferThinkingMetadata(provider, modelID string) (ThinkingSupport, []string) {
	id := normalizedInferenceModelID(modelID)
	if id == "" {
		return ThinkingSupportUnknown, nil
	}

	switch NormalizeProviderName(provider) {
	case ProviderOpenAI, ProviderOpenAICompatible:
		if isOpenAIChatLatestModel(id) {
			return ThinkingSupportSupported, []string{"medium"}
		}
		if isOpenAIReasoningModel(id) {
			return ThinkingSupportSupported, []string{"low", "medium", "high"}
		}
	case ProviderAnthropic:
		if isAnthropicThinkingModel(id) {
			return ThinkingSupportSupported, []string{"low", "medium", "high"}
		}
	case ProviderDeepSeek:
		if isDeepSeekReasonerModel(id) {
			return ThinkingSupportSupported, []string{"medium"}
		}
	case ProviderOllama:
		if isOllamaThinkingModel(modelID) {
			return ThinkingSupportSupported, []string{"low", "medium", "high"}
		}
	}

	return ThinkingSupportUnknown, nil
}

func IsOpenAIResponsesOnlyModel(modelID string) bool {
	id := normalizedInferenceModelID(modelID)
	return strings.Contains(id, "codex")
}

func UsesNativeOpenAIResponses(provider, baseURL string) bool {
	provider = NormalizeProviderName(provider)
	if provider == ProviderOpenAI {
		return true
	}
	if provider != ProviderOpenAICompatible {
		return false
	}
	baseURL = strings.ToLower(strings.TrimSpace(baseURL))
	return strings.Contains(baseURL, "api.openai.com")
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

func isAnthropicThinkingModel(id string) bool {
	if !strings.Contains(id, "claude") {
		return false
	}
	return strings.Contains(id, "3-7") ||
		strings.Contains(id, "claude-4") ||
		strings.Contains(id, "sonnet-4") ||
		strings.Contains(id, "opus-4")
}

func isDeepSeekReasonerModel(id string) bool {
	return strings.HasPrefix(id, "deepseek-reasoner")
}

func isOllamaThinkingModel(modelID string) bool {
	id := normalizeOllamaFamilyID(modelID)
	return strings.HasPrefix(id, "deepseek-r1") ||
		strings.HasPrefix(id, "gpt-oss") ||
		strings.HasPrefix(id, "qwen3") ||
		strings.HasPrefix(id, "qwq")
}

func normalizeOllamaFamilyID(modelID string) string {
	id := strings.ToLower(strings.TrimSpace(modelID))
	if id == "" {
		return ""
	}
	if slash := strings.LastIndexByte(id, '/'); slash >= 0 {
		id = strings.TrimSpace(id[slash+1:])
	}
	if colon := strings.IndexByte(id, ':'); colon >= 0 {
		id = strings.TrimSpace(id[:colon])
	}
	return id
}
