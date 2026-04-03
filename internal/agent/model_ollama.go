package agent

import (
	"context"
	"fmt"
	"strings"

	einoollama "github.com/cloudwego/eino-ext/components/model/ollama"
	"github.com/cloudwego/eino/components/model"
	"github.com/termia/termia/internal/providerpolicy"
)

func newOllamaModel(ctx context.Context, spec ModelSpec) (model.ToolCallingChatModel, error) {
	if strings.TrimSpace(spec.Model) == "" {
		return nil, fmt.Errorf("model name is required")
	}
	cfg := &einoollama.ChatModelConfig{
		BaseURL: strings.TrimSpace(spec.BaseURL),
		Model:   strings.TrimSpace(spec.Model),
		Timeout: defaultModelTimeout,
	}
	if spec.Temperature != nil {
		temp := float32(*spec.Temperature)
		cfg.Options = &einoollama.Options{
			Temperature: temp,
		}
	}
	if thinking, ok := ollamaThinking(spec.ThinkingLevel); ok {
		cfg.Thinking = thinking
	}
	return einoollama.NewChatModel(ctx, cfg)
}

func ollamaThinking(level string) (*einoollama.ThinkValue, bool) {
	level = providerpolicy.NormalizeThinkingLevel(level)
	switch level {
	case "low", "medium", "high":
		return &einoollama.ThinkValue{Value: level}, true
	default:
		return nil, false
	}
}
