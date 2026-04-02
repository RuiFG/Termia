package agent

import (
	"context"
	"fmt"
	"strings"

	agenticopenai "github.com/cloudwego/eino-ext/components/model/agenticopenai"
	einoopenai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/openai/openai-go/v3/responses"
	"github.com/termia/termia/internal/providerpolicy"
)

func newOpenAIModel(ctx context.Context, spec ModelSpec) (model.ToolCallingChatModel, error) {
	if strings.TrimSpace(spec.Model) == "" {
		return nil, fmt.Errorf("model name is required")
	}
	if providerpolicy.UsesNativeOpenAIResponses(spec.Provider, effectiveModelBaseURL(spec)) {
		return newOpenAIAgenticModel(ctx, spec)
	}
	return newOpenAICompatibleChatModel(ctx, spec)
}

func newOpenAIAgenticModel(ctx context.Context, spec ModelSpec) (model.ToolCallingChatModel, error) {
	timeout := defaultModelTimeout
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
	if providerpolicy.IsOpenAIResponsesOnlyModel(spec.Model) {
		return nil, fmt.Errorf("model %q requires the OpenAI Responses API", spec.Model)
	}
	cfg := &einoopenai.ChatModelConfig{
		APIKey:  resolveAPIKey(spec),
		BaseURL: effectiveModelBaseURL(spec),
		Model:   strings.TrimSpace(spec.Model),
		Timeout: defaultModelTimeout,
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

func openAIReasoningEffort(level string) (einoopenai.ReasoningEffortLevel, bool) {
	switch providerpolicy.NormalizeThinkingLevel(level) {
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
	if !providerpolicy.SupportsThinkingLevel(provider, modelID, level) {
		return "", false
	}
	return openAIReasoningEffort(level)
}

func openAIResponsesReasoning(level string) (*responses.ReasoningParam, bool) {
	switch providerpolicy.NormalizeThinkingLevel(level) {
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
	if !providerpolicy.SupportsThinkingLevel(provider, modelID, level) {
		return nil, false
	}
	return openAIResponsesReasoning(level)
}
