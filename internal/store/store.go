package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Feature struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	Status       string    `json:"status"`
	LeftOff      string    `json:"left_off"`
	KeyFiles     []string  `json:"key_files"`
	WorktreePath string    `json:"worktree_path"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type FeatureUpdate struct {
	Title        *string   `json:"title,omitempty"`
	Description  *string   `json:"description,omitempty"`
	Status       *string   `json:"status,omitempty"`
	LeftOff      *string   `json:"left_off,omitempty"`
	KeyFiles     *[]string `json:"key_files,omitempty"`
	WorktreePath *string   `json:"worktree_path,omitempty"`
}

type FeatureContext struct {
	Feature        Feature   `json:"feature"`
	RecentSessions []Session `json:"recent_sessions"`
}

var nonAlphaNum = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(title string) string {
	s := strings.ToLower(strings.TrimSpace(title))
	s = nonAlphaNum.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

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

func (s *Store) AddFeature(title, description string) (*Feature, error) {
	id := slugify(title)
	now := time.Now().UTC()
	_, err := s.db.Exec(
		`INSERT INTO features (id, title, description, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		id, title, description, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert feature: %w", err)
	}
	return s.GetFeature(id)
}

func (s *Store) GetFeature(id string) (*Feature, error) {
	row := s.db.QueryRow(
		`SELECT id, title, description, status, left_off, key_files, worktree_path, created_at, updated_at FROM features WHERE id = ?`,
		id,
	)
	var f Feature
	var keyFilesJSON string
	err := row.Scan(&f.ID, &f.Title, &f.Description, &f.Status, &f.LeftOff, &keyFilesJSON, &f.WorktreePath, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get feature %q: %w", id, err)
	}
	json.Unmarshal([]byte(keyFilesJSON), &f.KeyFiles)
	if f.KeyFiles == nil {
		f.KeyFiles = []string{}
	}
	return &f, nil
}

func (s *Store) UpdateFeature(id string, u FeatureUpdate) error {
	sets := []string{}
	args := []any{}
	if u.Title != nil {
		sets = append(sets, "title = ?")
		args = append(args, *u.Title)
	}
	if u.Description != nil {
		sets = append(sets, "description = ?")
		args = append(args, *u.Description)
	}
	if u.Status != nil {
		sets = append(sets, "status = ?")
		args = append(args, *u.Status)
	}
	if u.LeftOff != nil {
		sets = append(sets, "left_off = ?")
		args = append(args, *u.LeftOff)
	}
	if u.KeyFiles != nil {
		kf, _ := json.Marshal(*u.KeyFiles)
		sets = append(sets, "key_files = ?")
		args = append(args, string(kf))
	}
	if u.WorktreePath != nil {
		sets = append(sets, "worktree_path = ?")
		args = append(args, *u.WorktreePath)
	}
	if len(sets) == 0 {
		return nil
	}
	sets = append(sets, "updated_at = ?")
	args = append(args, time.Now().UTC())
	args = append(args, id)
	query := fmt.Sprintf("UPDATE features SET %s WHERE id = ?", strings.Join(sets, ", "))
	res, err := s.db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("update feature: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("feature %q not found", id)
	}
	return nil
}

func (s *Store) ListFeatures(status string) ([]Feature, error) {
	query := `SELECT id, title, description, status, left_off, key_files, worktree_path, created_at, updated_at FROM features`
	var args []any
	if status != "" {
		query += " WHERE status = ?"
		args = append(args, status)
	}
	query += " ORDER BY updated_at DESC"
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list features: %w", err)
	}
	defer rows.Close()
	var features []Feature
	for rows.Next() {
		var f Feature
		var keyFilesJSON string
		if err := rows.Scan(&f.ID, &f.Title, &f.Description, &f.Status, &f.LeftOff, &keyFilesJSON, &f.WorktreePath, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan feature: %w", err)
		}
		json.Unmarshal([]byte(keyFilesJSON), &f.KeyFiles)
		if f.KeyFiles == nil {
			f.KeyFiles = []string{}
		}
		features = append(features, f)
	}
	return features, nil
}

type Session struct {
	ID           int64     `json:"id"`
	FeatureID    string    `json:"feature_id"`
	Summary      string    `json:"summary"`
	FilesTouched []string  `json:"files_touched"`
	Commits      []string  `json:"commits"`
	AutoLinked   bool      `json:"auto_linked"`
	LinkReason   string    `json:"link_reason"`
	CreatedAt    time.Time `json:"created_at"`
}

type SessionInput struct {
	FeatureID    string   `json:"feature_id"`
	Summary      string   `json:"summary"`
	FilesTouched []string `json:"files_touched"`
	Commits      []string `json:"commits"`
	AutoLinked   bool     `json:"auto_linked"`
	LinkReason   string   `json:"link_reason"`
}

