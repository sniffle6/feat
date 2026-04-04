package store

import (
	"fmt"
	"time"
)

type Note struct {
	ID        int64     `json:"id"`
	FeatureID string    `json:"feature_id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Store) AddNote(featureID, content string) (*Note, error) {
	res, err := s.db.Exec(
		`INSERT INTO notes (feature_id, content) VALUES (?, ?)`,
		featureID, content,
	)
	if err != nil {
		return nil, fmt.Errorf("insert note: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.getNote(id)
}

func (s *Store) getNote(id int64) (*Note, error) {
	var n Note
	err := s.db.QueryRow(
		`SELECT id, feature_id, content, created_at FROM notes WHERE id = ?`, id,
	).Scan(&n.ID, &n.FeatureID, &n.Content, &n.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get note %d: %w", id, err)
	}
	return &n, nil
}

func (s *Store) DeleteNote(id int64) error {
	res, err := s.db.Exec(`DELETE FROM notes WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete note %d: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("note %d not found", id)
	}
	return nil
}

func (s *Store) GetNotesForFeature(featureID string) ([]Note, error) {
	rows, err := s.db.Query(
		`SELECT id, feature_id, content, created_at FROM notes WHERE feature_id = ? ORDER BY id DESC`,
		featureID,
	)
	if err != nil {
		return nil, fmt.Errorf("get notes: %w", err)
	}
	defer rows.Close()

	var notes []Note
	for rows.Next() {
		var n Note
		if err := rows.Scan(&n.ID, &n.FeatureID, &n.Content, &n.CreatedAt); err != nil {
			return nil, err
		}
		notes = append(notes, n)
	}
	return notes, nil
}
