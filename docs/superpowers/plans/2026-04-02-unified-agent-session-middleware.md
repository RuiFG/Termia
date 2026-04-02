# Unified Agent Session And Middleware Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace duplicated `tai`/`tui` agent orchestration with one session-centric application service that supports explicit run-scoped and session-scoped middleware.

**Architecture:** Keep `internal/agent` as the runtime/tool/model layer and introduce `internal/agentapp` as the application layer that owns session resolution, middleware, timeline reduction, and message persistence. `cmd/tai` and `internal/tui` become thin adapters that call the same service and render the same runtime events.

**Tech Stack:** Go 1.26, Cobra, Bubble Tea/Lipgloss, SQLite via `modernc.org/sqlite`, existing `internal/db`, existing `internal/sessionstate`, existing `internal/agent` runtime.

---

## Target File Structure

### Create
- `internal/agentapp/types.go`
- `internal/agentapp/session.go`
- `internal/agentapp/timeline.go`
- `internal/agentapp/middleware.go`
- `internal/agentapp/slash.go`
- `internal/agentapp/service.go`
- `internal/agentapp/session_test.go`
- `internal/agentapp/timeline_test.go`
- `internal/agentapp/middleware_test.go`
- `internal/agentapp/service_test.go`
- `internal/agent/runtime.go`
- `internal/agent/runtime_prompt.go`
- `internal/agent/runtime_events.go`

### Modify
- `internal/agent/agent.go`
- `internal/agent/agent_test.go`
- `cmd/tai.go`
- `cmd/tai_test.go`
- `cmd/tai_session_test.go`
- `internal/tui/app.go`
- `internal/tui/agent.go`
- `internal/tui/agent_test.go`
- `internal/tui/commands.go`
- `internal/tui/input.go`

### Delete
- `cmd/tai_session.go`

### Design Rules
- This is a direct cutover, not a compatibility migration.
- Do not keep duplicate reducer or session helper code after the new service is live.
- Do not move TUI rendering into `internal/agentapp`.
- Keep UI-only slash commands in TUI and shared agent commands in `internal/agentapp`.

### Task 1: Add Shared Session And Timeline Types

**Files:**
- Create: `internal/agentapp/types.go`
- Create: `internal/agentapp/timeline.go`
- Test: `internal/agentapp/session_test.go`
- Test: `internal/agentapp/timeline_test.go`

- [ ] **Step 1: Write the failing tests for session-state JSON and timeline reduction**

```go
package agentapp

import (
	"testing"

	runtimeagent "github.com/termia/termia/internal/agent"
)

func TestSessionStateJSONRoundTrip(t *testing.T) {
	state := SessionState{
		Mode:     runtimeagent.ModeTeam,
		TeamName: "ops",
		SessionMiddleware: []MiddlewareActivation{
			{Name: "persisted", Scope: MiddlewareScopeSession, Args: map[string]string{"mode": "safe"}},
		},
	}

	raw, err := EncodeSessionState(state)
	if err != nil {
		t.Fatalf("EncodeSessionState returned error: %v", err)
	}

	got, err := DecodeSessionState(raw)
	if err != nil {
		t.Fatalf("DecodeSessionState returned error: %v", err)
	}

	if got.Mode != runtimeagent.ModeTeam || got.TeamName != "ops" {
		t.Fatalf("unexpected state: %+v", got)
	}
	if len(got.SessionMiddleware) != 1 || got.SessionMiddleware[0].Name != "persisted" {
		t.Fatalf("unexpected middleware: %+v", got.SessionMiddleware)
	}
}

func TestAppendTimelineTextMergesAdjacentAssistantChunks(t *testing.T) {
	input := []TimelineEntry{{Role: "assistant", Content: "hel"}}
	got := AppendTimelineText(input, "assistant", "lo", true)
	if len(got) != 1 || got[0].Content != "hello" {
		t.Fatalf("unexpected merged timeline: %+v", got)
	}
}

func TestUpsertTimelineToolCallMergesPendingAndSuccess(t *testing.T) {
	pending := runtimeagent.ToolCallEvent{
		CallID:   "call-1",
		ToolName: "command",
		Summary:  "pwd",
		State:    runtimeagent.ToolCallStatePending,
	}
	success := runtimeagent.ToolCallEvent{
		CallID:   "call-1",
		ToolName: "command",
		Summary:  "pwd",
		Result:   "ok",
		State:    runtimeagent.ToolCallStateSuccess,
	}

	timeline := UpsertTimelineToolCall(nil, pending)
	timeline = UpsertTimelineToolCall(timeline, success)

	if len(timeline) != 1 {
		t.Fatalf("expected one timeline entry, got %d", len(timeline))
	}
	if timeline[0].ToolCall == nil || timeline[0].ToolCall.State != runtimeagent.ToolCallStateSuccess {
		t.Fatalf("unexpected tool timeline entry: %+v", timeline[0])
	}
}
```

