package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type PendingPrompt struct {
	PromptID     string
	SessionID    string
	Content      string
	PromptType   string
	PayloadJSON  string
	ResponseJSON string
	CreatedAt    int64
	Status       string
	ResolvedAt   *int64
}

const (
	PendingPromptStatusPending  = "pending"
	PendingPromptStatusResolved = "resolved"

	PendingPromptTypeCommand = "command"
	PendingPromptTypeAsk     = "ask"
)

func (d *DB) CreatePendingPrompt(prompt *PendingPrompt) error {
	if d == nil {
		return fmt.Errorf("database is nil")
	}
	if prompt == nil {
		return fmt.Errorf("pending prompt is nil")
	}
	promptID := strings.TrimSpace(prompt.PromptID)
	if promptID == "" {
		return fmt.Errorf("prompt id is empty")
	}
	sessionID := strings.TrimSpace(prompt.SessionID)
	if sessionID == "" {
		return fmt.Errorf("session id is empty")
	}
	content := strings.TrimSpace(prompt.Content)
	if content == "" {
		return fmt.Errorf("content is empty")
	}
	status := strings.TrimSpace(prompt.Status)
	if status == "" {
		status = PendingPromptStatusPending
	}
	if status != PendingPromptStatusPending && status != PendingPromptStatusResolved {
		return fmt.Errorf("invalid pending prompt status: %s", status)
	}

	promptType := strings.TrimSpace(prompt.PromptType)
	if promptType == "" {
		promptType = PendingPromptTypeCommand
	}
	if promptType != PendingPromptTypeCommand && promptType != PendingPromptTypeAsk {
		return fmt.Errorf("invalid pending prompt type: %s", promptType)
	}

	query := `
        INSERT INTO pending_prompts (prompt_id, session_id, content, prompt_type, payload_json, response_json, created_at, status, resolved_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
    `
	_, err := d.conn.Exec(query, promptID, sessionID, content, promptType, prompt.PayloadJSON, prompt.ResponseJSON, prompt.CreatedAt, status, prompt.ResolvedAt)
	if err != nil {
		return fmt.Errorf("failed to create pending prompt: %w", err)
	}
	prompt.Status = status
	prompt.PromptType = promptType
	return nil
}

func (d *DB) ListPendingPrompts(sessionID string, limit int) ([]PendingPrompt, error) {
	if d == nil {
		return nil, fmt.Errorf("database is nil")
	}
	query := `
        SELECT prompt_id, session_id, content, prompt_type, payload_json, response_json, created_at, status, resolved_at
        FROM pending_prompts
        WHERE status = ?
    `
	args := []any{PendingPromptStatusPending}
	if strings.TrimSpace(sessionID) != "" {
		query += " AND session_id = ?"
		args = append(args, sessionID)
	}
	query += " ORDER BY created_at ASC"
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := d.conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list pending prompts: %w", err)
	}
	defer rows.Close()

	var prompts []PendingPrompt
	for rows.Next() {
		var prompt PendingPrompt
		var payload sql.NullString
		var response sql.NullString
		var resolved sql.NullInt64
		if err := rows.Scan(
			&prompt.PromptID,
			&prompt.SessionID,
			&prompt.Content,
			&prompt.PromptType,
			&payload,
			&response,
			&prompt.CreatedAt,
			&prompt.Status,
			&resolved,
		); err != nil {
			return nil, fmt.Errorf("failed to scan pending prompt: %w", err)
		}
		if payload.Valid {
			prompt.PayloadJSON = payload.String
		}
		if response.Valid {
			prompt.ResponseJSON = response.String
		}
		if resolved.Valid {
			value := resolved.Int64
			prompt.ResolvedAt = &value
		}
		prompts = append(prompts, prompt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate pending prompts: %w", err)
	}
	return prompts, nil
}

