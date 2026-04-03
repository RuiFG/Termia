package agentapp

import (
	"fmt"
	"strings"
	"time"

	runtimeagent "github.com/termia/termia/internal/agent"
	"github.com/termia/termia/internal/db"
	"github.com/termia/termia/internal/sessionstate"
)

type SessionDB interface {
	GetAgentSession(id string) (db.AgentSession, bool, error)
	LatestAgentSession() (db.AgentSession, bool, error)
	CreateAgentSession(session *db.AgentSession) error
	UpdateAgentSessionRuntime(sessionID, mode, teamName, specSnapshotJSON string, updatedAt int64) error
	UpdateAgentSessionCwd(sessionID, cwd string, updatedAt int64) error
}

type SessionService struct {
	database SessionDB
}

func NewSessionService(database SessionDB) *SessionService {
	return &SessionService{database: database}
}

func (s *SessionService) Resolve(preferredID, cwd string, defaultState SessionState, now func() time.Time) (db.AgentSession, SessionState, error) {
	if s == nil || s.database == nil {
		return db.AgentSession{}, SessionState{}, fmt.Errorf("session database is nil")
	}
	if now == nil {
		now = time.Now
	}

	preferredID = strings.TrimSpace(preferredID)
	if preferredID != "" {
		if session, state, found, err := s.resolveByID(preferredID); err != nil {
			return db.AgentSession{}, SessionState{}, err
		} else if found {
			return s.applyRequestCwd(session, state, cwd, now())
		}
	}

	currentID := strings.TrimSpace(sessionstate.CurrentID())
	if currentID != "" && currentID != preferredID {
		if session, state, found, err := s.resolveByID(currentID); err != nil {
			return db.AgentSession{}, SessionState{}, err
		} else if found {
			return s.applyRequestCwd(session, state, cwd, now())
		}
	}

	if session, state, found, err := s.resolveLatest(); err != nil {
		return db.AgentSession{}, SessionState{}, err
	} else if found {
		return s.applyRequestCwd(session, state, cwd, now())
	}

	return s.createSession(cwd, defaultState, now())
}

func (s *SessionService) Create(cwd string, defaultState SessionState, now func() time.Time) (db.AgentSession, SessionState, error) {
	if s == nil || s.database == nil {
		return db.AgentSession{}, SessionState{}, fmt.Errorf("session database is nil")
	}
	if now == nil {
		now = time.Now
	}
	return s.createSession(cwd, defaultState, now())
}

func (s *SessionService) Update(sessionID string, state SessionState, now func() time.Time) (SessionState, error) {
	if s == nil || s.database == nil {
		return SessionState{}, fmt.Errorf("session database is nil")
	}
	if now == nil {
		now = time.Now
	}

	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return SessionState{}, fmt.Errorf("session id is empty")
	}

	state = normalizeResolvedSessionState(state)
	specSnapshotJSON, err := EncodeSessionState(state)
	if err != nil {
		return SessionState{}, err
	}
	if err := s.database.UpdateAgentSessionRuntime(sessionID, string(state.Mode), state.TeamName, specSnapshotJSON, now().UnixNano()); err != nil {
		return SessionState{}, err
	}
	return state, nil
}

func (s *SessionService) resolveByID(id string) (db.AgentSession, SessionState, bool, error) {
	session, found, err := s.database.GetAgentSession(strings.TrimSpace(id))
	if err != nil || !found {
		return db.AgentSession{}, SessionState{}, found, err
	}
	state, err := DecodeSessionState(session.SpecSnapshotJSON)
	if err != nil {
		return db.AgentSession{}, SessionState{}, false, err
	}
	state = normalizeResolvedSessionState(state)
	return session, state, true, nil
}

func (s *SessionService) resolveLatest() (db.AgentSession, SessionState, bool, error) {
	session, found, err := s.database.LatestAgentSession()
	if err != nil || !found {
		return db.AgentSession{}, SessionState{}, found, err
	}
	state, err := DecodeSessionState(session.SpecSnapshotJSON)
	if err != nil {
		return db.AgentSession{}, SessionState{}, false, err
	}
	state = normalizeResolvedSessionState(state)
	return session, state, true, nil
}

func (s *SessionService) createSession(cwd string, defaultState SessionState, now time.Time) (db.AgentSession, SessionState, error) {
	cwd = strings.TrimSpace(cwd)
	defaultState = normalizeResolvedSessionState(defaultState)

	specSnapshotJSON, err := EncodeSessionState(defaultState)
	if err != nil {
		return db.AgentSession{}, SessionState{}, err
	}
	state, err := DecodeSessionState(specSnapshotJSON)
	if err != nil {
		return db.AgentSession{}, SessionState{}, err
	}
	state = normalizeResolvedSessionState(state)

	ts := now.UnixNano()
	session := db.AgentSession{
		ID:               fmt.Sprintf("%d", ts),
		Name:             fmt.Sprintf("Session %s", now.Format("2006-01-02 15:04")),
		Mode:             string(state.Mode),
		TeamName:         strings.TrimSpace(state.TeamName),
		SpecSnapshotJSON: specSnapshotJSON,
		Cwd:              cwd,
		CreatedAt:        ts,
		UpdatedAt:        ts,
	}
	if err := s.database.CreateAgentSession(&session); err != nil {
		return db.AgentSession{}, SessionState{}, err
	}
	return session, state, nil
}

func normalizeResolvedSessionState(state SessionState) SessionState {
	state.TeamName = strings.TrimSpace(state.TeamName)
	if state.Mode != runtimeagent.ModeTeam {
		state.TeamName = ""
	}
	return state
}

func (s *SessionService) applyRequestCwd(session db.AgentSession, state SessionState, cwd string, now time.Time) (db.AgentSession, SessionState, error) {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" || cwd == strings.TrimSpace(session.Cwd) {
		return session, state, nil
	}
	if err := s.database.UpdateAgentSessionCwd(session.ID, cwd, now.UnixNano()); err != nil {
		return db.AgentSession{}, SessionState{}, err
	}
	session.Cwd = cwd
	session.UpdatedAt = now.UnixNano()
	return session, state, nil
}
