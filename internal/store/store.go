package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(projectDir string) (*Store, error) {
	featDir := filepath.Join(projectDir, ".feat")
	if err := os.MkdirAll(featDir, 0755); err != nil {
		return nil, fmt.Errorf("create .feat dir: %w", err)
	}

	dbPath := filepath.Join(featDir, "features.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	db.Exec("PRAGMA journal_mode=WAL")
	db.Exec("PRAGMA foreign_keys=ON")

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}
