package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	adkagent "google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool/toolconfirmation"
	"google.golang.org/genai"

	"github.com/termia/termia/internal/config"
	"github.com/termia/termia/internal/db"
)

type Runtime struct {
	cfg       *config.Config
	db        *db.DB
	responder HITLResponder
}

func NewRuntime(cfg *config.Config, database *db.DB, responder HITLResponder) *Runtime {
	return &Runtime{
		cfg:       cfg,
		db:        database,
		responder: responder,
	}
}

func (r *Runtime) Run(ctx context.Context, req RunRequest) (<-chan RuntimeEvent, error) {
	root, err := r.buildRootAgent(ctx, req)
	if err != nil {
		return nil, err
	}
	runr, err := runner.New(runner.Config{
		AppName:           defaultAppName,
		Agent:             root,
		SessionService:    session.InMemoryService(),
		AutoCreateSession: true,
	})
	if err != nil {
		return nil, fmt.Errorf("create runner: %w", err)
	}

	if req.SessionID == "" {
		req.SessionID = newSessionID()
	}

	output := make(chan RuntimeEvent, 32)
	go func() {
		defer close(output)
		_ = r.runConversation(ctx, runr, req, output)
	}()
	return output, nil
}

func (r *Runtime) runConversation(ctx context.Context, runr *runner.Runner, req RunRequest, output chan<- RuntimeEvent) error {
	next := buildPromptContent(req)
	maxLines, wait := streamDefaults(req)
	if req.StreamReader != nil {
		chunk, err := req.StreamReader.NextChunk(ctx, maxLines, wait)
		if err != nil && err != io.EOF {
			output <- RuntimeEvent{Kind: RuntimeEventError, Text: fmt.Sprintf("Error: %v", err)}
			return err
		}
		if chunk.Text != "" {
			next = buildStreamPromptContent(req, chunk, true)
		}
	}

	for next != nil {
		followUp, err := r.runTurn(ctx, runr, req.SessionID, next, output)
		if err != nil {
			output <- RuntimeEvent{Kind: RuntimeEventError, Text: fmt.Sprintf("Error: %v", err)}
			return err
		}
		if followUp != nil {
			next = followUp
			continue
		}

		if req.StreamReader == nil {
			next = nil
			continue
		}
		chunk, err := req.StreamReader.NextChunk(ctx, maxLines, wait)
		if err != nil {
			if err == io.EOF {
				if !req.StreamReader.CloseMessage() {
					next = nil
					continue
				}
				next = genai.NewContentFromText("The stream source has reached EOF. Provide a concise final assessment if needed.", genai.RoleUser)
				req.StreamReader = nil
				continue
			}
			output <- RuntimeEvent{Kind: RuntimeEventError, Text: fmt.Sprintf("Error: %v", err)}
			return err
		}
		if chunk.Text == "" {
			next = nil
			continue
		}
		next = buildStreamPromptContent(req, chunk, false)
	}
	return nil
}

