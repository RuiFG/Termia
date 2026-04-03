package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

func consumeRuntimeIterator(
	iter *adk.AsyncIterator[*adk.AgentEvent],
	output chan<- RuntimeEvent,
	seenToolCalls map[string]bool,
	seenToolResults map[string]bool,
	turnMessages *[]adk.Message,
) (*HITLRequest, error) {
	for {
		event, ok := iter.Next()
		if !ok {
			return nil, nil
		}
		if event == nil {
			continue
		}
		if event.Err != nil {
			return nil, event.Err
		}

		msg, err := collectEventMessage(event, output, seenToolCalls, seenToolResults)
		if err != nil {
			return nil, err
		}
		if msg != nil {
			*turnMessages = append(*turnMessages, msg)
		}

		if request, ok := parseHITLRequest(event); ok {
			return &request, nil
		}
	}
}

func collectEventMessage(
	event *adk.AgentEvent,
	output chan<- RuntimeEvent,
	seenToolCalls map[string]bool,
	seenToolResults map[string]bool,
) (adk.Message, error) {
	if event == nil || event.Output == nil || event.Output.MessageOutput == nil {
		return nil, nil
	}

	msg, emittedContent, err := materializeMessage(event.Output.MessageOutput, output)
	if err != nil {
		return nil, err
	}
	if msg == nil {
		return nil, nil
	}

	emitMessageEvents(event.AgentName, msg, output, seenToolCalls, seenToolResults)
	for _, contentEvent := range assistantContentEvents(msg) {
		if emittedContent.saw(contentEvent.Kind) {
			continue
		}
		output <- contentEvent
		emittedContent.mark(contentEvent.Kind)
	}

	return msg, nil
}

func materializeMessage(mv *adk.MessageVariant, output chan<- RuntimeEvent) (adk.Message, emittedAssistantContent, error) {
	if mv == nil {
		return nil, emittedAssistantContent{}, nil
	}
	if !mv.IsStreaming {
		return mv.Message, emittedAssistantContent{}, nil
	}

	defer mv.MessageStream.Close()

	var (
		chunks         []*schema.Message
		emittedContent emittedAssistantContent
	)
	for {
		chunk, err := mv.MessageStream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, emittedContent, err
		}
		if chunk == nil {
			continue
		}

		chunks = append(chunks, chunk)
		for _, contentEvent := range assistantContentEvents(chunk) {
			output <- contentEvent
			emittedContent.mark(contentEvent.Kind)
		}
	}

	if len(chunks) == 0 {
		return nil, emittedContent, nil
	}

	msg, err := schema.ConcatMessages(chunks)
	if err != nil {
		return nil, emittedContent, err
	}
	return msg, emittedContent, nil
}

func emitMessageEvents(
	agentName string,
	msg *schema.Message,
	output chan<- RuntimeEvent,
	seenToolCalls map[string]bool,
	seenToolResults map[string]bool,
) {
	for _, toolCall := range extractToolCallEvents(agentName, msg) {
		callID := toolCall.CallID
		if callID == "" {
			callID = toolCall.AgentName + ":" + toolCall.ToolName + ":" + toolCall.Summary
		}
		if seenToolCalls[callID] {
			continue
		}
		seenToolCalls[callID] = true
		output <- RuntimeEvent{Kind: RuntimeEventToolCall, ToolCall: &toolCall}
	}

	for _, toolCall := range extractToolResultEvents(agentName, msg) {
		callID := toolCall.CallID
		if callID == "" {
			callID = toolCall.AgentName + ":" + toolCall.ToolName + ":" + toolCall.Summary + ":" + string(toolCall.State)
		}
		if seenToolResults[callID] {
			continue
		}
		seenToolResults[callID] = true
		output <- RuntimeEvent{Kind: RuntimeEventToolResult, ToolCall: &toolCall}
	}

	if cwd, ok := extractCommandCwdEvent(msg); ok {
		output <- RuntimeEvent{Kind: RuntimeEventCwd, Cwd: cwd}
	}
}

type emittedAssistantContent struct {
	text      bool
	reasoning bool
}

func (e *emittedAssistantContent) mark(kind RuntimeEventKind) {
	switch kind {
	case RuntimeEventText:
		e.text = true
	case RuntimeEventReasoning:
		e.reasoning = true
	}
}

func (e emittedAssistantContent) saw(kind RuntimeEventKind) bool {
	switch kind {
	case RuntimeEventText:
		return e.text
	case RuntimeEventReasoning:
		return e.reasoning
	default:
		return false
	}
}

