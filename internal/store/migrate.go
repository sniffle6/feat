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

const schemaV14 = `
ALTER TABLE features ADD COLUMN spec_path TEXT NOT NULL DEFAULT '';
`

const schemaV15 = `
CREATE TABLE IF NOT EXISTS notes (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	feature_id TEXT NOT NULL REFERENCES features(id),
	content TEXT NOT NULL,
	created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
`

const schemaV16 = `
CREATE VIRTUAL TABLE IF NOT EXISTS search_index USING fts5(
	entity_type,
	entity_id,
	feature_id,
	field_name,
	content,
	tokenize='porter unicode61'
);

-- === Features triggers ===
CREATE TRIGGER IF NOT EXISTS search_features_insert AFTER INSERT ON features BEGIN
	INSERT INTO search_index(entity_type, entity_id, feature_id, field_name, content)
	VALUES
		('feature', NEW.id, NEW.id, 'title', NEW.title),
		('feature', NEW.id, NEW.id, 'description', NEW.description),
		('feature', NEW.id, NEW.id, 'left_off', NEW.left_off),
		('feature', NEW.id, NEW.id, 'notes', NEW.notes),
		('feature', NEW.id, NEW.id, 'key_files', NEW.key_files),
		('feature', NEW.id, NEW.id, 'tags', NEW.tags);
END;

CREATE TRIGGER IF NOT EXISTS search_features_delete AFTER DELETE ON features BEGIN
	DELETE FROM search_index WHERE entity_type = 'feature' AND entity_id = OLD.id;
END;

CREATE TRIGGER IF NOT EXISTS search_features_update AFTER UPDATE ON features BEGIN
	DELETE FROM search_index WHERE entity_type = 'feature' AND entity_id = OLD.id;
	INSERT INTO search_index(entity_type, entity_id, feature_id, field_name, content)
	VALUES
		('feature', NEW.id, NEW.id, 'title', NEW.title),
		('feature', NEW.id, NEW.id, 'description', NEW.description),
		('feature', NEW.id, NEW.id, 'left_off', NEW.left_off),
		('feature', NEW.id, NEW.id, 'notes', NEW.notes),
		('feature', NEW.id, NEW.id, 'key_files', NEW.key_files),
		('feature', NEW.id, NEW.id, 'tags', NEW.tags);
END;

-- === Decisions triggers ===
CREATE TRIGGER IF NOT EXISTS search_decisions_insert AFTER INSERT ON decisions BEGIN
	INSERT INTO search_index(entity_type, entity_id, feature_id, field_name, content)
	VALUES
		('decision', CAST(NEW.id AS TEXT), NEW.feature_id, 'approach', NEW.approach),
		('decision', CAST(NEW.id AS TEXT), NEW.feature_id, 'reason', NEW.reason);
END;

CREATE TRIGGER IF NOT EXISTS search_decisions_delete AFTER DELETE ON decisions BEGIN
	DELETE FROM search_index WHERE entity_type = 'decision' AND entity_id = CAST(OLD.id AS TEXT);
END;

-- === Issues triggers ===
CREATE TRIGGER IF NOT EXISTS search_issues_insert AFTER INSERT ON issues BEGIN
	INSERT INTO search_index(entity_type, entity_id, feature_id, field_name, content)
	VALUES ('issue', CAST(NEW.id AS TEXT), NEW.feature_id, 'description', NEW.description);
END;

CREATE TRIGGER IF NOT EXISTS search_issues_delete AFTER DELETE ON issues BEGIN
	DELETE FROM search_index WHERE entity_type = 'issue' AND entity_id = CAST(OLD.id AS TEXT);
END;

-- === Notes triggers ===
CREATE TRIGGER IF NOT EXISTS search_notes_insert AFTER INSERT ON notes BEGIN
	INSERT INTO search_index(entity_type, entity_id, feature_id, field_name, content)
	VALUES ('note', CAST(NEW.id AS TEXT), NEW.feature_id, 'content', NEW.content);
END;

CREATE TRIGGER IF NOT EXISTS search_notes_delete AFTER DELETE ON notes BEGIN
	DELETE FROM search_index WHERE entity_type = 'note' AND entity_id = CAST(OLD.id AS TEXT);
END;

-- === Sessions triggers ===
CREATE TRIGGER IF NOT EXISTS search_sessions_insert AFTER INSERT ON sessions BEGIN
	INSERT INTO search_index(entity_type, entity_id, feature_id, field_name, content)
	VALUES ('session', CAST(NEW.id AS TEXT), COALESCE(NEW.feature_id, ''), 'summary', NEW.summary);
END;

CREATE TRIGGER IF NOT EXISTS search_sessions_delete AFTER DELETE ON sessions BEGIN
	DELETE FROM search_index WHERE entity_type = 'session' AND entity_id = CAST(OLD.id AS TEXT);
END;

CREATE TRIGGER IF NOT EXISTS search_sessions_update AFTER UPDATE ON sessions BEGIN
	DELETE FROM search_index WHERE entity_type = 'session' AND entity_id = CAST(OLD.id AS TEXT);
	INSERT INTO search_index(entity_type, entity_id, feature_id, field_name, content)
	VALUES ('session', CAST(NEW.id AS TEXT), COALESCE(NEW.feature_id, ''), 'summary', NEW.summary);
END;

-- === Subtasks triggers ===
CREATE TRIGGER IF NOT EXISTS search_subtasks_insert AFTER INSERT ON subtasks BEGIN
	INSERT INTO search_index(entity_type, entity_id, feature_id, field_name, content)
	VALUES ('subtask', CAST(NEW.id AS TEXT), NEW.feature_id, 'title', NEW.title);
END;

CREATE TRIGGER IF NOT EXISTS search_subtasks_delete AFTER DELETE ON subtasks BEGIN
	DELETE FROM search_index WHERE entity_type = 'subtask' AND entity_id = CAST(OLD.id AS TEXT);
END;

-- === Task items triggers (JOIN through subtasks for feature_id) ===
CREATE TRIGGER IF NOT EXISTS search_task_items_insert AFTER INSERT ON task_items BEGIN
	INSERT INTO search_index(entity_type, entity_id, feature_id, field_name, content)
	SELECT 'task_item', CAST(NEW.id AS TEXT), s.feature_id, 'title', NEW.title
	FROM subtasks s WHERE s.id = NEW.subtask_id;
END;

CREATE TRIGGER IF NOT EXISTS search_task_items_delete AFTER DELETE ON task_items BEGIN
	DELETE FROM search_index WHERE entity_type = 'task_item' AND entity_id = CAST(OLD.id AS TEXT);
END;

CREATE TRIGGER IF NOT EXISTS search_task_items_update AFTER UPDATE ON task_items BEGIN
	DELETE FROM search_index WHERE entity_type = 'task_item' AND entity_id = CAST(OLD.id AS TEXT);
	INSERT INTO search_index(entity_type, entity_id, feature_id, field_name, content)
	SELECT 'task_item', CAST(NEW.id AS TEXT), s.feature_id, 'title', NEW.title
	FROM subtasks s WHERE s.id = NEW.subtask_id;
	INSERT INTO search_index(entity_type, entity_id, feature_id, field_name, content)
	SELECT 'task_item', CAST(NEW.id AS TEXT), s.feature_id, 'outcome', NEW.outcome
	FROM subtasks s WHERE s.id = NEW.subtask_id AND NEW.outcome != '';
END;

-- === Checkpoint observations triggers ===
CREATE TRIGGER IF NOT EXISTS search_observations_insert AFTER INSERT ON checkpoint_observations BEGIN
	INSERT INTO search_index(entity_type, entity_id, feature_id, field_name, content)
	VALUES ('observation', CAST(NEW.id AS TEXT), NEW.feature_id, 'summary_text', NEW.summary_text);
END;

CREATE TRIGGER IF NOT EXISTS search_observations_delete AFTER DELETE ON checkpoint_observations BEGIN
	DELETE FROM search_index WHERE entity_type = 'observation' AND entity_id = CAST(OLD.id AS TEXT);
END;

DELETE FROM search_index;

-- === Initial population from existing data ===
INSERT INTO search_index(entity_type, entity_id, feature_id, field_name, content)
SELECT 'feature', id, id, 'title', title FROM features WHERE title != ''
UNION ALL SELECT 'feature', id, id, 'description', description FROM features WHERE description != ''
UNION ALL SELECT 'feature', id, id, 'left_off', left_off FROM features WHERE left_off != ''
UNION ALL SELECT 'feature', id, id, 'notes', notes FROM features WHERE notes != ''
UNION ALL SELECT 'feature', id, id, 'key_files', key_files FROM features WHERE key_files != '[]'
UNION ALL SELECT 'feature', id, id, 'tags', tags FROM features WHERE tags != '[]';

INSERT INTO search_index(entity_type, entity_id, feature_id, field_name, content)
SELECT 'decision', CAST(id AS TEXT), feature_id, 'approach', approach FROM decisions WHERE approach != ''
UNION ALL SELECT 'decision', CAST(id AS TEXT), feature_id, 'reason', reason FROM decisions WHERE reason != '';

INSERT INTO search_index(entity_type, entity_id, feature_id, field_name, content)
SELECT 'issue', CAST(id AS TEXT), feature_id, 'description', description FROM issues WHERE description != '';

INSERT INTO search_index(entity_type, entity_id, feature_id, field_name, content)
SELECT 'note', CAST(id AS TEXT), feature_id, 'content', content FROM notes WHERE content != '';

INSERT INTO search_index(entity_type, entity_id, feature_id, field_name, content)
SELECT 'session', CAST(id AS TEXT), COALESCE(feature_id, ''), 'summary', summary FROM sessions WHERE summary != '';

INSERT INTO search_index(entity_type, entity_id, feature_id, field_name, content)
SELECT 'subtask', CAST(id AS TEXT), feature_id, 'title', title FROM subtasks WHERE title != '';

INSERT INTO search_index(entity_type, entity_id, feature_id, field_name, content)
SELECT 'task_item', CAST(ti.id AS TEXT), s.feature_id, 'title', ti.title
FROM task_items ti JOIN subtasks s ON s.id = ti.subtask_id WHERE ti.title != '';

INSERT INTO search_index(entity_type, entity_id, feature_id, field_name, content)
SELECT 'task_item', CAST(ti.id AS TEXT), s.feature_id, 'outcome', ti.outcome
FROM task_items ti JOIN subtasks s ON s.id = ti.subtask_id WHERE ti.outcome != '';

INSERT INTO search_index(entity_type, entity_id, feature_id, field_name, content)
SELECT 'observation', CAST(id AS TEXT), feature_id, 'summary_text', summary_text
FROM checkpoint_observations WHERE summary_text != '';
`

