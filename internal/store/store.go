package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Feature struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	Status       string    `json:"status"`
	Type         string    `json:"type"`
	LeftOff      string    `json:"left_off"`
	Notes        string    `json:"notes"`
	KeyFiles     []string  `json:"key_files"`
	Tags         []string  `json:"tags"`
	WorktreePath string    `json:"worktree_path"`
	SpecPath     string    `json:"spec_path,omitempty"`
	PlanPath     string    `json:"plan_path,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type FeatureUpdate struct {
	Title        *string   `json:"title,omitempty"`
	Description  *string   `json:"description,omitempty"`
	Status       *string   `json:"status,omitempty"`
	Type         *string   `json:"type,omitempty"`
	LeftOff      *string   `json:"left_off,omitempty"`
	Notes        *string   `json:"notes,omitempty"`
	KeyFiles     *[]string `json:"key_files,omitempty"`
	Tags         *[]string `json:"tags,omitempty"`
	WorktreePath *string   `json:"worktree_path,omitempty"`
	SpecPath     *string   `json:"spec_path,omitempty"`
	PlanPath     *string   `json:"plan_path,omitempty"`
	Force        *bool     `json:"force,omitempty"`
	ForceReason  *string   `json:"force_reason,omitempty"`
}

type FeatureContext struct {
	Feature        Feature    `json:"feature"`
	RecentSessions []Session  `json:"recent_sessions"`
	Decisions      []Decision `json:"decisions"`
}

var nonAlphaNum = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(title string) string {
	s := strings.ToLower(strings.TrimSpace(title))
	s = nonAlphaNum.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

type Store struct {
	db  *sql.DB
	dir string // .docket directory path
}

func Open(projectDir string) (*Store, error) {
	featDir := filepath.Join(projectDir, ".docket")
	if err := os.MkdirAll(featDir, 0755); err != nil {
		return nil, fmt.Errorf("create .docket dir: %w", err)
	}

	dbPath := filepath.Join(featDir, "features.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	db.Exec("PRAGMA journal_mode=WAL")
	db.Exec("PRAGMA busy_timeout=5000")
	db.Exec("PRAGMA foreign_keys=ON")

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return &Store{db: db, dir: featDir}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) DB() *sql.DB {
	return s.db
}

func (s *Store) AutoArchiveStale() ([]string, error) {
	rows, err := s.db.Query(`SELECT id FROM features WHERE status = 'done' AND updated_at < datetime('now', '-7 days')`)
	if err != nil {
		return nil, fmt.Errorf("query stale features: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan stale feature id: %w", err)
		}
		ids = append(ids, id)
	}

	archived := "archived"
	var archivedIDs []string
	for _, id := range ids {
		if err := s.UpdateFeature(id, FeatureUpdate{Status: &archived}); err != nil {
			fmt.Fprintf(os.Stderr, "docket: auto-archive %q: %v\n", id, err)
			continue
		}
		archivedIDs = append(archivedIDs, id)
	}
	return archivedIDs, nil
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
		`SELECT id, title, description, status, type, left_off, notes, key_files, tags, worktree_path, spec_path, plan_path, created_at, updated_at FROM features WHERE id = ?`,
		id,
	)
	var f Feature
	var keyFilesJSON, tagsJSON string
	err := row.Scan(&f.ID, &f.Title, &f.Description, &f.Status, &f.Type, &f.LeftOff, &f.Notes, &keyFilesJSON, &tagsJSON, &f.WorktreePath, &f.SpecPath, &f.PlanPath, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get feature %q: %w", id, err)
	}
	json.Unmarshal([]byte(keyFilesJSON), &f.KeyFiles)
	if f.KeyFiles == nil {
		f.KeyFiles = []string{}
	}
	json.Unmarshal([]byte(tagsJSON), &f.Tags)
	if f.Tags == nil {
		f.Tags = []string{}
	}
	return &f, nil
}

func (s *Store) UpdateFeature(id string, u FeatureUpdate) error {
	// Completion gate: check prerequisites before allowing status=done
	if u.Status != nil && *u.Status == "done" {
		done, total, _ := s.GetFeatureProgress(id)
		unchecked := total - done
		openIssues, _ := s.GetOpenIssueCount(id)

		if (unchecked > 0 || openIssues > 0) && (u.Force == nil || !*u.Force) {
			return fmt.Errorf("cannot mark feature %q as done: %d unchecked task items, %d open issues (use force=true to override)", id, unchecked, openIssues)
		}

		if (unchecked > 0 || openIssues > 0) && u.Force != nil && *u.Force {
			reason := "No reason given"
			if u.ForceReason != nil && *u.ForceReason != "" {
				reason = *u.ForceReason
			}
			approach := fmt.Sprintf("Force-completed with %d unchecked items, %d open issues", unchecked, openIssues)
			s.AddDecision(id, approach, "accepted", reason)
		}
	}

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
	if u.Type != nil {
		sets = append(sets, "type = ?")
		args = append(args, *u.Type)
	}
	if u.LeftOff != nil {
		sets = append(sets, "left_off = ?")
		args = append(args, *u.LeftOff)
	}
	if u.Notes != nil {
		sets = append(sets, "notes = ?")
		args = append(args, *u.Notes)
	}
	if u.KeyFiles != nil {
		kf, _ := json.Marshal(*u.KeyFiles)
		sets = append(sets, "key_files = ?")
		args = append(args, string(kf))
	}
	if u.Tags != nil {
		t, _ := json.Marshal(*u.Tags)
		sets = append(sets, "tags = ?")
		args = append(args, string(t))
	}
	if u.WorktreePath != nil {
		sets = append(sets, "worktree_path = ?")
		args = append(args, *u.WorktreePath)
	}
	if u.SpecPath != nil {
		sets = append(sets, "spec_path = ?")
		args = append(args, *u.SpecPath)
	}
	if u.PlanPath != nil {
		sets = append(sets, "plan_path = ?")
		args = append(args, *u.PlanPath)
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

func (s *Store) DeleteFeature(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Delete in FK-safe order (children before parent)
	deletes := []string{
		`DELETE FROM checkpoint_observations WHERE feature_id = ?`,
		`DELETE FROM checkpoint_jobs WHERE feature_id = ?`,
		`DELETE FROM task_items WHERE subtask_id IN (SELECT id FROM subtasks WHERE feature_id = ?)`,
		`DELETE FROM subtasks WHERE feature_id = ?`,
		`DELETE FROM decisions WHERE feature_id = ?`,
		`DELETE FROM issues WHERE feature_id = ?`,
		`DELETE FROM notes WHERE feature_id = ?`,
		`DELETE FROM sessions WHERE feature_id = ?`,
		`DELETE FROM work_sessions WHERE feature_id = ?`,
	}
	for _, q := range deletes {
		if _, err := tx.Exec(q, id); err != nil {
			return fmt.Errorf("delete cascade: %w", err)
		}
	}

	res, err := tx.Exec(`DELETE FROM features WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete feature: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("feature %q not found", id)
	}

	return tx.Commit()
}

