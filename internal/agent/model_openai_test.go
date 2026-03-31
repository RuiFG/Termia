package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"google.golang.org/adk/tool/toolconfirmation"
	"google.golang.org/genai"
)

func TestOpenAIToolCallIDShortensLongIDsDeterministically(t *testing.T) {
	longID := "toolconfirmation_" + "abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz0123456789"

	got1 := openAIToolCallID(longID, "request_input")
	got2 := openAIToolCallID(longID, "request_input")

	if got1 != got2 {
		t.Fatalf("expected deterministic ids, got %q and %q", got1, got2)
	}
	if len(got1) > maxOpenAIToolCallIDLen {
		t.Fatalf("expected id length <= %d, got %d (%q)", maxOpenAIToolCallIDLen, len(got1), got1)
	}
}

func TestOpenAIToolCallIDSanitizesEmptyAndUnsafeIDs(t *testing.T) {
	got := openAIToolCallID("call with spaces/unsafe", "request_input")
	if got == "" {
		t.Fatalf("expected non-empty sanitized id")
	}
	if len(got) > maxOpenAIToolCallIDLen {
		t.Fatalf("expected id length <= %d, got %d (%q)", maxOpenAIToolCallIDLen, len(got), got)
	}
}

func TestUserContentToMessagesUsesToolCallIDInCorrectField(t *testing.T) {
	msgs, err := userContentToMessages(&genai.Content{
		Role: "user",
		Parts: []*genai.Part{{
			FunctionResponse: &genai.FunctionResponse{
				Name: "request_input",
				ID:   "very-long-tool-call-id-with-lots-of-bytes-abcdefghijklmnopqrstuvwxyz-0123456789",
				Response: map[string]any{
					"confirmed": true,
					"payload":   map[string]any{"answer": "ok"},
				},
			},
		}},
	})
	if err != nil {
		t.Fatalf("userContentToMessages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	data, err := json.Marshal(msgs[0])
	if err != nil {
		t.Fatalf("marshal message: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatalf("unmarshal message: %v", err)
	}
	toolCallID, _ := body["tool_call_id"].(string)
	content, _ := body["content"].(string)
	if toolCallID == "" {
		t.Fatalf("expected tool_call_id to be populated: %s", data)
	}
	if len(toolCallID) > maxOpenAIToolCallIDLen {
		t.Fatalf("expected shortened tool_call_id, got %q", toolCallID)
	}
	if content == "" || content == toolCallID {
		t.Fatalf("expected content to contain tool payload and differ from tool_call_id: %s", data)
	}
}

func TestUserContentToMessagesFormatsRejectedToolConfirmationClearly(t *testing.T) {
	msgs, err := userContentToMessages(&genai.Content{
		Role: "user",
		Parts: []*genai.Part{{
			FunctionResponse: &genai.FunctionResponse{
				Name: toolconfirmation.FunctionCallName,
				ID:   "call-1",
				Response: map[string]any{
					"confirmed": false,
					"payload": map[string]any{
						"original_tool": "command",
						"command":       "pwd",
						"status":        "rejected",
					},
				},
			},
		}},
	})
	if err != nil {
		t.Fatalf("userContentToMessages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	data, err := json.Marshal(msgs[0])
	if err != nil {
		t.Fatalf("marshal message: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatalf("unmarshal message: %v", err)
	}
	content, _ := body["content"].(string)
	if !strings.Contains(content, `User rejected command "pwd".`) {
		t.Fatalf("expected rejection summary in tool content, got %q", content)
	}
}
