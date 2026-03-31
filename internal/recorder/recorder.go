package recorder

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/termia/termia/internal/db"
	"go.uber.org/zap"
)

// activeCmd tracks an in-flight command
type activeCmd struct {
	dbID        string // UUID for DB record
	startOffset int64
	ignored     bool
}

// Recorder is an async recording engine that handles command markers and transcript writing
type Recorder struct {
	db           *db.DB
	transcript   *TranscriptWriter
	logger       *zap.Logger
	commands     map[string]*activeCmd // tracks in-flight commands by CmdID
	mu           sync.Mutex
	markerCh     chan *Marker
	markerWG     sync.WaitGroup
	markerMu     sync.Mutex
	markerClosed bool
}

// New creates a new Recorder instance
func New(database *db.DB, transcriptDir string, logger *zap.Logger) (*Recorder, error) {
	// Create TranscriptWriter
	tw, err := NewTranscriptWriter(transcriptDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create transcript writer: %w", err)
	}

	r := &Recorder{
		db:         database,
		transcript: tw,
		logger:     logger,
		commands:   make(map[string]*activeCmd),
		markerCh:   make(chan *Marker, 100),
	}

	r.markerWG.Add(1)
	go r.markerLoop()

	return r, nil
}

// HandleMarker processes command markers from shell hooks
func (r *Recorder) HandleMarker(m *Marker) error {
	r.markerMu.Lock()
	if r.markerClosed {
		r.markerMu.Unlock()
		if r.logger != nil {
			r.logger.Warn("marker received after recorder closed")
		}
		return nil
	}
	if m == nil {
		r.markerMu.Unlock()
		return fmt.Errorf("marker is nil")
	}
	r.markerCh <- m
	r.markerMu.Unlock()
	return nil
}

func (r *Recorder) markerLoop() {
	defer r.markerWG.Done()
	for m := range r.markerCh {
		if m == nil {
			if r.logger != nil {
				r.logger.Warn("nil marker received")
			}
			continue
		}
		if err := r.processMarker(m); err != nil {
			if r.logger != nil {
				r.logger.Error("failed to process marker",
					zap.Error(err),
					zap.String("cmd_id", m.CmdID),
					zap.String("phase", m.Phase))
			}
		}
	}
}

func (r *Recorder) processMarker(m *Marker) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if m.IsStart() {
		// Get current offset
		offset := r.transcript.Offset()
		shouldRecord := shouldRecordCommand(m.Command)
		dbID := ""
		if shouldRecord {
			dbID = uuid.New().String()
		}

		// Create command record in DB
		if shouldRecord && r.db != nil {
			commandText := m.Command
			if commandText == "" {
				commandText = "(unknown)"
			}
			timestamp := m.TimestampNano()
			if timestamp == 0 {
				timestamp = time.Now().UnixNano()
			}
			cmd := &db.Command{
				ID:          dbID,
				TsStart:     timestamp,
				Command:     commandText,
				Cwd:         m.Cwd,
				StartOffset: &offset,
			}
			if err := r.db.CreateCommand(cmd); err != nil {
				return fmt.Errorf("failed to create command in DB: %w", err)
			}
		}

		// Track in-flight command
		r.commands[m.CmdID] = &activeCmd{
			dbID:        dbID,
			startOffset: offset,
			ignored:     !shouldRecord,
		}

		if !shouldRecord {
			r.logger.Debug("command ignored",
				zap.String("cmd_id", m.CmdID),
				zap.String("command", m.Command))
			return nil
		}

		r.logger.Debug("command started",
			zap.String("cmd_id", m.CmdID),
			zap.String("db_id", dbID),
			zap.String("command", m.Command),
			zap.Int64("offset", offset))

		return nil
	}

	if m.IsEnd() {
		// Look up active command
		cmd, exists := r.commands[m.CmdID]
		if !exists {
			r.logger.Warn("end marker for unknown command",
				zap.String("cmd_id", m.CmdID))
			return nil // Not fatal, just log it
		}
		if cmd.ignored {
			delete(r.commands, m.CmdID)
			r.logger.Debug("ignored command ended",
				zap.String("cmd_id", m.CmdID))
			return nil
		}

		// Get current offset
		endOffset := r.transcript.Offset()

		// Extract exit code
		exitCode := 0
		if m.ExitCode != nil {
			exitCode = *m.ExitCode
		}

		// Calculate output size
		outputSize := endOffset - cmd.startOffset

		// Update command in DB
		if r.db != nil {
			transcriptPath := r.transcript.Path()
			if err := r.db.UpdateCommandEnd(cmd.dbID, m.TimestampNano(), exitCode, endOffset, outputSize, &transcriptPath); err != nil {
				return fmt.Errorf("failed to update command in DB: %w", err)
			}
		}

		// Remove from active commands
		delete(r.commands, m.CmdID)

		r.logger.Debug("command ended",
			zap.String("cmd_id", m.CmdID),
			zap.String("db_id", cmd.dbID),
			zap.Int("exit_code", exitCode),
			zap.Int64("start_offset", cmd.startOffset),
			zap.Int64("end_offset", endOffset))

		return nil
	}

	return fmt.Errorf("unknown marker phase: %s", m.Phase)
}

func shouldRecordCommand(command string) bool {
	fields := strings.Fields(strings.TrimSpace(strings.ToLower(command)))
	if len(fields) == 0 {
		return true
	}

	first := normalizeCommandToken(fields[0])
	if first == "tui" || first == "tui.exe" {
		return false
	}
	if len(fields) < 2 {
		return true
	}

	second := normalizeCommandToken(fields[1])
	return !(isTermiaExecutable(first) && second == "tui")
}

func normalizeCommandToken(token string) string {
	token = strings.Trim(token, "\"'")
	token = filepath.Base(token)
	return strings.ToLower(token)
}

func isTermiaExecutable(token string) bool {
	return token == "termia" || token == "termia.exe"
}

// RecordBytes writes data to the transcript file and returns the current byte offset
func (r *Recorder) RecordBytes(data []byte) (int, error) {
	return r.transcript.Write(data)
}

// CurrentOffset returns the current transcript byte offset
func (r *Recorder) CurrentOffset() int64 {
	return r.transcript.Offset()
}

// TranscriptPath returns the path to the transcript file.
func (r *Recorder) TranscriptPath() string {
	return r.transcript.Path()
}

// Close closes the recorder and transcript writer
func (r *Recorder) Close() error {
	r.markerMu.Lock()
	if !r.markerClosed {
		if len(r.markerCh) > 0 {
			r.logger.Warn("closing recorder with pending markers",
				zap.Int("count", len(r.markerCh)))
		}
		r.markerClosed = true
		close(r.markerCh)
	}
	r.markerMu.Unlock()

	r.markerWG.Wait()

	r.mu.Lock()
	defer r.mu.Unlock()

	// Log any unclosed commands
	if len(r.commands) > 0 {
		r.logger.Warn("closing recorder with unclosed commands",
			zap.Int("count", len(r.commands)))
		for cmdID, cmd := range r.commands {
			r.logger.Debug("unclosed command",
				zap.String("cmd_id", cmdID),
				zap.String("db_id", cmd.dbID))
		}
	}

	return r.transcript.Close()
}
