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

func TestSetSessionState(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Auth System", "token auth")
	ws, _ := s.OpenWorkSession("auth-system", "session-123")

	if err := s.SetSessionState(ws.ID, "working"); err != nil {
		t.Fatalf("SetSessionState(working): %v", err)
	}
	ws2, _ := s.GetWorkSession(ws.ID)
	if ws2.SessionState != "working" {
		t.Errorf("SessionState = %q, want %q", ws2.SessionState, "working")
	}

	if err := s.SetSessionState(ws.ID, "needs_attention"); err != nil {
		t.Fatalf("SetSessionState(needs_attention): %v", err)
	}
	ws3, _ := s.GetWorkSession(ws.ID)
	if ws3.SessionState != "needs_attention" {
		t.Errorf("SessionState = %q, want %q", ws3.SessionState, "needs_attention")
	}
}

func TestSetSessionState_ClosedSession(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Auth System", "token auth")
	ws, _ := s.OpenWorkSession("auth-system", "session-123")
	s.CloseWorkSession(ws.ID)

	err := s.SetSessionState(ws.ID, "working")
	if err == nil {
		t.Fatal("expected error setting state on closed session")
	}
}

func TestGetActiveSessionStates(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Feature A", "")
	s.AddFeature("Feature B", "")
	s.AddFeature("Feature C", "")

	// A: working session
	wsA, _ := s.OpenWorkSession("feature-a", "session-1")
	s.SetSessionState(wsA.ID, "working")

	// B: needs_attention session
	// Opening B closes A (one active session at a time), so we get B only
	wsB, _ := s.OpenWorkSession("feature-b", "session-2")
	s.SetSessionState(wsB.ID, "needs_attention")

	// C: no session at all

	states, err := s.GetActiveSessionStates()
	if err != nil {
		t.Fatalf("GetActiveSessionStates: %v", err)
	}

	// B should be present (it's the latest open session with non-idle state)
	if states["feature-b"].State != "needs_attention" {
		t.Errorf("feature-b state = %q, want %q", states["feature-b"].State, "needs_attention")
	}

	// C should be absent
	if _, ok := states["feature-c"]; ok {
		t.Error("feature-c should not be in active session states")
	}
}

func TestGetActiveSessionStates_ExcludesIdle(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Feature A", "")
	ws, _ := s.OpenWorkSession("feature-a", "session-1")
	s.SetSessionState(ws.ID, "idle")

	states, err := s.GetActiveSessionStates()
	if err != nil {
		t.Fatalf("GetActiveSessionStates: %v", err)
	}
	if _, ok := states["feature-a"]; ok {
		t.Error("idle sessions should not appear in active session states")
	}
}

func TestTouchHeartbeat(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Auth System", "token auth")
	ws, _ := s.OpenWorkSession("auth-system", "session-123")

	s.TouchHeartbeat(ws.ID)

	ws2, _ := s.GetWorkSession(ws.ID)
	if ws2.LastHeartbeat == nil {
		t.Fatal("expected LastHeartbeat to be set after TouchHeartbeat")
	}
}

func TestGetActiveSessionStatesReturnsHeartbeat(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Feature A", "")
	ws, _ := s.OpenWorkSession("feature-a", "session-1")
	s.SetSessionState(ws.ID, "working")
	s.TouchHeartbeat(ws.ID)

	states, err := s.GetActiveSessionStates()
	if err != nil {
		t.Fatalf("GetActiveSessionStates: %v", err)
	}
	info, ok := states["feature-a"]
	if !ok {
		t.Fatal("expected feature-a in active session states")
	}
	if info.State != "working" {
		t.Errorf("State = %q, want %q", info.State, "working")
	}
	if info.LastHeartbeat == nil {
		t.Fatal("expected LastHeartbeat to be set")
	}
}

func TestOpenWorkSessionSetsHeartbeat(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Auth System", "token auth")

	ws, err := s.OpenWorkSession("auth-system", "session-123")
	if err != nil {
		t.Fatalf("OpenWorkSession: %v", err)
	}
	if ws.LastHeartbeat == nil {
		t.Fatal("expected LastHeartbeat to be set on new work session")
	}
}
