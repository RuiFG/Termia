package agent

import (
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"
)

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
			if text := strings.TrimSpace(block.Reasoning.Text); text != "" {
				if out.ReasoningContent != "" {
					out.ReasoningContent += "\n"
				}
				out.ReasoningContent += text
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
