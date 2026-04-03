package db

import (
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestListRecentCommandsSinceFiltersByTimestampAndOrdersByNewestFirst(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "termia.db"), zap.NewNop())
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer database.Close()

	since := time.Now().UnixNano()
	oldEnd := since - 1
	newEnd := since + 1
	newerEnd := since + 2
	durationMs := int64(1)
	exitCode := 0

	for _, command := range []Command{
		{ID: "old", TsStart: oldEnd - 1, TsEnd: &oldEnd, DurationMs: &durationMs, Command: "old", ExitCode: &exitCode, Cwd: "/tmp"},
		{ID: "new", TsStart: since, TsEnd: &newEnd, DurationMs: &durationMs, Command: "new", ExitCode: &exitCode, Cwd: "/tmp"},
		{ID: "newer", TsStart: since + 1, TsEnd: &newerEnd, DurationMs: &durationMs, Command: "newer", ExitCode: &exitCode, Cwd: "/tmp"},
		{ID: "running", TsStart: since + 2, TsEnd: nil, Command: "running", ExitCode: nil, Cwd: "/tmp"},
	} {
		cmd := command
		if err := database.CreateCommand(&cmd); err != nil {
			t.Fatalf("CreateCommand(%s) returned error: %v", cmd.ID, err)
		}
	}

	commands, err := database.ListRecentCommandsSince(since)
	if err != nil {
		t.Fatalf("ListRecentCommandsSince returned error: %v", err)
	}
	if len(commands) != 2 {
		t.Fatalf("expected two completed commands since the lower bound, got %#v", commands)
	}
	if commands[0].ID != "newer" || commands[1].ID != "new" {
		t.Fatalf("expected newest-first ordering, got %#v", commands)
	}
}
