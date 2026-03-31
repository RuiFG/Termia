package db

import "testing"

func TestAgentMessageMetadataRoundTrip(t *testing.T) {
	encoded, err := EncodeAgentMessageMetadata(AgentMessageMetadata{
		CitedCommands: []AgentMessageCommandMetadata{
			{ID: "cmd-1", Command: "git status"},
			{ID: "cmd-2", Command: "go test ./..."},
		},
		ToolCalls: []AgentMessageToolCallMetadata{
			{CallID: "call-1", AgentName: "assistant", ToolName: "command", Summary: "git status", State: "success", Result: "ok"},
			{CallID: "call-2", AgentName: "assistant", ToolName: "inspect_command_output", Summary: "cmd-1", State: "pending"},
		},
	})
	if err != nil {
		t.Fatalf("EncodeAgentMessageMetadata() error = %v", err)
	}
	metadata := ParseAgentMessageMetadata(AgentMessage{
		MetadataJSON: encoded,
	})
	if len(metadata.CitedCommands) != 2 {
		t.Fatalf("expected 2 cited commands, got %v", metadata.CitedCommands)
	}
	if len(metadata.ToolCalls) != 2 {
		t.Fatalf("expected 2 tool calls, got %v", metadata.ToolCalls)
	}
	if metadata.ToolCalls[0].State != "success" || metadata.ToolCalls[0].Result != "ok" {
		t.Fatalf("expected tool call state/result to round-trip, got %#v", metadata.ToolCalls[0])
	}
	ids := metadata.CommandIDs()
	if len(ids) != 2 {
		t.Fatalf("expected 2 command ids, got %v", ids)
	}
	if ids[0] != "cmd-1" || ids[1] != "cmd-2" {
		t.Fatalf("unexpected cited command ids: %v", ids)
	}
}