- [ ] **Step 2: Run the package tests and verify they fail**

Run: `go test ./internal/agentapp -run "TestSessionStateJSONRoundTrip|TestAppendTimelineTextMergesAdjacentAssistantChunks|TestUpsertTimelineToolCallMergesPendingAndSuccess" -v`

Expected: FAIL with missing package or undefined symbols such as `SessionState`, `EncodeSessionState`, or `TimelineEntry`.

- [ ] **Step 3: Create the shared types and reducer**

```go
package agentapp

import (
	"encoding/json"
	"strings"

	runtimeagent "github.com/termia/termia/internal/agent"
	"github.com/termia/termia/internal/textutil"
)

type MiddlewareScope string

const (
	MiddlewareScopeRun     MiddlewareScope = "run"
	MiddlewareScopeSession MiddlewareScope = "session"
)

type MiddlewareActivation struct {
	Name  string            `json:"name"`
	Scope MiddlewareScope   `json:"scope"`
	Args  map[string]string `json:"args,omitempty"`
}

type SessionState struct {
	Mode              runtimeagent.Mode      `json:"mode"`
	TeamName          string                 `json:"team_name,omitempty"`
	SessionMiddleware []MiddlewareActivation `json:"session_middleware,omitempty"`
}

type TimelineEntry struct {
	Role              string
	Content           string
	CitedCommandCount int
	ToolCall          *runtimeagent.ToolCallEvent
}

func DefaultSessionState() SessionState {
	return SessionState{Mode: runtimeagent.ModeAssistant}
}

func EncodeSessionState(state SessionState) (string, error) {
	if strings.TrimSpace(string(state.Mode)) == "" {
		state.Mode = runtimeagent.ModeAssistant
	}
	if state.Mode != runtimeagent.ModeTeam {
		state.TeamName = ""
	}
	data, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func DecodeSessionState(raw string) (SessionState, error) {
	if strings.TrimSpace(raw) == "" {
		return DefaultSessionState(), nil
	}
	var state SessionState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return SessionState{}, err
	}
	if strings.TrimSpace(string(state.Mode)) == "" {
		state.Mode = runtimeagent.ModeAssistant
	}
	if state.Mode != runtimeagent.ModeTeam {
		state.TeamName = ""
	}
	return state, nil
}

func NormalizeRole(role string) string {
	role = strings.TrimSpace(strings.ToLower(role))
	switch role {
	case "user":
		return "user"
	case "assistant", "agent":
		return "assistant"
	case "tool":
		return "tool"
	case "system":
		return "system"
	case "reasoning":
		return "reasoning"
	case "error":
		return "error"
	default:
		if role == "" {
			return "assistant"
		}
		return role
	}
}

func AppendTimelineText(entries []TimelineEntry, role, content string, appendToLast bool) []TimelineEntry {
	content = textutil.NormalizeLineEndings(content)
	if content == "" {
		return entries
	}
	role = NormalizeRole(role)
	if appendToLast && len(entries) > 0 {
		last := &entries[len(entries)-1]
		if last.ToolCall == nil && NormalizeRole(last.Role) == role {
			last.Content += content
			return entries
		}
	}
	return append(entries, TimelineEntry{Role: role, Content: content})
}

func UpsertTimelineToolCall(entries []TimelineEntry, call runtimeagent.ToolCallEvent) []TimelineEntry {
	call.CallID = strings.TrimSpace(call.CallID)
	call.ToolName = textutil.NormalizeInlineText(call.ToolName)
	call.Summary = textutil.NormalizeInlineText(call.Summary)
	call.Result = textutil.NormalizeInlineText(call.Result)
	call.AgentName = textutil.NormalizeInlineText(call.AgentName)
	if call.ToolName == "" {
		return entries
	}
	for i := len(entries) - 1; i >= 0; i-- {
		current := entries[i].ToolCall
		if current == nil {
			continue
		}
		if call.CallID != "" && current.CallID == call.CallID {
			merged := *current
			if merged.Result == "" {
				merged.Result = call.Result
			}
			if call.State != "" {
				merged.State = call.State
			}
			entries[i].ToolCall = &merged
			return entries
		}
	}
	return append(entries, TimelineEntry{Role: "tool", ToolCall: &call})
}

func MarkLatestPendingToolFailed(entries []TimelineEntry, reason string) []TimelineEntry {
	reason = textutil.NormalizeInlineText(reason)
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].ToolCall == nil {
			continue
		}
		if entries[i].ToolCall.State != runtimeagent.ToolCallStatePending {
			continue
		}
		call := *entries[i].ToolCall
		call.State = runtimeagent.ToolCallStateError
		if call.Result == "" {
			call.Result = reason
		}
		entries[i].ToolCall = &call
		return entries
	}
	return entries
}
```

