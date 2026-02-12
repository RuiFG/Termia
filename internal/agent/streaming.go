package agent

import (
	"sync"
	"time"

	"go.uber.org/zap"
)

// StreamHandler is a lightweight placeholder for future streaming metrics.
type StreamHandler struct {
	output chan string
	done   chan struct{}
	cancel chan struct{}
	logger *zap.Logger
	stats  StreamStats
	mu     sync.Mutex
	start  time.Time
}

// StreamStats summarizes streaming performance.
type StreamStats struct {
	TotalTokens      int
	PromptTokens     int
	CompletionTokens int
	Duration         time.Duration
	Model            string
}

// NewStreamHandler constructs a stream handler.
func NewStreamHandler(logger *zap.Logger) *StreamHandler {
	return &StreamHandler{
		output: make(chan string, 16),
		done:   make(chan struct{}),
		cancel: make(chan struct{}),
		logger: logger,
	}
}

// Wait blocks until streaming completes.
func (h *StreamHandler) Wait() {
	<-h.done
}

// Cancel stops streaming processing.
func (h *StreamHandler) Cancel() {
	select {
	case <-h.cancel:
		return
	default:
		close(h.cancel)
	}
}

// Stats returns streaming statistics.
func (h *StreamHandler) Stats() *StreamStats {
	h.mu.Lock()
	defer h.mu.Unlock()
	stats := h.stats
	return &stats
}
