package db

import (
	"fmt"

	"go.uber.org/zap"
)

// AgentEvent represents a trace event for agent team execution.
type AgentEvent struct {
	ID               string
	ExecutionID      string
	AgentName        string
	EventType        string
	PayloadJSON      string
	Model            string
	ToolName         *string
	TokensTotal      *int
	TokensPrompt     *int
	TokensCompletion *int
	LatencyMs        *int64
	Ts               int64
	TraceID          *string
	ParentTraceID    *string
}

// CreateAgentEvent inserts a new agent event into the database.
func (d *DB) CreateAgentEvent(e *AgentEvent) error {
	query := `
		INSERT INTO agent_events (
			id, execution_id, agent_name, event_type, payload_json,
			model, tool_name, tokens_total, tokens_prompt, tokens_completion,
			latency_ms, ts, trace_id, parent_trace_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := d.conn.Exec(query,
		e.ID, e.ExecutionID, e.AgentName, e.EventType, e.PayloadJSON,
		e.Model, e.ToolName, e.TokensTotal, e.TokensPrompt, e.TokensCompletion,
		e.LatencyMs, e.Ts, e.TraceID, e.ParentTraceID,
	)
	if err != nil {
		return fmt.Errorf("failed to create agent event: %w", err)
	}

	d.logger.Debug("agent event created", zap.String("execution_id", e.ExecutionID), zap.String("event_type", e.EventType))
	return nil
}
