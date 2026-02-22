package shell

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/termia/termia/internal/diagnostics"
)

// ShellType represents the type of shell
type ShellType string

const (
	ShellZsh     ShellType = "zsh"
	ShellBash    ShellType = "bash"
	ShellFish    ShellType = "fish"
	ShellUnknown ShellType = "unknown"
)

// ShellInfo contains information about the detected shell
type ShellInfo struct {
	Type    ShellType
	Path    string // full path to shell binary
	Version string // e.g., "zsh 5.9"
}

// Detect detects the current shell from environment variables
func Detect() ShellInfo {
	// Read $SHELL env var
	shellPath := os.Getenv("SHELL")
	if shellPath == "" {
		// Default to /bin/bash if not found
		shellPath = "/bin/bash"
	}
	if IsTermiaShellPath(shellPath) {
		shellPath = "/bin/bash"
	}

	// Determine type from path
	shellType := DetectFromPath(shellPath)

	// Try to get version
	version := getShellVersion(shellPath)

	return ShellInfo{
		Type:    shellType,
		Path:    shellPath,
		Version: version,
	}
}

// DetectFromPath parses shell type from path string
func DetectFromPath(shellPath string) ShellType {
	lower := strings.ToLower(shellPath)

	if strings.Contains(lower, "zsh") {
		return ShellZsh
	}
	if strings.Contains(lower, "bash") {
		return ShellBash
	}
	if strings.Contains(lower, "fish") {
		return ShellFish
	}

	return ShellUnknown
}

// IsTermiaShellPath returns true when the path points to the termia binary.
func IsTermiaShellPath(shellPath string) bool {
	if shellPath == "" {
		return false
	}

	base := filepath.Base(shellPath)
	return strings.EqualFold(base, "termia")
}

// getShellVersion attempts to get the shell version by running shell --version
func getShellVersion(shellPath string) string {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Run shell --version
	cmd := exec.CommandContext(ctx, shellPath, "--version")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := func() error {
		defer diagnostics.Track("startup.shell.version", nil)()
		return cmd.Run()
	}(); err != nil {
		return ""
	}

	// Get first line of output
	output := out.String()
	lines := strings.Split(output, "\n")
	if len(lines) > 0 {
		return strings.TrimSpace(lines[0])
	}

	return ""
}
