package store

import (
	"os"
	"testing"
)

func TestOpenCreatesDB(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	dbPath := dir + "/.feat/features.db"
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("db file not created: %v", err)
	}
}
