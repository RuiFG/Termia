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

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/prebuilt/deep"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"

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
	if strings.TrimSpace(req.SessionID) == "" {
		req.SessionID = newSessionID()
	}

	state := newRuntimeState()
	if cwd := strings.TrimSpace(req.Cwd); cwd != "" {
		state.set(commandStateCwdKey, cwd)
	}

	root, err := r.buildRootAgent(ctx, req, state)
	if err != nil {
		return nil, err
	}

	runr := adk.NewRunner(ctx, adk.RunnerConfig{
		Agent:           root,
		EnableStreaming: true,
		CheckPointStore: newMemoryCheckPointStore(),
	})

	output := make(chan RuntimeEvent, 32)
	go func() {
		defer close(output)
		_ = r.runConversation(ctx, runr, req, state, output)
	}()
	return output, nil
}

func (r *Runtime) runConversation(ctx context.Context, runr *adk.Runner, req RunRequest, state *runtimeState, output chan<- RuntimeEvent) error {
	history := make([]adk.Message, 0, 16)
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

	turnIndex := 0
	for next != nil {
		checkPointID := fmt.Sprintf("%s-turn-%d", req.SessionID, turnIndex)
		turnMessages, err := r.runTurn(ctx, runr, checkPointID, state.snapshot(), history, next, output)
		if err != nil {
			output <- RuntimeEvent{Kind: RuntimeEventError, Text: fmt.Sprintf("Error: %v", err)}
			return err
		}

		history = append(history, next)
		history = append(history, turnMessages...)
		turnIndex++

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
				next = schema.UserMessage("The stream source has reached EOF. Provide a concise final assessment if needed.")
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

func (r *Runtime) runTurn(
	ctx context.Context,
	runr *adk.Runner,
	checkPointID string,
	sessionValues map[string]any,
	history []adk.Message,
	input *schema.Message,
	output chan<- RuntimeEvent,
) ([]adk.Message, error) {
	seenToolCalls := make(map[string]bool)
	seenToolResults := make(map[string]bool)
	turnMessages := make([]adk.Message, 0, 16)

	messages := make([]adk.Message, 0, len(history)+1)
	messages = append(messages, history...)
	messages = append(messages, input)

	iter := runr.Run(
		ctx,
		messages,
		adk.WithCheckPointID(checkPointID),
		adk.WithSessionValues(sessionValues),
	)

	for {
		request, err := consumeRuntimeIterator(iter, output, seenToolCalls, seenToolResults, &turnMessages)
		if err != nil {
			return nil, err
		}
		if request == nil {
			return turnMessages, nil
		}
		if r.responder == nil {
			return nil, fmt.Errorf("hitl required for tool %s but no responder is configured", request.OriginalTool)
		}
		if strings.TrimSpace(request.ID) == "" {
			return nil, fmt.Errorf("hitl request is missing resume target id")
		}

		response, err := r.responder.Handle(ctx, *request)
		if err != nil {
			return nil, err
		}

		iter, err = runr.ResumeWithParams(ctx, checkPointID, &adk.ResumeParams{
			Targets: map[string]any{
				request.ID: hitlResumeData{
					Confirmed: response.Confirmed,
					Answers:   response.Answers,
				},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("resume interrupted run: %w", err)
		}
	}
}

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

	msg, emittedText, err := materializeMessage(event.Output.MessageOutput, output)
	if err != nil {
		return nil, err
	}
	if msg == nil {
		return nil, nil
	}

	emitMessageEvents(event.AgentName, msg, output, seenToolCalls, seenToolResults)
	if !emittedText {
		if text := assistantMessageText(msg); text != "" {
			output <- RuntimeEvent{Kind: RuntimeEventText, Text: text}
		}
	}

	return msg, nil
}

func materializeMessage(mv *adk.MessageVariant, output chan<- RuntimeEvent) (adk.Message, bool, error) {
	if mv == nil {
		return nil, false, nil
	}
	if !mv.IsStreaming {
		return mv.Message, false, nil
	}

	defer mv.MessageStream.Close()

	var (
		chunks      []*schema.Message
		emittedText bool
	)
	for {
		chunk, err := mv.MessageStream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, emittedText, err
		}
		if chunk == nil {
			continue
		}

		chunks = append(chunks, chunk)
		if text := assistantMessageText(chunk); text != "" {
			output <- RuntimeEvent{Kind: RuntimeEventText, Text: text}
			emittedText = true
		}
	}

	if len(chunks) == 0 {
		return nil, emittedText, nil
	}

	msg, err := schema.ConcatMessages(chunks)
	if err != nil {
		return nil, emittedText, err
	}
	return msg, emittedText, nil
}

func (r *Runtime) buildRootAgent(ctx context.Context, req RunRequest, state *runtimeState) (adk.Agent, error) {
	registry, err := NewToolRegistry(r.db, state, r.requireCommandConfirmation())
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

func (r *Runtime) requireCommandConfirmation() bool {
	if r.cfg == nil {
		return true
	}
	return r.cfg.Agent.RequireCommandConfirmation
}

func (r *Runtime) buildAssistantAgent(ctx context.Context, registry *ToolRegistry) (adk.Agent, error) {
	spec, err := AssistantSpecFromConfig(r.cfg)
	if err != nil {
		return nil, err
	}
	return r.buildChatModelAgent(ctx, spec, registry)
}

func (r *Runtime) buildTeamAgent(ctx context.Context, teamName string, registry *ToolRegistry) (adk.Agent, error) {
	if strings.TrimSpace(teamName) == "" && r.cfg != nil {
		teamName = r.cfg.Agent.DefaultTeam
	}
	if strings.TrimSpace(teamName) == "" {
		return nil, fmt.Errorf("team mode requires --team or agent.default_team")
	}

	spec, err := LoadTeamByName(r.cfg, teamName)
	if err != nil {
		return nil, err
	}

	subAgents := make([]adk.Agent, 0, len(spec.Agents))
	for _, member := range spec.Agents {
		subAgent, err := r.buildChatModelAgent(ctx, member, registry)
		if err != nil {
			return nil, fmt.Errorf("member %s: %w", member.Name, err)
		}
		subAgents = append(subAgents, subAgent)
	}

	model, err := NewModel(ctx, spec.Coordinator.Model)
	if err != nil {
		return nil, fmt.Errorf("coordinator: %w", err)
	}

	instruction := strings.TrimSpace(spec.Coordinator.Instruction)
	if instruction == "" {
		instruction = DefaultCoordinatorInstruction
	}
	description := strings.TrimSpace(spec.Coordinator.Description)
	if description == "" {
		description = spec.Coordinator.Name
	}

	return deep.New(ctx, &deep.Config{
		Name:                   spec.Coordinator.Name,
		Description:            description,
		ChatModel:              model,
		Instruction:            instruction,
		SubAgents:              subAgents,
		ToolsConfig:            buildToolsConfig(registry.Filter(spec.Coordinator.Tools)),
		WithoutGeneralSubAgent: len(subAgents) > 0,
	})
}

func (r *Runtime) buildChatModelAgent(ctx context.Context, spec AgentSpec, registry *ToolRegistry) (adk.Agent, error) {
	model, err := NewModel(ctx, spec.Model)
	if err != nil {
		return nil, err
	}
	description := strings.TrimSpace(spec.Description)
	if description == "" {
		description = spec.Name
	}

	return adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        spec.Name,
		Description: description,
		Instruction: spec.Instruction,
		Model:       model,
		ToolsConfig: buildToolsConfig(registry.Filter(spec.Tools)),
	})
}

func buildToolsConfig(tools []tool.BaseTool) adk.ToolsConfig {
	return adk.ToolsConfig{
		ToolsNodeConfig: compose.ToolsNodeConfig{
			Tools:               tools,
			ExecuteSequentially: true,
		},
		EmitInternalEvents: true,
	}
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

func buildPromptContent(req RunRequest) *schema.Message {
	return schema.UserMessage(buildPromptText(req, ""))
}

func buildStreamPromptContent(req RunRequest, chunk StreamChunk, first bool) *schema.Message {
	title := "New stream chunk"
	if first {
		title = "Initial stream chunk"
	}
	body := fmt.Sprintf("%s:\n%s", title, strings.TrimSpace(chunk.Text))
	return schema.UserMessage(buildPromptText(req, body))
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

func assistantMessageText(msg *schema.Message) string {
	if msg == nil || msg.Role != schema.Assistant {
		return ""
	}
	if msg.Content != "" {
		return msg.Content
	}
	if len(msg.AssistantGenMultiContent) == 0 {
		return ""
	}

	var sb strings.Builder
	for _, part := range msg.AssistantGenMultiContent {
		if part.Text != "" {
			sb.WriteString(part.Text)
		}
	}
	return sb.String()
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

var sessionCounter uint64

func newSessionID() string {
	n := atomic.AddUint64(&sessionCounter, 1)
	return fmt.Sprintf("session-%d-%d", time.Now().UnixNano(), n)
}
