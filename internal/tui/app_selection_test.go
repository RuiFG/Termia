package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/termia/termia/internal/agent"
	"github.com/termia/termia/internal/agentapp"
	"github.com/termia/termia/internal/config"
	"github.com/termia/termia/internal/db"
	"go.uber.org/zap"
)

func TestTextSelectionHighlightKeepsANSIAndAddsVisibleFeedback(t *testing.T) {
	var selection textSelection
	plain := []string{"HELLO", "WORLD"}
	rendered := []string{"\x1b[31mHELLO\x1b[0m", "\x1b[34mWORLD\x1b[0m"}

	selection.SetLines(plain)
	selection.SetRenderLines(rendered)
	selection.BeginSelection(0, 0)
	selection.UpdateSelection(1, 3)

	highlighted := selection.HighlightLines(16)
	if len(highlighted) != 2 {
		t.Fatalf("expected 2 highlighted lines, got %d", len(highlighted))
	}
	if strings.Contains(highlighted[0], "\x1b[7m\x1b[31m") {
		t.Fatalf("expected selected text to avoid reusing its original foreground color inside the selection, got %q", highlighted[0])
	}
	if !strings.Contains(highlighted[1], "\x1b[34m") {
		t.Fatalf("expected unselected suffix to keep its original foreground color, got %q", highlighted[1])
	}
	if highlighted[0] == padToWidth(rendered[0], 16) && highlighted[1] == padToWidth(rendered[1], 16) {
		t.Fatalf("expected highlighted output to differ from original render")
	}
}

func TestMouseReleaseKeepsContentSelectionActive(t *testing.T) {
	app := New(nil, config.DefaultConfig(), nil)
	app.width = 120
	app.height = 30
	app.ready = true
	app.layoutPanels()
	app.agent.AddMessage("assistant", "alpha beta gamma")

	innerX, innerY := panelInnerOrigin(contentPaneStyle, app.leftXStart, app.contentYStart)
	model, _ := app.handleMouse(tea.MouseMsg{
		X:      innerX,
		Y:      innerY,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})
	app = model.(App)
	model, cmd := app.handleMouse(tea.MouseMsg{
		X:      innerX + 5,
		Y:      innerY,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionRelease,
	})
	app = model.(App)

	if !app.contentSelection.HasSelection() {
		t.Fatalf("expected content selection to persist after mouse release")
	}
	if cmd != nil {
		t.Fatalf("expected mouse release to keep selection without auto-copy command")
	}
}

func TestRuntimeCwdEventUpdatesUIWithoutPersistingSessionCwd(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "termia.db"), zap.NewNop())
	if err != nil {
		t.Fatalf("db.Open returned error: %v", err)
	}
	defer database.Close()

	session, err := createSession(database, "/initial", agent.ModeAssistant, "")
	if err != nil {
		t.Fatalf("createSession returned error: %v", err)
	}

	app := New(database, config.DefaultConfig(), zap.NewNop())
	app.sessions = []db.AgentSession{session}
	app.activeSessionID = session.ID
	app.cwd = "/initial"
	app.sessionCwds = map[string]string{session.ID: "/initial"}

	model, _ := app.Update(agentEventMsg{
		event: agent.RuntimeEvent{Kind: agent.RuntimeEventCwd, Cwd: "/runtime"},
	})
	updated := model.(App)

	if updated.cwd != "/runtime" {
		t.Fatalf("expected runtime cwd to update app cwd, got %q", updated.cwd)
	}
	if got := updated.sessionCwds[session.ID]; got != "/runtime" {
		t.Fatalf("expected runtime cwd to update in-memory session cwd, got %q", got)
	}
	if got, ok := updated.sessionByID(session.ID); !ok || got.Cwd != "/runtime" {
		t.Fatalf("expected runtime cwd to update session list entry, got %#v (ok=%v)", got, ok)
	}

	persisted, ok, err := database.GetAgentSession(session.ID)
	if err != nil {
		t.Fatalf("GetAgentSession returned error: %v", err)
	}
	if !ok {
		t.Fatalf("expected persisted session to exist")
	}
	if persisted.Cwd != "/initial" {
		t.Fatalf("expected runtime cwd event to avoid db persistence, got persisted cwd %q", persisted.Cwd)
	}
}