func (s *Store) ListFeatures(status string) ([]Feature, error) {
	query := `SELECT id, title, description, status, type, left_off, notes, key_files, tags, worktree_path, spec_path, plan_path, created_at, updated_at FROM features`
	var args []any
	if status != "" {
		query += " WHERE status = ?"
		args = append(args, status)
	} else {
		query += " WHERE status != 'archived'"
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
		var keyFilesJSON, tagsJSON string
		if err := rows.Scan(&f.ID, &f.Title, &f.Description, &f.Status, &f.Type, &f.LeftOff, &f.Notes, &keyFilesJSON, &tagsJSON, &f.WorktreePath, &f.SpecPath, &f.PlanPath, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan feature: %w", err)
		}
		json.Unmarshal([]byte(keyFilesJSON), &f.KeyFiles)
		if f.KeyFiles == nil {
			f.KeyFiles = []string{}
		}
		json.Unmarshal([]byte(tagsJSON), &f.Tags)
		if f.Tags == nil {
			f.Tags = []string{}
		}
		features = append(features, f)
	}
	return features, nil
}

func (s *Store) ListFeaturesWithTag(status, tag string) ([]Feature, error) {
	query := `SELECT id, title, description, status, type, left_off, notes, key_files, tags, worktree_path, spec_path, plan_path, created_at, updated_at FROM features WHERE tags LIKE ? ESCAPE '\'`
	escaped := strings.NewReplacer("%", "\\%", "_", "\\_").Replace(tag)
	args := []any{"%" + `"` + escaped + `"` + "%"}
	if status != "" {
		query += " AND status = ?"
		args = append(args, status)
	} else {
		query += " AND status != 'archived'"
	}
	query += " ORDER BY updated_at DESC"
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list features by tag: %w", err)
	}
	defer rows.Close()
	var features []Feature
	for rows.Next() {
		var f Feature
		var keyFilesJSON, tagsJSON string
		if err := rows.Scan(&f.ID, &f.Title, &f.Description, &f.Status, &f.Type, &f.LeftOff, &f.Notes, &keyFilesJSON, &tagsJSON, &f.WorktreePath, &f.SpecPath, &f.PlanPath, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan feature: %w", err)
		}
		json.Unmarshal([]byte(keyFilesJSON), &f.KeyFiles)
		if f.KeyFiles == nil {
			f.KeyFiles = []string{}
		}
		json.Unmarshal([]byte(tagsJSON), &f.Tags)
		if f.Tags == nil {
			f.Tags = []string{}
		}
		features = append(features, f)
	}
	return features, nil
}

