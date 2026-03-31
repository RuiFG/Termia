package agent

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	einoclaude "github.com/cloudwego/eino-ext/components/model/claude"
	einodeepseek "github.com/cloudwego/eino-ext/components/model/deepseek"
	einoollama "github.com/cloudwego/eino-ext/components/model/ollama"
	einoopenai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
)

func NewModel(ctx context.Context, spec ModelSpec) (model.ToolCallingChatModel, error) {
	provider := strings.ToLower(strings.TrimSpace(spec.Provider))
	switch provider {
	case "openai":
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
	cfg := &einoopenai.ChatModelConfig{
		APIKey:  strings.TrimSpace(os.Getenv(spec.APIKeyEnv)),
		BaseURL: strings.TrimSpace(spec.BaseURL),
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
	return einoopenai.NewChatModel(ctx, cfg)
}

func newAnthropicModel(ctx context.Context, spec ModelSpec) (model.ToolCallingChatModel, error) {
	if strings.TrimSpace(spec.Model) == "" {
		return nil, fmt.Errorf("model name is required")
	}
	cfg := &einoclaude.Config{
		APIKey:    strings.TrimSpace(os.Getenv(spec.APIKeyEnv)),
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
	return einoclaude.NewChatModel(ctx, cfg)
}

func newDeepSeekModel(ctx context.Context, spec ModelSpec) (model.ToolCallingChatModel, error) {
	if strings.TrimSpace(spec.Model) == "" {
		return nil, fmt.Errorf("model name is required")
	}
	cfg := &einodeepseek.ChatModelConfig{
		APIKey:  strings.TrimSpace(os.Getenv(spec.APIKeyEnv)),
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