func (r *Runtime) runTurn(ctx context.Context, runr *runner.Runner, sessionID string, content *genai.Content, output chan<- RuntimeEvent) (*genai.Content, error) {
	sawPartial := false
	var followUp *genai.Content
	seenToolCalls := make(map[string]bool)
	seenToolResults := make(map[string]bool)

	for event, err := range runr.Run(ctx, defaultUserID, sessionID, content, adkagent.RunConfig{
		StreamingMode: adkagent.StreamingModeSSE,
	}) {
		if err != nil {
			return nil, err
		}
		if event == nil || event.Content == nil {
			continue
		}

		if request, ok := parseHITLRequest(event); ok {
			if r.responder == nil {
				return nil, fmt.Errorf("hitl required for tool %s but no responder is configured", request.OriginalTool)
			}
			response, err := r.responder.Handle(ctx, request)
			if err != nil {
				return nil, err
			}
			followUp = buildConfirmationResponseContent(request, response)
			continue
		}

		for _, toolCall := range extractToolCallEvents(event) {
			callID := toolCall.CallID
			if callID == "" {
				callID = toolCall.AgentName + ":" + toolCall.ToolName + ":" + toolCall.Summary
			}
			if seenToolCalls[callID] {
				continue
			}
			seenToolCalls[callID] = true
			output <- RuntimeEvent{
				Kind:     RuntimeEventToolCall,
				ToolCall: &toolCall,
			}
		}
		for _, toolCall := range extractToolResultEvents(event) {
			callID := toolCall.CallID
			if callID == "" {
				callID = toolCall.AgentName + ":" + toolCall.ToolName + ":" + toolCall.Summary + ":" + string(toolCall.State)
			}
			if seenToolResults[callID] {
				continue
			}
			seenToolResults[callID] = true
			output <- RuntimeEvent{
				Kind:     RuntimeEventToolResult,
				ToolCall: &toolCall,
			}
		}
		if cwd, ok := extractCommandCwdEvent(event); ok {
			output <- RuntimeEvent{
				Kind: RuntimeEventCwd,
				Cwd:  cwd,
			}
		}

		text := contentText(event.Content)
		if event.Partial {
			sawPartial = sawPartial || strings.TrimSpace(text) != ""
			if text != "" {
				output <- RuntimeEvent{Kind: RuntimeEventText, Text: text}
			}
			continue
		}
		if !sawPartial && text != "" && event.IsFinalResponse() {
			output <- RuntimeEvent{Kind: RuntimeEventText, Text: text}
		}
	}
	return followUp, nil
}

func (r *Runtime) buildRootAgent(ctx context.Context, req RunRequest) (adkagent.Agent, error) {
	registry, err := NewToolRegistry(r.db)
	if err != nil {
		return nil, err
	}

	mode := req.Mode
	if mode == "" && r.cfg != nil {
		mode = Mode(strings.ToLower(strings.TrimSpace(r.cfg.Agent.DefaultMode)))
	}
	if mode == "" {
		mode = ModeAssistant
	}

	switch mode {
	case ModeTeam:
		return r.buildTeamAgent(ctx, req.TeamName, registry)
	default:
		return r.buildAssistantAgent(ctx, registry)
	}
}

func (r *Runtime) buildAssistantAgent(ctx context.Context, registry *ToolRegistry) (adkagent.Agent, error) {
	spec, err := AssistantSpecFromConfig(r.cfg)
	if err != nil {
		return nil, err
	}
	model, err := NewModel(ctx, spec.Model)
	if err != nil {
		return nil, err
	}
	return llmagent.New(llmagent.Config{
		Name:        spec.Name,
		Description: spec.Description,
		Instruction: spec.Instruction,
		Model:       model,
		Tools:       registry.Filter(spec.Tools),
	})
}

func (r *Runtime) buildTeamAgent(ctx context.Context, teamName string, registry *ToolRegistry) (adkagent.Agent, error) {
	if strings.TrimSpace(teamName) == "" {
		if r.cfg != nil {
			teamName = r.cfg.Agent.DefaultTeam
		}
	}
	if strings.TrimSpace(teamName) == "" {
		return nil, fmt.Errorf("team mode requires --team or agent.default_team")
	}

	spec, err := LoadTeamByName(r.cfg, teamName)
	if err != nil {
		return nil, err
	}

	subAgents := make([]adkagent.Agent, 0, len(spec.Agents))
	for _, member := range spec.Agents {
		model, err := NewModel(ctx, member.Model)
		if err != nil {
			return nil, fmt.Errorf("member %s: %w", member.Name, err)
		}
		agentTools := registry.Filter(member.Tools)
		subAgent, err := llmagent.New(llmagent.Config{
			Name:        member.Name,
			Description: member.Description,
			Instruction: member.Instruction,
			Model:       model,
			Tools:       agentTools,
		})
		if err != nil {
			return nil, err
		}
		subAgents = append(subAgents, subAgent)
	}

	model, err := NewModel(ctx, spec.Coordinator.Model)
	if err != nil {
		return nil, fmt.Errorf("coordinator: %w", err)
	}
	instruction := spec.Coordinator.Instruction
	if strings.TrimSpace(instruction) == "" {
		instruction = DefaultCoordinatorInstruction
	}
	return llmagent.New(llmagent.Config{
		Name:        spec.Coordinator.Name,
		Description: spec.Coordinator.Description,
		Instruction: instruction,
		Model:       model,
		Tools:       registry.Filter(spec.Coordinator.Tools),
		SubAgents:   subAgents,
	})
}

