package agent

import (
	"testing"

	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

func TestBuildConfirmationResponseContentAddsRejectedPayload(t *testing.T) {
	content := buildConfirmationResponseContent(HITLRequest{
		FunctionCallID: "call-1",
		OriginalTool:   "command",
		Command:        "pwd",
	}, HITLResponse{Confirmed: false})
	if content == nil || len(content.Parts) != 1 || content.Parts[0] == nil || content.Parts[0].FunctionResponse == nil {
		t.Fatalf("expected tool confirmation content, got %#v", content)
	}
	response := content.Parts[0].FunctionResponse.Response
	payload, ok := response["payload"].(map[string]any)
	if !ok {
		t.Fatalf("expected payload map, got %#v", response["payload"])
	}
	if payload["status"] != "rejected" {
		t.Fatalf("expected rejected status, got %#v", payload)
	}
	if payload["command"] != "pwd" {
		t.Fatalf("expected rejected payload to include command, got %#v", payload)
	}
	if payload["original_tool"] != "command" {
		t.Fatalf("expected rejected payload to include original tool, got %#v", payload)
	}
	if response["confirmed"] != false {
		t.Fatalf("expected confirmed=false, got %#v", response["confirmed"])
	}
}

func TestExtractToolResultEventsCapturesCommandFailure(t *testing.T) {
	event := &session.Event{
		Author:      "assistant",
		LLMResponse: session.Event{}.LLMResponse,
	}
	event.Content = &genai.Content{
		Parts: []*genai.Part{
			{
				FunctionResponse: &genai.FunctionResponse{
					Name: "command",
					ID:   "call-1",
					Response: map[string]any{
						"command":   "pwd",
						"exit_code": float64(1),
					},
				},
			},
		},
	}

	results := extractToolResultEvents(event)
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
