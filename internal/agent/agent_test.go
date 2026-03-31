package agent

import (
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

func TestParseHITLRequestFromInterruptEvent(t *testing.T) {
	event := &adk.AgentEvent{
		Action: &adk.AgentAction{
			Interrupted: &adk.InterruptInfo{
				InterruptContexts: []*adk.InterruptCtx{{
					ID:          "agent:assistant;tool:command",
					IsRootCause: true,
					Info: &hitlInterruptInfo{
						Kind:         HITLKindConfirm,
						Title:        "Confirmation Required",
						Prompt:       "Approval required.",
						OriginalTool: "command",
						Command:      "pwd",
						Cwd:          "/tmp/project",
					},
				}},
			},
		},
	}

	request, ok := parseHITLRequest(event)
	if !ok {
		t.Fatal("expected HITL request to be extracted")
	}
	if request.ID != "agent:assistant;tool:command" {
		t.Fatalf("unexpected request id %q", request.ID)
	}
	if request.OriginalTool != "command" || request.Command != "pwd" {
		t.Fatalf("unexpected request payload %#v", request)
	}
}

func TestExtractToolResultEventsCapturesCommandFailure(t *testing.T) {
	msg := schema.ToolMessage(`{"command":"pwd","exit_code":1}`, "call-1", schema.WithToolName("command"))

	results := extractToolResultEvents("assistant", msg)
	if len(results) != 1 {
		t.Fatalf("expected 1 tool result, got %#v", results)
	}
	if results[0].CallID != "call-1" {
		t.Fatalf("expected call id to round-trip, got %#v", results[0])
	}
	if results[0].Summary != "pwd" {
		t.Fatalf("expected command summary, got %#v", results[0])
	}
	if results[0].Result != "exit 1" {
		t.Fatalf("expected command failure result, got %#v", results[0])
	}
	if results[0].State != ToolCallStateError {
		t.Fatalf("expected command failure state, got %#v", results[0])
	}
}
