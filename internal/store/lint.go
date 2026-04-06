package store

import "fmt"

// LintFinding is a single issue found by the board lint.
type LintFinding struct {
	ID     string
	Title  string
	Detail string // e.g., "last activity: 2026-03-28" or "3 unchecked items"
}

// LintReport holds all findings from a board health check.
type LintReport struct {
	Stale            []LintFinding
	GateBypasses     []LintFinding
	Empty            []LintFinding
	StuckDevComplete []LintFinding
}

// Total returns the total number of findings.
func (r *LintReport) Total() int {
	return len(r.Stale) + len(r.GateBypasses) + len(r.Empty) + len(r.StuckDevComplete)
}

// LintBoard runs all health checks and returns a combined report.
func (s *Store) LintBoard() (*LintReport, error) {
	report := &LintReport{}

	// 1. Stale features: in_progress with no work sessions in 7+ days
	//    Exclude features with currently-open sessions (long-running but active)
	rows, err := s.db.Query(`
		SELECT f.id, f.title, MAX(ws.started_at) as last_activity
		FROM features f
		LEFT JOIN work_sessions ws ON ws.feature_id = f.id
		WHERE f.status = 'in_progress'
		AND f.id NOT IN (SELECT feature_id FROM work_sessions WHERE status = 'open')
		GROUP BY f.id
		HAVING last_activity IS NULL OR last_activity < datetime('now', '-7 days')
	`)
	if err != nil {
		return nil, fmt.Errorf("lint stale: %w", err)
	}
	for rows.Next() {
		var id, title string
		var lastActivity *string
		rows.Scan(&id, &title, &lastActivity)
		detail := "no sessions"
		if lastActivity != nil {
			detail = fmt.Sprintf("last activity: %s", (*lastActivity)[:10])
		}
		report.Stale = append(report.Stale, LintFinding{ID: id, Title: title, Detail: detail})
	}
	rows.Close()

	// 2. Gate bypasses: done features with unchecked task items
	rows, err = s.db.Query(`
		SELECT f.id, f.title, COUNT(*) as unchecked
		FROM features f
		JOIN subtasks st ON st.feature_id = f.id AND st.archived = 0
		JOIN task_items ti ON ti.subtask_id = st.id
		WHERE f.status = 'done' AND ti.checked = 0
		GROUP BY f.id
	`)
	if err != nil {
		return nil, fmt.Errorf("lint gate bypasses: %w", err)
	}
	for rows.Next() {
		var id, title string
		var unchecked int
		rows.Scan(&id, &title, &unchecked)
		report.GateBypasses = append(report.GateBypasses, LintFinding{
			ID: id, Title: title, Detail: fmt.Sprintf("%d unchecked items", unchecked),
		})
	}
	rows.Close()

	// 3. Empty features: not done/archived, no sessions, no subtasks, no notes, 3+ days old
	rows, err = s.db.Query(`
		SELECT f.id, f.title FROM features f
		WHERE f.status NOT IN ('done', 'archived')
		AND f.id NOT IN (SELECT feature_id FROM work_sessions)
		AND f.id NOT IN (SELECT feature_id FROM subtasks)
		AND f.id NOT IN (SELECT feature_id FROM notes)
		AND f.created_at < datetime('now', '-3 days')
	`)
	if err != nil {
		return nil, fmt.Errorf("lint empty: %w", err)
	}
	for rows.Next() {
		var id, title string
		rows.Scan(&id, &title)
		report.Empty = append(report.Empty, LintFinding{ID: id, Title: title, Detail: "no activity"})
	}
	rows.Close()

	// 4. Stuck dev_complete: dev_complete for 7+ days
	rows, err = s.db.Query(`
		SELECT id, title, updated_at FROM features
		WHERE status = 'dev_complete'
		AND updated_at < datetime('now', '-7 days')
	`)
	if err != nil {
		return nil, fmt.Errorf("lint stuck dev_complete: %w", err)
	}
	for rows.Next() {
		var id, title, updatedAt string
		rows.Scan(&id, &title, &updatedAt)
		report.StuckDevComplete = append(report.StuckDevComplete, LintFinding{
			ID: id, Title: title, Detail: fmt.Sprintf("since: %s", updatedAt[:10]),
		})
	}
	rows.Close()

	return report, nil
}
