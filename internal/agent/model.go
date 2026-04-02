package agent

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/model"
	"github.com/termia/termia/internal/providerpolicy"
)

type modelBuilder interface {
	Build(context.Context, ModelSpec) (model.ToolCallingChatModel, error)
}

type modelBuilderFunc func(context.Context, ModelSpec) (model.ToolCallingChatModel, error)

func (f modelBuilderFunc) Build(ctx context.Context, spec ModelSpec) (model.ToolCallingChatModel, error) {
	return f(ctx, spec)
}

var providerModelBuilders = map[string]modelBuilder{
	providerpolicy.ProviderOpenAI:           modelBuilderFunc(newOpenAIModel),
	providerpolicy.ProviderOpenAICompatible: modelBuilderFunc(newOpenAIModel),
	providerpolicy.ProviderAnthropic:        modelBuilderFunc(newAnthropicModel),
	providerpolicy.ProviderDeepSeek:         modelBuilderFunc(newDeepSeekModel),
	providerpolicy.ProviderOllama:           modelBuilderFunc(newOllamaModel),
}

func NewModel(ctx context.Context, spec ModelSpec) (model.ToolCallingChatModel, error) {
	provider := providerpolicy.NormalizeProviderName(spec.Provider)
	builder, ok := providerModelBuilders[provider]
	if !ok {
		return nil, fmt.Errorf("unsupported provider %q", spec.Provider)
	}
	return builder.Build(ctx, spec)
}