- [ ] **Step 4: Run the new package tests and verify they pass**

Run: `go test ./internal/agentapp -run "TestSessionStateJSONRoundTrip|TestAppendTimelineTextMergesAdjacentAssistantChunks|TestUpsertTimelineToolCallMergesPendingAndSuccess" -v`

Expected: PASS for all three tests.

- [ ] **Step 5: Commit**

```bash
git add internal/agentapp/types.go internal/agentapp/timeline.go internal/agentapp/session_test.go internal/agentapp/timeline_test.go
git commit -m "refactor: add shared agentapp state and timeline types"
```

### Task 2: Add Session Resolution Helpers

**Files:**
- Create: `internal/agentapp/session.go`
- Test: `internal/agentapp/session_test.go`

- [ ] **Step 1: Extend the failing tests for active-session resolution**

```go
package agentapp

import (
	"testing"
	"time"

	"github.com/termia/termia/internal/db"
)

type fakeSessionDB struct {
	current    db.AgentSession
	hasCurrent bool
	created    *db.AgentSession
}

func (f *fakeSessionDB) GetAgentSession(id string) (db.AgentSession, bool, error) {
	if f.hasCurrent && f.current.ID == id {
		return f.current, true, nil
	}
	return db.AgentSession{}, false, nil
}

func (f *fakeSessionDB) LatestAgentSession() (db.AgentSession, bool, error) {
	if f.hasCurrent {
		return f.current, true, nil
	}
	return db.AgentSession{}, false, nil
}

func (f *fakeSessionDB) CreateAgentSession(session *db.AgentSession) error {
	copied := *session
	f.created = &copied
	return nil
}

func TestSessionServiceResolveCreatesAssistantSessionWhenMissing(t *testing.T) {
	svc := NewSessionService(&fakeSessionDB{})
	session, state, err := svc.Resolve("", "/tmp/project", DefaultSessionState(), time.Now)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if session.Cwd != "/tmp/project" {
		t.Fatalf("unexpected session cwd: %+v", session)
	}
	if state.Mode == "" {
		t.Fatalf("expected default mode, got %+v", state)
	}
}
```

- [ ] **Step 2: Run the session-service tests and verify they fail**

Run: `go test ./internal/agentapp -run "TestSessionServiceResolveCreatesAssistantSessionWhenMissing" -v`

Expected: FAIL with undefined symbols such as `NewSessionService`.

- [ ] **Step 3: Implement the session service**

```go
package agentapp

import (
	"fmt"
	"strings"
	"time"

	"github.com/termia/termia/internal/db"
	"github.com/termia/termia/internal/sessionstate"
)

type SessionDB interface {
	GetAgentSession(id string) (db.AgentSession, bool, error)
	LatestAgentSession() (db.AgentSession, bool, error)
	CreateAgentSession(session *db.AgentSession) error
}

type SessionService struct {
	db SessionDB
}

func NewSessionService(database SessionDB) *SessionService {
	return &SessionService{db: database}
}

func (s *SessionService) Resolve(preferredID, cwd string, defaultState SessionState, now func() time.Time) (db.AgentSession, SessionState, error) {
	if now == nil {
		now = time.Now
	}

	candidates := []string{
		strings.TrimSpace(preferredID),
		strings.TrimSpace(sessionstate.CurrentID()),
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		record, ok, err := s.db.GetAgentSession(candidate)
		if err != nil {
			return db.AgentSession{}, SessionState{}, err
		}
		if ok {
			state, err := DecodeSessionState(record.SpecSnapshotJSON)
			if err != nil {
				return db.AgentSession{}, SessionState{}, err
			}
			return record, state, nil
		}
	}

	record, ok, err := s.db.LatestAgentSession()
	if err != nil {
		return db.AgentSession{}, SessionState{}, err
	}
	if ok {
		state, err := DecodeSessionState(record.SpecSnapshotJSON)
		if err != nil {
			return db.AgentSession{}, SessionState{}, err
		}
		return record, state, nil
	}

	raw, err := EncodeSessionState(defaultState)
	if err != nil {
		return db.AgentSession{}, SessionState{}, err
	}

	ts := now().UnixNano()
	session := db.AgentSession{
		ID:               fmt.Sprintf("%d", ts),
		Name:             fmt.Sprintf("Session %s", now().Format("2006-01-02 15:04")),
		Mode:             string(defaultState.Mode),
		TeamName:         strings.TrimSpace(defaultState.TeamName),
		SpecSnapshotJSON: raw,
		Cwd:              strings.TrimSpace(cwd),
		CreatedAt:        ts,
		UpdatedAt:        ts,
	}
	if err := s.db.CreateAgentSession(&session); err != nil {
		return db.AgentSession{}, SessionState{}, err
	}
	return session, defaultState, nil
}
```