func assistantContentEvents(msg *schema.Message) []RuntimeEvent {
	if msg == nil || msg.Role != schema.Assistant {
		return nil
	}

	events := make([]RuntimeEvent, 0, len(msg.AssistantGenMultiContent)+2)
	if len(msg.AssistantGenMultiContent) > 0 {
		for _, part := range msg.AssistantGenMultiContent {
			switch {
			case part.Type == schema.ChatMessagePartTypeReasoning && part.Reasoning != nil && part.Reasoning.Text != "":
				events = append(events, RuntimeEvent{Kind: RuntimeEventReasoning, Text: part.Reasoning.Text})
			case part.Type == schema.ChatMessagePartTypeText && part.Text != "":
				events = append(events, RuntimeEvent{Kind: RuntimeEventText, Text: part.Text})
			case part.Reasoning != nil && part.Reasoning.Text != "":
				events = append(events, RuntimeEvent{Kind: RuntimeEventReasoning, Text: part.Reasoning.Text})
			case part.Text != "":
				events = append(events, RuntimeEvent{Kind: RuntimeEventText, Text: part.Text})
			}
		}
		if len(events) > 0 {
			return events
		}
	}

	if msg.ReasoningContent != "" {
		events = append(events, RuntimeEvent{Kind: RuntimeEventReasoning, Text: msg.ReasoningContent})
	}
	if msg.Content != "" {
		events = append(events, RuntimeEvent{Kind: RuntimeEventText, Text: msg.Content})
	}
	if len(events) == 0 {
		return nil
	}
	return events
}

func extractToolCallEvents(agentName string, msg *schema.Message) []ToolCallEvent {
	if msg == nil || msg.Role != schema.Assistant || len(msg.ToolCalls) == 0 {
		return nil
	}

	agentName = strings.TrimSpace(agentName)
	calls := make([]ToolCallEvent, 0, len(msg.ToolCalls))
	for _, call := range msg.ToolCalls {
		name := strings.TrimSpace(call.Function.Name)
		if name == "" {
			continue
		}
		args, rawArgs := decodeJSONObject(call.Function.Arguments)
		summary := summarizeToolCall(name, args)
		if summary == "" {
			summary = rawArgs
		}
		calls = append(calls, ToolCallEvent{
			CallID:    strings.TrimSpace(call.ID),
			AgentName: agentName,
			ToolName:  name,
			Summary:   summary,
			State:     ToolCallStatePending,
		})
	}
	if len(calls) == 0 {
		return nil
	}
	return calls
}

func extractToolResultEvents(agentName string, msg *schema.Message) []ToolCallEvent {
	if msg == nil || msg.Role != schema.Tool || strings.TrimSpace(msg.ToolName) == "" {
		return nil
	}

	name := strings.TrimSpace(msg.ToolName)
	response, raw := decodeJSONObject(msg.Content)
	result := summarizeToolResult(name, response)
	if result == "" {
		result = raw
	}

	return []ToolCallEvent{{
		CallID:    strings.TrimSpace(msg.ToolCallID),
		AgentName: strings.TrimSpace(agentName),
		ToolName:  name,
		Summary:   summarizeToolResultTarget(name, response),
		Result:    result,
		State:     summarizeToolResultState(name, response),
	}}
}

func extractCommandCwdEvent(msg *schema.Message) (string, bool) {
	if msg == nil || msg.Role != schema.Tool || strings.TrimSpace(msg.ToolName) != "command" {
		return "", false
	}

	response, _ := decodeJSONObject(msg.Content)
	rawChanged, ok := response["cwd_changed"]
	if !ok || !toolBoolArg(rawChanged) {
		return "", false
	}
	rawCwd, ok := response["cwd"]
	if !ok {
		return "", false
	}
	cwd, ok := rawCwd.(string)
	if !ok || strings.TrimSpace(cwd) == "" {
		return "", false
	}
	return strings.TrimSpace(cwd), true
}

func decodeJSONObject(raw string) (map[string]any, string) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, ""
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(trimmed), &decoded); err == nil {
		return decoded, trimmed
	}
	return nil, trimmed
}

func summarizeToolCall(name string, args map[string]any) string {
	switch strings.TrimSpace(name) {
	case "command":
		if command := toolStringArg(args, "command"); command != "" {
			return command
		}
	case "read_file", "list_dir", "stream_read":
		if path := toolStringArg(args, "path"); path != "" {
			return path
		}
	case "read_files":
		paths := toolStringSliceArg(args, "paths")
		switch len(paths) {
		case 0:
		case 1:
			return paths[0]
		default:
			return fmt.Sprintf("%d paths", len(paths))
		}
	case "inspect_command_output":
		commandID := toolStringArg(args, "command_id")
		query := toolStringArg(args, "query")
		switch {
		case commandID != "" && query != "":
			return fmt.Sprintf("%s matching %q", commandID, query)
		case commandID != "":
			return commandID
		case query != "":
			return fmt.Sprintf("matching %q", query)
		}
	case "request_input":
		return "requested user input"
	}
	return compactToolArgs(args)
}

