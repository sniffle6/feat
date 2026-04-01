package store

import "database/sql"

const schemaV1 = `
CREATE TABLE IF NOT EXISTS features (
	id TEXT PRIMARY KEY,
	title TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'planned',
	left_off TEXT NOT NULL DEFAULT '',
	key_files TEXT NOT NULL DEFAULT '[]',
	worktree_path TEXT NOT NULL DEFAULT '',
	created_at DATETIME NOT NULL DEFAULT (datetime('now')),
	updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS sessions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	feature_id TEXT REFERENCES features(id),
	summary TEXT NOT NULL DEFAULT '',
	files_touched TEXT NOT NULL DEFAULT '[]',
	commits TEXT NOT NULL DEFAULT '[]',
	auto_linked INTEGER NOT NULL DEFAULT 0,
	link_reason TEXT NOT NULL DEFAULT '',
	created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
`

const schemaV2 = `
ALTER TABLE sessions ADD COLUMN compacted INTEGER NOT NULL DEFAULT 0;
`

const schemaV3 = `
CREATE TABLE IF NOT EXISTS subtasks (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	feature_id TEXT NOT NULL REFERENCES features(id),
	title TEXT NOT NULL,
	position INTEGER NOT NULL DEFAULT 0,
	archived INTEGER NOT NULL DEFAULT 0,
	archived_at DATETIME,
	created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS task_items (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	subtask_id INTEGER NOT NULL REFERENCES subtasks(id),
	title TEXT NOT NULL,
	checked INTEGER NOT NULL DEFAULT 0,
	key_files TEXT NOT NULL DEFAULT '[]',
	outcome TEXT NOT NULL DEFAULT '',
	commit_hash TEXT NOT NULL DEFAULT '',
	position INTEGER NOT NULL DEFAULT 0,
	created_at DATETIME NOT NULL DEFAULT (datetime('now')),
	updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
`

const schemaV4 = `
ALTER TABLE features ADD COLUMN notes TEXT NOT NULL DEFAULT '';
`

const schemaV5 = `
CREATE TABLE IF NOT EXISTS decisions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	feature_id TEXT NOT NULL REFERENCES features(id),
	approach TEXT NOT NULL,
	outcome TEXT NOT NULL CHECK(outcome IN ('accepted', 'rejected')),
	reason TEXT NOT NULL DEFAULT '',
	created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
`

const schemaV6 = `
CREATE TABLE IF NOT EXISTS issues (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    feature_id TEXT NOT NULL REFERENCES features(id),
    task_item_id INTEGER REFERENCES task_items(id),
    description TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'open' CHECK(status IN ('open', 'resolved')),
    resolved_commit TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    resolved_at DATETIME
);
`

const schemaV7 = `
ALTER TABLE features ADD COLUMN type TEXT NOT NULL DEFAULT '';
`

const schemaV8 = `
ALTER TABLE features ADD COLUMN tags TEXT NOT NULL DEFAULT '[]';
`

const schemaV9 = `
CREATE TABLE IF NOT EXISTS work_sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    feature_id TEXT NOT NULL REFERENCES features(id),
    claude_session_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'open' CHECK(status IN ('open', 'closed')),
    started_at DATETIME NOT NULL DEFAULT (datetime('now')),
    ended_at DATETIME,
    handoff_stale INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS checkpoint_jobs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    work_session_id INTEGER NOT NULL REFERENCES work_sessions(id),
    feature_id TEXT NOT NULL,
    reason TEXT NOT NULL CHECK(reason IN ('stop', 'precompact', 'manual_checkpoint', 'manual_end_session')),
    trigger_type TEXT NOT NULL DEFAULT '',
    transcript_start_offset INTEGER NOT NULL DEFAULT 0,
    transcript_end_offset INTEGER NOT NULL DEFAULT 0,
    semantic_text TEXT NOT NULL DEFAULT '',
    mechanical_json TEXT NOT NULL DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'queued' CHECK(status IN ('queued', 'running', 'done', 'failed', 'skipped')),
    error TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    started_at DATETIME,
    finished_at DATETIME
);

CREATE TABLE IF NOT EXISTS checkpoint_observations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    checkpoint_job_id INTEGER NOT NULL REFERENCES checkpoint_jobs(id),
    work_session_id INTEGER NOT NULL REFERENCES work_sessions(id),
    feature_id TEXT NOT NULL,
    kind TEXT NOT NULL CHECK(kind IN ('summary', 'blocker', 'decision_candidate', 'dead_end', 'next_step', 'gotcha')),
    payload_json TEXT NOT NULL DEFAULT '{}',
    summary_text TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
`

const schemaV10 = `
CREATE TABLE IF NOT EXISTS checkpoint_jobs_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    work_session_id INTEGER NOT NULL REFERENCES work_sessions(id),
    feature_id TEXT NOT NULL,
    reason TEXT NOT NULL CHECK(reason IN ('stop', 'precompact', 'manual_checkpoint', 'manual_end_session', 'session_end')),
    trigger_type TEXT NOT NULL DEFAULT '',
    transcript_start_offset INTEGER NOT NULL DEFAULT 0,
    transcript_end_offset INTEGER NOT NULL DEFAULT 0,
    semantic_text TEXT NOT NULL DEFAULT '',
    mechanical_json TEXT NOT NULL DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'queued' CHECK(status IN ('queued', 'running', 'done', 'failed', 'skipped')),
    error TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    started_at DATETIME,
    finished_at DATETIME
);

INSERT OR IGNORE INTO checkpoint_jobs_new SELECT * FROM checkpoint_jobs;
DROP TABLE IF EXISTS checkpoint_jobs;
ALTER TABLE checkpoint_jobs_new RENAME TO checkpoint_jobs;
`

const schemaV11 = `
ALTER TABLE checkpoint_jobs ADD COLUMN retry_count INTEGER NOT NULL DEFAULT 0;
`

const schemaV12 = `
ALTER TABLE work_sessions ADD COLUMN session_state TEXT NOT NULL DEFAULT 'idle';
`

const schemaV13 = `
ALTER TABLE work_sessions ADD COLUMN last_heartbeat DATETIME;
`

func migrate(db *sql.DB) error {
	if _, err := db.Exec(schemaV1); err != nil {
		return err
	}
	// v2: add compacted column (ignore error if already exists)
	db.Exec(schemaV2)
	// v3: add subtasks and task_items tables (ignore error if already exists)
	db.Exec(schemaV3)
	// v4: add notes column (ignore error if already exists)
	db.Exec(schemaV4)
	// v5: add decisions table (ignore error if already exists)
	db.Exec(schemaV5)
	// v6: add issues table (ignore error if already exists)
	db.Exec(schemaV6)
	// v7: add type column to features (ignore error if already exists)
	db.Exec(schemaV7)
	// v8: add tags column to features (ignore error if already exists)
	db.Exec(schemaV8)
	// v9: add work_sessions, checkpoint_jobs, checkpoint_observations tables
	db.Exec(schemaV9)
	// v10: add 'session_end' to checkpoint_jobs reason CHECK constraint
	db.Exec(schemaV10)
	// v11: add retry_count column to checkpoint_jobs
	db.Exec(schemaV11)
	// v12: add session_state column to work_sessions
	db.Exec(schemaV12)
	// v13: add last_heartbeat column, add 'stale' to session_state CHECK constraint
	db.Exec(schemaV13)
	return nil
}
