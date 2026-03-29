package main

import (
	"os"
	"strings"
	"testing"

	"github.com/sniffle6/claude-docket/internal/store"
)

func setupTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestRenderExport_PopulatedFeature(t *testing.T) {
	s := setupTestStore(t)

	f, err := s.AddFeature("Test Feature", "A test feature for export")
	if err != nil {
		t.Fatalf("add feature: %v", err)
	}

	// Add subtask with task items
	st, _ := s.AddSubtask(f.ID, "Setup", 0)
	s.AddTaskItem(st.ID, "Create file", 0)
	item2, _ := s.AddTaskItem(st.ID, "Write code", 1)
	s.CompleteTaskItem(item2.ID, store.TaskItemCompletion{
		Outcome:    "done",
		CommitHash: "abc123",
	})

	// Add a decision
	s.AddDecision(f.ID, "Use stdout by default", "accepted", "simpler UX")

	// Log a session
	s.LogSession(store.SessionInput{
		FeatureID: f.ID,
		Summary:   "Initial setup",
		Commits:   []string{"abc123"},
	})

	md, err := renderExport(s, f.ID)
	if err != nil {
		t.Fatalf("renderExport: %v", err)
	}

	// Check all major sections are present
	checks := []string{
		"# Test Feature",
		"**Status:** planned",
		"A test feature for export",
		"### Setup",
		"- [ ] Create file",
		"- [x] Write code (`abc123`)",
		"## Decisions",
		"**Use stdout by default** → accepted",
		"## Sessions",
		"Initial setup [abc123]",
	}
	for _, want := range checks {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in output:\n%s", want, md)
		}
	}
}

func TestRenderExport_MissingFeature(t *testing.T) {
	s := setupTestStore(t)

	_, err := renderExport(s, "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing feature")
	}
}

func TestRunExport_FileFlag(t *testing.T) {
	s := setupTestStore(t)

	f, _ := s.AddFeature("File Test", "testing --file flag")

	md, _ := renderExport(s, f.ID)

	outPath := t.TempDir() + "/out.md"
	if err := os.WriteFile(outPath, []byte(md), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if !strings.Contains(string(got), "# File Test") {
		t.Errorf("file output missing feature title")
	}
}
