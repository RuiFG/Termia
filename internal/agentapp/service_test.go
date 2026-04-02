package agentapp

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	runtimeagent "github.com/termia/termia/internal/agent"
	"github.com/termia/termia/internal/config"
	"github.com/termia/termia/internal/db"
	"go.uber.org/zap"
)

type fakeRuntime struct {
	requests     []runtimeagent.RunRequest
	eventsPerRun [][]runtimeagent.RuntimeEvent
}

func (f *fakeRuntime) Run(ctx context.Context, req runtimeagent.RunRequest) (<-chan runtimeagent.RuntimeEvent, error) {
	runIndex := len(f.requests)
	f.requests = append(f.requests, req)

	var events []runtimeagent.RuntimeEvent
	if runIndex < len(f.eventsPerRun) {
		events = f.eventsPerRun[runIndex]
	}

	stream := make(chan runtimeagent.RuntimeEvent, len(events))
	go func() {
		defer close(stream)
		for _, event := range events {
			select {
			case <-ctx.Done():
				return
			case stream <- event:
			}
		}
	}()
	return stream, nil
}

func TestServiceRunEmitsRuntimeEventsAndTracksCommands(t *testing.T) {
	t.Setenv("TERMIA_SESSION_ID", "missing")
	runtime := &fakeRuntime{eventsPerRun: [][]runtimeagent.RuntimeEvent{
		{
			{
				Kind: runtimeagent.RuntimeEventToolCall,
				ToolCall: &runtimeagent.ToolCallEvent{
					ToolName: "command",
					State:    runtimeagent.ToolCallStatePending,
				},
			},
			{Kind: runtimeagent.RuntimeEventText, Text: "done"},
		},
		{},
	}}
	svc, _ := newTestService(t, runtime)

	stream, err := svc.Run(context.Background(), RunRequest{Query: "/ralph-loop inspect", Cwd: "/tmp/project"})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	var sawToolCall bool
	var sawRuntimeText bool
	var sawCompletionText bool
	for event := range stream {
		if event.Kind == runtimeagent.RuntimeEventError {
			t.Fatalf("unexpected service error: %s", event.Text)
		}
		if event.Kind == runtimeagent.RuntimeEventToolCall && event.ToolCall != nil && event.ToolCall.ToolName == "command" {
			sawToolCall = true
		}
		if event.Kind == runtimeagent.RuntimeEventText && event.Text == "done" {
			sawRuntimeText = true
		}
		if event.Kind == runtimeagent.RuntimeEventText && event.Text == "已完成" {
			sawCompletionText = true
		}
	}

	if !sawToolCall {
		t.Fatal("expected command tool call event")
	}
	if !sawRuntimeText {
		t.Fatal("expected runtime text event")
	}
	if !sawCompletionText {
		t.Fatal("expected ralph-loop completion text")
	}
	if len(svc.lastRunMiddleware) != 1 || svc.lastRunMiddleware[0].Name != "ralph-loop" {
		t.Fatalf("unexpected run middleware: %+v", svc.lastRunMiddleware)
	}
	if len(runtime.requests) != 2 {
		t.Fatalf("expected ralph-loop to trigger a second runtime pass, got %d", len(runtime.requests))
	}
	if runtime.requests[0].Query != "inspect" {
		t.Fatalf("expected first pass query to use slash args, got %+v", runtime.requests[0])
	}
	if runtime.requests[1].Query == "" {
		t.Fatalf("expected second pass continuation query, got %+v", runtime.requests[1])
	}
}

func TestServiceRunResolvesSharedSlashMiddleware(t *testing.T) {
	t.Setenv("TERMIA_SESSION_ID", "missing")
	runtime := &fakeRuntime{eventsPerRun: [][]runtimeagent.RuntimeEvent{{}}}
	svc, _ := newTestService(t, runtime)

	stream, err := svc.Run(context.Background(), RunRequest{Query: "/ralph-loop", Cwd: "/tmp/project"})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	var sawCompletionText bool
	for event := range stream {
		if event.Kind == runtimeagent.RuntimeEventError {
			t.Fatalf("unexpected service error: %s", event.Text)
		}
		if event.Kind == runtimeagent.RuntimeEventText && event.Text == "已完成" {
			sawCompletionText = true
		}
	}

	if len(svc.lastRunMiddleware) != 1 || svc.lastRunMiddleware[0].Name != "ralph-loop" || svc.lastRunMiddleware[0].Scope != MiddlewareScopeRun {
		t.Fatalf("unexpected run middleware: %+v", svc.lastRunMiddleware)
	}
	if !sawCompletionText {
		t.Fatal("expected no-command ralph-loop run to emit completion text")
	}
}

