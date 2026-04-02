package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/termia/termia/internal/agent"
	"github.com/termia/termia/internal/agentapp"
	"github.com/termia/termia/internal/config"
)

type fakeTUIAgentAppService struct {
	requests []agentapp.RunRequest
	stream   <-chan agent.RuntimeEvent
	err      error
}

func (f *fakeTUIAgentAppService) Run(_ context.Context, req agentapp.RunRequest) (<-chan agent.RuntimeEvent, error) {
	f.requests = append(f.requests, req)
	if f.stream == nil {
		ch := make(chan agent.RuntimeEvent)
		close(ch)
		f.stream = ch
	}
	return f.stream, f.err
}

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

func TestAgentModelRendersReasoningSeparatelyFromAssistantText(t *testing.T) {
	model := NewAgentModel(DefaultKeyMap())
	model.SetSize(60, 12)
	model.AppendReasoning("Plan the next step.")
	model.AppendToLast("Final answer.")

	view := model.viewport.View()
	normalized := strings.Join(strings.Fields(view), " ")
	if !strings.Contains(normalized, "… Plan the next step.") {
		t.Fatalf("expected reasoning block in viewport, got %q", view)
	}
	if !strings.Contains(normalized, "• Final answer.") {
		t.Fatalf("expected assistant block in viewport, got %q", view)
	}
}

func TestNewInitializesSlashSuggestionsWithSharedCommands(t *testing.T) {
	app := New(nil, &config.Config{}, nil)
	app.input.SetValue("/r")

	suggestions := app.input.SlashSuggestions()
	if len(suggestions) != 1 || suggestions[0].Name != "ralph-loop" {
		t.Fatalf("expected /r to suggest /ralph-loop, got %#v", suggestions)
	}

	expectedDesc := ""
	for _, cmd := range agentapp.DefaultSharedSlashCommands() {
		if cmd.Name == "ralph-loop" {
			expectedDesc = cmd.Description
			break
		}
	}
	if expectedDesc == "" {
		t.Fatalf("expected shared slash command metadata for /ralph-loop")
	}
	if suggestions[0].Desc != expectedDesc {
		t.Fatalf("expected /ralph-loop description %q, got %q", expectedDesc, suggestions[0].Desc)
	}
}

func TestExecuteSlashCommandSharedCommandNotHandledLocally(t *testing.T) {
	cmd := executeSlashCommand(&SlashCommand{Name: "ralph-loop"}, nil, nil)
	if cmd == nil {
		t.Fatalf("expected slash command cmd")
	}
	msg := cmd()
	result, ok := msg.(SlashCommandResult)
	if !ok {
		t.Fatalf("expected SlashCommandResult, got %T", msg)
	}
	if !strings.Contains(result.Output, "Unknown command: /ralph-loop") {
		t.Fatalf("expected shared slash command to be treated as non-local, got %q", result.Output)
	}
}

func TestRunAIQueryDelegatesToSharedService(t *testing.T) {
	stream := make(chan agent.RuntimeEvent)
	close(stream)
	service := &fakeTUIAgentAppService{stream: stream}
	app := App{
		agentService:     service,
		activeSessionID:  "session-123",
		cwd:              "/tmp/workspace",
		approvalRequests: make(chan approvalRequest),
		askRequests:      make(chan askRequest),
	}
	selected := []agent.Command{{ID: "cmd-1", Command: "pwd"}}

	gotStream, err := app.runAIQuery(context.Background(), "/ralph-loop inspect", selected)
	if err != nil {
		t.Fatalf("runAIQuery returned error: %v", err)
	}
	if gotStream != stream {
		t.Fatalf("expected runAIQuery to return service stream")
	}
	if len(service.requests) != 1 {
		t.Fatalf("expected one service request, got %d", len(service.requests))
	}

	req := service.requests[0]
	if req.SessionID != "session-123" {
		t.Fatalf("expected SessionID session-123, got %q", req.SessionID)
	}
	if req.Query != "/ralph-loop inspect" {
		t.Fatalf("expected query to pass through unchanged, got %q", req.Query)
	}
	if req.Cwd != "/tmp/workspace" {
		t.Fatalf("expected cwd /tmp/workspace, got %q", req.Cwd)
	}
	if req.Responder == nil {
		t.Fatalf("expected responder to be provided")
	}
	if len(req.SelectedCommands) != 1 || req.SelectedCommands[0].ID != "cmd-1" {
		t.Fatalf("expected selected command to be forwarded, got %#v", req.SelectedCommands)
	}

	selected[0].ID = "mutated"
	if req.SelectedCommands[0].ID != "cmd-1" {
		t.Fatalf("expected selected commands to be copied defensively")
	}
}