func TestSubmitInputAbortsWhenSessionRuntimePersistenceFails(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "termia.db"), zap.NewNop())
	if err != nil {
		t.Fatalf("db.Open returned error: %v", err)
	}

	session, err := createSession(database, "/cwd", agent.ModeAssistant, "")
	if err != nil {
		t.Fatalf("createSession returned error: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	app := New(database, config.DefaultConfig(), zap.NewNop())
	app.activeSessionID = session.ID
	app.sessions = []db.AgentSession{session}
	app.agentService = &fakeTUIAgentAppService{}
	app.input.SetValue("hello")

	model, cmd := app.submitInput()
	updated := model.(App)

	if cmd != nil {
		t.Fatalf("expected submit to abort before scheduling run cmd")
	}
	if updated.agentRunning {
		t.Fatalf("expected agent run not to start")
	}
	if len(updated.agentService.(*fakeTUIAgentAppService).requests) != 0 {
		t.Fatalf("expected shared service not to be called on persistence error")
	}
	if len(updated.agent.messages) == 0 {
		t.Fatalf("expected agent timeline to contain an error message")
	}
	for _, message := range updated.agent.messages {
		if normalizeConversationRole(message.Role) == "user" && strings.TrimSpace(message.Content) == "hello" {
			t.Fatalf("expected failed submit to avoid ghost user message, got %#v", message)
		}
	}
	last := updated.agent.messages[len(updated.agent.messages)-1]
	if normalizeConversationRole(last.Role) != "error" {
		t.Fatalf("expected last message to be error, got %#v", last)
	}
	if !strings.Contains(strings.ToLower(last.Content), "error") {
		t.Fatalf("expected error message content, got %q", last.Content)
	}
}

func TestUpdateSessionRuntimePreservesSessionMiddleware(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "termia.db"), zap.NewNop())
	if err != nil {
		t.Fatalf("db.Open returned error: %v", err)
	}
	defer database.Close()

	state := agentapp.SessionState{
		Mode:     agent.ModeAssistant,
		TeamName: "",
		SessionMiddleware: []agentapp.MiddlewareActivation{{
			Name:  "sticky",
			Scope: agentapp.MiddlewareScopeSession,
			Args:  map[string]string{"level": "2"},
		}},
	}
	snapshot, err := agentapp.EncodeSessionState(state)
	if err != nil {
		t.Fatalf("EncodeSessionState returned error: %v", err)
	}
	session := db.AgentSession{
		ID:               "session-1",
		Name:             "Session 1",
		Mode:             string(agent.ModeAssistant),
		SpecSnapshotJSON: snapshot,
		Cwd:              "/cwd",
		CreatedAt:        1,
		UpdatedAt:        1,
	}
	if err := database.CreateAgentSession(&session); err != nil {
		t.Fatalf("CreateAgentSession returned error: %v", err)
	}

	app := New(database, config.DefaultConfig(), zap.NewNop())
	app.sessions = []db.AgentSession{session}

	if err := app.updateSessionRuntime(session.ID, agent.ModeTeam, "ops"); err != nil {
		t.Fatalf("updateSessionRuntime returned error: %v", err)
	}

	updated, ok, err := database.GetAgentSession(session.ID)
	if err != nil {
		t.Fatalf("GetAgentSession returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected updated session to exist")
	}
	if updated.Mode != string(agent.ModeTeam) || updated.TeamName != "ops" {
		t.Fatalf("expected mode/team to update, got %+v", updated)
	}
	decoded, err := agentapp.DecodeSessionState(updated.SpecSnapshotJSON)
	if err != nil {
		t.Fatalf("DecodeSessionState returned error: %v", err)
	}
	if decoded.Mode != agent.ModeTeam || decoded.TeamName != "ops" {
		t.Fatalf("expected snapshot mode/team to update, got %+v", decoded)
	}
	if len(decoded.SessionMiddleware) != 1 || decoded.SessionMiddleware[0].Name != "sticky" || decoded.SessionMiddleware[0].Args["level"] != "2" {
		t.Fatalf("expected session middleware to be preserved, got %+v", decoded.SessionMiddleware)
	}
}