func (s *Store) LogSession(input SessionInput) (*Session, error) {
	ft, _ := json.Marshal(input.FilesTouched)
	cm, _ := json.Marshal(input.Commits)
	var featureID *string
	if input.FeatureID != "" {
		featureID = &input.FeatureID
	}
	res, err := s.db.Exec(
		`INSERT INTO sessions (feature_id, summary, files_touched, commits, auto_linked, link_reason) VALUES (?, ?, ?, ?, ?, ?)`,
		featureID, input.Summary, string(ft), string(cm), input.AutoLinked, input.LinkReason,
	)
	if err != nil {
		return nil, fmt.Errorf("insert session: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.getSession(id)
}

func (s *Store) getSession(id int64) (*Session, error) {
	row := s.db.QueryRow(
		`SELECT id, COALESCE(feature_id, ''), summary, files_touched, commits, auto_linked, link_reason, created_at FROM sessions WHERE id = ?`, id,
	)
	return scanSession(row)
}

func (s *Store) GetSessionsForFeature(featureID string) ([]Session, error) {
	rows, err := s.db.Query(
		`SELECT id, COALESCE(feature_id, ''), summary, files_touched, commits, auto_linked, link_reason, created_at FROM sessions WHERE feature_id = ? ORDER BY created_at DESC`, featureID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSessions(rows)
}

func (s *Store) GetUnlinkedSessions() ([]Session, error) {
	rows, err := s.db.Query(
		`SELECT id, COALESCE(feature_id, ''), summary, files_touched, commits, auto_linked, link_reason, created_at FROM sessions WHERE feature_id IS NULL ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSessions(rows)
}

func (s *Store) ReassignSession(sessionID int64, featureID string) error {
	_, err := s.db.Exec(`UPDATE sessions SET feature_id = ?, auto_linked = 0, link_reason = 'manual reassign' WHERE id = ?`, featureID, sessionID)
	return err
}

func scanSession(row *sql.Row) (*Session, error) {
	var sess Session
	var ft, cm string
	err := row.Scan(&sess.ID, &sess.FeatureID, &sess.Summary, &ft, &cm, &sess.AutoLinked, &sess.LinkReason, &sess.CreatedAt)
	if err != nil {
		return nil, err
	}
	json.Unmarshal([]byte(ft), &sess.FilesTouched)
	json.Unmarshal([]byte(cm), &sess.Commits)
	if sess.FilesTouched == nil {
		sess.FilesTouched = []string{}
	}
	if sess.Commits == nil {
		sess.Commits = []string{}
	}
	return &sess, nil
}

func scanSessions(rows *sql.Rows) ([]Session, error) {
	var sessions []Session
	for rows.Next() {
		var sess Session
		var ft, cm string
		if err := rows.Scan(&sess.ID, &sess.FeatureID, &sess.Summary, &ft, &cm, &sess.AutoLinked, &sess.LinkReason, &sess.CreatedAt); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(ft), &sess.FilesTouched)
		json.Unmarshal([]byte(cm), &sess.Commits)
		if sess.FilesTouched == nil {
			sess.FilesTouched = []string{}
		}
		if sess.Commits == nil {
			sess.Commits = []string{}
		}
		sessions = append(sessions, sess)
	}
	return sessions, nil
}

func (s *Store) GetReadyFeatures() ([]Feature, error) {
	rows, err := s.db.Query(
		`SELECT id, title, description, status, left_off, key_files, worktree_path, created_at, updated_at FROM features WHERE status IN ('in_progress', 'planned') ORDER BY CASE WHEN status='in_progress' THEN 0 ELSE 1 END, updated_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("get ready features: %w", err)
	}
	defer rows.Close()
	var features []Feature
	for rows.Next() {
		var f Feature
		var keyFilesJSON string
		if err := rows.Scan(&f.ID, &f.Title, &f.Description, &f.Status, &f.LeftOff, &keyFilesJSON, &f.WorktreePath, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan feature: %w", err)
		}
		json.Unmarshal([]byte(keyFilesJSON), &f.KeyFiles)
		if f.KeyFiles == nil {
			f.KeyFiles = []string{}
		}
		features = append(features, f)
	}
	return features, nil
}

func (s *Store) CompactSessions(featureID, summary string) (int, error) {
	// Count total sessions for feature
	var total int
	s.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE feature_id = ?`, featureID).Scan(&total)
	if total <= 3 {
		return 0, nil
	}

	// Get IDs of all but the last 3 sessions
	rows, err := s.db.Query(
		`SELECT id FROM sessions WHERE feature_id = ? ORDER BY created_at DESC LIMIT -1 OFFSET 3`,
		featureID,
	)
	if err != nil {
		return 0, fmt.Errorf("query old sessions: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		rows.Scan(&id)
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return 0, nil
	}

	// Insert compacted session
	_, err = s.db.Exec(
		`INSERT INTO sessions (feature_id, summary, files_touched, commits, auto_linked, link_reason, compacted) VALUES (?, ?, '[]', '[]', 0, 'compacted', 1)`,
		featureID, summary,
	)
	if err != nil {
		return 0, fmt.Errorf("insert compacted session: %w", err)
	}

	// Delete old sessions
	placeholders := make([]string, len(ids))
	delArgs := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		delArgs[i] = id
	}
	_, err = s.db.Exec(
		fmt.Sprintf("DELETE FROM sessions WHERE id IN (%s)", strings.Join(placeholders, ",")),
		delArgs...,
	)
	if err != nil {
		return 0, fmt.Errorf("delete old sessions: %w", err)
	}

	return len(ids), nil
}

func (s *Store) GetContext(id string) (*FeatureContext, error) {
	f, err := s.GetFeature(id)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.Query(
		`SELECT id, COALESCE(feature_id, ''), summary, files_touched, commits, auto_linked, link_reason, created_at FROM sessions WHERE feature_id = ? ORDER BY created_at DESC LIMIT 5`,
		id,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sessions, err := scanSessions(rows)
	if err != nil {
		return nil, err
	}
	if sessions == nil {
		sessions = []Session{}
	}

	return &FeatureContext{Feature: *f, RecentSessions: sessions}, nil
}
