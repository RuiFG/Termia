package agent

import (
	"context"
	"fmt"

	agenticopenai "github.com/cloudwego/eino-ext/components/model/agenticopenai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

const openAIAgenticGeneratedKey = "openai-generated"

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

func messagesToAgenticMessages(messages []*schema.Message) ([]*schema.AgenticMessage, error) {
	out := make([]*schema.AgenticMessage, 0, len(messages))
	for _, msg := range messages {
		agentic, err := messageToAgenticMessage(msg)
		if err != nil {
			return nil, err
		}
		out = append(out, agentic)
	}
	return out, nil
}

func messageToAgenticMessage(msg *schema.Message) (*schema.AgenticMessage, error) {
	if msg == nil {
		return nil, fmt.Errorf("message is nil")
	}

	switch msg.Role {
	case schema.System:
		blocks, err := messageInputBlocks(msg)
		if err != nil {
			return nil, err
		}
		return &schema.AgenticMessage{
			Role:          schema.AgenticRoleTypeSystem,
			ContentBlocks: blocks,
		}, nil
	case schema.User:
		blocks, err := messageInputBlocks(msg)
		if err != nil {
			return nil, err
		}
		return &schema.AgenticMessage{
			Role:          schema.AgenticRoleTypeUser,
			ContentBlocks: blocks,
		}, nil
	case schema.Tool:
		return &schema.AgenticMessage{
			Role: schema.AgenticRoleTypeUser,
			ContentBlocks: []*schema.ContentBlock{{
				Type: schema.ContentBlockTypeFunctionToolResult,
				FunctionToolResult: &schema.FunctionToolResult{
					CallID: msg.ToolCallID,
					Name:   msg.ToolName,
					Result: msg.Content,
				},
				Extra: cloneMap(msg.Extra),
			}},
		}, nil
	case schema.Assistant:
		blocks, err := assistantMessageBlocks(msg)
		if err != nil {
			return nil, err
		}
		return &schema.AgenticMessage{
			Role:          schema.AgenticRoleTypeAssistant,
			ContentBlocks: blocks,
			Extra: map[string]any{
				openAIAgenticGeneratedKey: true,
			},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported message role %q", msg.Role)
	}
}

func messageInputBlocks(msg *schema.Message) ([]*schema.ContentBlock, error) {
	blocks := make([]*schema.ContentBlock, 0, len(msg.UserInputMultiContent)+1)
	if len(msg.UserInputMultiContent) > 0 {
		for _, part := range msg.UserInputMultiContent {
			block, err := userInputPartToBlock(part)
			if err != nil {
				return nil, err
			}
			blocks = append(blocks, block)
		}
		return blocks, nil
	}
	if msg.Content != "" {
		blocks = append(blocks, &schema.ContentBlock{
			Type:          schema.ContentBlockTypeUserInputText,
			UserInputText: &schema.UserInputText{Text: msg.Content},
		})
	}
	return blocks, nil
}

func userInputPartToBlock(part schema.MessageInputPart) (*schema.ContentBlock, error) {
	switch part.Type {
	case schema.ChatMessagePartTypeText:
		return &schema.ContentBlock{
			Type:          schema.ContentBlockTypeUserInputText,
			UserInputText: &schema.UserInputText{Text: part.Text},
			Extra:         cloneMap(part.Extra),
		}, nil
	case schema.ChatMessagePartTypeImageURL:
		if part.Image == nil {
			return nil, fmt.Errorf("user image part is nil")
		}
		return &schema.ContentBlock{
			Type: schema.ContentBlockTypeUserInputImage,
			UserInputImage: &schema.UserInputImage{
				URL:        stringValue(part.Image.URL),
				Base64Data: stringValue(part.Image.Base64Data),
				MIMEType:   part.Image.MIMEType,
				Detail:     part.Image.Detail,
			},
			Extra: cloneMap(part.Extra),
		}, nil
	case schema.ChatMessagePartTypeAudioURL:
		if part.Audio == nil {
			return nil, fmt.Errorf("user audio part is nil")
		}
		return &schema.ContentBlock{
			Type: schema.ContentBlockTypeUserInputAudio,
			UserInputAudio: &schema.UserInputAudio{
				URL:        stringValue(part.Audio.URL),
				Base64Data: stringValue(part.Audio.Base64Data),
				MIMEType:   part.Audio.MIMEType,
			},
			Extra: cloneMap(part.Extra),
		}, nil
	case schema.ChatMessagePartTypeVideoURL:
		if part.Video == nil {
			return nil, fmt.Errorf("user video part is nil")
		}
		return &schema.ContentBlock{
			Type: schema.ContentBlockTypeUserInputVideo,
			UserInputVideo: &schema.UserInputVideo{
				URL:        stringValue(part.Video.URL),
				Base64Data: stringValue(part.Video.Base64Data),
				MIMEType:   part.Video.MIMEType,
			},
			Extra: cloneMap(part.Extra),
		}, nil
	case schema.ChatMessagePartTypeFileURL:
		if part.File == nil {
			return nil, fmt.Errorf("user file part is nil")
		}
		return &schema.ContentBlock{
			Type: schema.ContentBlockTypeUserInputFile,
			UserInputFile: &schema.UserInputFile{
				URL:        stringValue(part.File.URL),
				Name:       part.File.Name,
				Base64Data: stringValue(part.File.Base64Data),
				MIMEType:   part.File.MIMEType,
			},
			Extra: cloneMap(part.Extra),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported user input part type %q", part.Type)
	}
}

func assistantMessageBlocks(msg *schema.Message) ([]*schema.ContentBlock, error) {
	blocks := make([]*schema.ContentBlock, 0, len(msg.AssistantGenMultiContent)+len(msg.ToolCalls)+1)
	if len(msg.AssistantGenMultiContent) > 0 {
		for _, part := range msg.AssistantGenMultiContent {
			block, err := assistantOutputPartToBlock(part)
			if err != nil {
				return nil, err
			}
			blocks = append(blocks, block)
		}
	} else if msg.Content != "" {
		blocks = append(blocks, &schema.ContentBlock{
			Type:             schema.ContentBlockTypeAssistantGenText,
			AssistantGenText: &schema.AssistantGenText{Text: msg.Content},
		})
	}
	if msg.ReasoningContent != "" && !hasReasoningBlock(blocks) {
		blocks = append(blocks, &schema.ContentBlock{
			Type:      schema.ContentBlockTypeReasoning,
			Reasoning: &schema.Reasoning{Text: msg.ReasoningContent},
		})
	}
	for _, call := range msg.ToolCalls {
		block := &schema.ContentBlock{
			Type: schema.ContentBlockTypeFunctionToolCall,
			FunctionToolCall: &schema.FunctionToolCall{
				CallID:    call.ID,
				Name:      call.Function.Name,
				Arguments: call.Function.Arguments,
			},
			Extra: cloneMap(call.Extra),
		}
		if call.Index != nil {
			block.StreamingMeta = &schema.StreamingMeta{Index: *call.Index}
		}
		blocks = append(blocks, block)
	}
	return blocks, nil
}

func assistantOutputPartToBlock(part schema.MessageOutputPart) (*schema.ContentBlock, error) {
	block := &schema.ContentBlock{
		Extra: cloneMap(part.Extra),
	}
	if part.StreamingMeta != nil {
		block.StreamingMeta = &schema.StreamingMeta{Index: part.StreamingMeta.Index}
	}

	switch part.Type {
	case schema.ChatMessagePartTypeText:
		block.Type = schema.ContentBlockTypeAssistantGenText
		block.AssistantGenText = &schema.AssistantGenText{Text: part.Text}
	case schema.ChatMessagePartTypeReasoning:
		if part.Reasoning == nil {
			return nil, fmt.Errorf("assistant reasoning part is nil")
		}
		block.Type = schema.ContentBlockTypeReasoning
		block.Reasoning = &schema.Reasoning{
			Text:      part.Reasoning.Text,
			Signature: part.Reasoning.Signature,
		}
	case schema.ChatMessagePartTypeImageURL:
		if part.Image == nil {
			return nil, fmt.Errorf("assistant image part is nil")
		}
		block.Type = schema.ContentBlockTypeAssistantGenImage
		block.AssistantGenImage = &schema.AssistantGenImage{
			URL:        stringValue(part.Image.URL),
			Base64Data: stringValue(part.Image.Base64Data),
			MIMEType:   part.Image.MIMEType,
		}
	case schema.ChatMessagePartTypeAudioURL:
		if part.Audio == nil {
			return nil, fmt.Errorf("assistant audio part is nil")
		}
		block.Type = schema.ContentBlockTypeAssistantGenAudio
		block.AssistantGenAudio = &schema.AssistantGenAudio{
			URL:        stringValue(part.Audio.URL),
			Base64Data: stringValue(part.Audio.Base64Data),
			MIMEType:   part.Audio.MIMEType,
		}
	case schema.ChatMessagePartTypeVideoURL:
		if part.Video == nil {
			return nil, fmt.Errorf("assistant video part is nil")
		}
		block.Type = schema.ContentBlockTypeAssistantGenVideo
		block.AssistantGenVideo = &schema.AssistantGenVideo{
			URL:        stringValue(part.Video.URL),
			Base64Data: stringValue(part.Video.Base64Data),
			MIMEType:   part.Video.MIMEType,
		}
	default:
		return nil, fmt.Errorf("unsupported assistant output part type %q", part.Type)
	}
	return block, nil
}

func agenticMessageToMessage(msg *schema.AgenticMessage) (*schema.Message, error) {
	if msg == nil {
		return nil, nil
	}

	out := &schema.Message{
		ResponseMeta: agenticResponseMetaToMessage(msg.ResponseMeta),
	}

	switch msg.Role {
	case schema.AgenticRoleTypeSystem:
		out.Role = schema.System
	case schema.AgenticRoleTypeUser:
		out.Role = schema.User
	case schema.AgenticRoleTypeAssistant:
		out.Role = schema.Assistant
	default:
		return nil, fmt.Errorf("unsupported agentic message role %q", msg.Role)
	}

	parts := make([]schema.MessageOutputPart, 0, len(msg.ContentBlocks))
	for _, block := range msg.ContentBlocks {
		if block == nil {
			continue
		}
		switch block.Type {
		case schema.ContentBlockTypeAssistantGenText:
			if block.AssistantGenText == nil {
				continue
			}
			parts = append(parts, schema.MessageOutputPart{
				Type:          schema.ChatMessagePartTypeText,
				Text:          block.AssistantGenText.Text,
				Extra:         cloneMap(block.Extra),
				StreamingMeta: messageStreamingMeta(block.StreamingMeta),
			})
		case schema.ContentBlockTypeReasoning:
			if block.Reasoning == nil {
				continue
			}
			parts = append(parts, schema.MessageOutputPart{
				Type: schema.ChatMessagePartTypeReasoning,
				Reasoning: &schema.MessageOutputReasoning{
					Text:      block.Reasoning.Text,
					Signature: block.Reasoning.Signature,
				},
				Extra:         cloneMap(block.Extra),
				StreamingMeta: messageStreamingMeta(block.StreamingMeta),
			})
		case schema.ContentBlockTypeAssistantGenImage:
			if block.AssistantGenImage == nil {
				continue
			}
			parts = append(parts, schema.MessageOutputPart{
				Type: schema.ChatMessagePartTypeImageURL,
				Image: &schema.MessageOutputImage{
					MessagePartCommon: schema.MessagePartCommon{
						URL:        stringPtr(block.AssistantGenImage.URL),
						Base64Data: stringPtr(block.AssistantGenImage.Base64Data),
						MIMEType:   block.AssistantGenImage.MIMEType,
					},
				},
				Extra:         cloneMap(block.Extra),
				StreamingMeta: messageStreamingMeta(block.StreamingMeta),
			})
		case schema.ContentBlockTypeAssistantGenAudio:
			if block.AssistantGenAudio == nil {
				continue
			}
			parts = append(parts, schema.MessageOutputPart{
				Type: schema.ChatMessagePartTypeAudioURL,
				Audio: &schema.MessageOutputAudio{
					MessagePartCommon: schema.MessagePartCommon{
						URL:        stringPtr(block.AssistantGenAudio.URL),
						Base64Data: stringPtr(block.AssistantGenAudio.Base64Data),
						MIMEType:   block.AssistantGenAudio.MIMEType,
					},
				},
				Extra:         cloneMap(block.Extra),
				StreamingMeta: messageStreamingMeta(block.StreamingMeta),
			})
		case schema.ContentBlockTypeAssistantGenVideo:
			if block.AssistantGenVideo == nil {
				continue
			}
			parts = append(parts, schema.MessageOutputPart{
				Type: schema.ChatMessagePartTypeVideoURL,
				Video: &schema.MessageOutputVideo{
					MessagePartCommon: schema.MessagePartCommon{
						URL:        stringPtr(block.AssistantGenVideo.URL),
						Base64Data: stringPtr(block.AssistantGenVideo.Base64Data),
						MIMEType:   block.AssistantGenVideo.MIMEType,
					},
				},
				Extra:         cloneMap(block.Extra),
				StreamingMeta: messageStreamingMeta(block.StreamingMeta),
			})
		case schema.ContentBlockTypeFunctionToolCall:
			if block.FunctionToolCall == nil {
				continue
			}
			call := schema.ToolCall{
				ID: block.FunctionToolCall.CallID,
				Function: schema.FunctionCall{
					Name:      block.FunctionToolCall.Name,
					Arguments: block.FunctionToolCall.Arguments,
				},
				Type:  "function",
				Extra: cloneMap(block.Extra),
			}
			if block.StreamingMeta != nil {
				index := block.StreamingMeta.Index
				call.Index = &index
			}
			out.ToolCalls = append(out.ToolCalls, call)
		}
	}
	if len(parts) > 0 {
		out.AssistantGenMultiContent = parts
	}
	return out, nil
}

func agenticResponseMetaToMessage(meta *schema.AgenticResponseMeta) *schema.ResponseMeta {
	if meta == nil || meta.TokenUsage == nil {
		return nil
	}
	return &schema.ResponseMeta{
		Usage: meta.TokenUsage,
	}
}

func messageStreamingMeta(meta *schema.StreamingMeta) *schema.MessageStreamingMeta {
	if meta == nil {
		return nil
	}
	return &schema.MessageStreamingMeta{Index: meta.Index}
}

func hasReasoningBlock(blocks []*schema.ContentBlock) bool {
	for _, block := range blocks {
		if block != nil && block.Type == schema.ContentBlockTypeReasoning {
			return true
		}
	}
	return false
}

func cloneMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func stringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
