package agentapp

import (
	"fmt"
	"strings"
	"testing"
	"time"

	runtimeagent "github.com/termia/termia/internal/agent"
	"github.com/termia/termia/internal/db"
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

func TestSessionStateDefaultsAndClearsTeamName(t *testing.T) {
	state := SessionState{
		TeamName: "ops",
		SessionMiddleware: []MiddlewareActivation{
			{Name: "persisted", Scope: MiddlewareScopeSession},
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

	if got.Mode != runtimeagent.ModeAssistant {
		t.Fatalf("expected default assistant mode, got %+v", got)
	}
	if got.TeamName != "" {
		t.Fatalf("expected team name to be cleared, got %+v", got)
	}
	if len(got.SessionMiddleware) != 1 || got.SessionMiddleware[0].Name != "persisted" {
		t.Fatalf("unexpected middleware: %+v", got.SessionMiddleware)
	}
}

func TestSessionServiceResolveCreatesAssistantSessionWhenMissing(t *testing.T) {
	fixedNow := time.Date(2024, 1, 2, 3, 4, 5, 6, time.UTC)
	service := NewSessionService(&fakeSessionDB{})

	gotSession, gotState, err := service.Resolve("", "  /workdir/project  ", SessionState{
		TeamName: "  ops  ",
		SessionMiddleware: []MiddlewareActivation{
			{Name: "persisted", Scope: MiddlewareScopeSession},
		},
	}, func() time.Time {
		return fixedNow
	})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}

	if gotSession.ID != fmt.Sprintf("%d", fixedNow.UnixNano()) {
		t.Fatalf("unexpected session id: %+v", gotSession)
	}
	if gotSession.Name != "Session 2024-01-02 03:04" {
		t.Fatalf("unexpected session name: %+v", gotSession)
	}
	if gotSession.Mode != string(runtimeagent.ModeAssistant) {
		t.Fatalf("expected assistant mode, got %+v", gotSession)
	}
	if gotSession.TeamName != "" {
		t.Fatalf("expected team name to be cleared for assistant sessions, got %+v", gotSession)
	}
	if gotSession.Cwd != "/workdir/project" {
		t.Fatalf("expected cwd to be trimmed, got %+v", gotSession)
	}
	if gotSession.CreatedAt != fixedNow.UnixNano() || gotSession.UpdatedAt != fixedNow.UnixNano() {
		t.Fatalf("unexpected timestamps: %+v", gotSession)
	}
	if gotState.Mode != runtimeagent.ModeAssistant {
		t.Fatalf("expected assistant state, got %+v", gotState)
	}
	if gotState.TeamName != "" {
		t.Fatalf("expected assistant state team name to be cleared, got %+v", gotState)
	}
	if len(gotState.SessionMiddleware) != 1 || gotState.SessionMiddleware[0].Name != "persisted" {
		t.Fatalf("unexpected returned state: %+v", gotState)
	}
}

func TestSessionServiceResolveUsesPreferredIDBeforeCurrentAndLatest(t *testing.T) {
	preferredState := SessionState{Mode: runtimeagent.ModeTeam, TeamName: "  devs  "}
	preferredSnapshot, err := EncodeSessionState(preferredState)
	if err != nil {
		t.Fatalf("EncodeSessionState returned error: %v", err)
	}
	now := time.Unix(100, 0).UTC()
	database := &fakeSessionDB{
		sessions: map[string]db.AgentSession{
			"preferred": {
				ID:               "preferred",
				Name:             "Preferred",
				Mode:             string(runtimeagent.ModeTeam),
				TeamName:         "devs",
				SpecSnapshotJSON: preferredSnapshot,
				Cwd:              "/preferred",
				CreatedAt:        now.UnixNano(),
				UpdatedAt:        now.UnixNano(),
			},
			"current": {
				ID:               "current",
				Name:             "Current",
				Mode:             string(runtimeagent.ModeAssistant),
				SpecSnapshotJSON: "",
				Cwd:              "/current",
				CreatedAt:        now.Add(time.Minute).UnixNano(),
				UpdatedAt:        now.Add(time.Minute).UnixNano(),
			},
		},
		latestID: "current",
	}
	t.Setenv("TERMIA_SESSION_ID", "current")

	service := NewSessionService(database)
	gotSession, gotState, err := service.Resolve("  preferred  ", "/ignored", DefaultSessionState(), time.Now)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}

	if gotSession.ID != "preferred" {
		t.Fatalf("expected preferred session, got %+v", gotSession)
	}
	if gotState.Mode != runtimeagent.ModeTeam {
		t.Fatalf("expected decoded team mode, got %+v", gotState)
	}
	if gotState.TeamName != "devs" {
		t.Fatalf("expected decoded team name to be normalized by the resolver, got %+v", gotState)
	}
}

func TestSessionServiceResolveUsesCurrentIDBeforeLatest(t *testing.T) {
	currentState := SessionState{Mode: runtimeagent.ModeAssistant}
	currentSnapshot, err := EncodeSessionState(currentState)
	if err != nil {
		t.Fatalf("EncodeSessionState returned error: %v", err)
	}
	latestState := SessionState{Mode: runtimeagent.ModeTeam, TeamName: "latest"}
	latestSnapshot, err := EncodeSessionState(latestState)
	if err != nil {
		t.Fatalf("EncodeSessionState returned error: %v", err)
	}
	database := &fakeSessionDB{
		sessions: map[string]db.AgentSession{
			"current": {
				ID:               "current",
				Name:             "Current",
				Mode:             string(runtimeagent.ModeAssistant),
				SpecSnapshotJSON: currentSnapshot,
				Cwd:              "/current",
			},
			"latest": {
				ID:               "latest",
				Name:             "Latest",
				Mode:             string(runtimeagent.ModeTeam),
				TeamName:         "latest",
				SpecSnapshotJSON: latestSnapshot,
				Cwd:              "/latest",
			},
		},
		latestID: "latest",
	}
	t.Setenv("TERMIA_SESSION_ID", "current")

	service := NewSessionService(database)
	gotSession, gotState, err := service.Resolve("missing", "", DefaultSessionState(), time.Now)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}

	if gotSession.ID != "current" {
		t.Fatalf("expected current session, got %+v", gotSession)
	}
	if gotState.Mode != runtimeagent.ModeAssistant {
		t.Fatalf("expected decoded current state, got %+v", gotState)
	}
}

