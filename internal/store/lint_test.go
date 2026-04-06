package store

import (
	"testing"
)

func TestLintBoardStaleFeature(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Stale Feature", "should be flagged")
	status := "in_progress"
	s.UpdateFeature("stale-feature", FeatureUpdate{Status: &status})

	// Backdate the feature and ensure no recent work sessions
	s.db.Exec(`UPDATE features SET updated_at = datetime('now', '-10 days') WHERE id = 'stale-feature'`)

	report, err := s.LintBoard()
	if err != nil {
		t.Fatalf("LintBoard: %v", err)
	}
	if len(report.Stale) != 1 {
		t.Fatalf("Stale = %d, want 1", len(report.Stale))
	}
	if report.Stale[0].ID != "stale-feature" {
		t.Errorf("Stale[0].ID = %q, want stale-feature", report.Stale[0].ID)
	}
}

func TestLintBoardGateBypass(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Bypassed Feature", "has unchecked items")
	st, _ := s.AddSubtask("bypassed-feature", "Phase 1", 1)
	s.AddTaskItem(st.ID, "Task 1", 1)
	// Mark as done with force
	done := "done"
	force := true
	reason := "testing"
	s.UpdateFeature("bypassed-feature", FeatureUpdate{Status: &done, Force: &force, ForceReason: &reason})

	report, err := s.LintBoard()
	if err != nil {
		t.Fatalf("LintBoard: %v", err)
	}
	if len(report.GateBypasses) != 1 {
		t.Fatalf("GateBypasses = %d, want 1", len(report.GateBypasses))
	}
}

func TestLintBoardEmptyFeature(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Empty Feature", "never touched")
	// Backdate creation
	s.db.Exec(`UPDATE features SET created_at = datetime('now', '-5 days') WHERE id = 'empty-feature'`)

	report, err := s.LintBoard()
	if err != nil {
		t.Fatalf("LintBoard: %v", err)
	}
	if len(report.Empty) != 1 {
		t.Fatalf("Empty = %d, want 1", len(report.Empty))
	}
}

func TestLintBoardStuckDevComplete(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Stuck Feature", "forgot to merge")
	status := "dev_complete"
	s.UpdateFeature("stuck-feature", FeatureUpdate{Status: &status})
	s.db.Exec(`UPDATE features SET updated_at = datetime('now', '-10 days') WHERE id = 'stuck-feature'`)

	report, err := s.LintBoard()
	if err != nil {
		t.Fatalf("LintBoard: %v", err)
	}
	if len(report.StuckDevComplete) != 1 {
		t.Fatalf("StuckDevComplete = %d, want 1", len(report.StuckDevComplete))
	}
}

func TestLintBoardAllClear(t *testing.T) {
	s := openTestStore(t)
	// No features at all
	report, err := s.LintBoard()
	if err != nil {
		t.Fatalf("LintBoard: %v", err)
	}
	if report.Total() != 0 {
		t.Errorf("Total = %d, want 0", report.Total())
	}
}
