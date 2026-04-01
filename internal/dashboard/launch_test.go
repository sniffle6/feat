package dashboard

import (
	"strings"
	"testing"

	"github.com/sniffle6/claude-docket/internal/store"
)

func TestRenderLaunchPrompt(t *testing.T) {
	data := &store.LaunchData{
		Feature: store.Feature{
			ID:          "dashboard-writes",
			Title:       "Dashboard Writes",
			Description: "Add write operations to dashboard",
			Status:      "in_progress",
			LeftOff:     "Finished hook design",
			Notes:       "User wants Warp support",
			KeyFiles:    []string{"cmd/docket/hook.go", "dashboard/index.html"},
		},
		TaskItems: []store.TaskItem{
			{ID: 1, Title: "Update Stop hook"},
			{ID: 2, Title: "Add launch endpoint"},
		},
		Issues: []store.Issue{
			{ID: 1, Description: "Theme toggle broken"},
		},
		Subtasks: []store.Subtask{
			{ID: 1, Title: "Hook changes"},
		},
	}

	result := RenderLaunchPrompt(data)

	checks := []string{
		"# Resuming: Dashboard Writes",
		"in_progress",
		"Finished hook design",
		"Update Stop hook",
		"Add launch endpoint",
		"cmd/docket/hook.go",
		"Theme toggle broken",
		"User wants Warp support",
	}
	for _, check := range checks {
		if !strings.Contains(result, check) {
			t.Errorf("launch prompt missing %q", check)
		}
	}
}

func TestRenderLaunchPrompt_Empty(t *testing.T) {
	data := &store.LaunchData{
		Feature: store.Feature{
			ID:       "empty",
			Title:    "Empty Feature",
			Status:   "planned",
			KeyFiles: []string{},
		},
		TaskItems: []store.TaskItem{},
		Issues:    []store.Issue{},
		Subtasks:  []store.Subtask{},
	}

	result := RenderLaunchPrompt(data)

	if !strings.Contains(result, "# Resuming: Empty Feature") {
		t.Error("missing title")
	}
	// Should not contain section headers for empty sections
	if strings.Contains(result, "## Remaining Tasks") {
		t.Error("should not show Remaining Tasks when empty")
	}
	if strings.Contains(result, "## Open Issues") {
		t.Error("should not show Open Issues when empty")
	}
}
