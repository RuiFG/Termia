package db

import (
	"fmt"
)

// AgentSession represents a TUI conversation session.
type AgentSession struct {
	ID        string
	Name      string
	CreatedAt int64
	UpdatedAt int64
}

// CreateAgentSession inserts a new agent session.
func (d *DB) CreateAgentSession(session *AgentSession) error {
	if session == nil {
		return fmt.Errorf("session is nil")
	}
	query := `
		INSERT INTO agent_sessions (id, name, created_at, updated_at)
		VALUES (?, ?, ?, ?)
	`
	_, err := d.conn.Exec(query, session.ID, session.Name, session.CreatedAt, session.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to create agent session: %w", err)
	}
	return nil
}

// ListAgentSessions returns sessions ordered by updated time desc.
func (d *DB) ListAgentSessions(limit int) ([]AgentSession, error) {
	query := `
		SELECT id, name, created_at, updated_at
		FROM agent_sessions
		ORDER BY updated_at DESC
	`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := d.conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to list agent sessions: %w", err)
	}
	defer rows.Close()

	var sessions []AgentSession
	for rows.Next() {
		var s AgentSession
		if err := rows.Scan(&s.ID, &s.Name, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan agent session: %w", err)
		}
		sessions = append(sessions, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate agent sessions: %w", err)
	}
	return sessions, nil
}

// UpdateAgentSessionUpdatedAt updates the session's updated_at.
func (d *DB) UpdateAgentSessionUpdatedAt(sessionID string, updatedAt int64) error {
	if sessionID == "" {
		return fmt.Errorf("session id is empty")
	}
	_, err := d.conn.Exec("UPDATE agent_sessions SET updated_at = ? WHERE id = ?", updatedAt, sessionID)
	if err != nil {
		return fmt.Errorf("failed to update agent session: %w", err)
	}
	return nil
}
