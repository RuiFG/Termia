package tui

import (
	"strings"
	"testing"

	"github.com/termia/termia/internal/agent"
)

func TestRenderConversationTimelineGroupsUserAndAgent(t *testing.T) {
	view := renderConversationTimeline([]AgentMessage{
		{Role: "user", Content: "How did it go?", CitedCommandCount: 2},
		{Role: "assistant", Content: "# Summary\nAll good."},
	}, 40)

	normalized := strings.Join(strings.Fields(view), " ")
	if !strings.Contains(normalized, "> How did it go?") {
		t.Fatalf("expected user prompt prefix in timeline, got %q", view)
	}
	if !strings.Contains(normalized, "referenced 2 commands") {
		t.Fatalf("expected cited command summary in user timeline, got %q", view)
	}
	if !strings.Contains(normalized, "• Summary All good.") {
		t.Fatalf("expected assistant bullet in timeline, got %q", view)
	}
	if strings.Contains(view, "...") {
		t.Fatalf("expected conversation timeline without placeholder ellipsis, got %q", view)
	}
}

func TestAgentModelRendersToolCallsInlineWithAssistantFlow(t *testing.T) {
	model := NewAgentModel(DefaultKeyMap())
	model.SetSize(48, 12)
	model.AddMessage("user", "Check the log")
	model.AppendToLast("I will inspect the log.")
	model.AppendToolCall(AgentToolCall{CallID: "call-1", ToolName: "read_file", Summary: "/tmp/app.log", State: agent.ToolCallStatePending})
	model.AppendToolCall(AgentToolCall{CallID: "call-1", ToolName: "read_file", Summary: "/tmp/app.log", State: agent.ToolCallStateSuccess, Result: "120 lines"})
	model.AppendToLast("Done.")

	view := model.viewport.View()
	normalized := strings.Join(strings.Fields(view), " ")
	if !strings.Contains(normalized, "• I will inspect the log.") {
		t.Fatalf("expected assistant text in viewport, got %q", view)
	}
	if !strings.Contains(normalized, "• read_file /tmp/app.log · 120 lines") {
		t.Fatalf("expected tool call in viewport, got %q", view)
	}
	if !strings.Contains(normalized, "• Done.") {
		t.Fatalf("expected assistant text in viewport, got %q", view)
	}
}

func TestRenderConversationTimelineUsesLineDividerBetweenTurns(t *testing.T) {
	view := renderConversationTimeline([]AgentMessage{
		{Role: "user", Content: "first"},
		{Role: "assistant", Content: "one"},
		{Role: "user", Content: "second"},
		{Role: "assistant", Content: "two"},
	}, 20)

	if !strings.Contains(view, strings.Repeat("─", 20)) {
		t.Fatalf("expected full-width divider in timeline, got %q", view)
	}
}

func TestAgentModelNormalizesCarriageReturnsInTimelineAndToolCalls(t *testing.T) {
	model := NewAgentModel(DefaultKeyMap())
	model.SetSize(60, 12)
	model.AddMessage("user", "inspect\rports")
	model.AppendToLast("working\ron it")
	model.AppendToolCall(AgentToolCall{
		CallID:   "call-1",
		ToolName: "command\r",
		Summary:  "netstat\r-tuln",
		Result:   "open\rports",
		State:    agent.ToolCallStateSuccess,
	})

	view := model.viewport.View()
	if strings.Contains(view, "\r") {
		t.Fatalf("expected viewport to be free of carriage returns, got %q", view)
	}
	normalized := strings.Join(strings.Fields(view), " ")
	if !strings.Contains(normalized, "> inspect ports") {
		t.Fatalf("expected normalized user content, got %q", view)
	}
	if !strings.Contains(normalized, "• working on it") {
		t.Fatalf("expected normalized assistant content, got %q", view)
	}
	if !strings.Contains(normalized, "• command netstat -tuln · open ports") {
		t.Fatalf("expected normalized tool call content, got %q", view)
	}
}