func (d *DB) GetPendingPrompt(promptID string) (PendingPrompt, error) {
	if d == nil {
		return PendingPrompt{}, fmt.Errorf("database is nil")
	}
	promptID = strings.TrimSpace(promptID)
	if promptID == "" {
		return PendingPrompt{}, fmt.Errorf("prompt id is empty")
	}
	query := `
        SELECT prompt_id, session_id, content, prompt_type, payload_json, response_json, created_at, status, resolved_at
        FROM pending_prompts
        WHERE prompt_id = ?
    `
	var prompt PendingPrompt
	var payload sql.NullString
	var response sql.NullString
	var resolved sql.NullInt64
	if err := d.conn.QueryRow(query, promptID).Scan(
		&prompt.PromptID,
		&prompt.SessionID,
		&prompt.Content,
		&prompt.PromptType,
		&payload,
		&response,
		&prompt.CreatedAt,
		&prompt.Status,
		&resolved,
	); err != nil {
		return PendingPrompt{}, fmt.Errorf("failed to get pending prompt: %w", err)
	}
	if payload.Valid {
		prompt.PayloadJSON = payload.String
	}
	if response.Valid {
		prompt.ResponseJSON = response.String
	}
	if resolved.Valid {
		value := resolved.Int64
		prompt.ResolvedAt = &value
	}
	return prompt, nil
}

func (d *DB) CountPendingPrompts() (int, error) {
	if d == nil {
		return 0, fmt.Errorf("database is nil")
	}
	var count int
	if err := d.conn.QueryRow("SELECT COUNT(*) FROM pending_prompts WHERE status = ?", PendingPromptStatusPending).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count pending prompts: %w", err)
	}
	return count, nil
}

func (d *DB) CountPendingPromptsForSession(sessionID string) (int, error) {
	if d == nil {
		return 0, fmt.Errorf("database is nil")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return d.CountPendingPrompts()
	}
	var count int
	if err := d.conn.QueryRow(
		"SELECT COUNT(*) FROM pending_prompts WHERE status = ? AND session_id = ?",
		PendingPromptStatusPending,
		sessionID,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count pending prompts for session: %w", err)
	}
	return count, nil
}

func (d *DB) ResolvePendingPrompt(promptID string) error {
	if d == nil {
		return fmt.Errorf("database is nil")
	}
	promptID = strings.TrimSpace(promptID)
	if promptID == "" {
		return fmt.Errorf("prompt id is empty")
	}
	resolvedAt := time.Now().UnixNano()
	result, err := d.conn.Exec(
		"UPDATE pending_prompts SET status = ?, resolved_at = ? WHERE prompt_id = ?",
		PendingPromptStatusResolved,
		resolvedAt,
		promptID,
	)
	if err != nil {
		return fmt.Errorf("failed to resolve pending prompt: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("pending prompt not found: %s", promptID)
	}
	return nil
}

func (d *DB) ResolvePendingPromptWithResponse(promptID string, responseJSON string) error {
	if d == nil {
		return fmt.Errorf("database is nil")
	}
	promptID = strings.TrimSpace(promptID)
	if promptID == "" {
		return fmt.Errorf("prompt id is empty")
	}
	resolvedAt := time.Now().UnixNano()
	result, err := d.conn.Exec(
		"UPDATE pending_prompts SET status = ?, response_json = ?, resolved_at = ? WHERE prompt_id = ?",
		PendingPromptStatusResolved,
		strings.TrimSpace(responseJSON),
		resolvedAt,
		promptID,
	)
	if err != nil {
		return fmt.Errorf("failed to resolve pending prompt: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("pending prompt not found: %s", promptID)
	}
	return nil
}

func (d *DB) WritePendingPromptsCount(path string) error {
	if d == nil {
		return fmt.Errorf("database is nil")
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("count path is empty")
	}
	count, err := d.CountPendingPrompts()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create pending prompts count dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(strconv.Itoa(count)), 0644); err != nil {
		return fmt.Errorf("write pending prompts count file: %w", err)
	}
	return nil
}
