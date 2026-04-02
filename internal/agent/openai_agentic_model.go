package agent

import (
	"context"

	agenticopenai "github.com/cloudwego/eino-ext/components/model/agenticopenai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type openAIAgenticChatModel struct {
	inner model.AgenticModel
}

func newOpenAIAgenticChatModel(inner model.AgenticModel) model.ToolCallingChatModel {
	return &openAIAgenticChatModel{inner: inner}
}

func (m *openAIAgenticChatModel) Generate(
	ctx context.Context,
	input []*schema.Message,
	opts ...model.Option,
) (*schema.Message, error) {
	agenticInput, err := messagesToAgenticMessages(input)
	if err != nil {
		return nil, err
	}
	out, err := m.inner.Generate(ctx, agenticInput, opts...)
	if err != nil {
		return nil, err
	}
	return agenticMessageToMessage(out)
}

func (m *openAIAgenticChatModel) Stream(
	ctx context.Context,
	input []*schema.Message,
	opts ...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	agenticInput, err := messagesToAgenticMessages(input)
	if err != nil {
		return nil, err
	}
	stream, err := m.inner.Stream(ctx, agenticInput, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderWithConvert(stream, func(msg *schema.AgenticMessage) (*schema.Message, error) {
		return agenticMessageToMessage(msg)
	}), nil
}

func (m *openAIAgenticChatModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	inner, err := m.inner.WithTools(tools)
	if err != nil {
		return nil, err
	}
	return newOpenAIAgenticChatModel(inner), nil
}

var _ model.ToolCallingChatModel = (*openAIAgenticChatModel)(nil)
var _ model.AgenticModel = (*agenticopenai.Model)(nil)
