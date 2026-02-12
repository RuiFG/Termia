//go:build !windows

package wrapper

import (
	"bufio"

	"github.com/termia/termia/internal/recorder"
	"go.uber.org/zap"
)

// startMarkerReader reads marker messages from the shell integration.
func (w *Wrapper) startMarkerReader() {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				w.logger.Error("marker reader panic", zap.Any("panic", r))
			}
		}()

		w.logger.Debug("starting marker reader")

		scanner := bufio.NewScanner(w.markerR)

		for scanner.Scan() {
			line := scanner.Bytes()

			marker, err := recorder.ParseMarker(line)
			if err != nil {
				w.logger.Warn("failed to parse marker", zap.Error(err), zap.String("line", string(line)))
				continue
			}

			if w.recorder != nil && !w.noRecord {
				if err := w.recorder.HandleMarker(marker); err != nil {
					w.logger.Error("failed to handle marker", zap.Error(err), zap.String("cmdID", marker.CmdID))
				}
			}
		}

		if err := scanner.Err(); err != nil {
			w.logger.Error("marker reader error", zap.Error(err))
		}

		w.logger.Debug("marker reader exited")
	}()
}