func streamDefaults(req RunRequest) (int, time.Duration) {
	maxLines := req.StreamChunkLines
	if maxLines <= 0 {
		maxLines = 120
	}
	wait := req.StreamChunkWait
	if wait <= 0 {
		wait = 3 * time.Second
	}
	return maxLines, wait
}

func buildPromptContent(req RunRequest) *genai.Content {
	return genai.NewContentFromText(buildPromptText(req, ""), genai.RoleUser)
}

func buildStreamPromptContent(req RunRequest, chunk StreamChunk, first bool) *genai.Content {
	title := "New stream chunk"
	if first {
		title = "Initial stream chunk"
	}
	body := fmt.Sprintf("%s:\n%s", title, strings.TrimSpace(chunk.Text))
	return genai.NewContentFromText(buildPromptText(req, body), genai.RoleUser)
}

func buildPromptText(req RunRequest, streamChunk string) string {
	var sb strings.Builder
	if strings.TrimSpace(req.Cwd) != "" {
		sb.WriteString("Current working directory:\n")
		sb.WriteString(req.Cwd)
		sb.WriteString("\n\n")
	}
	if len(req.Messages) > 0 {
		sb.WriteString("Conversation transcript:\n")
		for _, message := range req.Messages {
			rendered := renderPromptMessage(message)
			if rendered == "" {
				continue
			}
			sb.WriteString(rendered)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	query, attachments := expandFileMentions(req.Query)
	if attachments != "" {
		sb.WriteString(attachments)
		sb.WriteString("\n\n")
	}
	sb.WriteString("User request:\n")
	sb.WriteString(renderPromptMessageBody(strings.TrimSpace(query), req.SelectedCommands))
	if strings.TrimSpace(streamChunk) != "" {
		if strings.TrimSpace(query) != "" || len(req.SelectedCommands) > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(strings.TrimSpace(streamChunk))
	}
	return sb.String()
}

func renderPromptMessage(message Message) string {
	role := strings.TrimSpace(message.Role)
	if role == "" {
		role = "user"
	}
	body := renderPromptMessageBody(message.Content, message.Commands)
	if body == "" {
		return ""
	}
	return fmt.Sprintf("- %s:\n%s", role, indentPromptBlock(body, "  "))
}

func renderPromptMessageBody(content string, commands []Command) string {
	sections := make([]string, 0, 2)
	if trimmed := strings.TrimSpace(content); trimmed != "" {
		sections = append(sections, trimmed)
	}
	if rendered := renderPromptCommands(commands); rendered != "" {
		sections = append(sections, rendered)
	}
	return strings.Join(sections, "\n\n")
}

func renderPromptCommands(commands []Command) string {
	if len(commands) == 0 {
		return ""
	}
	lines := make([]string, 0, len(commands)+1)
	lines = append(lines, "Referenced terminal commands (metadata only; use inspect_command_output to inspect output):")
	for _, command := range commands {
		if rendered := renderPromptCommand(command); rendered != "" {
			lines = append(lines, rendered)
		}
	}
	if len(lines) == 1 {
		return ""
	}
	return strings.Join(lines, "\n")
}

func renderPromptCommand(command Command) string {
	if strings.TrimSpace(command.ID) == "" || strings.TrimSpace(command.Command) == "" {
		return ""
	}
	fields := []string{
		fmt.Sprintf("id=%s", command.ID),
		fmt.Sprintf("command=%s", strconv.Quote(strings.TrimSpace(command.Command))),
	}
	if cwd := strings.TrimSpace(command.Cwd); cwd != "" {
		fields = append(fields, fmt.Sprintf("cwd=%s", strconv.Quote(cwd)))
	}
	if command.TsStart > 0 {
		fields = append(fields, fmt.Sprintf("started_at=%s", time.Unix(0, command.TsStart).UTC().Format(time.RFC3339)))
	}
	if command.ExitCode != nil {
		fields = append(fields, fmt.Sprintf("exit_code=%d", *command.ExitCode))
	}
	if command.DurationMs != nil {
		fields = append(fields, fmt.Sprintf("duration_ms=%d", *command.DurationMs))
	}
	if command.OutputSize != nil {
		fields = append(fields, fmt.Sprintf("output_size=%d", *command.OutputSize))
	}
	if command.TranscriptAvailable {
		fields = append(fields, "transcript_available=true")
	}
	return "- " + strings.Join(fields, " | ")
}

func indentPromptBlock(text, prefix string) string {
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

func expandFileMentions(query string) (string, string) {
	fields := strings.Fields(query)
	if len(fields) == 0 {
		return query, ""
	}

	var attachments []string
	for _, field := range fields {
		if !strings.HasPrefix(field, "@") || len(field) < 2 {
			continue
		}
		path := strings.TrimPrefix(field, "@")
		file, err := readFile(&ReadFileReq{Path: path, MaxLines: 200, MaxBytes: 32 * 1024})
		if err != nil {
			attachments = append(attachments, fmt.Sprintf("Attachment %s: error: %v", path, err))
			continue
		}
		attachments = append(attachments, fmt.Sprintf("Attachment %s:\n%s", file.Path, file.Content))
	}
	return query, strings.Join(attachments, "\n\n")
}

func parseHITLRequest(event *session.Event) (HITLRequest, bool) {
	if event == nil || event.Content == nil {
		return HITLRequest{}, false
	}
	for _, part := range event.Content.Parts {
		if part == nil || part.FunctionCall == nil {
			continue
		}
		call := part.FunctionCall
		if call.Name != toolconfirmation.FunctionCallName {
			continue
		}
		original, err := toolconfirmation.OriginalCallFrom(call)
		if err != nil {
			return HITLRequest{}, false
		}
		request := HITLRequest{
			ID:             call.ID,
			FunctionCallID: call.ID,
			OriginalTool:   original.Name,
			Kind:           HITLKindConfirm,
			Title:          "Confirmation Required",
			Prompt:         "Approval required.",
		}
		if toolConfirmation, ok := call.Args["toolConfirmation"]; ok {
			data, _ := json.Marshal(toolConfirmation)
			var payload struct {
				Hint    string          `json:"hint"`
				Payload json.RawMessage `json:"payload"`
			}
			_ = json.Unmarshal(data, &payload)
			if strings.TrimSpace(payload.Hint) != "" {
				request.Prompt = payload.Hint
			}
			if original.Name == "request_input" && len(payload.Payload) > 0 {
				var form inputFormPayload
				if json.Unmarshal(payload.Payload, &form) == nil {
					request.Kind = HITLKindInputForm
					request.Title = "Input Required"
					request.Questions = form.Questions
				}
			}
		}
		if original.Name == "command" {
			if command, ok := original.Args["command"].(string); ok {
				request.Command = command
			}
			if cwd, ok := original.Args["cwd"].(string); ok {
				request.Cwd = cwd
			}
		}
		return request, true
	}
	return HITLRequest{}, false
}

func buildConfirmationResponseContent(request HITLRequest, response HITLResponse) *genai.Content {
	payload := confirmationPayload(request, response)
	return &genai.Content{
		Role: "user",
		Parts: []*genai.Part{{
			FunctionResponse: &genai.FunctionResponse{
				Name: toolconfirmation.FunctionCallName,
				ID:   request.FunctionCallID,
				Response: map[string]any{
					"confirmed": response.Confirmed,
					"payload":   payload,
				},
			},
		}},
	}
}

func confirmationPayload(request HITLRequest, response HITLResponse) any {
	if len(response.Answers) > 0 {
		return map[string]any{
			"decision":      "provided",
			"original_tool": strings.TrimSpace(request.OriginalTool),
			"answers":       response.Answers,
		}
	}
	if response.Payload != nil {
		if payloadMap, ok := response.Payload.(map[string]any); ok {
			merged := make(map[string]any, len(payloadMap)+4)
			for key, value := range payloadMap {
				merged[key] = value
			}
			if _, ok := merged["decision"]; !ok {
				if response.Confirmed {
					merged["decision"] = "approved"
				} else {
					merged["decision"] = "rejected"
				}
			}
			if _, ok := merged["original_tool"]; !ok && strings.TrimSpace(request.OriginalTool) != "" {
				merged["original_tool"] = strings.TrimSpace(request.OriginalTool)
			}
			if _, ok := merged["command"]; !ok && strings.TrimSpace(request.Command) != "" {
				merged["command"] = strings.TrimSpace(request.Command)
			}
			return merged
		}
		return response.Payload
	}
	payload := map[string]any{
		"original_tool": strings.TrimSpace(request.OriginalTool),
	}
	if strings.TrimSpace(request.Command) != "" {
		payload["command"] = strings.TrimSpace(request.Command)
	}
	if response.Confirmed {
		payload["decision"] = "approved"
		payload["reason"] = "User approved this tool call."
		return payload
	}
	payload["decision"] = "rejected"
	payload["status"] = "rejected"
	payload["reason"] = "User rejected this tool call."
	payload["message"] = "Do not execute the tool. Ask the user for an alternative if needed."
	return payload
}

func contentText(c *genai.Content) string {
	if c == nil {
		return ""
	}
	var sb strings.Builder
	for _, p := range c.Parts {
		if p != nil && p.Text != "" && !p.Thought {
			sb.WriteString(p.Text)
		}
	}
	return sb.String()
}

func extractToolCallEvents(event *session.Event) []ToolCallEvent {
	if event == nil || event.Content == nil {
		return nil
	}
	author := strings.TrimSpace(event.Author)
	calls := make([]ToolCallEvent, 0, len(event.Content.Parts))
	for _, part := range event.Content.Parts {
		if part == nil || part.FunctionCall == nil {
			continue
		}
		call := part.FunctionCall
		if call == nil || strings.TrimSpace(call.Name) == "" || call.Name == toolconfirmation.FunctionCallName {
			continue
		}
		calls = append(calls, ToolCallEvent{
			CallID:    strings.TrimSpace(call.ID),
			AgentName: author,
			ToolName:  strings.TrimSpace(call.Name),
			Summary:   summarizeToolCall(call.Name, call.Args),
			State:     ToolCallStatePending,
		})
	}
	if len(calls) == 0 {
		return nil
	}
	return calls
}

func extractToolResultEvents(event *session.Event) []ToolCallEvent {
	if event == nil || event.Content == nil {
		return nil
	}
	author := strings.TrimSpace(event.Author)
	results := make([]ToolCallEvent, 0, len(event.Content.Parts))
	for _, part := range event.Content.Parts {
		if part == nil || part.FunctionResponse == nil {
			continue
		}
		response := part.FunctionResponse
		if response == nil {
			continue
		}
		name := strings.TrimSpace(response.Name)
		if name == "" || name == toolconfirmation.FunctionCallName {
			continue
		}
		results = append(results, ToolCallEvent{
			CallID:    strings.TrimSpace(response.ID),
			AgentName: author,
			ToolName:  name,
			Summary:   summarizeToolResultTarget(name, response.Response),
			Result:    summarizeToolResult(name, response.Response),
			State:     summarizeToolResultState(name, response.Response),
		})
	}
	if len(results) == 0 {
		return nil
	}
	return results
}

func extractCommandCwdEvent(event *session.Event) (string, bool) {
	if event == nil || event.Content == nil {
		return "", false
	}
	for _, part := range event.Content.Parts {
		if part == nil || part.FunctionResponse == nil {
			continue
		}
		response := part.FunctionResponse
		if response == nil || strings.TrimSpace(response.Name) != "command" {
			continue
		}
		rawChanged, ok := response.Response["cwd_changed"]
		if !ok || !toolBoolArg(rawChanged) {
			continue
		}
		rawCwd, ok := response.Response["cwd"]
		if !ok {
			continue
		}
		cwd, ok := rawCwd.(string)
		if !ok || strings.TrimSpace(cwd) == "" {
			continue
		}
		return strings.TrimSpace(cwd), true
	}
	return "", false
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

var sessionCounter uint64

func newSessionID() string {
	n := atomic.AddUint64(&sessionCounter, 1)
	return fmt.Sprintf("session-%d-%d", time.Now().UnixNano(), n)
}
