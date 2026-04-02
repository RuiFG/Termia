package agentapp

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	runtimeagent "github.com/termia/termia/internal/agent"
	"github.com/termia/termia/internal/config"
	"github.com/termia/termia/internal/db"
	"github.com/termia/termia/internal/textutil"
)

var serviceMessageIDSeq atomic.Int64

type Runtime interface {
	Run(context.Context, runtimeagent.RunRequest) (<-chan runtimeagent.RuntimeEvent, error)
}

type RuntimeFactory func(*config.Config, *db.DB, runtimeagent.HITLResponder) Runtime

type RunRequest struct {
	SessionID        string
	Query            string
	Cwd              string
	SelectedCommands []runtimeagent.Command
	StreamReader     *runtimeagent.StreamReader
	Responder        runtimeagent.HITLResponder
}

type Service struct {
	cfg               *config.Config
	db                *db.DB
	sessions          *SessionService
	registry          *Registry
	sharedCommands    []SharedSlashCommand
	runtimeFactory    RuntimeFactory
	lastRunMiddleware []MiddlewareActivation
}

func NewService(cfg *config.Config, database *db.DB, factory RuntimeFactory) *Service {
	return &Service{
		cfg:            cfg,
		db:             database,
		sessions:       NewSessionService(database),
		registry:       NewRegistry(DefaultMiddlewareSpecs()...),
		sharedCommands: DefaultSharedSlashCommands(),
		runtimeFactory: factory,
	}
}

func (s *Service) Run(ctx context.Context, req RunRequest) (<-chan runtimeagent.RuntimeEvent, error) {
	if s == nil || s.db == nil || s.sessions == nil || s.registry == nil || s.runtimeFactory == nil {
		return nil, fmt.Errorf("service is not initialized")
	}

	session, state, err := s.sessions.Resolve(req.SessionID, req.Cwd, DefaultSessionState(), nil)
	if err != nil {
		return nil, err
	}

	query, state, runMiddleware, err := s.applySharedSlashCommand(session.ID, state, req.Query)
	if err != nil {
		return nil, err
	}
	s.lastRunMiddleware = append([]MiddlewareActivation(nil), runMiddleware...)

	history, err := s.loadMessages(session.ID)
	if err != nil {
		return nil, err
	}

	middleware, err := s.buildMiddleware(state.SessionMiddleware, runMiddleware)
	if err != nil {
		return nil, err
	}

	runCtx := &RunContext{
		SessionID:        session.ID,
		Query:            strings.TrimSpace(query),
		Cwd:              selectRunCwd(req.Cwd, session.Cwd),
		State:            state,
		SelectedCommands: append([]runtimeagent.Command(nil), req.SelectedCommands...),
	}
	for _, item := range middleware {
		if err := item.BeforeRun(ctx, runCtx); err != nil {
			return nil, err
		}
	}
	if err := s.persistUserMessage(runCtx.SessionID, runCtx.Query, runCtx.SelectedCommands); err != nil {
		return nil, err
	}

	out := make(chan runtimeagent.RuntimeEvent, 32)
	go s.runLoop(ctx, out, runCtx, history, middleware, req.StreamReader, req.Responder)
	return out, nil
}

func (s *Service) applySharedSlashCommand(sessionID string, state SessionState, query string) (string, SessionState, []MiddlewareActivation, error) {
	query = strings.TrimSpace(query)
	command, ok := ResolveSharedSlashCommand(query, s.sharedCommands)
	if !ok {
		return query, state, nil, nil
	}
	args := strings.TrimSpace(strings.TrimPrefix(query, "/"+command.Name))
	if command.BuildActivation == nil {
		return "", SessionState{}, nil, fmt.Errorf("shared slash command %q has no activation builder", command.Name)
	}
	activation, err := command.BuildActivation(args)
	if err != nil {
		return "", SessionState{}, nil, err
	}
	if activation.Scope == MiddlewareScopeSession {
		state = appendSessionMiddlewareActivation(state, activation)
		updatedState, err := s.sessions.Update(sessionID, state, nil)
		if err != nil {
			return "", SessionState{}, nil, err
		}
		return args, updatedState, nil, nil
	}
	return args, state, []MiddlewareActivation{activation}, nil
}

