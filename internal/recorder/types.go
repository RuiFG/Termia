package recorder

import (
	"encoding/json"
	"fmt"
)

const (
	PhaseStart = "start"
	PhaseEnd   = "end"
)

// Marker represents a command marker from shell hooks via FD3
type Marker struct {
	CmdID    string  `json:"cmd_id"`
	Phase    string  `json:"phase"`
	Command  string  `json:"command,omitempty"`
	Cwd      string  `json:"cwd,omitempty"`
	ExitCode *int    `json:"exit_code,omitempty"`
	Ts       float64 `json:"ts"`
}

// ParseMarker parses a JSON marker from shell hooks
func ParseMarker(data []byte) (*Marker, error) {
	var m Marker
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}

	if m.CmdID == "" {
		return nil, fmt.Errorf("cmd_id is required")
	}

	if m.Phase != PhaseStart && m.Phase != PhaseEnd {
		return nil, fmt.Errorf("phase must be 'start' or 'end', got %q", m.Phase)
	}

	return &m, nil
}

// IsStart returns true if the phase is "start"
func (m *Marker) IsStart() bool {
	return m.Phase == PhaseStart
}

// IsEnd returns true if the phase is "end"
func (m *Marker) IsEnd() bool {
	return m.Phase == PhaseEnd
}

// TimestampNano converts the epoch timestamp to nanoseconds
func (m *Marker) TimestampNano() int64 {
	return int64(m.Ts * 1e9)
}
