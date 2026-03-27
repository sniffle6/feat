package mcp

import (
	"testing"

	"github.com/sniffyanimal/feat/internal/store"
)

func testStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestNewServerDoesNotPanic(t *testing.T) {
	s := testStore(t)
	srv := NewServer(s)
	if srv == nil {
		t.Fatal("NewServer returned nil")
	}
}

func TestFullWorkflowViaStore(t *testing.T) {
	s := testStore(t)

	// 1. Add a feature
	f, err := s.AddFeature("Bluetooth Panel", "BT device management")
	if err != nil {
		t.Fatalf("AddFeature: %v", err)
	}
	if f.ID != "bluetooth-panel" {
		t.Errorf("ID = %q", f.ID)
	}

	// 2. Update feature
	err = s.UpdateFeature("bluetooth-panel", store.FeatureUpdate{
		Status:       strPtr("in_progress"),
		LeftOff:      strPtr("handle disconnect events"),
		WorktreePath: strPtr("/tmp/worktrees/bluetooth-panel"),
		KeyFiles:     &[]string{"internal/wm/bluetooth.go", "internal/wm/wm.go"},
	})
	if err != nil {
		t.Fatalf("UpdateFeature: %v", err)
	}

	// 3. Log sessions
	_, err = s.LogSession(store.SessionInput{
		FeatureID: "bluetooth-panel",
		Summary:   "Added scanning overlay and device list",
	})
	if err != nil {
		t.Fatalf("LogSession: %v", err)
	}

	_, err = s.LogSession(store.SessionInput{
		FeatureID: "bluetooth-panel",
		Summary:   "Initial panel layout",
	})
	if err != nil {
		t.Fatalf("LogSession 2: %v", err)
	}

	// 4. Get context
	ctx, err := s.GetContext("bluetooth-panel")
	if err != nil {
		t.Fatalf("GetContext: %v", err)
	}
	if ctx.Feature.Status != "in_progress" {
		t.Errorf("Status = %q", ctx.Feature.Status)
	}
	if ctx.Feature.LeftOff != "handle disconnect events" {
		t.Errorf("LeftOff = %q", ctx.Feature.LeftOff)
	}
	if len(ctx.RecentSessions) != 2 {
		t.Errorf("Sessions = %d", len(ctx.RecentSessions))
	}

	// 5. List features
	features, err := s.ListFeatures("")
	if err != nil {
		t.Fatalf("ListFeatures: %v", err)
	}
	if len(features) != 1 {
		t.Errorf("Features = %d", len(features))
	}

	// 6. Verify MCP server creation with populated store
	srv := NewServer(s)
	if srv == nil {
		t.Fatal("NewServer returned nil after workflow")
	}
}

func strPtr(s string) *string { return &s }
