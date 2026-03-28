package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sniffle6/claude-docket/internal/store"
)

func TestRenderHandoff(t *testing.T) {
	data := &store.HandoffData{
		Feature: store.Feature{
			ID:        "auth-system",
			Title:     "Auth System",
			Status:    "in_progress",
			LeftOff:   "implementing refresh tokens",
			KeyFiles:  []string{"internal/auth/token.go", "internal/auth/middleware.go"},
			UpdatedAt: time.Date(2026, 3, 28, 14, 30, 0, 0, time.UTC),
		},
		Done:  2,
		Total: 5,
		NextTasks: []string{
			"Add refresh endpoint",
			"Auth middleware",
			"Integration tests",
		},
		SubtaskSummary: []store.HandoffSubtask{
			{Title: "Token handling", Done: 2, Total: 3},
			{Title: "Middleware", Done: 0, Total: 2},
		},
		RecentSessions: []store.Session{
			{Summary: "Started signing logic", CreatedAt: time.Date(2026, 3, 28, 14, 0, 0, 0, time.UTC)},
			{Summary: "Set up token struct", Commits: []string{"abc123"}, CreatedAt: time.Date(2026, 3, 28, 12, 0, 0, 0, time.UTC)},
		},
	}

	result := renderHandoff(data)

	checks := []string{
		"# Handoff: Auth System",
		"in_progress | Progress: 2/5",
		"2026-03-28 14:30",
		"## Left Off",
		"implementing refresh tokens",
		"## Next Tasks",
		"- [ ] Add refresh endpoint",
		"- [ ] Auth middleware",
		"- [ ] Integration tests",
		"## Key Files",
		"- internal/auth/token.go",
		"## Recent Activity",
		"Started signing logic",
		"Set up token struct [abc123]",
		"## Active Subtasks",
		"Token handling [2/3]",
		"Middleware [0/2]",
	}
	for _, want := range checks {
		if !strings.Contains(result, want) {
			t.Errorf("missing %q in output:\n%s", want, result)
		}
	}
}

func TestRenderHandoffMinimal(t *testing.T) {
	data := &store.HandoffData{
		Feature: store.Feature{
			ID:        "simple",
			Title:     "Simple Feature",
			Status:    "in_progress",
			KeyFiles:  []string{},
			UpdatedAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC),
		},
	}

	result := renderHandoff(data)

	if !strings.Contains(result, "# Handoff: Simple Feature") {
		t.Errorf("missing title in output:\n%s", result)
	}
	// Empty sections should be omitted
	for _, absent := range []string{"## Left Off", "## Next Tasks", "## Key Files", "## Recent Activity", "## Active Subtasks"} {
		if strings.Contains(result, absent) {
			t.Errorf("should omit empty section %q in output:\n%s", absent, result)
		}
	}
}

func TestWriteHandoffFile(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".docket"), 0755)

	data := &store.HandoffData{
		Feature: store.Feature{
			ID:        "test-feature",
			Title:     "Test Feature",
			Status:    "in_progress",
			KeyFiles:  []string{},
			UpdatedAt: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC),
		},
	}

	err := writeHandoffFile(dir, data)
	if err != nil {
		t.Fatalf("writeHandoffFile: %v", err)
	}

	path := filepath.Join(dir, ".docket", "handoff", "test-feature.md")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read handoff file: %v", err)
	}
	if !strings.Contains(string(content), "# Handoff: Test Feature") {
		t.Errorf("handoff file missing title:\n%s", content)
	}
}

func TestCleanStaleHandoffs(t *testing.T) {
	dir := t.TempDir()
	handoffDir := filepath.Join(dir, ".docket", "handoff")
	os.MkdirAll(handoffDir, 0755)

	// Create handoff files for two features
	os.WriteFile(filepath.Join(handoffDir, "active-feature.md"), []byte("active"), 0644)
	os.WriteFile(filepath.Join(handoffDir, "done-feature.md"), []byte("stale"), 0644)

	activeIDs := map[string]bool{"active-feature": true}
	cleanStaleHandoffs(dir, activeIDs)

	// Active should remain
	if _, err := os.Stat(filepath.Join(handoffDir, "active-feature.md")); err != nil {
		t.Error("active handoff file should still exist")
	}
	// Stale should be deleted
	if _, err := os.Stat(filepath.Join(handoffDir, "done-feature.md")); !os.IsNotExist(err) {
		t.Error("stale handoff file should be deleted")
	}
}
