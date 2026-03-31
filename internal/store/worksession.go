package store

import (
	"fmt"
	"time"
)

type WorkSession struct {
	ID              int64      `json:"id"`
	FeatureID       string     `json:"feature_id"`
	ClaudeSessionID string     `json:"claude_session_id"`
	Status          string     `json:"status"`
	StartedAt       time.Time  `json:"started_at"`
	EndedAt         *time.Time `json:"ended_at"`
	HandoffStale    bool       `json:"handoff_stale"`
}

// OpenWorkSession opens a new work session or resumes an existing open one
// for the same feature + claude session.
func (s *Store) OpenWorkSession(featureID, claudeSessionID string) (*WorkSession, error) {
	// Try to resume existing open session for same feature+claude session
	row := s.db.QueryRow(
		`SELECT id, feature_id, claude_session_id, status, started_at, ended_at, handoff_stale
         FROM work_sessions WHERE feature_id = ? AND claude_session_id = ? AND status = 'open'`,
		featureID, claudeSessionID,
	)
	ws, err := scanWorkSession(row)
	if err == nil {
		return ws, nil
	}

	// Close any other open sessions (one active session at a time)
	s.db.Exec(`UPDATE work_sessions SET status = 'closed', ended_at = datetime('now') WHERE status = 'open'`)

	now := time.Now().UTC()
	res, err := s.db.Exec(
		`INSERT INTO work_sessions (feature_id, claude_session_id, status, started_at) VALUES (?, ?, 'open', ?)`,
		featureID, claudeSessionID, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert work session: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.GetWorkSession(id)
}

func (s *Store) GetWorkSession(id int64) (*WorkSession, error) {
	row := s.db.QueryRow(
		`SELECT id, feature_id, claude_session_id, status, started_at, ended_at, handoff_stale
         FROM work_sessions WHERE id = ?`, id,
	)
	return scanWorkSession(row)
}

// GetActiveWorkSession returns the single open work session, or error if none.
func (s *Store) GetActiveWorkSession() (*WorkSession, error) {
	row := s.db.QueryRow(
		`SELECT id, feature_id, claude_session_id, status, started_at, ended_at, handoff_stale
         FROM work_sessions WHERE status = 'open' ORDER BY id DESC LIMIT 1`,
	)
	return scanWorkSession(row)
}

func (s *Store) CloseWorkSession(id int64) error {
	_, err := s.db.Exec(
		`UPDATE work_sessions SET status = 'closed', ended_at = datetime('now') WHERE id = ?`, id,
	)
	return err
}

func (s *Store) MarkHandoffStale(id int64) {
	s.db.Exec(`UPDATE work_sessions SET handoff_stale = 1 WHERE id = ?`, id)
}

type scannable interface {
	Scan(dest ...any) error
}

func scanWorkSession(row scannable) (*WorkSession, error) {
	var ws WorkSession
	var stale int
	err := row.Scan(&ws.ID, &ws.FeatureID, &ws.ClaudeSessionID, &ws.Status, &ws.StartedAt, &ws.EndedAt, &stale)
	if err != nil {
		return nil, err
	}
	ws.HandoffStale = stale != 0
	return &ws, nil
}
