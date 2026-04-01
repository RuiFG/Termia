package cmd

import (
	"strings"
	"testing"

	"github.com/termia/termia/internal/agent"
	"github.com/termia/termia/internal/db"
)

func TestTaiRendererNormalizesToolResultSummaryFromPendingCall(t *testing.T) {
	renderer := newTaiRenderer()
	if _, _, ok := renderer.prepareToolCallForRender(agent.ToolCallEvent{
		CallID:   "call-1",
		ToolName: "command",
		Summary:  "pwd",
		State:    agent.ToolCallStatePending,
	}); ok {
		t.Fatalf("expected pending tool call to stay buffered")
	}
	toolCall, _, ok := renderer.prepareToolCallForRender(agent.ToolCallEvent{
		CallID:   "call-1",
		ToolName: "command",
		State:    agent.ToolCallStateSuccess,
	})
	if !ok {
		t.Fatalf("expected final tool result to render")
	}
	if toolCall.Summary != "pwd" {
		t.Fatalf("expected summary to be restored from pending call, got %#v", toolCall)
	}
}

func TestTaiRendererDeduplicatesRepeatedFinalToolState(t *testing.T) {
	renderer := newTaiRenderer()
	first, _, ok := renderer.prepareToolCallForRender(agent.ToolCallEvent{
		CallID:   "call-1",
		ToolName: "command",
		Summary:  "ls",
		State:    agent.ToolCallStateSuccess,
	})
	if !ok {
		t.Fatalf("expected first final tool state to render")
	}
	if first.Summary != "ls" {
		t.Fatalf("expected first rendered call to keep summary, got %#v", first)
	}
	if _, _, ok := renderer.prepareToolCallForRender(agent.ToolCallEvent{
		CallID:   "call-1",
		ToolName: "command",
		Summary:  "ls",
		State:    agent.ToolCallStateSuccess,
	}); ok {
		t.Fatalf("expected repeated final tool state to be suppressed")
	}
}

func TestTaiRendererSkipsRequestInputToolLines(t *testing.T) {
	renderer := newTaiRenderer()
	if _, _, ok := renderer.prepareToolCallForRender(agent.ToolCallEvent{
		CallID:   "call-2",
		ToolName: "request_input",
		Summary:  "requested user input",
		Result:   "answered 1 question",
		State:    agent.ToolCallStateSuccess,
	}); ok {
		t.Fatalf("expected request_input tool lines to be suppressed in tai")
	}
}

func TestTaiRendererLatestToolStateWinsBeforeFlush(t *testing.T) {
	renderer := newTaiRenderer()
	errorCall, key, ok := renderer.prepareToolCallForRender(agent.ToolCallEvent{
		CallID:   "call-3",
		ToolName: "command",
		Summary:  "pwd",
		Result:   "exit 1",
		State:    agent.ToolCallStateError,
	})
	if !ok {
		t.Fatalf("expected error state to buffer")
	}
	renderer.bufferedToolOrder = append(renderer.bufferedToolOrder, key)
	renderer.bufferedTools[key] = errorCall

	successCall, key, ok := renderer.prepareToolCallForRender(agent.ToolCallEvent{
		CallID:   "call-3",
		ToolName: "command",
		Summary:  "pwd",
		Result:   "ok",
		State:    agent.ToolCallStateSuccess,
	})
	if !ok {
		t.Fatalf("expected success state to replace buffered error")
	}
	renderer.bufferedTools[key] = successCall

	got := renderer.bufferedTools[key]
	if got.State != agent.ToolCallStateSuccess {
		t.Fatalf("expected latest buffered state to win, got %#v", got)
	}
}

func TestTaiRendererUsesDisplayKeyToCollapseDuplicateCallIDs(t *testing.T) {
	renderer := newTaiRenderer()
	first, key, ok := renderer.prepareToolCallForRender(agent.ToolCallEvent{
		CallID:   "call-a",
		ToolName: "command",
		Summary:  "pwd",
		Result:   "exit 1",
		State:    agent.ToolCallStateError,
	})
	if !ok {
		t.Fatalf("expected first tool event to render")
	}
	renderer.bufferedToolOrder = append(renderer.bufferedToolOrder, key)
	renderer.bufferedTools[key] = first

	second, secondKey, ok := renderer.prepareToolCallForRender(agent.ToolCallEvent{
		CallID:   "call-b",
		ToolName: "command",
		Summary:  "pwd",
		Result:   "ok",
		State:    agent.ToolCallStateSuccess,
	})
	if !ok {
		t.Fatalf("expected second tool event to render")
	}
	if secondKey != key {
		t.Fatalf("expected duplicate display key, got %q vs %q", secondKey, key)
	}
	if !shouldReplaceBufferedTool(renderer.bufferedTools[key], second) {
		t.Fatalf("expected successful replacement to win over earlier error")
	}
	renderer.bufferedTools[key] = second

	if len(renderer.bufferedToolOrder) != 1 {
		t.Fatalf("expected one buffered line, got %#v", renderer.bufferedToolOrder)
	}
	if got := renderer.bufferedTools[key]; got.State != agent.ToolCallStateSuccess {
		t.Fatalf("expected latest displayed state to be success, got %#v", got)
	}
}

func TestRenderTaiToolCallKeepsCommandLineCompact(t *testing.T) {
	line := renderTaiToolCall(agent.ToolCallEvent{
		ToolName: "command",
		Summary:  "ls",
		Result:   "ok",
		State:    agent.ToolCallStateSuccess,
	})
	if !strings.Contains(line, "command ls") {
		t.Fatalf("expected command summary in rendered line, got %q", line)
	}
	if strings.Contains(line, "· ok") {
		t.Fatalf("expected success marker to stay compact, got %q", line)
	}
}

func TestNormalizeTaiHistoryModeDefaultsToCmd(t *testing.T) {
	mode, err := normalizeTaiHistoryMode("")
	if err != nil {
		t.Fatalf("expected default history mode, got error: %v", err)
	}
	if mode != "cmd" {
		t.Fatalf("expected default history mode cmd, got %q", mode)
	}
}

func TestFilterTaiHistoryCmdExcludesTaiCommands(t *testing.T) {
	commands := []db.Command{
		{Command: "tai 问我一个多选问题"},
		{Command: "ls"},
		{Command: "termia tui"},
		{Command: "pwd"},
	}
	filtered := filterTaiHistory(commands, "cmd")
	if len(filtered) != 2 {
		t.Fatalf("expected only non-AI commands, got %#v", filtered)
	}
	if strings.TrimSpace(filtered[0].Command) != "ls" || strings.TrimSpace(filtered[1].Command) != "pwd" {
		t.Fatalf("expected non-AI command order preserved, got %#v", filtered)
	}
}
