package db

import (
	"database/sql"
	"fmt"

	"go.uber.org/zap"
)

// Command represents a command executed by Termia.
type Command struct {
	ID             string
	TsStart        int64
	TsEnd          *int64
	DurationMs     *int64
	Command        string
	ExitCode       *int
	Cwd            string
	StartOffset    *int64
	EndOffset      *int64
	OutputSize     *int64
	TranscriptPath *string
	Tags           *string
	Favorite       bool
}

// CreateCommand inserts a new command into the database.
func (d *DB) CreateCommand(c *Command) error {
	query := `
		INSERT INTO commands (
			id, ts_start, ts_end, duration_ms, command, exit_code,
			cwd, start_offset, end_offset, output_size, transcript_path, tags, favorite
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := d.conn.Exec(query,
		c.ID, c.TsStart, c.TsEnd, c.DurationMs, c.Command, c.ExitCode,
		c.Cwd, c.StartOffset, c.EndOffset, c.OutputSize, c.TranscriptPath, c.Tags, c.Favorite,
	)
	if err != nil {
		return fmt.Errorf("failed to create command: %w", err)
	}

	d.logger.Debug("command created", zap.String("command_id", c.ID))

	return nil
}

// UpdateCommandEnd updates the end timestamp, exit code, and output metadata for a command.
func (d *DB) UpdateCommandEnd(id string, tsEnd int64, exitCode int, endOffset int64, outputSize int64, transcriptPath *string) error {
	// Calculate duration
	var tsStart int64
	err := d.conn.QueryRow("SELECT ts_start FROM commands WHERE id = ?", id).Scan(&tsStart)
	if err != nil {
		return fmt.Errorf("failed to get command start time: %w", err)
	}

	durationMs := (tsEnd - tsStart) / 1_000_000 // nanoseconds to milliseconds

	query := `
		UPDATE commands
		SET ts_end = ?, duration_ms = ?, exit_code = ?, end_offset = ?, output_size = ?, transcript_path = ?
		WHERE id = ?
	`

	result, err := d.conn.Exec(query, tsEnd, durationMs, exitCode, endOffset, outputSize, transcriptPath, id)
	if err != nil {
		return fmt.Errorf("failed to update command end: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("command not found: %s", id)
	}

	d.logger.Debug("command ended", zap.String("command_id", id), zap.Int("exit_code", exitCode))
	return nil
}

// GetCommand retrieves a command by ID.
func (d *DB) GetCommand(id string) (*Command, error) {
	query := `
		SELECT id, ts_start, ts_end, duration_ms, command, exit_code,
		       cwd, start_offset, end_offset, output_size, transcript_path, tags, favorite
		FROM commands
		WHERE id = ?
	`

	var c Command
	err := d.conn.QueryRow(query, id).Scan(
		&c.ID, &c.TsStart, &c.TsEnd, &c.DurationMs, &c.Command, &c.ExitCode,
		&c.Cwd, &c.StartOffset, &c.EndOffset, &c.OutputSize, &c.TranscriptPath, &c.Tags, &c.Favorite,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("command not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get command: %w", err)
	}

	return &c, nil
}

// ListCommands retrieves commands ordered by ts_start descending.
func (d *DB) ListCommands(limit int) ([]Command, error) {
	query := `
		SELECT id, ts_start, ts_end, duration_ms, command, exit_code,
		       cwd, start_offset, end_offset, output_size, transcript_path, tags, favorite
		FROM commands
		ORDER BY ts_start DESC
		LIMIT ?
	`

	rows, err := d.conn.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list commands: %w", err)
	}
	defer rows.Close()

	return d.scanCommands(rows)
}

// ListRecentCommands retrieves recent commands ordered by ts_end descending.
func (d *DB) ListRecentCommands(limit int) ([]Command, error) {
	query := `
		SELECT id, ts_start, ts_end, duration_ms, command, exit_code,
		       cwd, start_offset, end_offset, output_size, transcript_path, tags, favorite
		FROM commands
		WHERE ts_end IS NOT NULL
		ORDER BY ts_end DESC
		LIMIT ?
	`

	rows, err := d.conn.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list recent commands: %w", err)
	}
	defer rows.Close()

	return d.scanCommands(rows)
}

// ListRecentCommandsSince retrieves completed commands since the lower bound ordered by ts_end descending.
func (d *DB) ListRecentCommandsSince(since int64) ([]Command, error) {
	query := `
		SELECT id, ts_start, ts_end, duration_ms, command, exit_code,
		       cwd, start_offset, end_offset, output_size, transcript_path, tags, favorite
		FROM commands
		WHERE ts_end IS NOT NULL AND ts_end >= ?
		ORDER BY ts_end DESC
	`

	rows, err := d.conn.Query(query, since)
	if err != nil {
		return nil, fmt.Errorf("failed to list recent commands since %d: %w", since, err)
	}
	defer rows.Close()

	return d.scanCommands(rows)
}

// SearchCommands searches for commands matching the query string in the command text.
func (d *DB) SearchCommands(query string, limit int) ([]Command, error) {
	sqlQuery := `
		SELECT id, ts_start, ts_end, duration_ms, command, exit_code,
		       cwd, start_offset, end_offset, output_size, transcript_path, tags, favorite
		FROM commands
		WHERE command LIKE ?
		ORDER BY ts_end DESC
		LIMIT ?
	`

	searchPattern := "%" + query + "%"
	rows, err := d.conn.Query(sqlQuery, searchPattern, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search commands: %w", err)
	}
	defer rows.Close()

	return d.scanCommands(rows)
}

// DeleteCommand deletes a command by ID.
func (d *DB) DeleteCommand(id string) error {
	query := `DELETE FROM commands WHERE id = ?`

	result, err := d.conn.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete command: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("command not found: %s", id)
	}

	d.logger.Debug("command deleted", zap.String("command_id", id))
	return nil
}

// ToggleFavorite toggles the favorite status of a command.
func (d *DB) ToggleFavorite(id string) error {
	query := `UPDATE commands SET favorite = NOT favorite WHERE id = ?`

	result, err := d.conn.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to toggle favorite: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("command not found: %s", id)
	}

	d.logger.Debug("command favorite toggled", zap.String("command_id", id))
	return nil
}

// scanCommands is a helper function to scan multiple command rows.
func (d *DB) scanCommands(rows *sql.Rows) ([]Command, error) {
	var commands []Command
	for rows.Next() {
		var c Command
		err := rows.Scan(
			&c.ID, &c.TsStart, &c.TsEnd, &c.DurationMs, &c.Command, &c.ExitCode,
			&c.Cwd, &c.StartOffset, &c.EndOffset, &c.OutputSize, &c.TranscriptPath, &c.Tags, &c.Favorite,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan command: %w", err)
		}
		commands = append(commands, c)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating commands: %w", err)
	}

	return commands, nil
}