- [ ] **Step 4: Run the session-service tests and verify they pass**

Run: `go test ./internal/agentapp -run "TestSessionServiceResolveCreatesAssistantSessionWhenMissing|TestSessionStateJSONRoundTrip" -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/agentapp/session.go internal/agentapp/session_test.go
git commit -m "refactor: add shared agent session resolution"
```

### Task 3: Add Middleware Registry And Shared Slash Commands

**Files:**
- Create: `internal/agentapp/middleware.go`
- Create: `internal/agentapp/slash.go`
- Test: `internal/agentapp/middleware_test.go`

- [ ] **Step 1: Write failing tests for middleware registration and `/ralph-loop`**

```go
package agentapp

import (
	"context"
	"testing"
)

func TestRegistryRejectsScopeMismatch(t *testing.T) {
	registry := NewRegistry(DefaultMiddlewareSpecs()...)
	_, err := registry.Build(MiddlewareActivation{Name: "ralph-loop", Scope: MiddlewareScopeSession})
	if err == nil {
		t.Fatal("expected scope mismatch error")
	}
}

func TestResolveSharedSlashCommandBuildsRunScopedActivation(t *testing.T) {
	command, ok := ResolveSharedSlashCommand("/ralph-loop", DefaultSharedSlashCommands())
	if !ok {
		t.Fatal("expected command to resolve")
	}
	activation, err := command.BuildActivation("")
	if err != nil {
		t.Fatalf("BuildActivation returned error: %v", err)
	}
	if activation.Scope != MiddlewareScopeRun || activation.Name != "ralph-loop" {
		t.Fatalf("unexpected activation: %+v", activation)
	}
}

func TestRalphLoopRequestsContinueAfterCommandRun(t *testing.T) {
	registry := NewRegistry(DefaultMiddlewareSpecs()...)
	mw, err := registry.Build(MiddlewareActivation{Name: "ralph-loop", Scope: MiddlewareScopeRun})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	directive, err := mw.AfterRun(context.Background(), &RunContext{Query: "fix it"}, RunSummary{SawCommand: true})
	if err != nil {
		t.Fatalf("AfterRun returned error: %v", err)
	}
	if !directive.Continue || directive.NextQuery == "" {
		t.Fatalf("expected continue directive, got %+v", directive)
	}
}
```

- [ ] **Step 2: Run the middleware tests and verify they fail**

Run: `go test ./internal/agentapp -run "TestRegistryRejectsScopeMismatch|TestResolveSharedSlashCommandBuildsRunScopedActivation|TestRalphLoopRequestsContinueAfterCommandRun" -v`

Expected: FAIL with undefined symbols such as `NewRegistry`, `ResolveSharedSlashCommand`, or `RunSummary`.

- [ ] **Step 3: Implement the registry, slash registry, and builtin middleware**