func (s *Store) GetKnownTags() ([]string, error) {
	rows, err := s.db.Query(`SELECT tags FROM features WHERE tags != '[]'`)
	if err != nil {
		return nil, fmt.Errorf("get known tags: %w", err)
	}
	defer rows.Close()

	seen := make(map[string]bool)
	for rows.Next() {
		var tagsJSON string
		if err := rows.Scan(&tagsJSON); err != nil {
			continue
		}
		var tags []string
		if err := json.Unmarshal([]byte(tagsJSON), &tags); err != nil {
			continue
		}
		for _, t := range tags {
			seen[t] = true
		}
	}

	result := make([]string, 0, len(seen))
	for t := range seen {
		result = append(result, t)
	}
	sort.Strings(result)
	return result, nil
}

func (s *Store) CheckNewTags(tags []string) []string {
	known, _ := s.GetKnownTags()
	knownSet := make(map[string]bool, len(known))
	for _, t := range known {
		knownSet[t] = true
	}
	var newTags []string
	for _, t := range tags {
		if !knownSet[t] {
			newTags = append(newTags, t)
		}
	}
	return newTags
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
		`SELECT id, title, description, status, type, left_off, notes, key_files, tags, worktree_path, spec_path, plan_path, created_at, updated_at FROM features WHERE status IN ('in_progress', 'planned') ORDER BY CASE WHEN status='in_progress' THEN 0 ELSE 1 END, updated_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("get ready features: %w", err)
	}
	defer rows.Close()
	var features []Feature
	for rows.Next() {
		var f Feature
		var keyFilesJSON, tagsJSON string
		if err := rows.Scan(&f.ID, &f.Title, &f.Description, &f.Status, &f.Type, &f.LeftOff, &f.Notes, &keyFilesJSON, &tagsJSON, &f.WorktreePath, &f.SpecPath, &f.PlanPath, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan feature: %w", err)
		}
		json.Unmarshal([]byte(keyFilesJSON), &f.KeyFiles)
		if f.KeyFiles == nil {
			f.KeyFiles = []string{}
		}
		json.Unmarshal([]byte(tagsJSON), &f.Tags)
		if f.Tags == nil {
			f.Tags = []string{}
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

type QuickTrackInput struct {
	Title      string
	CommitHash string
	KeyFiles   []string
	Status     string // default "done"
}

type QuickTrackResult struct {
	Feature *Feature
	Created bool // true if new, false if updated existing
}

func (s *Store) QuickTrack(input QuickTrackInput) (*QuickTrackResult, error) {
	if input.Status == "" {
		input.Status = "done"
	}
	id := slugify(input.Title)

	existing, err := s.GetFeature(id)
	created := false

	if err != nil {
		// Feature doesn't exist — create it
		_, err = s.AddFeature(input.Title, "")
		if err != nil {
			return nil, fmt.Errorf("quick_track create: %w", err)
		}
		created = true
	}

	// Update status and key_files
	u := FeatureUpdate{Status: &input.Status}
	if len(input.KeyFiles) > 0 {
		merged := input.KeyFiles
		if existing != nil && len(existing.KeyFiles) > 0 {
			seen := make(map[string]bool)
			for _, f := range existing.KeyFiles {
				seen[f] = true
			}
			merged = append([]string{}, existing.KeyFiles...)
			for _, f := range input.KeyFiles {
				if !seen[f] {
					merged = append(merged, f)
				}
			}
		}
		u.KeyFiles = &merged
	}
	s.UpdateFeature(id, u)

	// Log session with commit if provided
	if input.CommitHash != "" {
		s.LogSession(SessionInput{
			FeatureID:  id,
			Summary:    input.Title,
			Commits:    []string{input.CommitHash},
			AutoLinked: true,
			LinkReason: "quick_track",
		})
	}

	f, err := s.GetFeature(id)
	if err != nil {
		return nil, err
	}
	return &QuickTrackResult{Feature: f, Created: created}, nil
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

	decisions, err := s.GetDecisionsForFeature(id)
	if err != nil {
		return nil, err
	}
	if decisions == nil {
		decisions = []Decision{}
	}

	return &FeatureContext{Feature: *f, RecentSessions: sessions, Decisions: decisions}, nil
}
