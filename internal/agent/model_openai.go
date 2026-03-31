package agent

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"iter"
	"os"
	"strconv"
	"strings"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	adkmodel "google.golang.org/adk/model"
	"google.golang.org/adk/tool/toolconfirmation"
	"google.golang.org/genai"
)

type openAIModel struct {
	client    openai.Client
	modelName string
}

const maxOpenAIToolCallIDLen = 40

func NewOpenAIModel(spec ModelSpec) (adkmodel.LLM, error) {
	if strings.TrimSpace(spec.Model) == "" {
		return nil, fmt.Errorf("model name is required")
	}

	apiKey := strings.TrimSpace(os.Getenv(spec.APIKeyEnv))
	opts := []option.RequestOption{}
	if apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	}
	if strings.TrimSpace(spec.BaseURL) != "" {
		opts = append(opts, option.WithBaseURL(spec.BaseURL))
	}

	client := openai.NewClient(opts...)
	return &openAIModel{client: client, modelName: spec.Model}, nil
}

func (m *openAIModel) Name() string { return m.modelName }

func (m *openAIModel) GenerateContent(ctx context.Context, req *adkmodel.LLMRequest, stream bool) iter.Seq2[*adkmodel.LLMResponse, error] {
	return func(yield func(*adkmodel.LLMResponse, error) bool) {
		messages, err := contentsToOpenAIMessages(req)
		if err != nil {
			yield(nil, fmt.Errorf("build messages: %w", err))
			return
		}

		tools := buildOpenAITools(req)
		params := openai.ChatCompletionNewParams{
			Model:    m.modelName,
			Messages: messages,
		}
		if len(tools) > 0 {
			params.Tools = tools
		}
		if req.Config != nil {
			if req.Config.Temperature != nil {
				params.Temperature = openai.Float(float64(*req.Config.Temperature))
			}
			if req.Config.MaxOutputTokens > 0 {
				params.MaxTokens = openai.Int(int64(req.Config.MaxOutputTokens))
			}
		}

		if stream {
			m.generateStream(ctx, params, yield)
			return
		}
		m.generateNonStream(ctx, params, yield)
	}
}

func (m *openAIModel) generateNonStream(ctx context.Context, params openai.ChatCompletionNewParams, yield func(*adkmodel.LLMResponse, error) bool) {
	resp, err := m.client.Chat.Completions.New(ctx, params)
	if err != nil {
		yield(nil, err)
		return
	}
	if len(resp.Choices) == 0 {
		yield(&adkmodel.LLMResponse{TurnComplete: true}, nil)
		return
	}

	content := openAIChoiceToGenaiContent(resp.Choices[0].Message)
	yield(&adkmodel.LLMResponse{
		Content:      content,
		TurnComplete: true,
		FinishReason: genai.FinishReasonStop,
	}, nil)
}

func (m *openAIModel) generateStream(ctx context.Context, params openai.ChatCompletionNewParams, yield func(*adkmodel.LLMResponse, error) bool) {
	stream := m.client.Chat.Completions.NewStreaming(ctx, params)
	defer stream.Close()

	acc := openai.ChatCompletionAccumulator{}
	for stream.Next() {
		chunk := stream.Current()
		acc.AddChunk(chunk)
		if len(chunk.Choices) == 0 {
			continue
		}
		text := chunk.Choices[0].Delta.Content
		if text == "" {
			continue
		}
		if !yield(&adkmodel.LLMResponse{
			Content: &genai.Content{
				Role:  "model",
				Parts: []*genai.Part{{Text: text}},
			},
			Partial: true,
		}, nil) {
			return
		}
	}

	if err := stream.Err(); err != nil {
		yield(nil, err)
		return
	}

	if len(acc.Choices) == 0 {
		yield(&adkmodel.LLMResponse{TurnComplete: true}, nil)
		return
	}

	yield(&adkmodel.LLMResponse{
		Content:      openAIChoiceToGenaiContent(acc.Choices[0].Message),
		TurnComplete: true,
		FinishReason: genai.FinishReasonStop,
	}, nil)
}

func contentsToOpenAIMessages(req *adkmodel.LLMRequest) ([]openai.ChatCompletionMessageParamUnion, error) {
	var msgs []openai.ChatCompletionMessageParamUnion

	if req.Config != nil && req.Config.SystemInstruction != nil {
		text := extractText(req.Config.SystemInstruction)
		if text != "" {
			msgs = append(msgs, openai.SystemMessage(text))
		}
	}

	for _, c := range req.Contents {
		if c == nil {
			continue
		}
		converted, err := contentToOpenAIMessages(c)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, converted...)
	}

	return msgs, nil
}

