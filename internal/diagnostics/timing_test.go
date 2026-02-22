package diagnostics

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTimingEnabled(t *testing.T) {
	tempDir := t.TempDir()
	previousTmpDir := os.Getenv("TMPDIR")
	previousTiming := os.Getenv("TERMIA_STARTUP_TIMING")

	if err := os.Setenv("TMPDIR", tempDir); err != nil {
		t.Fatalf("set TMPDIR: %v", err)
	}
	if err := os.Setenv("TERMIA_STARTUP_TIMING", "1"); err != nil {
		t.Fatalf("set TERMIA_STARTUP_TIMING: %v", err)
	}

	t.Cleanup(func() {
		if previousTmpDir == "" {
			_ = os.Unsetenv("TMPDIR")
		} else {
			_ = os.Setenv("TMPDIR", previousTmpDir)
		}

		if previousTiming == "" {
			_ = os.Unsetenv("TERMIA_STARTUP_TIMING")
		} else {
			_ = os.Setenv("TERMIA_STARTUP_TIMING", previousTiming)
		}
	})

	done := Track("test", nil)
	done()

	logPath := filepath.Join(os.TempDir(), "termia-startup-timing.log")
	contents, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}

	if !strings.Contains(string(contents), "step=test") {
		t.Fatalf("expected log to contain step=test, got: %s", contents)
	}
}

func TestTimingDisabled(t *testing.T) {
	tempDir := t.TempDir()
	previousTmpDir := os.Getenv("TMPDIR")
	previousTiming := os.Getenv("TERMIA_STARTUP_TIMING")

	if err := os.Setenv("TMPDIR", tempDir); err != nil {
		t.Fatalf("set TMPDIR: %v", err)
	}
	if err := os.Unsetenv("TERMIA_STARTUP_TIMING"); err != nil {
		t.Fatalf("unset TERMIA_STARTUP_TIMING: %v", err)
	}

	t.Cleanup(func() {
		if previousTmpDir == "" {
			_ = os.Unsetenv("TMPDIR")
		} else {
			_ = os.Setenv("TMPDIR", previousTmpDir)
		}

		if previousTiming == "" {
			_ = os.Unsetenv("TERMIA_STARTUP_TIMING")
		} else {
			_ = os.Setenv("TERMIA_STARTUP_TIMING", previousTiming)
		}
	})

	done := Track("disabled", nil)
	done()

	logPath := filepath.Join(os.TempDir(), "termia-startup-timing.log")
	if _, err := os.Stat(logPath); err == nil {
		t.Fatalf("expected no log file, but it exists")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat log: %v", err)
	}
}
