package mcp

import (
	"testing"

	"github.com/sniffle6/claude-docket/internal/store"
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

func TestNotesInWorkflow(t *testing.T) {
	s := testStore(t)

	f, err := s.AddFeature("Notes Feature", "testing notes in MCP")
	if err != nil {
		t.Fatalf("AddFeature: %v", err)
	}

	err = s.UpdateFeature(f.ID, store.FeatureUpdate{Notes: strPtr("user notes here")})
	if err != nil {
		t.Fatalf("UpdateFeature: %v", err)
	}

	f, _ = s.GetFeature(f.ID)
	if f.Notes != "user notes here" {
		t.Errorf("Notes = %q, want %q", f.Notes, "user notes here")
	}
}

func TestBatchAddSubtasks(t *testing.T) {
	s := testStore(t)
	s.AddFeature("Batch Test", "")

	// Add 3 subtasks at once
	s.AddSubtask("batch-test", "Phase 1", 1)
	s.AddSubtask("batch-test", "Phase 2", 2)
	s.AddSubtask("batch-test", "Phase 3", 3)

	subtasks, err := s.GetSubtasksForFeature("batch-test", false)
	if err != nil {
		t.Fatalf("GetSubtasksForFeature: %v", err)
	}
	if len(subtasks) != 3 {
		t.Fatalf("got %d subtasks, want 3", len(subtasks))
	}
	if subtasks[0].Title != "Phase 1" || subtasks[2].Title != "Phase 3" {
		t.Errorf("subtask titles wrong: %q, %q", subtasks[0].Title, subtasks[2].Title)
	}
}

func TestBatchAddTaskItems(t *testing.T) {
	s := testStore(t)
	s.AddFeature("Batch Items", "")
	st, _ := s.AddSubtask("batch-items", "Phase 1", 1)

	// Add 3 items at once
	s.AddTaskItem(st.ID, "Item A", 1)
	s.AddTaskItem(st.ID, "Item B", 2)
	s.AddTaskItem(st.ID, "Item C", 3)

	items, err := s.GetTaskItemsForSubtask(st.ID)
	if err != nil {
		t.Fatalf("GetTaskItemsForSubtask: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("got %d items, want 3", len(items))
	}
	if items[0].Title != "Item A" || items[2].Title != "Item C" {
		t.Errorf("item titles wrong: %q, %q", items[0].Title, items[2].Title)
	}
}

func TestDevCompleteStatusInWorkflow(t *testing.T) {
	s := testStore(t)

	s.AddFeature("Dev Done Feature", "")
	err := s.UpdateFeature("dev-done-feature", store.FeatureUpdate{Status: strPtr("dev_complete")})
	if err != nil {
		t.Fatalf("UpdateFeature: %v", err)
	}

	f, _ := s.GetFeature("dev-done-feature")
	if f.Status != "dev_complete" {
		t.Errorf("Status = %q, want dev_complete", f.Status)
	}
}
