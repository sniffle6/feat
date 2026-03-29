package store

import (
	"testing"
)

func TestAddAndGetDecisions(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	// Create a feature to attach decisions to
	f, err := s.AddFeature("Test Feature", "desc")
	if err != nil {
		t.Fatalf("AddFeature: %v", err)
	}

	// Add a rejected decision
	d1, err := s.AddDecision(f.ID, "Use websockets", "rejected", "Too complex for MVP")
	if err != nil {
		t.Fatalf("AddDecision: %v", err)
	}
	if d1.Approach != "Use websockets" {
		t.Errorf("got approach %q, want %q", d1.Approach, "Use websockets")
	}
	if d1.Outcome != "rejected" {
		t.Errorf("got outcome %q, want %q", d1.Outcome, "rejected")
	}
	if d1.Reason != "Too complex for MVP" {
		t.Errorf("got reason %q, want %q", d1.Reason, "Too complex for MVP")
	}

	// Add an accepted decision
	d2, err := s.AddDecision(f.ID, "Use polling", "accepted", "Simple and sufficient")
	if err != nil {
		t.Fatalf("AddDecision: %v", err)
	}

	// Retrieve all decisions
	decisions, err := s.GetDecisionsForFeature(f.ID)
	if err != nil {
		t.Fatalf("GetDecisionsForFeature: %v", err)
	}
	if len(decisions) != 2 {
		t.Fatalf("got %d decisions, want 2", len(decisions))
	}
	// Ordered by created_at DESC, so d2 first
	if decisions[0].ID != d2.ID {
		t.Errorf("expected newest decision first, got id %d want %d", decisions[0].ID, d2.ID)
	}
}

func TestAddDecisionInvalidOutcome(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	f, _ := s.AddFeature("Test Feature", "desc")

	_, err = s.AddDecision(f.ID, "Something", "maybe", "dunno")
	if err == nil {
		t.Fatal("expected error for invalid outcome, got nil")
	}
}

func TestAddDecisionInvalidFeature(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	_, err = s.AddDecision("nonexistent", "Something", "rejected", "reason")
	if err == nil {
		t.Fatal("expected error for nonexistent feature, got nil")
	}
}
