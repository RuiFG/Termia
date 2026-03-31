package tui

import "testing"

func TestExecuteSlashCommandExitQuits(t *testing.T) {
	cmd := executeSlashCommand(&SlashCommand{Name: "exit"}, nil, nil)
	if cmd == nil {
		t.Fatalf("expected /exit to produce a command")
	}
	msg := cmd()
	result, ok := msg.(SlashCommandResult)
	if !ok {
		t.Fatalf("expected SlashCommandResult, got %T", msg)
	}
	if !result.Quit {
		t.Fatalf("expected /exit to request quit")
	}
}
