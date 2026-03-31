package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/termia/termia/internal/db"
)

func TestBuildPromptTextEmbedsCommandMetadataPerUserMessage(t *testing.T) {
	outputSize := int64(128)
	prompt := buildPromptText(RunRequest{
		Cwd:   "/workspace",
		Query: "Why did this fail?",
		Messages: []Message{
			{
				Role:    "user",
				Content: "Check the previous command.",
				Commands: []Command{{
					ID:                  "cmd-1",
					Command:             "grep -n read app.log",
					Cwd:                 "/srv/app",
					OutputSize:          &outputSize,
					TranscriptAvailable: true,
				}},
			},
			{
				Role:    "assistant",
				Content: "I need to inspect the actual output.",
			},
		},
		SelectedCommands: []Command{{
			ID:                  "cmd-2",
			Command:             "go test ./...",
			Cwd:                 "/workspace",
			TranscriptAvailable: true,
		}},
	}, "")

	if strings.Contains(prompt, "Recent terminal commands:") {
		t.Fatalf("expected command metadata to be attached to user messages, got prompt:\n%s", prompt)
	}
	for _, want := range []string{
		"Conversation transcript:",
		`command="grep -n read app.log"`,
		"id=cmd-1",
		"id=cmd-2",
		"inspect_command_output",
		"User request:",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected prompt to contain %q, got:\n%s", want, prompt)
		}
	}
}

func TestInspectCommandOutputReadsChunkAndSearches(t *testing.T) {
	dir := t.TempDir()
	transcriptPath := filepath.Join(dir, "cmd-1.txt")
	if err := os.WriteFile(transcriptPath, []byte("alpha\nread line\nbeta\nREAD line\n"), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	outputSize := int64(30)
	store := stubCommandDB{
		command: &db.Command{
			ID:             "cmd-1",
			Command:        "grep read -n app.log",
			Cwd:            "/srv/app",
			OutputSize:     &outputSize,
			TranscriptPath: &transcriptPath,
		},
	}

	readRsp, err := inspectCommandOutput(store, &InspectCommandOutputReq{
		CommandID: "cmd-1",
		MaxLines:  2,
	})
	if err != nil {
		t.Fatalf("inspectCommandOutput(read): %v", err)
	}
	if readRsp.Chunk != "alpha\nread line" {
		t.Fatalf("unexpected chunk: %q", readRsp.Chunk)
	}
	if !readRsp.Truncated {
		t.Fatalf("expected truncated chunk when max_lines limits output")
	}

	searchRsp, err := inspectCommandOutput(store, &InspectCommandOutputReq{
		CommandID:  "cmd-1",
		Query:      "read",
		IgnoreCase: true,
	})
	if err != nil {
		t.Fatalf("inspectCommandOutput(search): %v", err)
	}
	if searchRsp.MatchCount != 2 {
		t.Fatalf("expected 2 matches, got %d", searchRsp.MatchCount)
	}
	for _, want := range []string{"2: read line", "4: READ line"} {
		if !strings.Contains(searchRsp.Excerpt, want) {
			t.Fatalf("expected excerpt to contain %q, got %q", want, searchRsp.Excerpt)
		}
	}
}

type stubCommandDB struct {
	command *db.Command
}

func (s stubCommandDB) CreateCommand(cmd *db.Command) error {
	return nil
}

func (s stubCommandDB) UpdateCommandEnd(id string, tsEnd int64, exitCode int, endOffset, outputSize int64, transcriptPath *string) error {
	return nil
}

func (s stubCommandDB) GetCommand(id string) (*db.Command, error) {
	if s.command == nil || s.command.ID != id {
		return nil, os.ErrNotExist
	}
	return s.command, nil
}
