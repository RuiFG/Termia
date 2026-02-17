//go:build !windows

package wrapper

import (
	"errors"
	"io"
	"os"
	"sync"

	"github.com/muesli/cancelreader"
	"go.uber.org/zap"
)

// bridgeState tracks the pause/resume state of stdin bridging.
type bridgeState struct {
	mu     sync.Mutex
	paused bool
	cond   *sync.Cond
}

func newBridgeState() *bridgeState {
	state := &bridgeState{}
	state.cond = sync.NewCond(&state.mu)
	return state
}

func (s *bridgeState) waitIfPaused() {
	s.mu.Lock()
	for s.paused {
		s.cond.Wait()
	}
	s.mu.Unlock()
}

func (s *bridgeState) pause() {
	s.mu.Lock()
	s.paused = true
	s.mu.Unlock()
}

func (s *bridgeState) resume() {
	s.mu.Lock()
	s.paused = false
	s.cond.Broadcast()
	s.mu.Unlock()
}

func (s *bridgeState) isPaused() bool {
	s.mu.Lock()
	paused := s.paused
	s.mu.Unlock()
	return paused
}

// startBridge starts bidirectional I/O bridging between terminal and PTY.
func (w *Wrapper) startBridge() {
	w.bridgeState = newBridgeState()

	// Goroutine 1: stdin → PTY (with pause/resume support)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				w.logger.Error("stdin bridge panic", zap.Any("panic", r))
			}
		}()

		buf := make([]byte, 4096)
		for {
			w.bridgeState.waitIfPaused()

			reader, err := w.getInputReader()
			if err != nil {
				w.logger.Error("failed to init stdin reader", zap.Error(err))
				return
			}

			// Read from stdin
			n, err := reader.Read(buf)
			if err != nil {
				if errors.Is(err, cancelreader.ErrCanceled) {
					w.resetInputReader()
					continue
				}
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

			w.bridgeState.waitIfPaused()

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

func (w *Wrapper) getInputReader() (cancelreader.CancelReader, error) {
	w.bridgeInputMu.Lock()
	defer w.bridgeInputMu.Unlock()

	if w.bridgeInput != nil {
		return w.bridgeInput, nil
	}

	reader, err := cancelreader.NewReader(os.Stdin)
	if err != nil {
		return nil, err
	}

	w.bridgeInput = reader
	return reader, nil
}

func (w *Wrapper) resetInputReader() {
	w.bridgeInputMu.Lock()
	if w.bridgeInput != nil {
		_ = w.bridgeInput.Close()
		w.bridgeInput = nil
	}
	w.bridgeInputMu.Unlock()
}

func (w *Wrapper) cancelInputReader() {
	w.bridgeInputMu.Lock()
	reader := w.bridgeInput
	w.bridgeInputMu.Unlock()

	if reader != nil {
		reader.Cancel()
	}
}

// Pause stops stdin→PTY bridging (for TUI takeover).
func (w *Wrapper) Pause() {
	if w.bridgeState == nil {
		w.logger.Info("pause requested before bridge ready")
		return
	}

	w.bridgeState.pause()
	w.cancelInputReader()
	w.logger.Info("bridge paused")
}

// Resume resumes stdin→PTY bridging.
func (w *Wrapper) Resume() {
	if w.bridgeState == nil {
		w.logger.Info("resume requested before bridge ready")
		return
	}

	w.bridgeState.resume()
	w.logger.Info("bridge resumed")
}
