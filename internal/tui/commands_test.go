package tui

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/termia/termia/internal/db"
	"go.uber.org/zap"
)

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

func TestLoadCommandsCmdSkipsTermiaBinaryLaunchCommands(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "termia.db"), zap.NewNop())
	if err != nil {
		t.Fatalf("db.Open returned error: %v", err)
	}
	defer database.Close()

	cwd := t.TempDir()
	t.Setenv("TERMIA_BIN", filepath.Join(cwd, "bin", "custom-launcher"))

	now := time.Now().UnixNano()
	if err := database.CreateCommand(&db.Command{ID: "launch", TsStart: now, Command: "./bin/custom-launcher", Cwd: cwd}); err != nil {
		t.Fatalf("CreateCommand returned error: %v", err)
	}
	if err := database.UpdateCommandEnd("launch", now+1, 0, 0, 0, nil); err != nil {
		t.Fatalf("UpdateCommandEnd returned error: %v", err)
	}
	if err := database.CreateCommand(&db.Command{ID: "same-name", TsStart: now + 2, Command: "./other/custom-launcher", Cwd: cwd}); err != nil {
		t.Fatalf("CreateCommand returned error: %v", err)
	}
	if err := database.UpdateCommandEnd("same-name", now+3, 0, 0, 0, nil); err != nil {
		t.Fatalf("UpdateCommandEnd returned error: %v", err)
	}
	if err := database.CreateCommand(&db.Command{ID: "user", TsStart: now + 4, Command: "pwd", Cwd: cwd}); err != nil {
		t.Fatalf("CreateCommand returned error: %v", err)
	}
	if err := database.UpdateCommandEnd("user", now+5, 0, 0, 0, nil); err != nil {
		t.Fatalf("UpdateCommandEnd returned error: %v", err)
	}

	msg := loadCommandsCmd(database)()
	loaded, ok := msg.(commandsLoadedMsg)
	if !ok {
		t.Fatalf("expected commandsLoadedMsg, got %T", msg)
	}
	if len(loaded.commands) != 2 {
		t.Fatalf("expected one visible command, got %#v", loaded.commands)
	}
	if loaded.commands[0].ID != "user" || loaded.commands[1].ID != "same-name" {
		t.Fatalf("expected only the active Termia launcher command to be hidden, got %#v", loaded.commands)
	}
}