func TestSessionServiceResolveUsesLatestWhenPreferredAndCurrentMissing(t *testing.T) {
	latestState := SessionState{Mode: runtimeagent.ModeTeam, TeamName: "latest"}
	latestSnapshot, err := EncodeSessionState(latestState)
	if err != nil {
		t.Fatalf("EncodeSessionState returned error: %v", err)
	}
	database := &fakeSessionDB{
		sessions: map[string]db.AgentSession{
			"latest": {
				ID:               "latest",
				Name:             "Latest",
				Mode:             string(runtimeagent.ModeTeam),
				TeamName:         "latest",
				SpecSnapshotJSON: latestSnapshot,
				Cwd:              "/latest",
			},
		},
		latestID: "latest",
	}
	t.Setenv("TERMIA_SESSION_ID", "missing")

	service := NewSessionService(database)
	gotSession, gotState, err := service.Resolve("missing", "", DefaultSessionState(), time.Now)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}

	if gotSession.ID != "latest" {
		t.Fatalf("expected latest session, got %+v", gotSession)
	}
	if gotState.Mode != runtimeagent.ModeTeam || gotState.TeamName != "latest" {
		t.Fatalf("expected latest decoded state, got %+v", gotState)
	}
}

func TestSessionServiceResolveUpdatesExistingSessionCwdFromRequest(t *testing.T) {
	state := SessionState{Mode: runtimeagent.ModeAssistant}
	snapshot, err := EncodeSessionState(state)
	if err != nil {
		t.Fatalf("EncodeSessionState returned error: %v", err)
	}
	database := &fakeSessionDB{
		sessions: map[string]db.AgentSession{
			"session-1": {
				ID:               "session-1",
				Name:             "Session 1",
				Mode:             string(runtimeagent.ModeAssistant),
				SpecSnapshotJSON: snapshot,
				Cwd:              "/old",
			},
		},
	}

	service := NewSessionService(database)
	gotSession, gotState, err := service.Resolve("session-1", "  /new  ", DefaultSessionState(), func() time.Time {
		return time.Unix(99, 0)
	})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}

	if gotSession.Cwd != "/new" {
		t.Fatalf("expected resolved session cwd to use request cwd, got %+v", gotSession)
	}
	if gotState.Mode != runtimeagent.ModeAssistant {
		t.Fatalf("expected existing session state to remain intact, got %+v", gotState)
	}
	stored := database.sessions["session-1"]
	if stored.Cwd != "/new" {
		t.Fatalf("expected request cwd to persist to session row, got %+v", stored)
	}
	if stored.UpdatedAt != time.Unix(99, 0).UnixNano() {
		t.Fatalf("expected cwd persistence timestamp to update, got %+v", stored)
	}
}

