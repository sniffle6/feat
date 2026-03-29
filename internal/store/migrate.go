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
	return nil
}
