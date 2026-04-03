package agentapp

import (
	"testing"

	runtimeagent "github.com/termia/termia/internal/agent"
)

func TestAppendTimelineTextMergesAdjacentAssistantChunks(t *testing.T) {
	input := []TimelineEntry{{Role: "assistant", Content: "hel"}}
	got := AppendTimelineText(input, "assistant", "lo", true)

	if len(got) != 1 {
		t.Fatalf("expected one timeline entry, got %d", len(got))
	}
	if got[0].Content != "hello" {
		t.Fatalf("expected merged content, got %+v", got[0])
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
	if timeline[0].ToolCall.Result != "ok" {
		t.Fatalf("expected merged result, got %+v", timeline[0].ToolCall)
	}
}

func TestUpsertTimelineToolCallFallsBackToToolNameAndSummaryWithoutCallID(t *testing.T) {
	pending := runtimeagent.ToolCallEvent{
		ToolName: "command",
		Summary:  "pwd",
		State:    runtimeagent.ToolCallStatePending,
	}
	success := runtimeagent.ToolCallEvent{
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
	if timeline[0].ToolCall.Result != "ok" {
		t.Fatalf("expected merged result, got %+v", timeline[0].ToolCall)
	}
}

func TestUpsertTimelineToolCallOverwritesSummaryAndResult(t *testing.T) {
	pending := runtimeagent.ToolCallEvent{
		CallID:   "call-1",
		ToolName: "command",
		Summary:  "old summary",
		Result:   "old result",
		State:    runtimeagent.ToolCallStatePending,
	}
	success := runtimeagent.ToolCallEvent{
		CallID:   "call-1",
		ToolName: "command",
		Summary:  "new summary",
		Result:   "new result",
		State:    runtimeagent.ToolCallStateSuccess,
	}

	timeline := UpsertTimelineToolCall(nil, pending)
	timeline = UpsertTimelineToolCall(timeline, success)

	if len(timeline) != 1 {
		t.Fatalf("expected one timeline entry, got %d", len(timeline))
	}
	if timeline[0].ToolCall == nil {
		t.Fatalf("expected tool call entry, got %+v", timeline[0])
	}
	if timeline[0].ToolCall.Summary != "new summary" {
		t.Fatalf("expected updated summary, got %+v", timeline[0].ToolCall)
	}
	if timeline[0].ToolCall.Result != "new result" {
		t.Fatalf("expected updated result, got %+v", timeline[0].ToolCall)
	}
}

func TestMarkLatestPendingToolFailed(t *testing.T) {
	timeline := []TimelineEntry{
		{
			Role: "tool",
			ToolCall: &runtimeagent.ToolCallEvent{
				ToolName: "command",
				Summary:  "first",
				State:    runtimeagent.ToolCallStatePending,
			},
		},
		{
			Role: "tool",
			ToolCall: &runtimeagent.ToolCallEvent{
				ToolName: "command",
				Summary:  "second",
				State:    runtimeagent.ToolCallStatePending,
			},
		},
	}

	got := MarkLatestPendingToolFailed(timeline, "boom")

	if got[1].ToolCall == nil {
		t.Fatalf("expected latest pending call to remain present")
	}
	if got[1].ToolCall.State != runtimeagent.ToolCallStateError {
		t.Fatalf("expected latest pending call to be marked failed, got %+v", got[1].ToolCall)
	}
	if got[1].ToolCall.Result != "boom" {
		t.Fatalf("expected failure reason to populate result, got %+v", got[1].ToolCall)
	}
	if got[0].ToolCall.State != runtimeagent.ToolCallStatePending {
		t.Fatalf("expected earlier pending call to remain unchanged, got %+v", got[0].ToolCall)
	}
}

func TestUpsertTimelineToolCallDoesNotFallbackMergeWithoutState(t *testing.T) {
	pending := runtimeagent.ToolCallEvent{
		ToolName: "command",
		Summary:  "pwd",
		State:    runtimeagent.ToolCallStatePending,
	}
	update := runtimeagent.ToolCallEvent{
		ToolName: "command",
		Summary:  "pwd",
		Result:   "still running",
	}

	timeline := UpsertTimelineToolCall(nil, pending)
	timeline = UpsertTimelineToolCall(timeline, update)

	if len(timeline) != 2 {
		t.Fatalf("expected a separate entry when state is empty, got %+v", timeline)
	}
}

func TestUpsertTimelineToolCallKeepsExistingAgentAndToolIdentity(t *testing.T) {
	pending := runtimeagent.ToolCallEvent{
		CallID:    "call-1",
		AgentName: "alice",
		ToolName:  "command",
		Summary:   "pwd",
		State:     runtimeagent.ToolCallStatePending,
	}
	success := runtimeagent.ToolCallEvent{
		CallID:    "call-1",
		AgentName: "bob",
		ToolName:  "shell",
		Summary:   "pwd",
		Result:    "ok",
		State:     runtimeagent.ToolCallStateSuccess,
	}

	timeline := UpsertTimelineToolCall(nil, pending)
	timeline = UpsertTimelineToolCall(timeline, success)

	if len(timeline) != 1 || timeline[0].ToolCall == nil {
		t.Fatalf("expected merged tool call, got %+v", timeline)
	}
	if timeline[0].ToolCall.AgentName != "alice" {
		t.Fatalf("expected existing agent identity to win, got %+v", timeline[0].ToolCall)
	}
	if timeline[0].ToolCall.ToolName != "command" {
		t.Fatalf("expected existing tool identity to win, got %+v", timeline[0].ToolCall)
	}
}
