package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino-ext/components/model/claude"
	"github.com/cloudwego/eino-ext/components/model/deepseek"
	"github.com/cloudwego/eino-ext/components/model/ollama"
	"github.com/cloudwego/eino-ext/components/model/openai"
	einomodel "github.com/cloudwego/eino/components/model"
)

// NewModel creates an Eino ToolCallingChatModel from the resolved AgentConfig.
func NewModel(cfg *AgentConfig) (einomodel.ToolCallingChatModel, error) {
	if cfg == nil {
		return nil, fmt.Errorf("agent config is nil")
	}

	ctx := context.Background()

	switch cfg.Provider {
	case "openai":
		var maxTokens *int
		if cfg.MaxTokens > 0 {
			maxTokens = intPtr(cfg.MaxTokens)
		}
		var temperature *float32
		if cfg.Temperature > 0 {
			temperature = float32Ptr(cfg.Temperature)
		}
		return openai.NewChatModel(ctx, &openai.ChatModelConfig{
			APIKey:      cfg.APIKey,
			Model:       cfg.Model,
			BaseURL:     cfg.BaseURL,
			MaxTokens:   maxTokens,
			Temperature: temperature,
		})

	case "deepseek":
		return deepseek.NewChatModel(ctx, &deepseek.ChatModelConfig{
			APIKey:      cfg.APIKey,
			Model:       cfg.Model,
			BaseURL:     cfg.BaseURL,
			MaxTokens:   maxInt(cfg.MaxTokens, 0),
			Temperature: float32(cfg.Temperature),
		})

	case "anthropic":
		maxTokens := cfg.MaxTokens
		if maxTokens <= 0 {
			maxTokens = 2000
		}
		var baseURL *string
		if strings.TrimSpace(cfg.BaseURL) != "" {
			baseURL = &cfg.BaseURL
		}
		var temperature *float32
		if cfg.Temperature > 0 {
			temperature = float32Ptr(cfg.Temperature)
		}
		return claude.NewChatModel(ctx, &claude.Config{
			APIKey:      cfg.APIKey,
			Model:       cfg.Model,
			BaseURL:     baseURL,
			MaxTokens:   maxTokens,
			Temperature: temperature,
		})

	case "ollama":
		return ollama.NewChatModel(ctx, &ollama.ChatModelConfig{
			BaseURL: cfg.BaseURL,
			Model:   cfg.Model,
		})

	default:
		return nil, fmt.Errorf("unsupported provider: %s", cfg.Provider)
	}
}

func intPtr(v int) *int { return &v }

func float32Ptr(v float64) *float32 {
	value := float32(v)
	return &value
}

func maxInt(value int, min int) int {
	if value < min {
		return min
	}
	return value
}