```go
package agentapp

import (
	"context"
	"fmt"
	"sort"
	"strings"

	runtimeagent "github.com/termia/termia/internal/agent"
)

type RunContext struct {
	SessionID        string
	Query            string
	Cwd              string
	State            SessionState
	SelectedCommands []runtimeagent.Command
}

type RunSummary struct {
	SawCommand bool
}

type RunDirective struct {
	Continue bool
	NextQuery string
	EmitText string
}

type Middleware interface {
	BeforeRun(context.Context, *RunContext) error
	AfterRun(context.Context, *RunContext, RunSummary) (RunDirective, error)
}

type MiddlewareFactory func(MiddlewareActivation) (Middleware, error)

type MiddlewareSpec struct {
	Name        string
	Description string
	Scope       MiddlewareScope
	Factory     MiddlewareFactory
}

type Registry struct {
	specs map[string]MiddlewareSpec
}

func NewRegistry(specs ...MiddlewareSpec) *Registry {
	index := make(map[string]MiddlewareSpec, len(specs))
	for _, spec := range specs {
		index[spec.Name] = spec
	}
	return &Registry{specs: index}
}

func (r *Registry) Build(activation MiddlewareActivation) (Middleware, error) {
	spec, ok := r.specs[activation.Name]
	if !ok {
		return nil, fmt.Errorf("unknown middleware %q", activation.Name)
	}
	if spec.Scope != activation.Scope {
		return nil, fmt.Errorf("middleware %q requires %s scope", activation.Name, spec.Scope)
	}
	return spec.Factory(activation)
}

type SharedSlashCommand struct {
	Name            string
	Description     string
	Scope           MiddlewareScope
	BuildActivation func(args string) (MiddlewareActivation, error)
}

func ResolveSharedSlashCommand(input string, commands []SharedSlashCommand) (SharedSlashCommand, bool) {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/") {
		return SharedSlashCommand{}, false
	}
	name := strings.TrimPrefix(strings.SplitN(input, " ", 2)[0], "/")
	for _, command := range commands {
		if command.Name == name {
			return command, true
		}
	}
	return SharedSlashCommand{}, false
}

func DefaultSharedSlashCommands() []SharedSlashCommand {
	commands := []SharedSlashCommand{
		{
			Name:        "ralph-loop",
			Description: "repeat the run until a turn ends without command execution",
			Scope:       MiddlewareScopeRun,
			BuildActivation: func(args string) (MiddlewareActivation, error) {
				return MiddlewareActivation{Name: "ralph-loop", Scope: MiddlewareScopeRun}, nil
			},
		},
	}
	sort.Slice(commands, func(i, j int) bool { return commands[i].Name < commands[j].Name })
	return commands
}

type ralphLoopMiddleware struct{}

func (m *ralphLoopMiddleware) BeforeRun(context.Context, *RunContext) error {
	return nil
}

func (m *ralphLoopMiddleware) AfterRun(_ context.Context, _ *RunContext, summary RunSummary) (RunDirective, error) {
	if !summary.SawCommand {
		return RunDirective{EmitText: "已完成"}, nil
	}
	return RunDirective{
		Continue: true,
		NextQuery: "Continue from the current state. Only stop when no shell command is needed. When finished, reply with 已完成。",
	}, nil
}

func DefaultMiddlewareSpecs() []MiddlewareSpec {
	return []MiddlewareSpec{
		{
			Name:        "ralph-loop",
			Description: "repeat the run until a turn ends without command execution",
			Scope:       MiddlewareScopeRun,
			Factory: func(MiddlewareActivation) (Middleware, error) {
				return &ralphLoopMiddleware{}, nil
			},
		},
	}
}
```

- [ ] **Step 4: Run the middleware tests and verify they pass**

Run: `go test ./internal/agentapp -run "TestRegistryRejectsScopeMismatch|TestResolveSharedSlashCommandBuildsRunScopedActivation|TestRalphLoopRequestsContinueAfterCommandRun" -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/agentapp/middleware.go internal/agentapp/slash.go internal/agentapp/middleware_test.go
git commit -m "feat: add shared middleware and slash registries"
```

### Task 4: Build The Unified Agent Application Service

**Files:**
- Create: `internal/agentapp/service.go`
- Test: `internal/agentapp/service_test.go`

- [ ] **Step 1: Write the failing application-service tests**

```go
package agentapp

import (
	"context"
	"testing"

	runtimeagent "github.com/termia/termia/internal/agent"
)

type fakeRuntime struct {
	events []runtimeagent.RuntimeEvent
}

func (f fakeRuntime) Run(ctx context.Context, req runtimeagent.RunRequest) (<-chan runtimeagent.RuntimeEvent, error) {
	ch := make(chan runtimeagent.RuntimeEvent, len(f.events))
	go func() {
		defer close(ch)
		for _, event := range f.events {
			select {
			case <-ctx.Done():
				return
			case ch <- event:
			}
		}
	}()
	return ch, nil
}

func TestServiceRunEmitsRuntimeEventsAndTracksCommands(t *testing.T) {
	svc := newTestService(fakeRuntime{events: []runtimeagent.RuntimeEvent{
		{Kind: runtimeagent.RuntimeEventToolCall, ToolCall: &runtimeagent.ToolCallEvent{ToolName: "command", State: runtimeagent.ToolCallStatePending}},
		{Kind: runtimeagent.RuntimeEventText, Text: "done"},
	}})

	stream, err := svc.Run(context.Background(), RunRequest{Query: "check", Cwd: "/tmp/project"})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	var sawText bool
	for event := range stream {
		if event.Kind == runtimeagent.RuntimeEventText && event.Text == "done" {
			sawText = true
		}
	}

	if !sawText {
		t.Fatal("expected runtime text event")
	}
}

func TestServiceRunResolvesSharedSlashMiddleware(t *testing.T) {
	svc := newTestService(fakeRuntime{})
	_, err := svc.Run(context.Background(), RunRequest{Query: "/ralph-loop", Cwd: "/tmp/project"})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(svc.lastRunMiddleware) != 1 || svc.lastRunMiddleware[0].Name != "ralph-loop" {
		t.Fatalf("unexpected run middleware: %+v", svc.lastRunMiddleware)
	}
}
```