func (s *Service) buildMiddleware(sessionMiddleware, runMiddleware []MiddlewareActivation) ([]Middleware, error) {
	activations := append(append([]MiddlewareActivation(nil), sessionMiddleware...), runMiddleware...)
	if len(activations) == 0 {
		return nil, nil
	}

	items := make([]Middleware, 0, len(activations))
	for _, activation := range activations {
		item, err := s.registry.Build(activation)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Service) runLoop(
	ctx context.Context,
	out chan<- runtimeagent.RuntimeEvent,
	runCtx *RunContext,
	history []runtimeagent.Message,
	middleware []Middleware,
	streamReader *runtimeagent.StreamReader,
	responder runtimeagent.HITLResponder,
) {
	defer close(out)

	query := strings.TrimSpace(runCtx.Query)
	cwd := strings.TrimSpace(runCtx.Cwd)
	messages := append([]runtimeagent.Message(nil), history...)

	for {
		if !hasRuntimeInput(query, runCtx.SelectedCommands, streamReader) {
			timeline := make([]TimelineEntry, 0, 1)
			directive, ok := s.runAfterRunMiddleware(ctx, out, runCtx, middleware, RunSummary{}, &timeline)
			if err := s.persistTimelineMessages(runCtx.SessionID, timeline); err != nil {
				emitRuntimeEvent(ctx, out, runtimeagent.RuntimeEvent{Kind: runtimeagent.RuntimeEventError, Text: err.Error()})
				return
			}
			if !ok || !directive.Continue || strings.TrimSpace(directive.NextQuery) == "" {
				return
			}
			query = strings.TrimSpace(directive.NextQuery)
			runCtx.Query = query
			runCtx.Cwd = cwd
			continue
		}

		stream, err := s.startRuntime(ctx, runCtx, messages, query, cwd, streamReader, responder)
		if err != nil {
			emitRuntimeEvent(ctx, out, runtimeagent.RuntimeEvent{Kind: runtimeagent.RuntimeEventError, Text: err.Error()})
			_ = s.persistTimelineMessages(runCtx.SessionID, []TimelineEntry{{Role: "error", Content: err.Error()}})
			return
		}
		streamReader = nil

		timeline := make([]TimelineEntry, 0, 8)
		summary := RunSummary{}
		for event := range stream {
			switch event.Kind {
			case runtimeagent.RuntimeEventText:
				timeline = AppendTimelineText(timeline, "assistant", event.Text, true)
			case runtimeagent.RuntimeEventReasoning:
				timeline = AppendTimelineText(timeline, "reasoning", event.Text, true)
			case runtimeagent.RuntimeEventToolCall, runtimeagent.RuntimeEventToolResult:
				if event.ToolCall != nil {
					timeline = UpsertTimelineToolCall(timeline, *event.ToolCall)
					if event.Kind == runtimeagent.RuntimeEventToolCall && strings.TrimSpace(event.ToolCall.ToolName) == "command" {
						summary.SawCommand = true
					}
				}
			case runtimeagent.RuntimeEventCwd:
				cwd = strings.TrimSpace(event.Cwd)
				if cwd != "" {
					_ = s.db.UpdateAgentSessionCwd(runCtx.SessionID, cwd, time.Now().UnixNano())
				}
			case runtimeagent.RuntimeEventError:
				timeline = MarkLatestPendingToolFailed(timeline, event.Text)
				timeline = AppendTimelineText(timeline, "error", event.Text, false)
			}
			if !emitRuntimeEvent(ctx, out, event) {
				return
			}
		}

		directive, ok := s.runAfterRunMiddleware(ctx, out, runCtx, middleware, summary, &timeline)
		if err := s.persistTimelineMessages(runCtx.SessionID, timeline); err != nil {
			emitRuntimeEvent(ctx, out, runtimeagent.RuntimeEvent{Kind: runtimeagent.RuntimeEventError, Text: err.Error()})
			return
		}
		messages = append(messages, runCtxMessage(query, runCtx.SelectedCommands))
		messages = append(messages, runtimeMessagesFromTimeline(timeline)...)

		if !ok || !directive.Continue || strings.TrimSpace(directive.NextQuery) == "" {
			return
		}
		query = strings.TrimSpace(directive.NextQuery)
		runCtx.Query = query
		runCtx.Cwd = cwd
	}
}

func hasRuntimeInput(query string, commands []runtimeagent.Command, streamReader *runtimeagent.StreamReader) bool {
	return strings.TrimSpace(query) != "" || len(commands) > 0 || streamReader != nil
}

func (s *Service) startRuntime(
	ctx context.Context,
	runCtx *RunContext,
	messages []runtimeagent.Message,
	query, cwd string,
	streamReader *runtimeagent.StreamReader,
	responder runtimeagent.HITLResponder,
) (<-chan runtimeagent.RuntimeEvent, error) {
	runtime := s.runtimeFactory(s.cfg, s.db, responder)
	if runtime == nil {
		return nil, fmt.Errorf("runtime factory returned nil")
	}
	return runtime.Run(ctx, runtimeagent.RunRequest{
		Mode:             runCtx.State.Mode,
		TeamName:         runCtx.State.TeamName,
		SessionID:        runCtx.SessionID,
		Query:            strings.TrimSpace(query),
		Cwd:              strings.TrimSpace(cwd),
		SelectedCommands: append([]runtimeagent.Command(nil), runCtx.SelectedCommands...),
		Messages:         append([]runtimeagent.Message(nil), messages...),
		StreamReader:     streamReader,
	})
}

func (s *Service) runAfterRunMiddleware(
	ctx context.Context,
	out chan<- runtimeagent.RuntimeEvent,
	runCtx *RunContext,
	middleware []Middleware,
	summary RunSummary,
	timeline *[]TimelineEntry,
) (RunDirective, bool) {
	var combined RunDirective
	for _, item := range middleware {
		directive, err := item.AfterRun(ctx, runCtx, summary)
		if err != nil {
			*timeline = MarkLatestPendingToolFailed(*timeline, err.Error())
			*timeline = AppendTimelineText(*timeline, "error", err.Error(), false)
			emitRuntimeEvent(ctx, out, runtimeagent.RuntimeEvent{Kind: runtimeagent.RuntimeEventError, Text: err.Error()})
			return RunDirective{}, false
		}
		if strings.TrimSpace(directive.EmitText) != "" {
			*timeline = AppendTimelineText(*timeline, "assistant", directive.EmitText, true)
			if !emitRuntimeEvent(ctx, out, runtimeagent.RuntimeEvent{Kind: runtimeagent.RuntimeEventText, Text: directive.EmitText}) {
				return RunDirective{}, false
			}
		}
		if directive.Continue {
			combined.Continue = true
			if strings.TrimSpace(directive.NextQuery) != "" {
				combined.NextQuery = strings.TrimSpace(directive.NextQuery)
			}
		}
	}
	return combined, true
}

func (s *Service) loadMessages(sessionID string) ([]runtimeagent.Message, error) {
	messages, err := s.db.ListAgentMessages(strings.TrimSpace(sessionID))
	if err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return nil, nil
	}

	output := make([]runtimeagent.Message, 0, len(messages))
	for _, msg := range messages {
		role := NormalizeRole(msg.Role)
		if role == "tool" || role == "reasoning" || role == "error" {
			continue
		}
		content := textutil.NormalizeTrimmedText(msg.Content)
		if content == "" {
			continue
		}
		output = append(output, runtimeagent.Message{
			Role:     role,
			Content:  content,
			Commands: runtimeCommandsFromMessageMetadata(db.ParseAgentMessageMetadata(msg)),
		})
	}
	if len(output) == 0 {
		return nil, nil
	}
	return output, nil
}

