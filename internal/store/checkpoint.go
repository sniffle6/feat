package store

import (
	"encoding/json"
	"fmt"
	"time"
)

type FileEdit struct {
	Path  string `json:"path"`
	Count int    `json:"count"`
}

type TestRunFact struct {
	Command string `json:"command"`
	Passed  bool   `json:"passed"`
}

type CommitFact struct {
	Hash    string `json:"hash"`
	Message string `json:"message"`
}

type ErrorFact struct {
	Tool    string `json:"tool"`
	Message string `json:"message"`
}

type MechanicalFacts struct {
	FilesEdited []FileEdit    `json:"files_edited,omitempty"`
	TestRuns    []TestRunFact `json:"test_runs,omitempty"`
	Commits     []CommitFact  `json:"commits,omitempty"`
	Errors      []ErrorFact   `json:"errors,omitempty"`
}

type CheckpointJob struct {
	ID                    int64      `json:"id"`
	WorkSessionID         int64      `json:"work_session_id"`
	FeatureID             string     `json:"feature_id"`
	Reason                string     `json:"reason"`
	TriggerType           string     `json:"trigger_type"`
	TranscriptStartOffset int64      `json:"transcript_start_offset"`
	TranscriptEndOffset   int64      `json:"transcript_end_offset"`
	SemanticText          string     `json:"semantic_text"`
	MechanicalJSON        string     `json:"mechanical_json"`
	Status                string     `json:"status"`
	Error                 string     `json:"error"`
	RetryCount            int        `json:"retry_count"`
	CreatedAt             time.Time  `json:"created_at"`
	StartedAt             *time.Time `json:"started_at"`
	FinishedAt            *time.Time `json:"finished_at"`
}

type CheckpointJobInput struct {
	WorkSessionID         int64
	FeatureID             string
	Reason                string
	TriggerType           string
	TranscriptStartOffset int64
	TranscriptEndOffset   int64
	SemanticText          string
	MechanicalFacts       MechanicalFacts
}

type CheckpointObservation struct {
	ID              int64     `json:"id"`
	CheckpointJobID int64     `json:"checkpoint_job_id"`
	WorkSessionID   int64     `json:"work_session_id"`
	FeatureID       string    `json:"feature_id"`
	Kind            string    `json:"kind"`
	PayloadJSON     string    `json:"payload_json"`
	SummaryText     string    `json:"summary_text"`
	CreatedAt       time.Time `json:"created_at"`
}

type CheckpointObservationInput struct {
	CheckpointJobID int64
	WorkSessionID   int64
	FeatureID       string
	Kind            string
	PayloadJSON     string
	SummaryText     string
}

