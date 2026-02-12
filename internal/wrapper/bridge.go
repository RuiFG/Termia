//go:build !windows

package wrapper

import (
	"io"
	"os"
	"sync"

	"go.uber.org/zap"
)

// bridgeState tracks the pause/resume state of stdin bridging.
type bridgeState struct {
	mu     sync.Mutex
	paused bool
	cond   *sync.Cond
}

// startBridge starts bidirectional I/O bridging between terminal and PTY.
func (w *Wrapper) startBridge() {
	state := &bridgeState{}
	state.cond = sync.NewCond(&state.mu)

	// Goroutine 1: stdin → PTY (with pause/resume support)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				w.logger.Error("stdin bridge panic", zap.Any("panic", r))
			}
		}()

		buf := make([]byte, 4096)
		for {
			// Check if paused
			state.mu.Lock()
			for state.paused {
				state.cond.Wait()
			}
			state.mu.Unlock()

			// Read from stdin
			n, err := os.Stdin.Read(buf)
			if err != nil {
				if err != io.EOF {
					w.logger.Debug("stdin read error", zap.Error(err))
				}
				return
			}

			// Write to PTY
			if _, err := w.ptmx.Write(buf[:n]); err != nil {
				w.logger.Debug("PTY write error", zap.Error(err))
				return
			}
		}
	}()

	// Goroutine 2: PTY → stdout (with recording)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				w.logger.Error("output bridge panic", zap.Any("panic", r))
			}
		}()

		buf := make([]byte, 4096)
		var offset int64 = 0

		for {
			// Read from PTY
			n, err := w.ptmx.Read(buf)
			if err != nil {
				if err != io.EOF {
					w.logger.Debug("PTY read error", zap.Error(err))
				}
				return
			}

			// Write to stdout
			if _, err := os.Stdout.Write(buf[:n]); err != nil {
				w.logger.Debug("stdout write error", zap.Error(err))
				return
			}

			// Record to transcript (if recorder exists)
			if w.recorder != nil && !w.noRecord {
				if _, err := w.recorder.RecordBytes(buf[:n]); err != nil {
					w.logger.Error("failed to record output", zap.Error(err))
				}
			}

			offset += int64(n)
		}
	}()
}

// Pause stops stdin→PTY bridging (for TUI takeover).
func (w *Wrapper) Pause() {
	// Note: This would need access to the bridge state.
	// For now, we'll add a field to Wrapper to hold the state.
	w.logger.Info("pause requested (not fully implemented)")
}

// Resume resumes stdin→PTY bridging.
func (w *Wrapper) Resume() {
	w.logger.Info("resume requested (not fully implemented)")
}