- [ ] **Step 2: Run the service tests and verify they fail**

Run: `go test ./internal/agentapp -run "TestServiceRunEmitsRuntimeEventsAndTracksCommands|TestServiceRunResolvesSharedSlashMiddleware" -v`

Expected: FAIL with undefined symbols such as `RunRequest`, `Run`, or `newTestService`.

- [ ] **Step 3: Implement the application service**

```go
package agentapp

import (
	"context"
	"strings"

	runtimeagent "github.com/termia/termia/internal/agent"
	"github.com/termia/termia/internal/config"
	"github.com/termia/termia/internal/db"
)

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
	runMiddleware := make([]MiddlewareActivation, 0, 1)
	if command, ok := ResolveSharedSlashCommand(req.Query, s.sharedCommands); ok {
		args := strings.TrimSpace(strings.TrimPrefix(req.Query, "/"+command.Name))
		activation, err := command.BuildActivation(strings.TrimSpace(args))
		if err != nil {
			return nil, err
		}
		runMiddleware = append(runMiddleware, activation)
		req.Query = ""
	}
	s.lastRunMiddleware = append([]MiddlewareActivation(nil), runMiddleware...)

	session, state, err := s.sessions.Resolve(req.SessionID, req.Cwd, DefaultSessionState(), nil)
	if err != nil {
		return nil, err
	}

	middleware := make([]Middleware, 0, len(state.SessionMiddleware)+len(runMiddleware))
	for _, activation := range append(append([]MiddlewareActivation(nil), state.SessionMiddleware...), runMiddleware...) {
		item, err := s.registry.Build(activation)
		if err != nil {
			return nil, err
		}
		middleware = append(middleware, item)
	}

	runCtx := &RunContext{
		SessionID:        session.ID,
		Query:            strings.TrimSpace(req.Query),
		Cwd:              strings.TrimSpace(req.Cwd),
		State:            state,
		SelectedCommands: req.SelectedCommands,
	}
	for _, item := range middleware {
		if err := item.BeforeRun(ctx, runCtx); err != nil {
			return nil, err
		}
	}

	runtime := s.runtimeFactory(s.cfg, s.db, req.Responder)
	stream, err := runtime.Run(ctx, runtimeagent.RunRequest{
		Mode:             state.Mode,
		TeamName:         state.TeamName,
		SessionID:        session.ID,
		Query:            runCtx.Query,
		Cwd:              runCtx.Cwd,
		SelectedCommands: runCtx.SelectedCommands,
		StreamReader:     req.StreamReader,
	})
	if err != nil {
		return nil, err
	}

	out := make(chan runtimeagent.RuntimeEvent, 32)
	go func() {
		defer close(out)
		summary := RunSummary{}
		for event := range stream {
			if event.Kind == runtimeagent.RuntimeEventToolCall && event.ToolCall != nil && event.ToolCall.ToolName == "command" {
				summary.SawCommand = true
			}
			out <- event
		}
		for _, item := range middleware {
			directive, err := item.AfterRun(ctx, runCtx, summary)
			if err != nil {
				out <- runtimeagent.RuntimeEvent{Kind: runtimeagent.RuntimeEventError, Text: err.Error()}
				return
			}
			if directive.EmitText != "" {
				out <- runtimeagent.RuntimeEvent{Kind: runtimeagent.RuntimeEventText, Text: directive.EmitText}
			}
		}
	}()
	return out, nil
}
```

- [ ] **Step 4: Run the service tests and the full package tests**

Run: `go test ./internal/agentapp -v`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/agentapp/service.go internal/agentapp/service_test.go
git commit -m "refactor: add unified agent application service"
```

### Task 5: Split `internal/agent` Into Runtime-Focused Files

**Files:**
- Create: `internal/agent/runtime.go`
- Create: `internal/agent/runtime_prompt.go`
- Create: `internal/agent/runtime_events.go`
- Modify: `internal/agent/agent.go`
- Modify: `internal/agent/agent_test.go`

- [ ] **Step 1: Write or extend tests that pin runtime behavior before moving code**

```go
package agent

import "testing"

