package agent

import (
	"context"
	"fmt"
	"strings"

	einoclaude "github.com/cloudwego/eino-ext/components/model/claude"
	"github.com/cloudwego/eino/components/model"
	"github.com/termia/termia/internal/providerpolicy"
)

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

func anthropicThinkingBudget(level string) (int, bool) {
	switch providerpolicy.NormalizeThinkingLevel(level) {
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
