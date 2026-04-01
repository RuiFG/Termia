package tui

import (
	"testing"

	"github.com/termia/termia/internal/db"
)

func TestSelectInitialSessionIDPrefersMatchingCurrentSession(t *testing.T) {
	sessions := []db.AgentSession{
		{ID: "latest"},
		{ID: "current"},
	}

	got := selectInitialSessionID(sessions, "current")
	if got != "current" {
		t.Fatalf("expected current session to be selected, got %q", got)
	}
}

func TestSelectInitialSessionIDFallsBackToLatestSession(t *testing.T) {
	sessions := []db.AgentSession{
		{ID: "latest"},
		{ID: "older"},
	}

	got := selectInitialSessionID(sessions, "missing")
	if got != "latest" {
		t.Fatalf("expected fallback to latest session, got %q", got)
	}
}

func TestSelectInitialSessionIDHandlesEmptySessions(t *testing.T) {
	if got := selectInitialSessionID(nil, "current"); got != "" {
		t.Fatalf("expected empty selection, got %q", got)
	}
}

func TestFormatSessionMessagesNormalizesCarriageReturns(t *testing.T) {
	messages := formatSessionMessages([]db.AgentMessage{
		{
			Role:    "assistant",
			Content: "first\rsecond",
		},
		{
			Role:         "tool",
			MetadataJSON: `{"tool_calls":[{"tool_name":"command\r","summary":"pwd\rnow","result":"ok\rdone","state":"success"}]}`,
		},
	})
	if len(messages) != 2 {
		t.Fatalf("expected assistant and tool messages, got %#v", messages)
	}
	if got := messages[0].Content; got != "first\nsecond" {
		t.Fatalf("expected normalized assistant content, got %q", got)
	}
	if messages[1].ToolCall == nil {
		t.Fatalf("expected tool call metadata to load")
	}
	if got := messages[1].ToolCall.ToolName; got != "command" {
		t.Fatalf("expected normalized tool name, got %q", got)
	}
	if got := messages[1].ToolCall.Summary; got != "pwd now" {
		t.Fatalf("expected normalized tool summary, got %q", got)
	}
	if got := messages[1].ToolCall.Result; got != "ok done" {
		t.Fatalf("expected normalized tool result, got %q", got)
	}
}
