package agentapp

import (
	"encoding/json"
	"strings"

	runtimeagent "github.com/termia/termia/internal/agent"
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
