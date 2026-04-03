package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
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
	srv := NewServer(s, t.TempDir(), nil)
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
	srv := NewServer(s, t.TempDir(), nil)
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

func TestDeleteFeature(t *testing.T) {
	s := testStore(t)

	// Create feature with related data
	f, err := s.AddFeature("Delete Me", "to be deleted")
	if err != nil {
		t.Fatalf("AddFeature: %v", err)
	}

	// Add subtask with task items
	st, _ := s.AddSubtask(f.ID, "Phase 1", 1)
	s.AddTaskItem(st.ID, "Item A", 1)
	s.AddTaskItem(st.ID, "Item B", 2)

	// Add decision, note, issue
	s.AddDecision(f.ID, "use REST", "accepted", "simpler")
	s.AddNote(f.ID, "some note")
	s.AddIssue(f.ID, "a bug", nil)

	// Add session
	s.LogSession(store.SessionInput{
		FeatureID: f.ID,
		Summary:   "did stuff",
	})

	// Add work session
	s.OpenWorkSession(f.ID, "claude-123")

	// Delete
	err = s.DeleteFeature(f.ID)
	if err != nil {
		t.Fatalf("DeleteFeature: %v", err)
	}

	// Verify feature is gone
	_, err = s.GetFeature(f.ID)
	if err == nil {
		t.Error("expected error getting deleted feature")
	}

	// Verify subtasks gone
	subtasks, _ := s.GetSubtasksForFeature(f.ID, true)
	if len(subtasks) != 0 {
		t.Errorf("expected 0 subtasks, got %d", len(subtasks))
	}

	// Verify sessions gone
	sessions, _ := s.GetSessionsForFeature(f.ID)
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}

	// Verify decisions gone
	decisions, _ := s.GetDecisionsForFeature(f.ID)
	if len(decisions) != 0 {
		t.Errorf("expected 0 decisions, got %d", len(decisions))
	}

	// Verify notes gone
	notes, _ := s.GetNotesForFeature(f.ID)
	if len(notes) != 0 {
		t.Errorf("expected 0 notes, got %d", len(notes))
	}

	// Verify issues gone
	issues, _ := s.GetIssuesForFeature(f.ID)
	if len(issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(issues))
	}

	// Verify task items gone
	items, _ := s.GetTaskItemsForSubtask(st.ID)
	if len(items) != 0 {
		t.Errorf("expected 0 task items, got %d", len(items))
	}

	// Verify work sessions gone
	ws, _ := s.GetOpenWorkSessionForFeature(f.ID)
	if ws != nil {
		t.Error("expected no open work session after delete")
	}
}

func TestDeleteFeatureNotFound(t *testing.T) {
	s := testStore(t)

	err := s.DeleteFeature("nonexistent")
	if err == nil {
		t.Error("expected error deleting nonexistent feature")
	}
}

func TestDeleteFeatureMCPConfirmRequired(t *testing.T) {
	s := testStore(t)
	s.AddFeature("Confirm Test", "")

	handler := deleteFeatureHandler(s)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"id":      "confirm-test",
		"confirm": false,
	}
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "confirm") {
		t.Errorf("expected confirmation message, got: %s", text)
	}
}

func TestDeleteFeatureMCPSuccess(t *testing.T) {
	s := testStore(t)
	s.AddFeature("Delete Via MCP", "")

	handler := deleteFeatureHandler(s)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"id":      "delete-via-mcp",
		"confirm": true,
	}
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Deleted") {
		t.Errorf("expected success message, got: %s", text)
	}

	_, getErr := s.GetFeature("delete-via-mcp")
	if getErr == nil {
		t.Error("feature should be deleted")
	}
}

func TestComputeKeyFileOverlaps_NoOverlap(t *testing.T) {
	features := []store.Feature{
		{ID: "feature-a", KeyFiles: []string{"a.go", "b.go"}},
		{ID: "feature-b", KeyFiles: []string{"c.go", "d.go"}},
	}
	overlaps := computeKeyFileOverlaps(features)
	if len(overlaps) != 0 {
		t.Errorf("expected no overlaps, got %v", overlaps)
	}
}

func TestComputeKeyFileOverlaps_SingleOverlap(t *testing.T) {
	features := []store.Feature{
		{ID: "feature-a", KeyFiles: []string{"shared.go", "a.go"}},
		{ID: "feature-b", KeyFiles: []string{"shared.go", "b.go"}},
	}
	overlaps := computeKeyFileOverlaps(features)
	if len(overlaps) != 1 {
		t.Fatalf("expected 1 overlap, got %d", len(overlaps))
	}
	ids := overlaps["shared.go"]
	if len(ids) != 2 || ids[0] != "feature-a" || ids[1] != "feature-b" {
		t.Errorf("unexpected overlap: %v", ids)
	}
}

func TestComputeKeyFileOverlaps_MultipleOverlaps(t *testing.T) {
	features := []store.Feature{
		{ID: "feature-a", KeyFiles: []string{"shared.go", "also.go"}},
		{ID: "feature-b", KeyFiles: []string{"shared.go", "also.go"}},
		{ID: "feature-c", KeyFiles: []string{"shared.go", "unique.go"}},
	}
	overlaps := computeKeyFileOverlaps(features)
	if len(overlaps) != 2 {
		t.Fatalf("expected 2 overlaps, got %d: %v", len(overlaps), overlaps)
	}
	if len(overlaps["shared.go"]) != 3 {
		t.Errorf("shared.go should have 3 features, got %v", overlaps["shared.go"])
	}
	if len(overlaps["also.go"]) != 2 {
		t.Errorf("also.go should have 2 features, got %v", overlaps["also.go"])
	}
}

func TestComputeKeyFileOverlaps_EmptyKeyFiles(t *testing.T) {
	features := []store.Feature{
		{ID: "feature-a", KeyFiles: []string{}},
		{ID: "feature-b", KeyFiles: []string{"a.go"}},
	}
	overlaps := computeKeyFileOverlaps(features)
	if len(overlaps) != 0 {
		t.Errorf("expected no overlaps, got %v", overlaps)
	}
}

func TestFormatOverlapWarning_Empty(t *testing.T) {
	result := formatOverlapWarning(nil)
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestFormatOverlapWarning_WithOverlaps(t *testing.T) {
	overlaps := map[string][]string{
		"store.go": {"feature-a", "feature-b"},
	}
	result := formatOverlapWarning(overlaps)
	if !strings.Contains(result, "Key file conflicts") {
		t.Errorf("expected warning header, got %q", result)
	}
	if !strings.Contains(result, "store.go") {
		t.Errorf("expected file name in output, got %q", result)
	}
	if !strings.Contains(result, "feature-a") || !strings.Contains(result, "feature-b") {
		t.Errorf("expected feature IDs in output, got %q", result)
	}
}