func TestServiceRunPersistsUserAndAssistantMessagesAndLoadsHistory(t *testing.T) {
	t.Setenv("TERMIA_SESSION_ID", "missing")
	runtime := &fakeRuntime{eventsPerRun: [][]runtimeagent.RuntimeEvent{{
		{Kind: runtimeagent.RuntimeEventCwd, Cwd: "/tmp/next"},
		{Kind: runtimeagent.RuntimeEventText, Text: "new answer"},
	}}}
	svc, database := newTestService(t, runtime)

	session := createServiceTestSession(t, database, SessionState{Mode: runtimeagent.ModeAssistant}, "/tmp/project")
	createServiceTestMessage(t, database, session.ID, "user", "old question", 1)
	createServiceTestMessage(t, database, session.ID, "assistant", "old answer", 2)

	stream, err := svc.Run(context.Background(), RunRequest{
		SessionID: session.ID,
		Query:     "new question",
		Cwd:       "/tmp/project",
		SelectedCommands: []runtimeagent.Command{{
			ID:      "cmd-1",
			Command: "pwd",
			Cwd:     "/tmp/project",
		}},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	for event := range stream {
		if event.Kind == runtimeagent.RuntimeEventError {
			t.Fatalf("unexpected service error: %s", event.Text)
		}
	}

	if len(runtime.requests) != 1 {
		t.Fatalf("expected one runtime request, got %d", len(runtime.requests))
	}
	if len(runtime.requests[0].Messages) != 2 {
		t.Fatalf("expected existing history to be loaded, got %+v", runtime.requests[0].Messages)
	}
	if runtime.requests[0].Messages[0].Role != "user" || runtime.requests[0].Messages[0].Content != "old question" {
		t.Fatalf("unexpected first history message: %+v", runtime.requests[0].Messages[0])
	}
	if runtime.requests[0].Messages[1].Role != "assistant" || runtime.requests[0].Messages[1].Content != "old answer" {
		t.Fatalf("unexpected second history message: %+v", runtime.requests[0].Messages[1])
	}

	messages, err := database.ListAgentMessages(session.ID)
	if err != nil {
		t.Fatalf("ListAgentMessages returned error: %v", err)
	}
	if len(messages) != 4 {
		t.Fatalf("expected existing plus new user/assistant messages, got %#v", messages)
	}
	if messages[2].Role != "user" || messages[2].Content != "new question" {
		t.Fatalf("unexpected persisted user message: %#v", messages[2])
	}
	metadata := db.ParseAgentMessageMetadata(messages[2])
	if len(metadata.CitedCommands) != 1 || metadata.CitedCommands[0].ID != "cmd-1" {
		t.Fatalf("expected selected command metadata to persist, got %#v", metadata.CitedCommands)
	}
	if messages[3].Role != "assistant" || messages[3].Content != "new answer" {
		t.Fatalf("unexpected persisted assistant message: %#v", messages[3])
	}

	gotSession, ok, err := database.GetAgentSession(session.ID)
	if err != nil {
		t.Fatalf("GetAgentSession returned error: %v", err)
	}
	if !ok {
		t.Fatalf("expected session to exist")
	}
	if gotSession.Cwd != "/tmp/next" {
		t.Fatalf("expected session cwd to be updated from runtime event, got %+v", gotSession)
	}
}

func newTestService(t *testing.T, runtime Runtime) (*Service, *db.DB) {
	t.Helper()

	database, err := db.Open(filepath.Join(t.TempDir(), "termia.db"), zap.NewNop())
	if err != nil {
		t.Fatalf("db.Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = database.Close()
	})

	service := NewService(&config.Config{}, database, func(_ *config.Config, _ *db.DB, _ runtimeagent.HITLResponder) Runtime {
		return runtime
	})
	return service, database
}

func createServiceTestSession(t *testing.T, database *db.DB, state SessionState, cwd string) db.AgentSession {
	t.Helper()

	snapshot, err := EncodeSessionState(state)
	if err != nil {
		t.Fatalf("EncodeSessionState returned error: %v", err)
	}

	session := db.AgentSession{
		ID:               "session-1",
		Name:             "Session 1",
		Mode:             string(state.Mode),
		TeamName:         state.TeamName,
		SpecSnapshotJSON: snapshot,
		Cwd:              cwd,
		CreatedAt:        time.Unix(10, 0).UnixNano(),
		UpdatedAt:        time.Unix(10, 0).UnixNano(),
	}
	if err := database.CreateAgentSession(&session); err != nil {
		t.Fatalf("CreateAgentSession returned error: %v", err)
	}
	return session
}

func createServiceTestMessage(t *testing.T, database *db.DB, sessionID, role, content string, seq int64) {
	t.Helper()

	if err := database.CreateAgentMessage(&db.AgentMessage{
		ID:        sessionID + "-" + role + "-" + time.Unix(seq, 0).UTC().Format("150405"),
		SessionID: sessionID,
		Role:      role,
		Content:   content,
		CreatedAt: time.Unix(seq, 0).UnixNano(),
	}); err != nil {
		t.Fatalf("CreateAgentMessage returned error: %v", err)
	}
}
