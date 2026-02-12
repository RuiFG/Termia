package db

import (
	"database/sql"
	"fmt"

	"go.uber.org/zap"
)

// Analysis represents an AI analysis of one or more commands.
type Analysis struct {
	ID         string
	CommandIDs string // JSON array of command IDs
	Prompt     string
	Response   string
	Model      string
	TokensUsed *int
	CreatedAt  int64
}

// AgentExecution represents a tai agent plan and execution state.
type AgentExecution struct {
	ID             string
	Task           string
	Plan           string // JSON array of planned steps
	Status         string
	StepsCompleted int
	StepsTotal     int
	Model          string
	CreatedAt      int64
	CompletedAt    *int64
}

// CreateAnalysis inserts a new analysis into the database.
func (d *DB) CreateAnalysis(a *Analysis) error {
	query := `
		INSERT INTO analyses (id, command_ids, prompt, response, model, tokens_used, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	_, err := d.conn.Exec(query,
		a.ID, a.CommandIDs, a.Prompt, a.Response, a.Model, a.TokensUsed, a.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create analysis: %w", err)
	}

	d.logger.Info("analysis created", zap.String("analysis_id", a.ID), zap.String("model", a.Model))
	return nil
}

// ListAnalyses retrieves the most recent analyses, ordered by created_at descending.
func (d *DB) ListAnalyses(limit int) ([]Analysis, error) {
	query := `
		SELECT id, command_ids, prompt, response, model, tokens_used, created_at
		FROM analyses
		ORDER BY created_at DESC
		LIMIT ?
	`

	rows, err := d.conn.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list analyses: %w", err)
	}
	defer rows.Close()

	var analyses []Analysis
	for rows.Next() {
		var a Analysis
		err := rows.Scan(
			&a.ID, &a.CommandIDs, &a.Prompt, &a.Response, &a.Model, &a.TokensUsed, &a.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan analysis: %w", err)
		}
		analyses = append(analyses, a)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating analyses: %w", err)
	}

	return analyses, nil
}

// CreateAgentExecution inserts a new agent execution into the database.
func (d *DB) CreateAgentExecution(e *AgentExecution) error {
	query := `
		INSERT INTO agent_executions (
			id, task, plan, status, steps_completed, steps_total,
			model, created_at, completed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := d.conn.Exec(query,
		e.ID, e.Task, e.Plan, e.Status, e.StepsCompleted, e.StepsTotal,
		e.Model, e.CreatedAt, e.CompletedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create agent execution: %w", err)
	}

	d.logger.Info("agent execution created", zap.String("execution_id", e.ID), zap.String("task", e.Task))
	return nil
}

// UpdateAgentExecution updates the status, steps completed, and completion time for an agent execution.
func (d *DB) UpdateAgentExecution(id string, status string, stepsCompleted int, completedAt *int64) error {
	query := `
		UPDATE agent_executions
		SET status = ?, steps_completed = ?, completed_at = ?
		WHERE id = ?
	`

	result, err := d.conn.Exec(query, status, stepsCompleted, completedAt, id)
	if err != nil {
		return fmt.Errorf("failed to update agent execution: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("agent execution not found: %s", id)
	}

	d.logger.Info("agent execution updated", zap.String("execution_id", id), zap.String("status", status))
	return nil
}

// GetAgentExecution retrieves an agent execution by ID.
func (d *DB) GetAgentExecution(id string) (*AgentExecution, error) {
	query := `
		SELECT id, task, plan, status, steps_completed, steps_total,
		       model, created_at, completed_at
		FROM agent_executions
		WHERE id = ?
	`

	var e AgentExecution
	err := d.conn.QueryRow(query, id).Scan(
		&e.ID, &e.Task, &e.Plan, &e.Status, &e.StepsCompleted, &e.StepsTotal,
		&e.Model, &e.CreatedAt, &e.CompletedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("agent execution not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get agent execution: %w", err)
	}

	return &e, nil
}