func TestBuildPromptTextIncludesCommandMetadata(t *testing.T) {
	req := RunRequest{
		Query: "summarize",
		SelectedCommands: []Command{
			{ID: "cmd-1", Command: "go test ./...", Cwd: "/tmp/project", TranscriptAvailable: true},
		},
	}
	text := buildPromptText(req, "")
	if text == "" {
		t.Fatal("expected prompt text")
	}
	if !containsAll(text, "Referenced terminal commands", "cmd-1", "go test ./...") {
		t.Fatalf("unexpected prompt text: %q", text)
	}
}
```

- [ ] **Step 2: Run the targeted runtime tests and verify current behavior**

Run: `go test ./internal/agent -run "TestBuildPromptTextIncludesCommandMetadata" -v`

Expected: PASS before the move.

- [ ] **Step 3: Move code without changing behavior**

```go
// internal/agent/runtime.go
package agent

// Place these methods in this file without changing their signatures or bodies:
// - func (r *Runtime) Run(...)
// - func (r *Runtime) runConversation(...)
// - func (r *Runtime) runTurn(...)
// - func (r *Runtime) buildRootAgent(...)
// - func (r *Runtime) buildAssistantAgent(...)
// - func (r *Runtime) buildTeamAgent(...)
// - func (r *Runtime) buildChatModelAgent(...)

// internal/agent/runtime_prompt.go
package agent

// Place these helpers in this file without changing their signatures or bodies:
// - buildPromptContent
// - buildStreamPromptContent
// - buildPromptText
// - renderPromptMessage
// - renderPromptMessageBody
// - renderPromptCommands
// - renderPromptCommand
// - indentPromptBlock
// - expandFileMentions

// internal/agent/runtime_events.go
package agent

// Place these helpers in this file without changing their signatures or bodies:
// - consumeRuntimeIterator
// - collectEventMessage
// - materializeMessage
// - emitMessageEvents
// - assistantContentEvents
// - extractToolCallEvents
// - extractToolResultEvents
// - extractCommandCwdEvent
// - summarizeToolCall
// - summarizeToolResultTarget
// - summarizeToolResult
// - summarizeToolResultState
```

- [ ] **Step 4: Run the full runtime package tests and verify they still pass**

Run: `go test ./internal/agent -v`

Expected: PASS with no behavior regressions.

- [ ] **Step 5: Commit**

```bash
git add internal/agent/runtime.go internal/agent/runtime_prompt.go internal/agent/runtime_events.go internal/agent/agent.go internal/agent/agent_test.go
git commit -m "refactor: split agent runtime responsibilities"
```

### Task 6: Cut `tai` Over To The Shared Application Service

**Files:**
- Modify: `cmd/tai.go`
- Modify: `cmd/tai_test.go`
- Delete: `cmd/tai_session.go`
- Modify: `cmd/tai_session_test.go`

- [ ] **Step 1: Write a failing `tai` test that proves it uses the shared service**

```go
package cmd

import "testing"

func TestTaiUsesSharedAgentApplicationService(t *testing.T) {
	service := &fakeAgentAppService{}
	runTaiWithService(t, service, "inspect this")
	if service.runCount != 1 {
		t.Fatalf("expected one service run, got %d", service.runCount)
	}
}
```

- [ ] **Step 2: Run the `cmd` tests and verify the new test fails**

Run: `go test ./cmd -run "TestTaiUsesSharedAgentApplicationService" -v`

Expected: FAIL with undefined helpers such as `fakeAgentAppService`.

- [ ] **Step 3: Replace adapter-owned session logic with the shared service**

```go
service := agentapp.NewService(cfg, database, func(cfg *config.Config, database *db.DB, responder agent.HITLResponder) agentapp.Runtime {
	return agent.NewRuntime(cfg, database, responder)
})

stream, err := service.Run(ctx, agentapp.RunRequest{
	Query:            userQuery,
	Cwd:              runCwd,
	SelectedCommands: taiAgentCommandsFromDBCommands(selectedCommands),
	StreamReader:     streamReader,
	Responder:        agent.NewCLIResponder(),
})
if err != nil {
	return fmt.Errorf("failed to run analysis: %w", err)
}
```

- [ ] **Step 4: Delete the duplicated session helper file and clean the tests**

```go
// Remove cmd/tai_session.go entirely.
// Delete cmd/tai_session_test.go after copying its session-resolution assertions
// into internal/agentapp/session_test.go.
```

- [ ] **Step 5: Run all `cmd` tests and verify they pass**

Run: `go test ./cmd -v`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/tai.go cmd/tai_test.go cmd/tai_session_test.go
git rm cmd/tai_session.go
git commit -m "refactor: route tai through shared agent application service"
```

### Task 7: Cut TUI Over To The Shared Service And Split Slash Commands

**Files:**
- Modify: `internal/tui/app.go`
- Modify: `internal/tui/agent.go`
- Modify: `internal/tui/agent_test.go`
- Modify: `internal/tui/commands.go`
- Modify: `internal/tui/input.go`

- [ ] **Step 1: Write failing tests for shared slash suggestions and TUI runtime delegation**

