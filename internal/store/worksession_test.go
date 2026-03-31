package store

import (
	"testing"
)

func TestOpenWorkSession(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Auth System", "token auth")

	ws, err := s.OpenWorkSession("auth-system", "session-123")
	if err != nil {
		t.Fatalf("OpenWorkSession: %v", err)
	}
	if ws.FeatureID != "auth-system" {
		t.Errorf("FeatureID = %q, want %q", ws.FeatureID, "auth-system")
	}
	if ws.ClaudeSessionID != "session-123" {
		t.Errorf("ClaudeSessionID = %q, want %q", ws.ClaudeSessionID, "session-123")
	}
	if ws.Status != "open" {
		t.Errorf("Status = %q, want %q", ws.Status, "open")
	}
}

func TestOpenWorkSessionResumesExisting(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Auth System", "token auth")

	ws1, _ := s.OpenWorkSession("auth-system", "session-123")
	ws2, _ := s.OpenWorkSession("auth-system", "session-123")

	if ws1.ID != ws2.ID {
		t.Errorf("expected same work session ID, got %d and %d", ws1.ID, ws2.ID)
	}
}

func TestGetActiveWorkSession(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Auth System", "token auth")

	_, err := s.GetActiveWorkSession()
	if err == nil {
		t.Fatal("expected error when no active work session")
	}

	s.OpenWorkSession("auth-system", "session-123")

	ws, err := s.GetActiveWorkSession()
	if err != nil {
		t.Fatalf("GetActiveWorkSession: %v", err)
	}
	if ws.FeatureID != "auth-system" {
		t.Errorf("FeatureID = %q, want %q", ws.FeatureID, "auth-system")
	}
}

func TestCloseWorkSession(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Auth System", "token auth")
	ws, _ := s.OpenWorkSession("auth-system", "session-123")

	err := s.CloseWorkSession(ws.ID)
	if err != nil {
		t.Fatalf("CloseWorkSession: %v", err)
	}

	_, err = s.GetActiveWorkSession()
	if err == nil {
		t.Fatal("expected no active work session after close")
	}
}

func TestMarkHandoffStale(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Auth System", "token auth")
	ws, _ := s.OpenWorkSession("auth-system", "session-123")

	s.MarkHandoffStale(ws.ID)
	ws2, _ := s.GetWorkSession(ws.ID)
	if !ws2.HandoffStale {
		t.Error("expected HandoffStale to be true")
	}
}
