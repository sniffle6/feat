package store

import (
	"fmt"
	"testing"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestAddFeature(t *testing.T) {
	s := openTestStore(t)
	f, err := s.AddFeature("Bluetooth Panel", "BT device management overlay")
	if err != nil {
		t.Fatalf("AddFeature: %v", err)
	}
	if f.ID != "bluetooth-panel" {
		t.Errorf("ID = %q, want %q", f.ID, "bluetooth-panel")
	}
	if f.Status != "planned" {
		t.Errorf("Status = %q, want %q", f.Status, "planned")
	}
}

func TestAddFeatureDuplicateSlug(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Bluetooth Panel", "first")
	_, err := s.AddFeature("Bluetooth Panel", "second")
	if err == nil {
		t.Fatal("expected error for duplicate slug")
	}
}

func TestGetFeature(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Web Browser", "w3m integration")
	f, err := s.GetFeature("web-browser")
	if err != nil {
		t.Fatalf("GetFeature: %v", err)
	}
	if f.Title != "Web Browser" {
		t.Errorf("Title = %q, want %q", f.Title, "Web Browser")
	}
}

func TestGetFeatureNotFound(t *testing.T) {
	s := openTestStore(t)
	_, err := s.GetFeature("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent feature")
	}
}

func TestUpdateFeature(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Settings Menu", "user preferences")
	err := s.UpdateFeature("settings-menu", FeatureUpdate{
		Status:  strPtr("in_progress"),
		LeftOff: strPtr("need to add save button"),
	})
	if err != nil {
		t.Fatalf("UpdateFeature: %v", err)
	}
	f, _ := s.GetFeature("settings-menu")
	if f.Status != "in_progress" {
		t.Errorf("Status = %q, want %q", f.Status, "in_progress")
	}
	if f.LeftOff != "need to add save button" {
		t.Errorf("LeftOff = %q, want %q", f.LeftOff, "need to add save button")
	}
}

func TestListFeatures(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Feature A", "")
	s.AddFeature("Feature B", "")
	s.UpdateFeature("feature-b", FeatureUpdate{Status: strPtr("in_progress")})

	all, _ := s.ListFeatures("")
	if len(all) != 2 {
		t.Fatalf("ListFeatures('') = %d, want 2", len(all))
	}

	inProgress, _ := s.ListFeatures("in_progress")
	if len(inProgress) != 1 {
		t.Fatalf("ListFeatures('in_progress') = %d, want 1", len(inProgress))
	}
}

func strPtr(s string) *string { return &s }

func TestGetContext(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Bluetooth Panel", "BT device management")
	s.UpdateFeature("bluetooth-panel", FeatureUpdate{
		Status:       strPtr("in_progress"),
		LeftOff:      strPtr("handle disconnect events"),
		WorktreePath: strPtr("/tmp/worktrees/bluetooth-panel"),
		KeyFiles:     &[]string{"internal/wm/bluetooth.go", "internal/wm/wm.go"},
	})
	s.LogSession(SessionInput{FeatureID: "bluetooth-panel", Summary: "Added scanning overlay"})
	s.LogSession(SessionInput{FeatureID: "bluetooth-panel", Summary: "Initial panel layout"})

	ctx, err := s.GetContext("bluetooth-panel")
	if err != nil {
		t.Fatalf("GetContext: %v", err)
	}
	if ctx.Feature.Status != "in_progress" {
		t.Errorf("Status = %q", ctx.Feature.Status)
	}
	if len(ctx.RecentSessions) != 2 {
		t.Errorf("RecentSessions = %d, want 2", len(ctx.RecentSessions))
	}
}

func TestGetContextNotFound(t *testing.T) {
	s := openTestStore(t)
	_, err := s.GetContext("nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetReadyFeatures(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Done Feature", "")
	s.UpdateFeature("done-feature", FeatureUpdate{Status: strPtr("done")})
	s.AddFeature("Blocked Feature", "")
	s.UpdateFeature("blocked-feature", FeatureUpdate{Status: strPtr("blocked")})
	s.AddFeature("Active Feature", "")
	s.UpdateFeature("active-feature", FeatureUpdate{Status: strPtr("in_progress")})
	s.AddFeature("Planned Feature", "")

	ready, err := s.GetReadyFeatures()
	if err != nil {
		t.Fatalf("GetReadyFeatures: %v", err)
	}
	if len(ready) != 2 {
		t.Fatalf("got %d ready, want 2", len(ready))
	}
	// in_progress should come first
	if ready[0].Status != "in_progress" {
		t.Errorf("first ready status = %q, want in_progress", ready[0].Status)
	}
	if ready[1].Status != "planned" {
		t.Errorf("second ready status = %q, want planned", ready[1].Status)
	}
}

func TestCompactSessions(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Big Feature", "")

	// Log 6 sessions
	for i := 0; i < 6; i++ {
		s.LogSession(SessionInput{
			FeatureID: "big-feature",
			Summary:   fmt.Sprintf("session %d", i+1),
		})
	}

	sessions, _ := s.GetSessionsForFeature("big-feature")
	if len(sessions) != 6 {
		t.Fatalf("pre-compact: got %d sessions, want 6", len(sessions))
	}

	n, err := s.CompactSessions("big-feature", "Prior work: sessions 1-3 did initial setup")
	if err != nil {
		t.Fatalf("CompactSessions: %v", err)
	}
	if n != 3 {
		t.Errorf("compacted %d, want 3", n)
	}

	sessions, _ = s.GetSessionsForFeature("big-feature")
	// 3 kept + 1 compacted = 4
	if len(sessions) != 4 {
		t.Fatalf("post-compact: got %d sessions, want 4", len(sessions))
	}
}

func TestCompactSessionsTooFew(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Small Feature", "")
	s.LogSession(SessionInput{FeatureID: "small-feature", Summary: "only one"})

	n, err := s.CompactSessions("small-feature", "summary")
	if err != nil {
		t.Fatalf("CompactSessions: %v", err)
	}
	if n != 0 {
		t.Errorf("compacted %d, want 0", n)
	}
}
