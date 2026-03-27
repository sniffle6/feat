package store

import "testing"

func TestLogSession(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Bluetooth Panel", "")
	sess, err := s.LogSession(SessionInput{
		FeatureID:    "bluetooth-panel",
		Summary:      "Added scanning overlay",
		FilesTouched: []string{"internal/wm/bluetooth.go"},
		Commits:      []string{"abc1234"},
		AutoLinked:   true,
		LinkReason:   "user loaded via get_context",
	})
	if err != nil {
		t.Fatalf("LogSession: %v", err)
	}
	if sess.ID == 0 {
		t.Error("expected non-zero session ID")
	}
	if sess.FeatureID != "bluetooth-panel" {
		t.Errorf("FeatureID = %q, want %q", sess.FeatureID, "bluetooth-panel")
	}
}

func TestLogSessionUnlinked(t *testing.T) {
	s := openTestStore(t)
	sess, err := s.LogSession(SessionInput{
		Summary:      "Explored codebase",
		FilesTouched: []string{"README.md"},
	})
	if err != nil {
		t.Fatalf("LogSession: %v", err)
	}
	if sess.FeatureID != "" {
		t.Errorf("FeatureID = %q, want empty", sess.FeatureID)
	}
}

func TestGetSessionsForFeature(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Web Browser", "")
	s.LogSession(SessionInput{FeatureID: "web-browser", Summary: "session 1"})
	s.LogSession(SessionInput{FeatureID: "web-browser", Summary: "session 2"})
	s.LogSession(SessionInput{Summary: "unlinked session"})
	sessions, err := s.GetSessionsForFeature("web-browser")
	if err != nil {
		t.Fatalf("GetSessionsForFeature: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("got %d sessions, want 2", len(sessions))
	}
}

func TestGetUnlinkedSessions(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Feature A", "")
	s.LogSession(SessionInput{FeatureID: "feature-a", Summary: "linked"})
	s.LogSession(SessionInput{Summary: "unlinked 1"})
	s.LogSession(SessionInput{Summary: "unlinked 2"})
	sessions, err := s.GetUnlinkedSessions()
	if err != nil {
		t.Fatalf("GetUnlinkedSessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("got %d sessions, want 2", len(sessions))
	}
}

func TestReassignSession(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Feature A", "")
	sess, _ := s.LogSession(SessionInput{Summary: "unlinked"})
	err := s.ReassignSession(sess.ID, "feature-a")
	if err != nil {
		t.Fatalf("ReassignSession: %v", err)
	}
	sessions, _ := s.GetSessionsForFeature("feature-a")
	if len(sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(sessions))
	}
}
