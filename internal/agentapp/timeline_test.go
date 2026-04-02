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
