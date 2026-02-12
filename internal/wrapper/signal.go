//go:build !windows

package wrapper

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/creack/pty"
	"go.uber.org/zap"
	"golang.org/x/term"
)

// startSignalHandler handles terminal resize signals.
func (w *Wrapper) startSignalHandler() {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				w.logger.Error("signal handler panic", zap.Any("panic", r))
			}
		}()

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGWINCH)

		w.logger.Debug("starting signal handler")

		for {
			select {
			case <-w.done:
				signal.Stop(sigCh)
				w.logger.Debug("signal handler exited")
				return

			case <-sigCh:
				// Handle terminal resize
				if err := inheritSize(w.ptmx); err != nil {
					w.logger.Warn("failed to resize PTY", zap.Error(err))
				} else {
					w.logger.Debug("PTY resized")
				}
			}
		}
	}()
}

// inheritSize copies the terminal size from stdin to the PTY.
func inheritSize(ptmx *os.File) error {
	// Get current terminal size of stdin
	width, height, err := term.GetSize(int(os.Stdin.Fd()))
	if err != nil {
		return err
	}

	// Set PTY size
	ws := &pty.Winsize{
		Rows: uint16(height),
		Cols: uint16(width),
	}

	return pty.Setsize(ptmx, ws)
}
