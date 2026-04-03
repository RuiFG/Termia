package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestParseDirectoryChangeCommand(t *testing.T) {
	req, ok := parseDirectoryChangeCommand("cd ../dist")
	if !ok {
		t.Fatal("expected plain cd command to be recognized")
	}
	if req.RawTarget != "../dist" {
		t.Fatalf("unexpected target %q", req.RawTarget)
	}
	if req.FollowUpCommand != "" {
		t.Fatalf("expected plain cd to have no follow-up command, got %q", req.FollowUpCommand)
	}

	req, ok = parseDirectoryChangeCommand("cd ../dist && ls")
	if !ok {
		t.Fatal("expected leading cd with follow-up command to be recognized")
	}
	if req.RawTarget != "../dist" {
		t.Fatalf("unexpected compound target %q", req.RawTarget)
	}
	if req.FollowUpCommand != "ls" {
		t.Fatalf("expected follow-up command ls, got %q", req.FollowUpCommand)
	}

	if _, ok := parseDirectoryChangeCommand("cd ../dist || ls"); ok {
		t.Fatal("expected cd with unsupported follow-up operator to be rejected")
	}
}

func TestExecuteCommandPersistsLeadingDirectoryChangeBeforeFollowUpCommand(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "child")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}

	shellPath := filepath.Join(root, "fake-shell.cmd")
	script := "@echo off\r\n" +
		"setlocal\r\n" +
		"if \"%~1\"==\"-lc\" shift\r\n" +
		"set \"CMD=%~1\"\r\n" +
		"if /I \"%CMD%\"==\"pwd\" (\r\n" +
		"  echo %CD%\r\n" +
		"  exit /b 0\r\n" +
		")\r\n" +
		"echo %CD% [%CMD%]\r\n"
	if err := os.WriteFile(shellPath, []byte(script), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	t.Setenv("TERMIA_SHELL", shellPath)

	ctx := context.Background()
	state := newRuntimeState()
	stdout, stderr, exitCode, cwd, cwdChanged, err := executeCommand(ctx, state, "cd child && pwd", root, commandCwdModeSession, nil)
	if err != nil {
		t.Fatalf("executeCommand returned error: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if !cwdChanged {
		t.Fatal("expected cwd change to be reported")
	}
	if cwd != target {
		t.Fatalf("expected cwd %q, got %q", target, cwd)
	}
	if strings.TrimSpace(stdout) != target {
		t.Fatalf("expected stdout to reflect follow-up command running in %q, got %q", target, stdout)
	}
	if got := readCommandStateString(ctx, state, commandStateCwdKey); got != target {
		t.Fatalf("expected command state cwd %q, got %q", target, got)
	}
}

func TestExtractCommandCwdEvent(t *testing.T) {
	msg := schema.ToolMessage(`{"cwd":"/tmp/project","cwd_changed":true}`, "call-1", schema.WithToolName("command"))

	cwd, ok := extractCommandCwdEvent(msg)
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