```go
package tui

import "testing"

func TestInputModelIncludesSharedAgentSlashSuggestions(t *testing.T) {
	model := NewInputModel()
	model.SetSlashSuggestions([]SlashSuggestion{
		{Name: "exit", Desc: "exit tui"},
		{Name: "ralph-loop", Desc: "repeat runs until no command is needed"},
	})
	model.SetValue("/r")
	suggestions := model.SlashSuggestions()
	if len(suggestions) != 1 || suggestions[0].Name != "ralph-loop" {
		t.Fatalf("unexpected suggestions: %+v", suggestions)
	}
}

func TestAgentPanelNoLongerOwnsTimelineReducerRules(t *testing.T) {
	model := NewAgentModel(DefaultKeyMap())
	model.SetMessages([]AgentMessage{{Role: "assistant", Content: "one"}})
	if len(model.Messages()) != 1 {
		t.Fatalf("unexpected messages: %+v", model.Messages())
	}
}
```

- [ ] **Step 2: Run the TUI tests and verify the new tests fail**

Run: `go test ./internal/tui -run "TestInputModelIncludesSharedAgentSlashSuggestions|TestAgentPanelNoLongerOwnsTimelineReducerRules" -v`

Expected: FAIL with missing `SetSlashSuggestions` or `Messages` helpers.

- [ ] **Step 3: Make TUI a thin adapter over `agentapp.Service`**

```go
func (a *App) runAIQuery(ctx context.Context, query string, selectedCommands []agent.Command) (<-chan agent.RuntimeEvent, error) {
	if a.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	service := agentapp.NewService(a.cfg, a.db, func(cfg *config.Config, database *db.DB, responder agent.HITLResponder) agentapp.Runtime {
		return agent.NewRuntime(cfg, database, responder)
	})

	return service.Run(ctx, agentapp.RunRequest{
		SessionID:        strings.TrimSpace(a.activeSessionID),
		Query:            query,
		Cwd:              strings.TrimSpace(a.cwd),
		SelectedCommands: selectedCommands,
		Responder:        newTUIResponder(a.approvalRequests, a.askRequests),
	})
}
```

- [ ] **Step 4: Remove reducer logic from the TUI panel and keep rendering only**

```go
// internal/tui/agent.go
// Delete:
// - appendTimelineText
// - upsertTimelineToolCall
// - markLatestPendingToolFailed
// - normalizeToolCall
// - mergeToolCall
//
// Replace call sites in app.go with:
// - agentapp.AppendTimelineText
// - agentapp.UpsertTimelineToolCall
// - agentapp.MarkLatestPendingToolFailed
```

- [ ] **Step 5: Split slash commands into UI-local and shared-agent sets**

```go
// internal/tui/commands.go
func localSlashCommands() []SlashSuggestion {
	return []SlashSuggestion{
		{Name: "exit", Desc: "exit tui"},
	}
}

// internal/tui/input.go
func (m *InputModel) SetSlashSuggestions(suggestions []SlashSuggestion) {
	m.slashSuggestions = append([]SlashSuggestion(nil), suggestions...)
}

// internal/tui/app.go during initialization
shared := agentapp.DefaultSharedSlashCommands()
suggestions := localSlashCommands()
for _, command := range shared {
	suggestions = append(suggestions, SlashSuggestion{Name: command.Name, Desc: command.Description})
}
a.input.SetSlashSuggestions(suggestions)
```

- [ ] **Step 6: Run the TUI tests and then the full suite**

Run: `go test ./internal/tui -v`
Expected: PASS

Run: `go test ./...`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/tui/app.go internal/tui/agent.go internal/tui/agent_test.go internal/tui/commands.go internal/tui/input.go
git commit -m "refactor: route tui through shared agent application service"
```

## Self-Review

### Spec Coverage
- Single session truth source: Tasks 2, 4, 6, 7
- `tai` and `tui` sharing the same session: Tasks 4, 6, 7
- Explicit middleware scope at registration: Task 3
- Run-scoped and session-scoped middleware support: Tasks 1, 2, 3, 4
- Shared slash command behavior: Tasks 3 and 7
- Runtime/application boundary cleanup: Task 5
- Removing duplicated timeline logic: Tasks 1 and 7

### Placeholder Scan
- No `TODO`, `TBD`, or “implement later” markers remain in the tasks.
- Every task has concrete file paths, tests, commands, and commit messages.

### Type Consistency
- `MiddlewareScope`, `MiddlewareActivation`, `SessionState`, `RunContext`, `RunSummary`, and `RunDirective` are used consistently across tasks.
- `internal/agentapp` owns application-layer orchestration; `internal/agent` stays runtime-only throughout the plan.
