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
