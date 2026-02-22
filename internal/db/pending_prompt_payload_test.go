package db

import (
	"bytes"
	"testing"
)

func TestParsePendingPromptPayloadLegacyFallback(t *testing.T) {
	prompt := PendingPrompt{
		Content:     "  echo hi  ",
		PayloadJSON: "",
	}
	got := ParsePendingPromptPayload(prompt)
	if got.Type != PendingPromptTypeCommand {
		t.Fatalf("expected type %q, got %q", PendingPromptTypeCommand, got.Type)
	}
	if got.Command != "echo hi" {
		t.Fatalf("expected command %q, got %q", "echo hi", got.Command)
	}
	if len(got.Payload) != 0 {
		t.Fatalf("expected empty payload, got %q", string(got.Payload))
	}
}

func TestParsePendingPromptPayloadLegacyInvalidJSON(t *testing.T) {
	prompt := PendingPrompt{
		Content:     "ls",
		PayloadJSON: "{not-json}",
	}
	got := ParsePendingPromptPayload(prompt)
	if got.Type != PendingPromptTypeCommand {
		t.Fatalf("expected type %q, got %q", PendingPromptTypeCommand, got.Type)
	}
	if got.Command != "ls" {
		t.Fatalf("expected command %q, got %q", "ls", got.Command)
	}
}

func TestParsePendingPromptPayloadDefaults(t *testing.T) {
	cases := []struct {
		name        string
		payloadJSON string
		content     string
		wantType    string
		wantCommand string
	}{
		{
			name:        "missing type",
			payloadJSON: `{"command":"echo hi"}`,
			content:     "ignored",
			wantType:    PendingPromptTypeCommand,
			wantCommand: "echo hi",
		},
		{
			name:        "command type without command",
			payloadJSON: `{"type":"command"}`,
			content:     "pwd",
			wantType:    PendingPromptTypeCommand,
			wantCommand: "pwd",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prompt := PendingPrompt{
				Content:     tc.content,
				PayloadJSON: tc.payloadJSON,
			}
			got := ParsePendingPromptPayload(prompt)
			if got.Type != tc.wantType {
				t.Fatalf("expected type %q, got %q", tc.wantType, got.Type)
			}
			if got.Command != tc.wantCommand {
				t.Fatalf("expected command %q, got %q", tc.wantCommand, got.Command)
			}
		})
	}
}

func TestParsePendingPromptPayloadAskPayload(t *testing.T) {
	payloadJSON := `{"type":"ask","payload":{"questions":["hi"]}}`
	prompt := PendingPrompt{
		Content:     "ignored",
		PayloadJSON: payloadJSON,
	}
	got := ParsePendingPromptPayload(prompt)
	if got.Type != PendingPromptTypeAsk {
		t.Fatalf("expected type %q, got %q", PendingPromptTypeAsk, got.Type)
	}
	if got.Command != "" {
		t.Fatalf("expected empty command, got %q", got.Command)
	}
	expected := []byte(`{"questions":["hi"]}`)
	if !bytes.Equal(got.Payload, expected) {
		t.Fatalf("expected payload %q, got %q", string(expected), string(got.Payload))
	}
}
