package agent

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	agenticopenai "github.com/cloudwego/eino-ext/components/model/agenticopenai"
	einoclaude "github.com/cloudwego/eino-ext/components/model/claude"
	einodeepseek "github.com/cloudwego/eino-ext/components/model/deepseek"
	einoollama "github.com/cloudwego/eino-ext/components/model/ollama"
	einoopenai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/openai/openai-go/v3/responses"
	"github.com/termia/termia/internal/llm"
)

func NewModel(ctx context.Context, spec ModelSpec) (model.ToolCallingChatModel, error) {
	provider := strings.ToLower(strings.TrimSpace(spec.Provider))
	switch provider {
	case "openai", "openai_compatible":
		return newOpenAIModel(ctx, spec)
	case "anthropic", "claude":
		return newAnthropicModel(ctx, spec)
	case "deepseek":
		return newDeepSeekModel(ctx, spec)
	case "ollama":
		return newOllamaModel(ctx, spec)
	default:
		return nil, fmt.Errorf("unsupported provider %q", spec.Provider)
	}
}

func newOpenAIModel(ctx context.Context, spec ModelSpec) (model.ToolCallingChatModel, error) {
	if strings.TrimSpace(spec.Model) == "" {
		return nil, fmt.Errorf("model name is required")
	}
	if usesNativeOpenAIResponses(spec) {
		return newOpenAIAgenticModel(ctx, spec)
	}
	return newOpenAICompatibleChatModel(ctx, spec)
}