func (s *Service) persistUserMessage(sessionID, query string, commands []runtimeagent.Command) error {
	query = textutil.NormalizeTrimmedText(query)
	metadataJSON, err := db.EncodeAgentMessageMetadata(db.AgentMessageMetadata{
		CitedCommands: db.AgentMessageCommandMetadataFromCommands(dbCommandsFromRuntimeCommands(commands)),
	})
	if err != nil {
		return err
	}
	if query == "" && metadataJSON == "" {
		return nil
	}
	return s.db.CreateAgentMessage(&db.AgentMessage{
		ID:           nextMessageID(0),
		SessionID:    strings.TrimSpace(sessionID),
		Role:         "user",
		Content:      query,
		MetadataJSON: metadataJSON,
		CreatedAt:    time.Now().UnixNano(),
	})
}

func (s *Service) persistTimelineMessages(sessionID string, timeline []TimelineEntry) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || len(timeline) == 0 {
		return nil
	}

	createdAt := time.Now().UnixNano()
	persisted := 0
	for _, entry := range timeline {
		if !renderableTimelineEntry(entry) {
			continue
		}
		metadataJSON, err := encodeTimelineEntryMetadata(entry)
		if err != nil {
			return err
		}
		if err := s.db.CreateAgentMessage(&db.AgentMessage{
			ID:           nextMessageID(int64(persisted)),
			SessionID:    sessionID,
			Role:         NormalizeRole(entry.Role),
			Content:      textutil.NormalizeTrimmedText(entry.Content),
			MetadataJSON: metadataJSON,
			CreatedAt:    createdAt + int64(persisted),
		}); err != nil {
			return err
		}
		persisted++
	}
	return nil
}

