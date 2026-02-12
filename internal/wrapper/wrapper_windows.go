//go:build windows

package wrapper

import (
	"fmt"

	"github.com/termia/termia/internal/db"
	"go.uber.org/zap"
)

// Wrapper wraps a shell process in a PTY and records all I/O and commands.
// On Windows, PTY-based wrapping is not supported.
type Wrapper struct{}

// Options configures the wrapper.
type Options struct {
	Shell    string
	Args     []string
	NoRecord bool
	DB       *db.DB
	Logger   *zap.Logger
}

// New returns an error on Windows since PTY wrapping is not supported.
func New(opts Options) (*Wrapper, error) {
	return nil, fmt.Errorf("PTY wrapper is not supported on Windows; Termia targets Linux/macOS")
}

// Start is not supported on Windows.
func (w *Wrapper) Start() error {
	return fmt.Errorf("not supported on Windows")
}

// Wait is not supported on Windows.
func (w *Wrapper) Wait() error {
	return fmt.Errorf("not supported on Windows")
}

// Close is a no-op on Windows.
func (w *Wrapper) Close() error {
	return nil
}

// SessionID returns empty on Windows.
func (w *Wrapper) SessionID() string {
	return ""
}

// Pause is a no-op on Windows.
func (w *Wrapper) Pause() {}

// Resume is a no-op on Windows.
func (w *Wrapper) Resume() {}
