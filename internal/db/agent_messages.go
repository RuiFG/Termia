package db

import "fmt"

// AgentMessage represents a single message in a session.
type AgentMessage struct {
	ID        string
	SessionID string
	Role      string
	Content   string
	CreatedAt int64
}

// CreateAgentMessage inserts a new agent message and bumps session updated_at.
func (d *DB) CreateAgentMessage(msg *AgentMessage) error {
	if msg == nil {
		return fmt.Errorf("message is nil")
	}
	query := `
		INSERT INTO agent_messages (id, session_id, role, content, created_at)
		VALUES (?, ?, ?, ?, ?)
	`
	_, err := d.conn.Exec(query, msg.ID, msg.SessionID, msg.Role, msg.Content, msg.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to create agent message: %w", err)
	}
	if err := d.UpdateAgentSessionUpdatedAt(msg.SessionID, msg.CreatedAt); err != nil {
		return err
	}
	return nil
}

// ListAgentMessages returns messages for a session ordered by created time.
func (d *DB) ListAgentMessages(sessionID string) ([]AgentMessage, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session id is empty")
	}
	query := `
		SELECT id, session_id, role, content, created_at
		FROM agent_messages
		WHERE session_id = ?
		ORDER BY created_at ASC
	`
	rows, err := d.conn.Query(query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to list agent messages: %w", err)
	}
	defer rows.Close()

	var messages []AgentMessage
	for rows.Next() {
		var m AgentMessage
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan agent message: %w", err)
		}
		messages = append(messages, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate agent messages: %w", err)
	}
	return messages, nil
}
