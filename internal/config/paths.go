package config

import (
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
)

// TermiaDir returns the base directory for Termia data.
// Uses XDG_DATA_HOME/termia on Unix-like systems, or ~/.termia as fallback.
func TermiaDir() string {
	// Use XDG data home (typically ~/.local/share on Linux)
	return filepath.Join(xdg.DataHome, "termia")
}

// DBPath returns the path to the history database file.
func DBPath() string {
	return filepath.Join(TermiaDir(), "db", "history.db")
}

// ConfigPath returns the path to the configuration file.
func ConfigPath() string {
	return filepath.Join(TermiaDir(), "config.toml")
}

// TranscriptsDir returns the directory where command transcripts are stored.
func TranscriptsDir() string {
	return filepath.Join(TermiaDir(), "transcripts")
}

func HistoryQueuePath() string {
	return filepath.Join(TermiaDir(), "history.queue")
}

func PendingPromptsCountPath() string {
	return filepath.Join(TermiaDir(), "pending_prompts.count")
}

func CurrentSessionPath() string {
	return filepath.Join(TermiaDir(), "current_session")
}

// ShellDir returns the directory for shell integration files.
func ShellDir() string {
	return filepath.Join(TermiaDir(), "shell")
}

// TeamsDir returns the directory that stores team definition files.
func TeamsDir() string {
	return filepath.Join(TermiaDir(), "teams")
}

// AgentsDir returns the directory for agent/team definitions.
// Deprecated: use TeamsDir.
func AgentsDir() string {
	return TeamsDir()
}

// CacheDir returns the directory for cached data.
func CacheDir() string {
	return filepath.Join(TermiaDir(), "cache")
}

// EnsureDirs creates all required Termia directories if they don't exist.
// Returns an error if any directory creation fails.
func EnsureDirs() error {
	dirs := []string{
		TermiaDir(),
		filepath.Dir(DBPath()), // db directory
		TranscriptsDir(),
		ShellDir(),
		TeamsDir(),
		CacheDir(),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	return nil
}
