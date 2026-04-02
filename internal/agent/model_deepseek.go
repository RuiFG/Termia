package agent

import (
	"context"
	"fmt"
	"strings"

	einodeepseek "github.com/cloudwego/eino-ext/components/model/deepseek"
	"github.com/cloudwego/eino/components/model"
)

func newDeepSeekModel(ctx context.Context, spec ModelSpec) (model.ToolCallingChatModel, error) {
	if strings.TrimSpace(spec.Model) == "" {
		return nil, fmt.Errorf("model name is required")
	}
	cfg := &einodeepseek.ChatModelConfig{
		APIKey:  resolveAPIKey(spec),
		BaseURL: strings.TrimSpace(spec.BaseURL),
		Model:   strings.TrimSpace(spec.Model),
		Timeout: defaultModelTimeout,
	}
	if spec.MaxTokens > 0 {
		cfg.MaxTokens = spec.MaxTokens
	}
	if spec.Temperature != nil {
		cfg.Temperature = float32(*spec.Temperature)
	}
	return einodeepseek.NewChatModel(ctx, cfg)
}
