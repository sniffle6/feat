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

func migrate(db *sql.DB) error {
	if _, err := db.Exec(schemaV1); err != nil {
		return err
	}
	// v2: add compacted column (ignore error if already exists)
	db.Exec(schemaV2)
	return nil
}