// schemaV17 recreates the FTS5 table with UNINDEXED metadata columns.
// Without UNINDEXED, MATCH queries hit entity_type/entity_id/feature_id/field_name,
// causing searches for "decision" or "title" to match metadata instead of content.
const schemaV17 = `
DROP TABLE IF EXISTS search_index;
CREATE VIRTUAL TABLE search_index USING fts5(
	entity_type UNINDEXED,
	entity_id UNINDEXED,
	feature_id UNINDEXED,
	field_name UNINDEXED,
	content,
	tokenize='porter unicode61'
);
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
	// v13: add last_heartbeat column to work_sessions
	db.Exec(schemaV13)
	// v14: add spec_path column to features
	db.Exec(schemaV14)
	// v15: add notes table
	db.Exec(schemaV15)
	// v16: add FTS5 search index with triggers and initial population
	db.Exec(schemaV16)
	// v17: recreate search_index with UNINDEXED metadata columns
	db.Exec(schemaV17)
	// Populate search index if empty (first run or after v17 recreate)
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM search_index").Scan(&count); err == nil && count == 0 {
		populateSearchIndex(db)
	}
	return nil
}

func populateSearchIndex(db *sql.DB) {
	populations := []string{
		`INSERT INTO search_index(entity_type, entity_id, feature_id, field_name, content)
		 SELECT 'feature', id, id, 'title', title FROM features WHERE title != ''
		 UNION ALL SELECT 'feature', id, id, 'description', description FROM features WHERE description != ''
		 UNION ALL SELECT 'feature', id, id, 'left_off', left_off FROM features WHERE left_off != ''
		 UNION ALL SELECT 'feature', id, id, 'notes', notes FROM features WHERE notes != ''
		 UNION ALL SELECT 'feature', id, id, 'key_files', key_files FROM features WHERE key_files != '[]'
		 UNION ALL SELECT 'feature', id, id, 'tags', tags FROM features WHERE tags != '[]'`,

		`INSERT INTO search_index(entity_type, entity_id, feature_id, field_name, content)
		 SELECT 'decision', CAST(id AS TEXT), feature_id, 'approach', approach FROM decisions WHERE approach != ''
		 UNION ALL SELECT 'decision', CAST(id AS TEXT), feature_id, 'reason', reason FROM decisions WHERE reason != ''`,

		`INSERT INTO search_index(entity_type, entity_id, feature_id, field_name, content)
		 SELECT 'issue', CAST(id AS TEXT), feature_id, 'description', description FROM issues WHERE description != ''`,

		`INSERT INTO search_index(entity_type, entity_id, feature_id, field_name, content)
		 SELECT 'note', CAST(id AS TEXT), feature_id, 'content', content FROM notes WHERE content != ''`,

		`INSERT INTO search_index(entity_type, entity_id, feature_id, field_name, content)
		 SELECT 'session', CAST(id AS TEXT), COALESCE(feature_id, ''), 'summary', summary FROM sessions WHERE summary != ''`,

		`INSERT INTO search_index(entity_type, entity_id, feature_id, field_name, content)
		 SELECT 'subtask', CAST(id AS TEXT), feature_id, 'title', title FROM subtasks WHERE title != ''`,

		`INSERT INTO search_index(entity_type, entity_id, feature_id, field_name, content)
		 SELECT 'task_item', CAST(ti.id AS TEXT), s.feature_id, 'title', ti.title
		 FROM task_items ti JOIN subtasks s ON s.id = ti.subtask_id WHERE ti.title != ''`,

		`INSERT INTO search_index(entity_type, entity_id, feature_id, field_name, content)
		 SELECT 'task_item', CAST(ti.id AS TEXT), s.feature_id, 'outcome', ti.outcome
		 FROM task_items ti JOIN subtasks s ON s.id = ti.subtask_id WHERE ti.outcome != ''`,

		`INSERT INTO search_index(entity_type, entity_id, feature_id, field_name, content)
		 SELECT 'observation', CAST(id AS TEXT), feature_id, 'summary_text', summary_text
		 FROM checkpoint_observations WHERE summary_text != ''`,
	}
	for _, sql := range populations {
		db.Exec(sql)
	}
}
