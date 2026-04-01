package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/termia/termia/internal/agent"
	"github.com/termia/termia/internal/config"
	"github.com/termia/termia/internal/db"
)

func TestResolveTaiRuntimeUsesSessionDefaults(t *testing.T) {
	prevCfg := cfg
	cfg = &config.Config{}
	t.Cleanup(func() {
		cfg = prevCfg
	})

	cmd := newTaiRuntimeTestCommand(t)
	mode, teamName, err := resolveTaiRuntime(cmd, db.AgentSession{
		Mode:     string(agent.ModeTeam),
		TeamName: "ops",
	}, true)
	if err != nil {
		t.Fatalf("resolveTaiRuntime returned error: %v", err)
	}
	if mode != agent.ModeTeam {
		t.Fatalf("expected team mode, got %q", mode)
	}
	if teamName != "ops" {
		t.Fatalf("expected existing team name, got %q", teamName)
	}
}

func TestResolveTaiRuntimeUsesTeamNameFromModeFlag(t *testing.T) {
	prevCfg := cfg
	cfg = &config.Config{}
	t.Cleanup(func() {
		cfg = prevCfg
	})

	prevMode := taiMode
	taiMode = "ops"
	t.Cleanup(func() {
		taiMode = prevMode
	})

	cmd := newTaiRuntimeTestCommand(t)
	if err := cmd.Flags().Set("mode", "ops"); err != nil {
		t.Fatalf("failed to set mode flag: %v", err)
	}

	mode, teamName, err := resolveTaiRuntime(cmd, db.AgentSession{}, false)
	if err != nil {
		t.Fatalf("resolveTaiRuntime returned error: %v", err)
	}
	if mode != agent.ModeTeam {
		t.Fatalf("expected team mode, got %q", mode)
	}
	if teamName != "ops" {
		t.Fatalf("expected mode flag to carry team name, got %q", teamName)
	}
}

func TestResolveTaiRuntimeRejectsLiteralTeamMode(t *testing.T) {
	prevMode := taiMode
	taiMode = "team"
	t.Cleanup(func() {
		taiMode = prevMode
	})

	cmd := newTaiRuntimeTestCommand(t)
	if err := cmd.Flags().Set("mode", "team"); err != nil {
		t.Fatalf("failed to set mode flag: %v", err)
	}
	if _, _, err := resolveTaiRuntime(cmd, db.AgentSession{}, false); err == nil {
		t.Fatalf("expected literal team mode to fail")
	}
}

func TestTaiConversationMessagesFromDBSkipsToolAndError(t *testing.T) {
	messages := taiConversationMessagesFromDB([]db.AgentMessage{
		{
			Role:         "user",
			Content:      "question",
			MetadataJSON: `{"cited_commands":[{"id":"cmd-1","command":"pwd"}]}`,
		},
		{
			Role:         "tool",
			Content:      "",
			MetadataJSON: `{"tool_calls":[{"tool_name":"command","summary":"pwd","state":"success"}]}`,
		},
		{
			Role:    "assistant",
			Content: "answer",
		},
		{
			Role:    "error",
			Content: "failed",
		},
	})

	if len(messages) != 2 {
		t.Fatalf("expected user and assistant messages only, got %#v", messages)
	}
	if messages[0].Role != "user" || messages[1].Role != "assistant" {
		t.Fatalf("unexpected message roles: %#v", messages)
	}
	if len(messages[0].Commands) != 1 || messages[0].Commands[0].ID != "cmd-1" {
		t.Fatalf("expected cited command metadata to be preserved, got %#v", messages[0].Commands)
	}
}

func TestTaiConversationMessagesFromDBNormalizesCarriageReturns(t *testing.T) {
	messages := taiConversationMessagesFromDB([]db.AgentMessage{
		{Role: "assistant", Content: "first\rsecond\r\nthird"},
	})
	if len(messages) != 1 {
		t.Fatalf("expected one assistant message, got %#v", messages)
	}
	if got := messages[0].Content; got != "first\nsecond\nthird" {
		t.Fatalf("expected normalized line endings, got %q", got)
	}
}

func TestTaiNormalizeToolCallNormalizesInlineFields(t *testing.T) {
	toolCall := taiNormalizeToolCall(agent.ToolCallEvent{
		CallID:    " call-1 ",
		AgentName: " assistant\r ",
		ToolName:  " command\r ",
		Summary:   " netstat\r\n-tuln ",
		Result:    " open\r ports ",
	})
	if toolCall.CallID != "call-1" {
		t.Fatalf("expected trimmed call id, got %q", toolCall.CallID)
	}
	if toolCall.AgentName != "assistant" {
		t.Fatalf("expected normalized agent name, got %q", toolCall.AgentName)
	}
	if toolCall.ToolName != "command" {
		t.Fatalf("expected normalized tool name, got %q", toolCall.ToolName)
	}
	if toolCall.Summary != "netstat -tuln" {
		t.Fatalf("expected normalized summary, got %q", toolCall.Summary)
	}
	if toolCall.Result != "open ports" {
		t.Fatalf("expected normalized result, got %q", toolCall.Result)
	}
}

func newTaiRuntimeTestCommand(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "tai"}
	cmd.Flags().String("mode", "assistant", "")
	return cmd
}