func newOpenAIAgenticModel(ctx context.Context, spec ModelSpec) (model.ToolCallingChatModel, error) {
	timeout := 2 * time.Minute
	cfg := &agenticopenai.Config{
		APIKey:  resolveAPIKey(spec),
		BaseURL: effectiveModelBaseURL(spec),
		Model:   strings.TrimSpace(spec.Model),
		Timeout: &timeout,
	}
	if spec.MaxTokens > 0 {
		maxTokens := spec.MaxTokens
		cfg.MaxTokens = &maxTokens
	}
	if spec.Temperature != nil {
		temp := float32(*spec.Temperature)
		cfg.Temperature = &temp
	}
	if reasoning, ok := openAIResponsesReasoningForModel(spec.Provider, spec.Model, spec.ThinkingLevel); ok {
		cfg.Reasoning = reasoning
		cfg.Include = append(cfg.Include, responses.ResponseIncludableReasoningEncryptedContent)
	}
	inner, err := agenticopenai.New(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return newOpenAIAgenticChatModel(inner), nil
}

func newOpenAICompatibleChatModel(ctx context.Context, spec ModelSpec) (model.ToolCallingChatModel, error) {
	if llm.IsOpenAIResponsesOnlyModel(spec.Model) {
		return nil, fmt.Errorf("model %q requires the OpenAI Responses API", spec.Model)
	}
	cfg := &einoopenai.ChatModelConfig{
		APIKey:  resolveAPIKey(spec),
		BaseURL: effectiveModelBaseURL(spec),
		Model:   strings.TrimSpace(spec.Model),
		Timeout: 2 * time.Minute,
	}
	if spec.MaxTokens > 0 {
		maxTokens := spec.MaxTokens
		cfg.MaxCompletionTokens = &maxTokens
	}
	if spec.Temperature != nil {
		temp := float32(*spec.Temperature)
		cfg.Temperature = &temp
	}
	if effort, ok := openAIReasoningEffortForModel(spec.Provider, spec.Model, spec.ThinkingLevel); ok {
		cfg.ReasoningEffort = effort
	}
	return einoopenai.NewChatModel(ctx, cfg)
}

func newAnthropicModel(ctx context.Context, spec ModelSpec) (model.ToolCallingChatModel, error) {
	if strings.TrimSpace(spec.Model) == "" {
		return nil, fmt.Errorf("model name is required")
	}
	cfg := &einoclaude.Config{
		APIKey:    resolveAPIKey(spec),
		Model:     strings.TrimSpace(spec.Model),
		MaxTokens: spec.MaxTokens,
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 2000
	}
	if spec.Temperature != nil {
		temp := float32(*spec.Temperature)
		cfg.Temperature = &temp
	}
	if baseURL := strings.TrimSpace(spec.BaseURL); baseURL != "" {
		cfg.BaseURL = &baseURL
	}
	if budget, ok := anthropicThinkingBudget(spec.ThinkingLevel); ok {
		cfg.Thinking = &einoclaude.Thinking{Enable: true, BudgetTokens: budget}
	}
	return einoclaude.NewChatModel(ctx, cfg)
}

func newDeepSeekModel(ctx context.Context, spec ModelSpec) (model.ToolCallingChatModel, error) {
	if strings.TrimSpace(spec.Model) == "" {
		return nil, fmt.Errorf("model name is required")
	}
	cfg := &einodeepseek.ChatModelConfig{
		APIKey:  resolveAPIKey(spec),
		BaseURL: strings.TrimSpace(spec.BaseURL),
		Model:   strings.TrimSpace(spec.Model),
		Timeout: 2 * time.Minute,
	}
	if spec.MaxTokens > 0 {
		cfg.MaxTokens = spec.MaxTokens
	}
	if spec.Temperature != nil {
		cfg.Temperature = float32(*spec.Temperature)
	}
	return einodeepseek.NewChatModel(ctx, cfg)
}

func newOllamaModel(ctx context.Context, spec ModelSpec) (model.ToolCallingChatModel, error) {
	if strings.TrimSpace(spec.Model) == "" {
		return nil, fmt.Errorf("model name is required")
	}
	cfg := &einoollama.ChatModelConfig{
		BaseURL: strings.TrimSpace(spec.BaseURL),
		Model:   strings.TrimSpace(spec.Model),
		Timeout: 2 * time.Minute,
	}
	if spec.Temperature != nil {
		temp := float32(*spec.Temperature)
		cfg.Options = &einoollama.Options{
			Temperature: temp,
		}
	}
	return einoollama.NewChatModel(ctx, cfg)
}

func openAIReasoningEffort(level string) (einoopenai.ReasoningEffortLevel, bool) {
	switch llm.NormalizeThinkingLevel(level) {
	case "low":
		return einoopenai.ReasoningEffortLevelLow, true
	case "medium":
		return einoopenai.ReasoningEffortLevelMedium, true
	case "high":
		return einoopenai.ReasoningEffortLevelHigh, true
	default:
		return "", false
	}
}

func openAIReasoningEffortForModel(provider, modelID, level string) (einoopenai.ReasoningEffortLevel, bool) {
	if !llm.SupportsThinkingLevel(provider, modelID, level) {
		return "", false
	}
	return openAIReasoningEffort(level)
}

func openAIResponsesReasoning(level string) (*responses.ReasoningParam, bool) {
	switch llm.NormalizeThinkingLevel(level) {
	case "low":
		return &responses.ReasoningParam{
			Effort:  responses.ReasoningEffortLow,
			Summary: responses.ReasoningSummaryAuto,
		}, true
	case "medium":
		return &responses.ReasoningParam{
			Effort:  responses.ReasoningEffortMedium,
			Summary: responses.ReasoningSummaryAuto,
		}, true
	case "high":
		return &responses.ReasoningParam{
			Effort:  responses.ReasoningEffortHigh,
			Summary: responses.ReasoningSummaryAuto,
		}, true
	default:
		return nil, false
	}
}

func openAIResponsesReasoningForModel(provider, modelID, level string) (*responses.ReasoningParam, bool) {
	if !llm.SupportsThinkingLevel(provider, modelID, level) {
		return nil, false
	}
	return openAIResponsesReasoning(level)
}

func usesNativeOpenAIResponses(spec ModelSpec) bool {
	provider := strings.ToLower(strings.TrimSpace(spec.Provider))
	if provider == "openai" {
		return true
	}
	if provider != "openai_compatible" {
		return false
	}
	baseURL := strings.ToLower(strings.TrimSpace(spec.BaseURL))
	return strings.Contains(baseURL, "api.openai.com")
}

func effectiveModelBaseURL(spec ModelSpec) string {
	baseURL := strings.TrimSpace(spec.BaseURL)
	if baseURL != "" {
		return baseURL
	}
	return strings.TrimSpace(llm.DefaultBaseURL(spec.Provider))
}

func anthropicThinkingBudget(level string) (int, bool) {
	switch llm.NormalizeThinkingLevel(level) {
	case "low":
		return 4000, true
	case "medium":
		return 8000, true
	case "high":
		return 16000, true
	default:
		return 0, false
	}
}

func resolveAPIKey(spec ModelSpec) string {
	if apiKey := strings.TrimSpace(spec.APIKey); apiKey != "" {
		return apiKey
	}
	if envKey := strings.TrimSpace(spec.APIKeyEnv); envKey != "" {
		return strings.TrimSpace(os.Getenv(envKey))
	}
	return ""
}