func summarizeToolResultTarget(name string, response map[string]any) string {
	switch strings.TrimSpace(name) {
	case "command":
		if command := toolStringArg(response, "command"); command != "" {
			return command
		}
	case "read_file", "list_dir", "stream_read":
		if path := toolStringArg(response, "path"); path != "" {
			return path
		}
	case "inspect_command_output":
		command := toolStringArg(response, "command")
		commandID := toolStringArg(response, "command_id")
		switch {
		case command != "":
			return command
		case commandID != "":
			return commandID
		}
	}
	return ""
}

func summarizeToolResult(name string, response map[string]any) string {
	switch strings.TrimSpace(name) {
	case "command":
		exitCode, ok := toolIntArg(response["exit_code"])
		if !ok {
			return ""
		}
		if exitCode == 0 {
			return "ok"
		}
		return fmt.Sprintf("exit %d", exitCode)
	case "request_input":
		if toolBoolArg(response["cancelled"]) {
			return "cancelled"
		}
		if answers, ok := response["answers"].([]any); ok && len(answers) > 0 {
			if len(answers) == 1 {
				return "answered 1 question"
			}
			return fmt.Sprintf("answered %d questions", len(answers))
		}
		return "answered"
	case "inspect_command_output":
		if count, ok := toolIntArg(response["match_count"]); ok && count > 0 {
			if count == 1 {
				return "1 match"
			}
			return fmt.Sprintf("%d matches", count)
		}
		if lines, ok := toolIntArg(response["lines_read"]); ok && lines > 0 {
			if lines == 1 {
				return "1 line"
			}
			return fmt.Sprintf("%d lines", lines)
		}
	case "list_dir":
		if count, ok := toolSliceLen(response["entries"]); ok {
			if count == 1 {
				return "1 entry"
			}
			return fmt.Sprintf("%d entries", count)
		}
	case "read_files":
		if count, ok := toolSliceLen(response["files"]); ok {
			if count == 1 {
				return "1 file"
			}
			return fmt.Sprintf("%d files", count)
		}
	case "read_file", "stream_read":
		if lines, ok := toolIntArg(response["lines_read"]); ok && lines > 0 {
			if lines == 1 {
				return "1 line"
			}
			return fmt.Sprintf("%d lines", lines)
		}
		if bytes, ok := toolIntArg(response["bytes_read"]); ok && bytes > 0 {
			return fmt.Sprintf("%d bytes", bytes)
		}
	}
	return "done"
}

func summarizeToolResultState(name string, response map[string]any) ToolCallState {
	if errText := toolStringArg(response, "error"); errText != "" {
		return ToolCallStateError
	}
	switch strings.TrimSpace(name) {
	case "command":
		exitCode, ok := toolIntArg(response["exit_code"])
		if ok && exitCode != 0 {
			return ToolCallStateError
		}
	}
	return ToolCallStateSuccess
}

func toolStringArg(args map[string]any, key string) string {
	if len(args) == 0 {
		return ""
	}
	raw, ok := args[key]
	if !ok {
		return ""
	}
	value, ok := raw.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func toolStringSliceArg(args map[string]any, key string) []string {
	if len(args) == 0 {
		return nil
	}
	raw, ok := args[key]
	if !ok || raw == nil {
		return nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		value, ok := item.(string)
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		result = append(result, value)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func compactToolArgs(args map[string]any) string {
	if len(args) == 0 {
		return ""
	}
	var fields []string
	for _, key := range []string{"path", "command_id", "cwd", "query"} {
		if value := toolStringArg(args, key); value != "" {
			fields = append(fields, fmt.Sprintf("%s=%q", key, value))
		}
	}
	if len(fields) == 0 {
		return "running"
	}
	return strings.Join(fields, " ")
}

func toolIntArg(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case float32:
		return int(typed), true
	default:
		return 0, false
	}
}

func toolSliceLen(value any) (int, bool) {
	items, ok := value.([]any)
	if !ok {
		return 0, false
	}
	return len(items), true
}

func toolBoolArg(value any) bool {
	typed, ok := value.(bool)
	return ok && typed
}
