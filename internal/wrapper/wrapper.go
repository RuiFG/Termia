//go:build !windows

package wrapper

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/creack/pty"
	"github.com/termia/termia/internal/config"
	"github.com/termia/termia/internal/db"
	"github.com/termia/termia/internal/recorder"
	"github.com/termia/termia/internal/shell"
	"go.uber.org/zap"
	"golang.org/x/term"
)

// Wrapper wraps a shell process in a PTY and records all I/O and commands.
type Wrapper struct {
	shell         string
	args          []string
	ptmx          *os.File
	cmd           *exec.Cmd
	db            *db.DB
	recorder      *recorder.Recorder
	markerR       *os.File
	markerW       *os.File
	logger        *zap.Logger
	done          chan struct{}
	noRecord      bool
	transcriptDir string

	// Terminal state restoration
	oldState *term.State
}

// Options configures the wrapper.
type Options struct {
	Shell    string
	Args     []string
	NoRecord bool
	DB       *db.DB
	Logger   *zap.Logger
}

// New creates a new wrapper instance.
func New(opts Options) (*Wrapper, error) {

	// Detect shell
	shellPath := opts.Shell
	if shellPath == "" {
		shellPath = os.Getenv("SHELL")
	}
	if shellPath == "" {
		shellPath = "/bin/bash"
	}
	if shell.IsTermiaShellPath(shellPath) {
		shellPath = "/bin/bash"
	}

	// Create marker pipe
	markerR, markerW, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create marker pipe: %w", err)
	}

	logger := opts.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	w := &Wrapper{
		shell:         shellPath,
		db:            opts.DB,
		markerR:       markerR,
		markerW:       markerW,
		logger:        logger,
		done:          make(chan struct{}),
		noRecord:      opts.NoRecord,
		transcriptDir: config.TranscriptsDir(),
		args:          opts.Args,
	}

	return w, nil
}

// Start spawns the shell in a PTY and begins recording.
func (w *Wrapper) Start() error {
	// Check if already wrapped
	if os.Getenv("TERMIA_WRAPPED") == "1" {
		return fmt.Errorf("already running inside Termia wrapper")
	}

	// Build command with shell-specific integration
	var cmd *exec.Cmd
	extraEnv := []string{}
	shellInfo := shell.DetectFromPath(w.shell)
	switch shellInfo {
	case shell.ShellBash:
		args := shell.GetShimCommand(shell.ShellBash, w.shell)
		cmdArgs := append([]string{}, args[1:]...)
		cmdArgs = append(cmdArgs, w.args...)
		cmd = exec.Command(args[0], cmdArgs...)
	case shell.ShellZsh:
		cmd = exec.Command(w.shell, w.args...)
		extraEnv = append(extraEnv, fmt.Sprintf("ZDOTDIR=%s", config.ShellDir()))
	default:
		cmd = exec.Command(w.shell, w.args...)
	}

	// Resolve the absolute path to our own binary so shell functions
	// (tui, tai) can invoke it without relying on PATH.
	binPath, err := os.Executable()
	if err != nil {
		// Fallback: use os.Args[0] as-is
		binPath = os.Args[0]
	}

	// Set environment variables
	baseEnv := os.Environ()
	cmd.Env = append(baseEnv, extraEnv...)
	cmd.Env = append(cmd.Env,
		fmt.Sprintf("TERMIA_SHELL_DIR=%s", config.ShellDir()),
		fmt.Sprintf("TERMIA_BIN=%s", binPath),
		"TERMIA_INTERNAL=1",
		"TERMIA_INTERNAL_PATTERN=^(echo \"🚀 Termia active! Type 'tui' for history, 'tai' for AI help.\")$",
		"TERMIA_INTEGRATION_LOADED=",
		"TERMIA_MARKER_FD=3",
		"TERMIA_WRAPPED=1",
		"TERMIA_WELCOME=1",
	)

	// Pass marker write FD as FD3 to child
	cmd.ExtraFiles = []*os.File{w.markerW}

	// Start PTY
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return fmt.Errorf("failed to start PTY: %w", err)
	}

	w.ptmx = ptmx
	w.cmd = cmd

	// Close write end of marker pipe in parent
	w.markerW.Close()
	w.markerW = nil

	// Set terminal to raw mode
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("failed to set raw mode: %w", err)
	}
	w.oldState = oldState

	// Inherit terminal size
	if err := inheritSize(w.ptmx); err != nil {
		w.logger.Warn("failed to inherit terminal size", zap.Error(err))
	}

	if !w.noRecord {
		rec, err := recorder.New(w.db, w.transcriptDir, w.logger)
		if err != nil {
			return fmt.Errorf("failed to create recorder: %w", err)
		}
		w.recorder = rec
	}

	// Start goroutines
	w.startBridge()
	w.startMarkerReader()
	w.startSignalHandler()

	w.logger.Info("wrapper started",
		zap.String("shell", w.shell),
	)

	return nil
}

// Wait waits for the shell to exit and cleans up.
func (w *Wrapper) Wait() error {
	// Wait for command to exit
	err := w.cmd.Wait()

	// Signal done
	close(w.done)

	// Close PTY
	if w.ptmx != nil {
		w.ptmx.Close()
	}

	if w.recorder != nil {
		if err := w.recorder.Close(); err != nil {
			w.logger.Error("failed to close recorder", zap.Error(err))
		}
		w.recorder = nil
	}

	// Restore terminal state
	if w.oldState != nil {
		if restoreErr := term.Restore(int(os.Stdin.Fd()), w.oldState); restoreErr != nil {
			w.logger.Error("failed to restore terminal", zap.Error(restoreErr))
		}
	}

	_ = err // exit code is tracked per-command

	w.logger.Info("wrapper exited", zap.Error(err))

	return err
}

// Close performs cleanup.
func (w *Wrapper) Close() error {
	// Kill process if still running
	if w.cmd != nil && w.cmd.Process != nil {
		w.cmd.Process.Kill()
	}

	// Close marker reader
	if w.markerR != nil {
		w.markerR.Close()
	}

	// Close PTY
	if w.ptmx != nil {
		w.ptmx.Close()
	}

	// Restore terminal
	if w.oldState != nil {
		term.Restore(int(os.Stdin.Fd()), w.oldState)
	}

	return nil
}

// SessionID returns an empty ID.
func (w *Wrapper) SessionID() string {
	return ""
}
