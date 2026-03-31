package tui

import (
	"strings"
	"testing"

	"github.com/termia/termia/internal/db"
)

func TestHistoryDetailInfoLinesStripANSIForSelection(t *testing.T) {
	exitCode := 0
	m := NewHistoryDetailModel(DefaultKeyMap())
	m.SetSize(80, 12)
	m.SetCommand(&db.Command{
		ID:       "cmd-1",
		Command:  "echo hello",
		Cwd:      "/mnt/d/Projects/Termia/dist",
		TsStart:  1739419200000000000,
		ExitCode: &exitCode,
	})
	m.SetContent("plain output")

	joined := strings.Join(m.lines, "\n")
	if strings.Contains(joined, "\x1b") {
		t.Fatalf("expected detail content to be plain text, got %q", joined)
	}
	if !strings.Contains(joined, "COMMAND: echo hello") {
		t.Fatalf("expected plain COMMAND line, got %q", joined)
	}
	if !strings.Contains(joined, "CWD:     /mnt/d/Projects/Termia/dist") {
		t.Fatalf("expected plain CWD line, got %q", joined)
	}
}