func contentToOpenAIMessages(c *genai.Content) ([]openai.ChatCompletionMessageParamUnion, error) {
	switch strings.ToLower(c.Role) {
	case "user":
		return userContentToMessages(c)
	case "model":
		return modelContentToMessages(c)
	default:
		text := extractText(c)
		if text == "" {
			return nil, nil
		}
		return []openai.ChatCompletionMessageParamUnion{openai.UserMessage(text)}, nil
	}
}

func userContentToMessages(c *genai.Content) ([]openai.ChatCompletionMessageParamUnion, error) {
	var toolMsgs []openai.ChatCompletionMessageParamUnion
	var textParts []string
	for _, p := range c.Parts {
		if p == nil {
			continue
		}
		if p.FunctionResponse != nil {
			fr := p.FunctionResponse
			toolMsgs = append(toolMsgs, openai.ToolMessage(formatOpenAIToolResponse(fr), openAIToolCallID(fr.ID, fr.Name)))
			continue
		}
		if p.Text != "" {
			textParts = append(textParts, p.Text)
		}
	}

	var result []openai.ChatCompletionMessageParamUnion
	if len(textParts) > 0 {
		result = append(result, openai.UserMessage(strings.Join(textParts, "\n")))
	}
	result = append(result, toolMsgs...)
	return result, nil
}

func formatOpenAIToolResponse(fr *genai.FunctionResponse) string {
	if fr == nil {
		return "{}"
	}
	data, err := json.Marshal(fr.Response)
	if err != nil {
		data = []byte("{}")
	}
	if strings.TrimSpace(fr.Name) != toolconfirmation.FunctionCallName {
		return string(data)
	}

	responseMap := fr.Response
	confirmed := toolBoolArg(responseMap["confirmed"])
	payload, _ := responseMap["payload"].(map[string]any)
	toolName := toolStringArg(payload, "original_tool")
	command := toolStringArg(payload, "command")
	if confirmed {
		summary := "User approved this tool call."
		if toolName != "" {
			summary = "User approved tool " + strconv.Quote(toolName) + "."
		}
		if command != "" {
			summary = "User approved command " + strconv.Quote(command) + "."
		}
		return summary + "\n" + string(data)
	}
	summary := "User rejected this tool call. Do not execute it."
	switch {
	case command != "":
		summary = "User rejected command " + strconv.Quote(command) + ". Do not execute it."
	case toolName != "":
		summary = "User rejected tool " + strconv.Quote(toolName) + ". Do not execute it."
	}
	return summary + " Ask for an alternative if needed.\n" + string(data)
}

func modelContentToMessages(c *genai.Content) ([]openai.ChatCompletionMessageParamUnion, error) {
	var textParts []string
	var toolCalls []openai.ChatCompletionMessageToolCallParam

	for _, p := range c.Parts {
		if p == nil || p.Thought {
			continue
		}
		if p.Text != "" {
			textParts = append(textParts, p.Text)
		}
		if p.FunctionCall != nil {
			args, err := json.Marshal(p.FunctionCall.Args)
			if err != nil {
				args = []byte("{}")
			}
			id := openAIToolCallID(p.FunctionCall.ID, p.FunctionCall.Name)
			toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCallParam{
				ID:   id,
				Type: "function",
				Function: openai.ChatCompletionMessageToolCallFunctionParam{
					Name:      p.FunctionCall.Name,
					Arguments: string(args),
				},
			})
		}
	}

	text := strings.Join(textParts, "")
	if len(toolCalls) > 0 {
		msg := openai.ChatCompletionAssistantMessageParam{ToolCalls: toolCalls}
		if text != "" {
			msg.Content = openai.ChatCompletionAssistantMessageParamContentUnion{OfString: openai.String(text)}
		}
		return []openai.ChatCompletionMessageParamUnion{{OfAssistant: &msg}}, nil
	}
	if text == "" {
		return nil, nil
	}
	return []openai.ChatCompletionMessageParamUnion{openai.AssistantMessage(text)}, nil
}

func buildOpenAITools(req *adkmodel.LLMRequest) []openai.ChatCompletionToolParam {
	if req.Config == nil || len(req.Config.Tools) == 0 {
		return nil
	}

	var result []openai.ChatCompletionToolParam
	for _, t := range req.Config.Tools {
		if t == nil {
			continue
		}
		for _, decl := range t.FunctionDeclarations {
			if decl == nil {
				continue
			}
			result = append(result, openai.ChatCompletionToolParam{
				Type: "function",
				Function: openai.FunctionDefinitionParam{
					Name:        decl.Name,
					Description: openai.String(decl.Description),
					Parameters:  buildFunctionParams(decl),
				},
			})
		}
	}
	return result
}