func encodeTimelineEntryMetadata(entry TimelineEntry) (string, error) {
	if entry.ToolCall == nil {
		return "", nil
	}
	call := *entry.ToolCall
	return db.EncodeAgentMessageMetadata(db.AgentMessageMetadata{
		ToolCalls: []db.AgentMessageToolCallMetadata{{
			CallID:    call.CallID,
			AgentName: call.AgentName,
			ToolName:  call.ToolName,
			Summary:   call.Summary,
			Result:    call.Result,
			State:     string(call.State),
		}},
	})
}

func renderableTimelineEntry(entry TimelineEntry) bool {
	if entry.ToolCall != nil {
		return strings.TrimSpace(entry.ToolCall.ToolName) != ""
	}
	return textutil.NormalizeTrimmedText(entry.Content) != ""
}

func runtimeMessagesFromTimeline(timeline []TimelineEntry) []runtimeagent.Message {
	if len(timeline) == 0 {
		return nil
	}

	messages := make([]runtimeagent.Message, 0, len(timeline))
	for _, entry := range timeline {
		role := NormalizeRole(entry.Role)
		if entry.ToolCall != nil || role == "tool" || role == "reasoning" || role == "error" {
			continue
		}
		content := textutil.NormalizeTrimmedText(entry.Content)
		if content == "" {
			continue
		}
		messages = append(messages, runtimeagent.Message{Role: role, Content: content})
	}
	if len(messages) == 0 {
		return nil
	}
	return messages
}

func runtimeCommandsFromMessageMetadata(metadata db.AgentMessageMetadata) []runtimeagent.Command {
	if len(metadata.CitedCommands) == 0 {
		return nil
	}

	commands := make([]runtimeagent.Command, 0, len(metadata.CitedCommands))
	for _, command := range metadata.CitedCommands {
		if strings.TrimSpace(command.ID) == "" || strings.TrimSpace(command.Command) == "" {
			continue
		}
		commands = append(commands, runtimeagent.Command{
			ID:                  strings.TrimSpace(command.ID),
			TsStart:             command.TsStart,
			TsEnd:               command.TsEnd,
			Command:             strings.TrimSpace(command.Command),
			Cwd:                 strings.TrimSpace(command.Cwd),
			ExitCode:            command.ExitCode,
			DurationMs:          command.DurationMs,
			OutputSize:          command.OutputSize,
			TranscriptAvailable: command.TranscriptAvailable,
		})
	}
	if len(commands) == 0 {
		return nil
	}
	return commands
}

func dbCommandsFromRuntimeCommands(commands []runtimeagent.Command) []db.Command {
	if len(commands) == 0 {
		return nil
	}

	output := make([]db.Command, 0, len(commands))
	for _, command := range commands {
		item := db.Command{
			ID:         strings.TrimSpace(command.ID),
			TsStart:    command.TsStart,
			TsEnd:      command.TsEnd,
			DurationMs: command.DurationMs,
			Command:    strings.TrimSpace(command.Command),
			ExitCode:   command.ExitCode,
			Cwd:        strings.TrimSpace(command.Cwd),
			OutputSize: command.OutputSize,
		}
		if command.TranscriptAvailable {
			path := "<transcript>"
			item.TranscriptPath = &path
		}
		output = append(output, item)
	}
	if len(output) == 0 {
		return nil
	}
	return output
}

func runCtxMessage(query string, commands []runtimeagent.Command) runtimeagent.Message {
	return runtimeagent.Message{
		Role:     "user",
		Content:  strings.TrimSpace(query),
		Commands: append([]runtimeagent.Command(nil), commands...),
	}
}

func selectRunCwd(requestCwd, sessionCwd string) string {
	if cwd := strings.TrimSpace(requestCwd); cwd != "" {
		return cwd
	}
	return strings.TrimSpace(sessionCwd)
}

func nextMessageID(offset int64) string {
	return strconv.FormatInt(time.Now().UnixNano()+offset, 10) + "-" + strconv.FormatInt(serviceMessageIDSeq.Add(1), 36)
}

func emitRuntimeEvent(ctx context.Context, out chan<- runtimeagent.RuntimeEvent, event runtimeagent.RuntimeEvent) bool {
	select {
	case <-ctx.Done():
		return false
	case out <- event:
		return true
	}
}

func appendSessionMiddlewareActivation(state SessionState, activation MiddlewareActivation) SessionState {
	for _, current := range state.SessionMiddleware {
		if middlewareActivationEqual(current, activation) {
			return state
		}
	}
	state.SessionMiddleware = append(state.SessionMiddleware, activation)
	return state
}

func middlewareActivationEqual(left, right MiddlewareActivation) bool {
	if left.Name != right.Name || left.Scope != right.Scope || len(left.Args) != len(right.Args) {
		return false
	}
	for key, value := range left.Args {
		if right.Args[key] != value {
			return false
		}
	}
	return true
}
