package db

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type PendingPrompt struct {
	PromptID  string
	SessionID string
	Content   string
	CreatedAt int64
	Status    string
}

const (
	PendingPromptStatusPending  = "pending"
	PendingPromptStatusResolved = "resolved"
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

	query := `
        INSERT INTO pending_prompts (prompt_id, session_id, content, created_at, status)
        VALUES (?, ?, ?, ?, ?)
    `
	_, err := d.conn.Exec(query, promptID, sessionID, content, prompt.CreatedAt, status)
	if err != nil {
		return fmt.Errorf("failed to create pending prompt: %w", err)
	}
	prompt.Status = status
	return nil
}

func (d *DB) ListPendingPrompts(sessionID string, limit int) ([]PendingPrompt, error) {
	if d == nil {
		return nil, fmt.Errorf("database is nil")
	}
	query := `
        SELECT prompt_id, session_id, content, created_at, status
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
		if err := rows.Scan(&prompt.PromptID, &prompt.SessionID, &prompt.Content, &prompt.CreatedAt, &prompt.Status); err != nil {
			return nil, fmt.Errorf("failed to scan pending prompt: %w", err)
		}
		prompts = append(prompts, prompt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate pending prompts: %w", err)
	}
	return prompts, nil
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
	result, err := d.conn.Exec("UPDATE pending_prompts SET status = ? WHERE prompt_id = ?", PendingPromptStatusResolved, promptID)
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