func buildFunctionParams(decl *genai.FunctionDeclaration) openai.FunctionParameters {
	if decl.ParametersJsonSchema != nil {
		data, err := json.Marshal(decl.ParametersJsonSchema)
		if err == nil {
			var raw map[string]any
			if json.Unmarshal(data, &raw) == nil {
				return openai.FunctionParameters(sanitizeSchemaForOpenAI(raw, true))
			}
		}
	}
	if decl.Parameters != nil {
		data, err := json.Marshal(decl.Parameters)
		if err == nil {
			var raw map[string]any
			if json.Unmarshal(data, &raw) == nil {
				return openai.FunctionParameters(sanitizeSchemaForOpenAI(raw, true))
			}
		}
	}
	return openai.FunctionParameters{"type": "object", "properties": map[string]any{}}
}

func sanitizeSchemaForOpenAI(schema map[string]any, topLevel bool) map[string]any {
	result := make(map[string]any, len(schema))
	for k, v := range schema {
		switch k {
		case "additionalProperties":
			if _, isBool := v.(bool); isBool {
				continue
			}
			if sub, ok := v.(map[string]any); ok {
				result[k] = sanitizeSchemaForOpenAI(sub, false)
			}
		case "type":
			result[k] = collapseNullableType(v)
		case "properties":
			if props, ok := v.(map[string]any); ok {
				cleaned := make(map[string]any, len(props))
				for pk, pv := range props {
					if sub, ok := pv.(map[string]any); ok {
						cleaned[pk] = sanitizeSchemaForOpenAI(sub, false)
					} else {
						cleaned[pk] = pv
					}
				}
				result[k] = cleaned
			} else {
				result[k] = v
			}
		case "items":
			if sub, ok := v.(map[string]any); ok {
				result[k] = sanitizeSchemaForOpenAI(sub, false)
			} else {
				result[k] = v
			}
		default:
			result[k] = v
		}
	}

	if topLevel {
		if _, ok := result["type"]; !ok {
			result["type"] = "object"
		}
		if _, ok := result["properties"]; !ok {
			result["properties"] = map[string]any{}
		}
	}
	return result
}

func collapseNullableType(v any) any {
	switch t := v.(type) {
	case string:
		return t
	case []any:
		for _, item := range t {
			if s, ok := item.(string); ok && s != "null" {
				return s
			}
		}
		if len(t) > 0 {
			return t[0]
		}
	}
	return v
}

func openAIChoiceToGenaiContent(msg openai.ChatCompletionMessage) *genai.Content {
	var parts []*genai.Part
	if msg.Content != "" {
		parts = append(parts, &genai.Part{Text: msg.Content})
	}
	for _, tc := range msg.ToolCalls {
		var args map[string]any
		if tc.Function.Arguments != "" {
			_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
		}
		if args == nil {
			args = map[string]any{}
		}
		parts = append(parts, &genai.Part{
			FunctionCall: &genai.FunctionCall{
				ID:   tc.ID,
				Name: tc.Function.Name,
				Args: args,
			},
		})
	}
	return &genai.Content{Role: "model", Parts: parts}
}

func extractText(c *genai.Content) string {
	if c == nil {
		return ""
	}
	var sb strings.Builder
	for _, p := range c.Parts {
		if p != nil && p.Text != "" {
			sb.WriteString(p.Text)
		}
	}
	return sb.String()
}

func openAIToolCallID(id, name string) string {
	raw := strings.TrimSpace(id)
	if raw == "" {
		raw = "call_" + strings.TrimSpace(name)
	}
	sanitized := sanitizeOpenAIToolCallID(raw)
	if sanitized != "" && len(sanitized) <= maxOpenAIToolCallIDLen {
		return sanitized
	}
	prefix := sanitizeOpenAIToolCallID(name)
	if prefix == "" {
		prefix = "call"
	}
	if len(prefix) > 7 {
		prefix = prefix[:7]
	}
	sum := sha1.Sum([]byte(raw))
	return prefix + "_" + hex.EncodeToString(sum[:16])
}

func sanitizeOpenAIToolCallID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var sb strings.Builder
	sb.Grow(len(value))
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			sb.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			sb.WriteRune(r + ('a' - 'A'))
		case r >= '0' && r <= '9':
			sb.WriteRune(r)
		case r == '_' || r == '-':
			sb.WriteRune(r)
		}
	}
	return sb.String()
}