func TestSessionServiceUpdatePersistsSessionState(t *testing.T) {
	database := &fakeSessionDB{
		sessions: map[string]db.AgentSession{
			"session-1": {
				ID:   "session-1",
				Name: "Session 1",
			},
		},
	}
	service := NewSessionService(database)

	state := SessionState{
		Mode:     runtimeagent.ModeTeam,
		TeamName: "  ops  ",
		SessionMiddleware: []MiddlewareActivation{{
			Name:  "sticky",
			Scope: MiddlewareScopeSession,
		}},
	}
	got, err := service.Update(" session-1 ", state, func() time.Time {
		return time.Unix(99, 0)
	})
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}

	if got.Mode != runtimeagent.ModeTeam || got.TeamName != "ops" {
		t.Fatalf("expected normalized state, got %+v", got)
	}
	session := database.sessions["session-1"]
	if session.Mode != string(runtimeagent.ModeTeam) || session.TeamName != "ops" {
		t.Fatalf("expected runtime metadata to persist, got %+v", session)
	}
	if session.UpdatedAt != time.Unix(99, 0).UnixNano() {
		t.Fatalf("expected updated timestamp to persist, got %+v", session)
	}
	decoded, err := DecodeSessionState(session.SpecSnapshotJSON)
	if err != nil {
		t.Fatalf("DecodeSessionState returned error: %v", err)
	}
	if len(decoded.SessionMiddleware) != 1 || decoded.SessionMiddleware[0].Name != "sticky" {
		t.Fatalf("expected session middleware to persist, got %+v", decoded)
	}
}

type fakeSessionDB struct {
	sessions map[string]db.AgentSession
	latestID string
}

func (f *fakeSessionDB) GetAgentSession(id string) (db.AgentSession, bool, error) {
	if f == nil {
		return db.AgentSession{}, false, nil
	}
	if session, ok := f.sessions[strings.TrimSpace(id)]; ok {
		return session, true, nil
	}
	return db.AgentSession{}, false, nil
}

func (f *fakeSessionDB) LatestAgentSession() (db.AgentSession, bool, error) {
	if f == nil || strings.TrimSpace(f.latestID) == "" {
		return db.AgentSession{}, false, nil
	}
	session, ok := f.sessions[strings.TrimSpace(f.latestID)]
	if !ok {
		return db.AgentSession{}, false, nil
	}
	return session, true, nil
}

func (f *fakeSessionDB) CreateAgentSession(session *db.AgentSession) error {
	if f.sessions == nil {
		f.sessions = map[string]db.AgentSession{}
	}
	f.sessions[session.ID] = *session
	f.latestID = session.ID
	return nil
}

func (f *fakeSessionDB) UpdateAgentSessionRuntime(sessionID, mode, teamName, specSnapshotJSON string, updatedAt int64) error {
	if f.sessions == nil {
		return nil
	}
	session, ok := f.sessions[strings.TrimSpace(sessionID)]
	if !ok {
		return nil
	}
	session.Mode = mode
	session.TeamName = teamName
	session.SpecSnapshotJSON = specSnapshotJSON
	session.UpdatedAt = updatedAt
	f.sessions[strings.TrimSpace(sessionID)] = session
	return nil
}

func (f *fakeSessionDB) UpdateAgentSessionCwd(sessionID, cwd string, updatedAt int64) error {
	if f.sessions == nil {
		return nil
	}
	session, ok := f.sessions[strings.TrimSpace(sessionID)]
	if !ok {
		return nil
	}
	session.Cwd = strings.TrimSpace(cwd)
	session.UpdatedAt = updatedAt
	f.sessions[strings.TrimSpace(sessionID)] = session
	return nil
}
