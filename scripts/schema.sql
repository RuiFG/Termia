-- Termia SQLite Schema
-- Version: 1.0.0 (MVP / P0)
--
-- SQLite with WAL mode for concurrent access from wrapper + tui + tai processes

PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;
PRAGMA synchronous = NORMAL;
PRAGMA foreign_keys = ON;

-- ============================================================
-- MVP (P0) Tables — Core recording infrastructure
-- ============================================================

-- Commands: every command executed in Termia
CREATE TABLE IF NOT EXISTS commands (
    id              TEXT PRIMARY KEY,          -- UUIDv7

    -- Timing
    ts_start        INTEGER NOT NULL,          -- Command start (ns)
    ts_end          INTEGER,                   -- Command end (ns)
    duration_ms     INTEGER,                   -- Execution time

    -- Command details
    command         TEXT NOT NULL,             -- Raw command string
    exit_code       INTEGER,                   -- Shell exit status
    cwd             TEXT NOT NULL,             -- Working directory

    -- Output correlation (for transcript files)
    start_offset    INTEGER,                   -- Byte offset in transcript
    end_offset      INTEGER,                   -- Byte offset in transcript
    output_size     INTEGER,                   -- Bytes of output
    transcript_path TEXT,                      -- Path to raw transcript file

    -- Metadata
    tags            TEXT,                      -- JSON array ["docker", "build"]
    favorite        BOOLEAN DEFAULT 0          -- User marked as favorite
);

-- AI analyses: stores tai responses for reference
CREATE TABLE IF NOT EXISTS analyses (
    id              TEXT PRIMARY KEY,
    command_ids     TEXT NOT NULL,             -- JSON array of command IDs

    prompt          TEXT NOT NULL,             -- User's question
    response        TEXT NOT NULL,             -- LLM response
    model           TEXT NOT NULL,             -- "gpt-4o" | "claude-3-7-sonnet"
    tokens_used     INTEGER,                   -- Total tokens

    created_at      INTEGER NOT NULL
);

-- Agent executions: stores tai agent plans + results
CREATE TABLE IF NOT EXISTS agent_executions (
    id              TEXT PRIMARY KEY,

    task            TEXT NOT NULL,             -- User's natural language task
    plan            TEXT NOT NULL,             -- JSON array of planned steps
    status          TEXT NOT NULL,             -- "running" | "completed" | "aborted"
    steps_completed INTEGER DEFAULT 0,
    steps_total     INTEGER NOT NULL,

    model           TEXT NOT NULL,
    created_at      INTEGER NOT NULL,
    completed_at    INTEGER
);

-- Agent event log (team execution tracing)
CREATE TABLE IF NOT EXISTS agent_events (
    id                TEXT PRIMARY KEY,
    execution_id      TEXT NOT NULL,
    agent_name        TEXT NOT NULL,
    event_type        TEXT NOT NULL,
    payload_json      TEXT,
    model             TEXT,
    tool_name         TEXT,
    tokens_total      INTEGER,
    tokens_prompt     INTEGER,
    tokens_completion INTEGER,
    latency_ms        INTEGER,
    ts                INTEGER NOT NULL,
    trace_id          TEXT,
    parent_trace_id   TEXT
);

-- Agent sessions (TUI conversations)
CREATE TABLE IF NOT EXISTS agent_sessions (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL
);

-- Agent messages (TUI conversation messages)
CREATE TABLE IF NOT EXISTS agent_messages (
    id              TEXT PRIMARY KEY,
    session_id      TEXT NOT NULL,
    role            TEXT NOT NULL,
    content         TEXT NOT NULL,
    created_at      INTEGER NOT NULL,

    FOREIGN KEY (session_id) REFERENCES agent_sessions(id) ON DELETE CASCADE
);

-- ============================================================
-- P0 Indexes — Performance critical for TUI and tai queries
-- ============================================================

CREATE INDEX IF NOT EXISTS idx_commands_ts_end    ON commands(ts_end DESC);
CREATE INDEX IF NOT EXISTS idx_commands_exit      ON commands(exit_code, ts_end DESC);
CREATE INDEX IF NOT EXISTS idx_commands_cwd       ON commands(cwd, ts_end DESC);
CREATE INDEX IF NOT EXISTS idx_analyses_created   ON analyses(created_at DESC);

-- ============================================================
-- P1 Tables — Added post-MVP
-- ============================================================

-- Command outputs (for non-PTY commands, e.g., from agent)
CREATE TABLE IF NOT EXISTS command_outputs (
    id              TEXT PRIMARY KEY,
    command_id      TEXT NOT NULL,

    stream          TEXT CHECK(stream IN ('stdout', 'stderr', 'combined')),
    content         TEXT,                     -- Small outputs (< 1MB)
    content_path    TEXT,                     -- Large outputs → file path
    content_size    INTEGER,                  -- Bytes
    content_sha256  TEXT,                     -- Integrity check

    created_at      INTEGER NOT NULL,

    FOREIGN KEY (command_id) REFERENCES commands(id) ON DELETE CASCADE
);

-- Config storage (key-value)
CREATE TABLE IF NOT EXISTS config (
    key             TEXT PRIMARY KEY,
    value           TEXT NOT NULL,
    updated_at      INTEGER NOT NULL
);

-- Sync metadata (for cloud sync)
CREATE TABLE IF NOT EXISTS sync_state (
    last_sync_at      INTEGER,
    last_sync_cursor  TEXT,                   -- Server cursor for incremental sync
    encrypted_api_keys BLOB,                  -- Encrypted LLM API keys
    agent_memory      BLOB                    -- Encrypted agent context
);

-- Full-text search
CREATE VIRTUAL TABLE IF NOT EXISTS commands_fts USING fts5(
    command, cwd,
    content='commands',
    content_rowid='rowid'
);

-- Triggers to keep FTS in sync
CREATE TRIGGER IF NOT EXISTS commands_ai AFTER INSERT ON commands BEGIN
    INSERT INTO commands_fts(rowid, command, cwd)
    VALUES (new.rowid, new.command, new.cwd);
END;

CREATE TRIGGER IF NOT EXISTS commands_ad AFTER DELETE ON commands BEGIN
    DELETE FROM commands_fts WHERE rowid = old.rowid;
END;
