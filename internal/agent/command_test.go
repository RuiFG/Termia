package agent

import (
	"testing"

	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

func TestParseDirectoryChangeCommand(t *testing.T) {
	req, ok := parseDirectoryChangeCommand("cd ../dist")
	if !ok {
		t.Fatal("expected plain cd command to be recognized")
	}
	if req.RawTarget != "../dist" {
		t.Fatalf("unexpected target %q", req.RawTarget)
	}

	if _, ok := parseDirectoryChangeCommand("cd ../dist && ls"); ok {
		t.Fatal("expected compound cd command to be rejected")
	}
}

func TestExtractCommandCwdEvent(t *testing.T) {
	event := &session.Event{
		LLMResponse: session.Event{}.LLMResponse,
	}
	event.Content = &genai.Content{
		Parts: []*genai.Part{{
			FunctionResponse: &genai.FunctionResponse{
				Name: "command",
				Response: map[string]any{
					"cwd":         "/tmp/project",
					"cwd_changed": true,
				},
			},
		}},
	}

	cwd, ok := extractCommandCwdEvent(event)
	if !ok {
		t.Fatal("expected cwd event to be extracted")
	}
	if cwd != "/tmp/project" {
		t.Fatalf("unexpected cwd %q", cwd)
	}
}

func TestEffectiveCommandCwdPrefersSessionUnlessOverrideRequested(t *testing.T) {
	got := effectiveCommandCwd("/mnt/d/Projects/Termia", "/mnt/d/Projects", commandCwdModeSession)
	if got != "/mnt/d/Projects/Termia" {
		t.Fatalf("expected session cwd, got %q", got)
	}

	got = effectiveCommandCwd("/mnt/d/Projects/Termia", "/mnt/d/Projects", commandCwdModeOverride)
	if got != "/mnt/d/Projects" {
		t.Fatalf("expected explicit override cwd, got %q", got)
	}
}
