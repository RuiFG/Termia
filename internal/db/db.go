package db

import (
	"database/sql"
	"fmt"

	"github.com/termia/termia/embedded"
	"github.com/termia/termia/internal/diagnostics"
	"go.uber.org/zap"
	_ "modernc.org/sqlite"
)

// DB wraps the SQLite database connection with a logger.
type DB struct {
	conn   *sql.DB
	logger *zap.Logger
}

// Open opens a SQLite database at the given path with optimized settings for Termia.
// It automatically runs migrations after opening the connection.
func Open(dbPath string, logger *zap.Logger) (*DB, error) {
	// Build DSN with SQLite optimizations
	dsn := fmt.Sprintf("%s?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL&_foreign_keys=ON", dbPath)

	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test connection
	if err := func() error {
		defer diagnostics.Track("startup.db.ping", nil)()
		return conn.Ping()
	}(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	db := &DB{
		conn:   conn,
		logger: logger,
	}

	// Run migrations
	if err := func() error {
		defer diagnostics.Track("startup.db.migrate", nil)()
		return db.Migrate()
	}(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	logger.Info("database opened successfully", zap.String("path", dbPath))
	return db, nil
}

// Close closes the database connection.
func (d *DB) Close() error {
	if d.conn != nil {
		d.logger.Info("closing database connection")
		return d.conn.Close()
	}
	return nil
}

// Conn exposes the underlying sql.DB connection.
func (d *DB) Conn() *sql.DB {
	return d.conn
}

// schemaVersion is bumped whenever schema.sql changes.
// Migrate() skips execution if PRAGMA user_version already matches.
const schemaVersion = 7

// Migrate executes the embedded schema SQL to create or update database tables.
// Uses PRAGMA user_version to skip re-execution on subsequent opens, which
// avoids the expensive FTS5 virtual table + trigger re-parse every launch.
func (d *DB) Migrate() error {
	var currentVersion int
	if err := d.conn.QueryRow("PRAGMA user_version").Scan(&currentVersion); err == nil {
		if currentVersion >= schemaVersion {
			d.logger.Debug("schema up to date", zap.Int("version", currentVersion))
			return nil
		}
	}

	d.logger.Debug("running database migrations", zap.Int("from", currentVersion), zap.Int("to", schemaVersion))

	// Execute the embedded schema (CREATE IF NOT EXISTS is idempotent)
	if _, err := d.conn.Exec(embedded.SchemaSQL); err != nil {
		return fmt.Errorf("failed to execute schema: %w", err)
	}
	if currentVersion < 6 {
		if err := d.ensureAgentSessionsCwd(); err != nil {
			return err
		}
	}
	if currentVersion < 7 {
		if err := d.ensurePendingPromptsColumns(); err != nil {
			return err
		}
	}

	// Stamp version so future opens skip migration
	if _, err := d.conn.Exec(fmt.Sprintf("PRAGMA user_version = %d", schemaVersion)); err != nil {
		return fmt.Errorf("failed to set schema version: %w", err)
	}

	d.logger.Debug("database migrations completed", zap.Int("version", schemaVersion))
	return nil
}

func (d *DB) ensureAgentSessionsCwd() error {
	rows, err := d.conn.Query("PRAGMA table_info(agent_sessions)")
	if err != nil {
		return fmt.Errorf("failed to inspect agent_sessions: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid     int
			name    string
			colType string
			notNull int
			dflt    sql.NullString
			pk      int
		)
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			return fmt.Errorf("failed to scan agent_sessions columns: %w", err)
		}
		if name == "cwd" {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to read agent_sessions columns: %w", err)
	}
	if _, err := d.conn.Exec("ALTER TABLE agent_sessions ADD COLUMN cwd TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("failed to add agent_sessions cwd: %w", err)
	}
	return nil
}

func (d *DB) ensurePendingPromptsColumns() error {
	rows, err := d.conn.Query("PRAGMA table_info(pending_prompts)")
	if err != nil {
		return fmt.Errorf("failed to inspect pending_prompts: %w", err)
	}
	defer rows.Close()

	existing := map[string]bool{}
	for rows.Next() {
		var (
			cid     int
			name    string
			colType string
			notNull int
			dflt    sql.NullString
			pk      int
		)
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			return fmt.Errorf("failed to scan pending_prompts columns: %w", err)
		}
		existing[name] = true
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to read pending_prompts columns: %w", err)
	}

	if !existing["prompt_type"] {
		if _, err := d.conn.Exec("ALTER TABLE pending_prompts ADD COLUMN prompt_type TEXT NOT NULL DEFAULT 'command'"); err != nil {
			return fmt.Errorf("failed to add pending_prompts prompt_type: %w", err)
		}
	}
	if !existing["payload_json"] {
		if _, err := d.conn.Exec("ALTER TABLE pending_prompts ADD COLUMN payload_json TEXT"); err != nil {
			return fmt.Errorf("failed to add pending_prompts payload_json: %w", err)
		}
	}
	if !existing["response_json"] {
		if _, err := d.conn.Exec("ALTER TABLE pending_prompts ADD COLUMN response_json TEXT"); err != nil {
			return fmt.Errorf("failed to add pending_prompts response_json: %w", err)
		}
	}
	if !existing["resolved_at"] {
		if _, err := d.conn.Exec("ALTER TABLE pending_prompts ADD COLUMN resolved_at INTEGER"); err != nil {
			return fmt.Errorf("failed to add pending_prompts resolved_at: %w", err)
		}
	}
	return nil
}
