package store

import (
	"fmt"
	"time"
)

type Decision struct {
	ID        int64     `json:"id"`
	FeatureID string    `json:"feature_id"`
	Approach  string    `json:"approach"`
	Outcome   string    `json:"outcome"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Store) AddDecision(featureID, approach, outcome, reason string) (*Decision, error) {
	res, err := s.db.Exec(
		`INSERT INTO decisions (feature_id, approach, outcome, reason) VALUES (?, ?, ?, ?)`,
		featureID, approach, outcome, reason,
	)
	if err != nil {
		return nil, fmt.Errorf("insert decision: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.getDecision(id)
}

func (s *Store) getDecision(id int64) (*Decision, error) {
	var d Decision
	err := s.db.QueryRow(
		`SELECT id, feature_id, approach, outcome, reason, created_at FROM decisions WHERE id = ?`, id,
	).Scan(&d.ID, &d.FeatureID, &d.Approach, &d.Outcome, &d.Reason, &d.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get decision %d: %w", id, err)
	}
	return &d, nil
}

func (s *Store) GetDecisionsForFeature(featureID string) ([]Decision, error) {
	rows, err := s.db.Query(
		`SELECT id, feature_id, approach, outcome, reason, created_at FROM decisions WHERE feature_id = ? ORDER BY id DESC`,
		featureID,
	)
	if err != nil {
		return nil, fmt.Errorf("get decisions: %w", err)
	}
	defer rows.Close()

	var decisions []Decision
	for rows.Next() {
		var d Decision
		if err := rows.Scan(&d.ID, &d.FeatureID, &d.Approach, &d.Outcome, &d.Reason, &d.CreatedAt); err != nil {
			return nil, err
		}
		decisions = append(decisions, d)
	}
	return decisions, nil
}