// EnqueueCheckpointJob inserts a checkpoint job. Idempotent — if a job with
// the same work_session_id + transcript offsets + feature_id already exists,
// returns the existing job.
func (s *Store) EnqueueCheckpointJob(input CheckpointJobInput) (*CheckpointJob, error) {
	// Idempotency check
	row := s.db.QueryRow(
		`SELECT id FROM checkpoint_jobs
         WHERE work_session_id = ? AND feature_id = ?
           AND transcript_start_offset = ? AND transcript_end_offset = ?`,
		input.WorkSessionID, input.FeatureID,
		input.TranscriptStartOffset, input.TranscriptEndOffset,
	)
	var existingID int64
	if err := row.Scan(&existingID); err == nil {
		return s.GetCheckpointJob(existingID)
	}

	mechJSON, _ := json.Marshal(input.MechanicalFacts)
	res, err := s.db.Exec(
		`INSERT INTO checkpoint_jobs
         (work_session_id, feature_id, reason, trigger_type,
          transcript_start_offset, transcript_end_offset,
          semantic_text, mechanical_json, status)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'queued')`,
		input.WorkSessionID, input.FeatureID, input.Reason, input.TriggerType,
		input.TranscriptStartOffset, input.TranscriptEndOffset,
		input.SemanticText, string(mechJSON),
	)
	if err != nil {
		return nil, fmt.Errorf("enqueue checkpoint job: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.GetCheckpointJob(id)
}

// DequeueCheckpointJob atomically picks the oldest queued job and marks it running.
func (s *Store) DequeueCheckpointJob() (*CheckpointJob, error) {
	row := s.db.QueryRow(
		`UPDATE checkpoint_jobs
		 SET status = 'running', started_at = datetime('now')
		 WHERE id = (
		     SELECT id FROM checkpoint_jobs
		     WHERE status = 'queued'
		     ORDER BY id ASC LIMIT 1
		 )
		 RETURNING id, work_session_id, feature_id, reason, trigger_type,
		           transcript_start_offset, transcript_end_offset,
		           semantic_text, mechanical_json, status, error,
		           retry_count, created_at, started_at, finished_at`,
	)
	job, err := scanCheckpointJob(row)
	if err != nil {
		return nil, nil // no queued jobs
	}
	return job, nil
}

func (s *Store) CompleteCheckpointJob(id int64, errMsg *string) error {
	if errMsg != nil {
		return s.FailCheckpointJob(id, *errMsg)
	}
	_, err := s.db.Exec(
		`UPDATE checkpoint_jobs SET status = 'done', finished_at = datetime('now') WHERE id = ?`, id,
	)
	return err
}

const maxCheckpointRetries = 3

func (s *Store) FailCheckpointJob(id int64, errMsg string) error {
	row := s.db.QueryRow(`SELECT retry_count FROM checkpoint_jobs WHERE id = ?`, id)
	var retryCount int
	if err := row.Scan(&retryCount); err != nil {
		return err
	}

	if retryCount < maxCheckpointRetries {
		_, err := s.db.Exec(
			`UPDATE checkpoint_jobs SET status = 'queued', retry_count = retry_count + 1, error = ?, started_at = NULL WHERE id = ?`,
			errMsg, id,
		)
		return err
	}

	_, err := s.db.Exec(
		`UPDATE checkpoint_jobs SET status = 'failed', error = ?, finished_at = datetime('now') WHERE id = ?`,
		errMsg, id,
	)
	return err
}

func scanCheckpointJob(row scannable) (*CheckpointJob, error) {
	var job CheckpointJob
	err := row.Scan(
		&job.ID, &job.WorkSessionID, &job.FeatureID, &job.Reason, &job.TriggerType,
		&job.TranscriptStartOffset, &job.TranscriptEndOffset,
		&job.SemanticText, &job.MechanicalJSON, &job.Status, &job.Error, &job.RetryCount,
		&job.CreatedAt, &job.StartedAt, &job.FinishedAt,
	)
	if err != nil {
		return nil, err
	}
	return &job, nil
}

func (s *Store) GetCheckpointJob(id int64) (*CheckpointJob, error) {
	row := s.db.QueryRow(
		`SELECT id, work_session_id, feature_id, reason, trigger_type,
                transcript_start_offset, transcript_end_offset,
                semantic_text, mechanical_json, status, error, retry_count,
                created_at, started_at, finished_at
         FROM checkpoint_jobs WHERE id = ?`, id,
	)
	job, err := scanCheckpointJob(row)
	if err != nil {
		return nil, fmt.Errorf("get checkpoint job %d: %w", id, err)
	}
	return job, nil
}

func (s *Store) AddCheckpointObservation(input CheckpointObservationInput) (*CheckpointObservation, error) {
	if input.PayloadJSON == "" {
		input.PayloadJSON = "{}"
	}
	res, err := s.db.Exec(
		`INSERT INTO checkpoint_observations
         (checkpoint_job_id, work_session_id, feature_id, kind, payload_json, summary_text)
         VALUES (?, ?, ?, ?, ?, ?)`,
		input.CheckpointJobID, input.WorkSessionID, input.FeatureID,
		input.Kind, input.PayloadJSON, input.SummaryText,
	)
	if err != nil {
		return nil, fmt.Errorf("add checkpoint observation: %w", err)
	}
	id, _ := res.LastInsertId()
	row := s.db.QueryRow(
		`SELECT id, checkpoint_job_id, work_session_id, feature_id, kind, payload_json, summary_text, created_at
         FROM checkpoint_observations WHERE id = ?`, id,
	)
	var obs CheckpointObservation
	err = row.Scan(&obs.ID, &obs.CheckpointJobID, &obs.WorkSessionID, &obs.FeatureID,
		&obs.Kind, &obs.PayloadJSON, &obs.SummaryText, &obs.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &obs, nil
}

func (s *Store) GetObservationsForWorkSession(workSessionID int64) ([]CheckpointObservation, error) {
	rows, err := s.db.Query(
		`SELECT id, checkpoint_job_id, work_session_id, feature_id, kind, payload_json, summary_text, created_at
         FROM checkpoint_observations WHERE work_session_id = ? ORDER BY id ASC`, workSessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var obs []CheckpointObservation
	for rows.Next() {
		var o CheckpointObservation
		if err := rows.Scan(&o.ID, &o.CheckpointJobID, &o.WorkSessionID, &o.FeatureID,
			&o.Kind, &o.PayloadJSON, &o.SummaryText, &o.CreatedAt); err != nil {
			return nil, err
		}
		obs = append(obs, o)
	}
	return obs, nil
}

// GetObservationsForFeature returns observations across all work sessions
// for a feature, ordered by ID DESC (newest first), limited to `limit` rows.
func (s *Store) GetObservationsForFeature(featureID string, limit int) ([]CheckpointObservation, error) {
	rows, err := s.db.Query(
		`SELECT id, checkpoint_job_id, work_session_id, feature_id, kind, payload_json, summary_text, created_at
		 FROM checkpoint_observations WHERE feature_id = ? ORDER BY id DESC LIMIT ?`,
		featureID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("get observations for feature: %w", err)
	}
	defer rows.Close()
	var obs []CheckpointObservation
	for rows.Next() {
		var o CheckpointObservation
		if err := rows.Scan(&o.ID, &o.CheckpointJobID, &o.WorkSessionID, &o.FeatureID,
			&o.Kind, &o.PayloadJSON, &o.SummaryText, &o.CreatedAt); err != nil {
			return nil, err
		}
		obs = append(obs, o)
	}
	return obs, nil
}

// UpdateFeatureSynthesis writes the synthesis text and high water mark observation ID.
func (s *Store) UpdateFeatureSynthesis(featureID string, synthesis string, obsID int64) error {
	res, err := s.db.Exec(
		`UPDATE features SET synthesis = ?, synthesis_obs_id = ?, updated_at = ? WHERE id = ?`,
		synthesis, obsID, time.Now().UTC(), featureID,
	)
	if err != nil {
		return fmt.Errorf("update feature synthesis: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("feature %q not found", featureID)
	}
	return nil
}

// GetMechanicalFactsForWorkSession merges mechanical facts from all checkpoint
// jobs in a work session into a single MechanicalFacts.
func (s *Store) GetMechanicalFactsForWorkSession(workSessionID int64) (*MechanicalFacts, error) {
	rows, err := s.db.Query(
		`SELECT mechanical_json FROM checkpoint_jobs WHERE work_session_id = ? ORDER BY id ASC`,
		workSessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	merged := &MechanicalFacts{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			continue
		}
		var facts MechanicalFacts
		if err := json.Unmarshal([]byte(raw), &facts); err != nil {
			continue
		}
		merged.FilesEdited = append(merged.FilesEdited, facts.FilesEdited...)
		merged.TestRuns = append(merged.TestRuns, facts.TestRuns...)
		merged.Commits = append(merged.Commits, facts.Commits...)
		merged.Errors = append(merged.Errors, facts.Errors...)
	}
	return merged, nil
}
