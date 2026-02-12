package recorder

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// TranscriptWriter manages transcript file writing with offset tracking
type TranscriptWriter struct {
	file   *os.File
	offset int64 // atomic counter
	path   string
	mu     sync.Mutex
}

// NewTranscriptWriter creates a new transcript writer.
func NewTranscriptWriter(dir string) (*TranscriptWriter, error) {
	// Create directory structure: dir/YYYY-MM-DD/command-<time>.log
	now := time.Now()
	dateDir := now.Format("2006-01-02")
	fullDir := filepath.Join(dir, dateDir)

	if err := os.MkdirAll(fullDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create transcript directory: %w", err)
	}

	// Create transcript file path
	filename := fmt.Sprintf("command-%d.log", now.UnixNano())
	path := filepath.Join(fullDir, filename)

	// Open file for writing (create or append)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open transcript file: %w", err)
	}

	// Get current file size for offset
	stat, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to stat transcript file: %w", err)
	}

	return &TranscriptWriter{
		file:   file,
		offset: stat.Size(),
		path:   path,
	}, nil
}

// Write writes data to the transcript file and updates the offset
func (tw *TranscriptWriter) Write(data []byte) (int, error) {
	tw.mu.Lock()
	defer tw.mu.Unlock()

	n, err := tw.file.Write(data)
	if err != nil {
		return n, err
	}

	// Update offset atomically
	atomic.AddInt64(&tw.offset, int64(n))

	return n, nil
}

// Offset returns the current byte offset in the transcript file
func (tw *TranscriptWriter) Offset() int64 {
	return atomic.LoadInt64(&tw.offset)
}

// Path returns the full path to the transcript file
func (tw *TranscriptWriter) Path() string {
	return tw.path
}

// Close syncs and closes the transcript file
func (tw *TranscriptWriter) Close() error {
	tw.mu.Lock()
	defer tw.mu.Unlock()

	if err := tw.file.Sync(); err != nil {
		tw.file.Close()
		return fmt.Errorf("failed to sync transcript file: %w", err)
	}

	if err := tw.file.Close(); err != nil {
		return fmt.Errorf("failed to close transcript file: %w", err)
	}

	return nil
}
