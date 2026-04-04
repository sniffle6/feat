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
	// Opening B does NOT close A (feature-scoped: different features stay open)
	wsB, _ := s.OpenWorkSession("feature-b", "session-2")
	s.SetSessionState(wsB.ID, "needs_attention")

	// C: no session at all

	states, err := s.GetActiveSessionStates()
	if err != nil {
		t.Fatalf("GetActiveSessionStates: %v", err)
	}

	// A should be present (feature-scoped close means it stays open)
	if states["feature-a"].State != "working" {
		t.Errorf("feature-a state = %q, want %q", states["feature-a"].State, "working")
	}

	// B should be present
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

func TestOpenWorkSessionUpgradesPlaceholder(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Tag Editing", "dashboard tag editing")

	// Simulate dashboard launch: creates placeholder session with "launching" state
	s.CreatePlaceholderSession("tag-editing")
	ph, _ := s.GetOpenWorkSessionForFeature("tag-editing")
	if ph == nil {
		t.Fatal("expected placeholder session")
	}
	if ph.SessionState != "launching" {
		t.Errorf("placeholder SessionState = %q, want %q", ph.SessionState, "launching")
	}
	if ph.ClaudeSessionID != "dashboard-launch" {
		t.Errorf("placeholder ClaudeSessionID = %q, want %q", ph.ClaudeSessionID, "dashboard-launch")
	}

	// Simulate real Claude session starting: should upgrade placeholder, not create new
	ws, err := s.OpenWorkSession("tag-editing", "real-session-abc")
	if err != nil {
		t.Fatalf("OpenWorkSession: %v", err)
	}
	if ws.ID != ph.ID {
		t.Errorf("expected upgrade (same ID %d), got new session %d", ph.ID, ws.ID)
	}
	if ws.ClaudeSessionID != "real-session-abc" {
		t.Errorf("ClaudeSessionID = %q, want %q", ws.ClaudeSessionID, "real-session-abc")
	}
	// State should still be "launching" — caller sets to "working"
	if ws.SessionState != "launching" {
		t.Errorf("SessionState = %q, want %q (preserved from placeholder)", ws.SessionState, "launching")
	}
}

func TestGetWorkSessionByClaudeSession(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Feature A", "")
	s.AddFeature("Feature B", "")

	// Create placeholder for A (dashboard launch)
	s.CreatePlaceholderSession("feature-a")

	// Create real session for B
	wsB, _ := s.OpenWorkSession("feature-b", "session-xyz")

	// GetWorkSessionByClaudeSession should find B's session, not A's placeholder
	ws, err := s.GetWorkSessionByClaudeSession("session-xyz")
	if err != nil {
		t.Fatalf("GetWorkSessionByClaudeSession: %v", err)
	}
	if ws.ID != wsB.ID {
		t.Errorf("got session %d, want %d (session-xyz)", ws.ID, wsB.ID)
	}

	// Querying for unknown session returns error — no fallback to unrelated sessions
	_, err = s.GetWorkSessionByClaudeSession("unknown-session")
	if err == nil {
		t.Error("expected error for unknown session, got nil")
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

func TestOpenWorkSessionFeatureScoped(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Feature A", "")
	s.AddFeature("Feature B", "")

	wsA, err := s.OpenWorkSession("feature-a", "session-1")
	if err != nil {
		t.Fatal(err)
	}

	wsB, err := s.OpenWorkSession("feature-b", "session-2")
	if err != nil {
		t.Fatal(err)
	}

	// Session A should still be open
	reloaded, err := s.GetWorkSession(wsA.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Status != "open" {
		t.Errorf("session A should still be open, got %q", reloaded.Status)
	}

	// Both sessions open
	if wsB.Status != "open" {
		t.Errorf("session B should be open, got %q", wsB.Status)
	}
}

func TestOpenWorkSessionSupersedes(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Feature A", "")

	wsA, err := s.OpenWorkSession("feature-a", "session-1")
	if err != nil {
		t.Fatal(err)
	}

	wsB, err := s.OpenWorkSession("feature-a", "session-2")
	if err != nil {
		t.Fatal(err)
	}

	// Session A should be closed (superseded)
	reloaded, err := s.GetWorkSession(wsA.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Status != "closed" {
		t.Errorf("session A should be closed, got %q", reloaded.Status)
	}
	if wsB.Status != "open" {
		t.Errorf("session B should be open, got %q", wsB.Status)
	}
}

func TestSetMcpPid(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Feature A", "")

	ws, _ := s.OpenWorkSession("feature-a", "session-1")
	if ws.McpPid != nil {
		t.Fatalf("expected nil McpPid, got %v", *ws.McpPid)
	}

	pid := int64(12345)
	if err := s.SetMcpPid(ws.ID, &pid); err != nil {
		t.Fatal(err)
	}

	reloaded, _ := s.GetWorkSession(ws.ID)
	if reloaded.McpPid == nil || *reloaded.McpPid != 12345 {
		t.Errorf("expected McpPid=12345, got %v", reloaded.McpPid)
	}

	// Clear it
	if err := s.SetMcpPid(ws.ID, nil); err != nil {
		t.Fatal(err)
	}
	reloaded, _ = s.GetWorkSession(ws.ID)
	if reloaded.McpPid != nil {
		t.Errorf("expected nil McpPid after clear, got %v", *reloaded.McpPid)
	}
}
