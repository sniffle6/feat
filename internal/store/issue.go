package store

import (
	"database/sql"
	"fmt"
	"time"
)

type Issue struct {
	ID             int64      `json:"id"`
	FeatureID      string     `json:"feature_id"`
	TaskItemID     *int64     `json:"task_item_id"`
	Description    string     `json:"description"`
	Status         string     `json:"status"`
	ResolvedCommit string     `json:"resolved_commit"`
	CreatedAt      time.Time  `json:"created_at"`
	ResolvedAt     *time.Time `json:"resolved_at,omitempty"`
}

func (s *Store) AddIssue(featureID, description string, taskItemID *int64) (*Issue, error) {
	res, err := s.db.Exec(
		`INSERT INTO issues (feature_id, task_item_id, description) VALUES (?, ?, ?)`,
		featureID, taskItemID, description,
	)
	if err != nil {
		return nil, fmt.Errorf("insert issue: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.getIssue(id)
}

func (s *Store) getIssue(id int64) (*Issue, error) {
	var issue Issue
	var taskItemID *int64
	var resolvedAt *time.Time
	err := s.db.QueryRow(
		`SELECT id, feature_id, task_item_id, description, status, resolved_commit, created_at, resolved_at FROM issues WHERE id = ?`, id,
	).Scan(&issue.ID, &issue.FeatureID, &taskItemID, &issue.Description, &issue.Status, &issue.ResolvedCommit, &issue.CreatedAt, &resolvedAt)
	if err != nil {
		return nil, fmt.Errorf("get issue %d: %w", id, err)
	}
	issue.TaskItemID = taskItemID
	issue.ResolvedAt = resolvedAt
	return &issue, nil
}

func (s *Store) ResolveIssue(id int64, commitHash string) error {
	res, err := s.db.Exec(
		`UPDATE issues SET status = 'resolved', resolved_commit = ?, resolved_at = ? WHERE id = ?`,
		commitHash, time.Now().UTC(), id,
	)
	if err != nil {
		return fmt.Errorf("resolve issue: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("issue %d not found", id)
	}
	return nil
}

func (s *Store) GetIssuesForFeature(featureID string) ([]Issue, error) {
	rows, err := s.db.Query(
		`SELECT id, feature_id, task_item_id, description, status, resolved_commit, created_at, resolved_at
		 FROM issues WHERE feature_id = ?
		 ORDER BY CASE WHEN status = 'open' THEN 0 ELSE 1 END, id DESC`,
		featureID,
	)
	if err != nil {
		return nil, fmt.Errorf("get issues: %w", err)
	}
	defer rows.Close()
	return scanIssues(rows)
}

func (s *Store) GetOpenIssueCount(featureID string) (int, error) {
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM issues WHERE feature_id = ? AND status = 'open'`,
		featureID,
	).Scan(&count)
	return count, err
}

func (s *Store) GetAllOpenIssues() ([]Issue, error) {
	rows, err := s.db.Query(
		`SELECT id, feature_id, task_item_id, description, status, resolved_commit, created_at, resolved_at
		 FROM issues WHERE status = 'open' ORDER BY id DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("get all open issues: %w", err)
	}
	defer rows.Close()
	return scanIssues(rows)
}

func scanIssues(rows *sql.Rows) ([]Issue, error) {
	var issues []Issue
	for rows.Next() {
		var issue Issue
		var taskItemID *int64
		var resolvedAt *time.Time
		if err := rows.Scan(&issue.ID, &issue.FeatureID, &taskItemID, &issue.Description, &issue.Status, &issue.ResolvedCommit, &issue.CreatedAt, &resolvedAt); err != nil {
			return nil, err
		}
		issue.TaskItemID = taskItemID
		issue.ResolvedAt = resolvedAt
		issues = append(issues, issue)
	}
	return issues, nil
}
