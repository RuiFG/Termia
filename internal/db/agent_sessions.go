package db

import (
	"database/sql"
	"fmt"
)

// AgentSession represents a TUI conversation session.
type AgentSession struct {
	ID               string
	Name             string
	Mode             string
	TeamName         string
	SpecSnapshotJSON string
	Cwd              string
	CreatedAt        int64
	UpdatedAt        int64
}

// CreateAgentSession inserts a new agent session.
func (d *DB) CreateAgentSession(session *AgentSession) error {
	if session == nil {
		return fmt.Errorf("session is nil")
	}
	query := `
		INSERT INTO agent_sessions (id, name, mode, team_name, spec_snapshot_json, cwd, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := d.conn.Exec(
		query,
		session.ID,
		session.Name,
		session.Mode,
		session.TeamName,
		session.SpecSnapshotJSON,
		session.Cwd,
		session.CreatedAt,
		session.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create agent session: %w", err)
	}
	return nil
}

// ListAgentSessions returns sessions ordered by updated time desc.
func (d *DB) ListAgentSessions(limit int) ([]AgentSession, error) {
	query := `
		SELECT id, name, mode, team_name, spec_snapshot_json, cwd, created_at, updated_at
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
		if err := rows.Scan(
			&s.ID,
			&s.Name,
			&s.Mode,
			&s.TeamName,
			&s.SpecSnapshotJSON,
			&s.Cwd,
			&s.CreatedAt,
			&s.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan agent session: %w", err)
		}
		sessions = append(sessions, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate agent sessions: %w", err)
	}
	return sessions, nil
}

func (d *DB) GetAgentSession(sessionID string) (AgentSession, bool, error) {
	if sessionID == "" {
		return AgentSession{}, false, fmt.Errorf("session id is empty")
	}
	query := `
		SELECT id, name, mode, team_name, spec_snapshot_json, cwd, created_at, updated_at
		FROM agent_sessions
		WHERE id = ?
	`
	var session AgentSession
	err := d.conn.QueryRow(query, sessionID).Scan(
		&session.ID,
		&session.Name,
		&session.Mode,
		&session.TeamName,
		&session.SpecSnapshotJSON,
		&session.Cwd,
		&session.CreatedAt,
		&session.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return AgentSession{}, false, nil
	}
	if err != nil {
		return AgentSession{}, false, fmt.Errorf("failed to get agent session: %w", err)
	}
	return session, true, nil
}

func (d *DB) LatestAgentSession() (AgentSession, bool, error) {
	query := `
		SELECT id, name, mode, team_name, spec_snapshot_json, cwd, created_at, updated_at
		FROM agent_sessions
		ORDER BY updated_at DESC
		LIMIT 1
	`
	var session AgentSession
	err := d.conn.QueryRow(query).Scan(
		&session.ID,
		&session.Name,
		&session.Mode,
		&session.TeamName,
		&session.SpecSnapshotJSON,
		&session.Cwd,
		&session.CreatedAt,
		&session.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return AgentSession{}, false, nil
	}
	if err != nil {
		return AgentSession{}, false, fmt.Errorf("failed to get latest agent session: %w", err)
	}
	return session, true, nil
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

func (d *DB) UpdateAgentSessionCwd(sessionID, cwd string, updatedAt int64) error {
	if sessionID == "" {
		return fmt.Errorf("session id is empty")
	}
	_, err := d.conn.Exec("UPDATE agent_sessions SET cwd = ?, updated_at = ? WHERE id = ?", cwd, updatedAt, sessionID)
	if err != nil {
		return fmt.Errorf("failed to update session cwd: %w", err)
	}
	return nil
}

func (d *DB) UpdateAgentSessionRuntime(sessionID, mode, teamName, specSnapshotJSON string, updatedAt int64) error {
	if sessionID == "" {
		return fmt.Errorf("session id is empty")
	}
	_, err := d.conn.Exec(
		"UPDATE agent_sessions SET mode = ?, team_name = ?, spec_snapshot_json = ?, updated_at = ? WHERE id = ?",
		mode,
		teamName,
		specSnapshotJSON,
		updatedAt,
		sessionID,
	)
	if err != nil {
		return fmt.Errorf("failed to update session runtime metadata: %w", err)
	}
	return nil
}
