# Context Reset Orchestration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Generate structured handoff files at session end so fresh sessions can cold-start with full context.

**Architecture:** Stop hook writes mechanical handoff markdown to `.docket/handoff/<id>.md` using data from the store. SessionStart hook reads the top feature's handoff file and injects it as the system message. Board-manager agent enriches handoffs with synthesized context after commits.

**Tech Stack:** Go, SQLite (existing store), file I/O, markdown templating

---

### Task 1: Store — HandoffData struct and GetHandoffData method

**Files:**
- Create: `internal/store/handoff.go`
- Test: `internal/store/handoff_test.go`

- [ ] **Step 1: Write the failing test**

In `internal/store/handoff_test.go`:

```go
package store

import "testing"

func TestGetHandoffData(t *testing.T) {
	s := openTestStore(t)

	// Create feature with subtasks, items, and sessions
	s.AddFeature("Auth System", "token-based auth")
	s.UpdateFeature("auth-system", FeatureUpdate{
		Status:   strPtr("in_progress"),
		LeftOff:  strPtr("implementing refresh tokens"),
		KeyFiles: &[]string{"internal/auth/token.go", "internal/auth/middleware.go"},
	})

	st, _ := s.AddSubtask("auth-system", "Token handling", 1)
	s.AddTaskItem(st.ID, "Create token struct", 1)
	s.AddTaskItem(st.ID, "Add signing logic", 2)
	s.AddTaskItem(st.ID, "Add refresh endpoint", 3)
	s.CompleteTaskItem(1, TaskItemCompletion{Outcome: "done", CommitHash: "abc123"})

	st2, _ := s.AddSubtask("auth-system", "Middleware", 2)
	s.AddTaskItem(st2.ID, "Auth middleware", 1)

	s.LogSession(SessionInput{FeatureID: "auth-system", Summary: "Set up token struct", Commits: []string{"abc123"}})
	s.LogSession(SessionInput{FeatureID: "auth-system", Summary: "Started signing logic"})

	data, err := s.GetHandoffData("auth-system")
	if err != nil {
		t.Fatalf("GetHandoffData: %v", err)
	}

	if data.Feature.ID != "auth-system" {
		t.Errorf("Feature.ID = %q, want %q", data.Feature.ID, "auth-system")
	}
	if data.Done != 1 || data.Total != 4 {
		t.Errorf("Progress = %d/%d, want 1/4", data.Done, data.Total)
	}
	if len(data.NextTasks) != 3 {
		t.Fatalf("NextTasks = %d, want 3", len(data.NextTasks))
	}
	if data.NextTasks[0] != "Add signing logic" {
		t.Errorf("NextTasks[0] = %q, want %q", data.NextTasks[0], "Add signing logic")
	}
	if data.NextTasks[2] != "Auth middleware" {
		t.Errorf("NextTasks[2] = %q, want %q", data.NextTasks[2], "Auth middleware")
	}
	if len(data.SubtaskSummary) != 2 {
		t.Fatalf("SubtaskSummary = %d, want 2", len(data.SubtaskSummary))
	}
	if data.SubtaskSummary[0].Done != 1 || data.SubtaskSummary[0].Total != 3 {
		t.Errorf("Subtask 0 progress = %d/%d, want 1/3", data.SubtaskSummary[0].Done, data.SubtaskSummary[0].Total)
	}
	if len(data.RecentSessions) != 2 {
		t.Errorf("RecentSessions = %d, want 2", len(data.RecentSessions))
	}
}

func TestGetHandoffDataNotFound(t *testing.T) {
	s := openTestStore(t)
	_, err := s.GetHandoffData("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent feature")
	}
}

func TestGetHandoffDataNoSubtasks(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Simple Feature", "no subtasks")
	s.UpdateFeature("simple-feature", FeatureUpdate{Status: strPtr("in_progress")})

	data, err := s.GetHandoffData("simple-feature")
	if err != nil {
		t.Fatalf("GetHandoffData: %v", err)
	}
	if data.Done != 0 || data.Total != 0 {
		t.Errorf("Progress = %d/%d, want 0/0", data.Done, data.Total)
	}
	if len(data.NextTasks) != 0 {
		t.Errorf("NextTasks = %d, want 0", len(data.NextTasks))
	}
	if len(data.SubtaskSummary) != 0 {
		t.Errorf("SubtaskSummary = %d, want 0", len(data.SubtaskSummary))
	}
}

func TestGetHandoffDataCapsNextTasksAtThree(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Big Feature", "many items")
	s.UpdateFeature("big-feature", FeatureUpdate{Status: strPtr("in_progress")})
	st, _ := s.AddSubtask("big-feature", "Phase 1", 1)
	for i := 1; i <= 6; i++ {
		s.AddTaskItem(st.ID, fmt.Sprintf("Task %d", i), i)
	}

	data, err := s.GetHandoffData("big-feature")
	if err != nil {
		t.Fatalf("GetHandoffData: %v", err)
	}
	if len(data.NextTasks) != 3 {
		t.Errorf("NextTasks = %d, want 3 (capped)", len(data.NextTasks))
	}
}

func TestGetHandoffDataRecentSessionsCappedAtThree(t *testing.T) {
	s := openTestStore(t)
	s.AddFeature("Chatty Feature", "many sessions")
	for i := 0; i < 5; i++ {
		s.LogSession(SessionInput{FeatureID: "chatty-feature", Summary: fmt.Sprintf("session %d", i)})
	}

	data, err := s.GetHandoffData("chatty-feature")
	if err != nil {
		t.Fatalf("GetHandoffData: %v", err)
	}
	if len(data.RecentSessions) != 3 {
		t.Errorf("RecentSessions = %d, want 3 (capped)", len(data.RecentSessions))
	}
}
```

Note: `openTestStore`, `strPtr`, and `fmt` are already available from `feature_test.go` in the same package.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestGetHandoffData -v`
Expected: FAIL — `GetHandoffData` not defined

- [ ] **Step 3: Write the implementation**

In `internal/store/handoff.go`:

```go
package store

type HandoffSubtask struct {
	Title string
	Done  int
	Total int
}

type HandoffData struct {
	Feature        Feature
	Done           int
	Total          int
	NextTasks      []string // up to 3 uncompleted task item titles
	SubtaskSummary []HandoffSubtask
	RecentSessions []Session // last 3
}

func (s *Store) GetHandoffData(featureID string) (*HandoffData, error) {
	f, err := s.GetFeature(featureID)
	if err != nil {
		return nil, err
	}

	done, total, _ := s.GetFeatureProgress(featureID)

	subtasks, _ := s.GetSubtasksForFeature(featureID, false)
	var nextTasks []string
	var subtaskSummary []HandoffSubtask
	for _, st := range subtasks {
		stDone := 0
		for _, item := range st.Items {
			if item.Checked {
				stDone++
			} else if len(nextTasks) < 3 {
				nextTasks = append(nextTasks, item.Title)
			}
		}
		subtaskSummary = append(subtaskSummary, HandoffSubtask{
			Title: st.Title,
			Done:  stDone,
			Total: len(st.Items),
		})
	}

	rows, err := s.db.Query(
		`SELECT id, COALESCE(feature_id, ''), summary, files_touched, commits, auto_linked, link_reason, created_at FROM sessions WHERE feature_id = ? ORDER BY created_at DESC LIMIT 3`,
		featureID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sessions, _ := scanSessions(rows)

	return &HandoffData{
		Feature:        *f,
		Done:           done,
		Total:          total,
		NextTasks:      nextTasks,
		SubtaskSummary: subtaskSummary,
		RecentSessions: sessions,
	}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/ -run TestGetHandoffData -v`
Expected: all 5 tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/store/handoff.go internal/store/handoff_test.go
git commit -m "feat: add GetHandoffData store method for session handoffs"
```

---

### Task 2: Handoff file renderer and writer

**Files:**
- Create: `cmd/docket/handoff.go`
- Test: `cmd/docket/handoff_test.go`

- [ ] **Step 1: Write the failing test**

In `cmd/docket/handoff_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/docket/ -run "TestRenderHandoff|TestWriteHandoff|TestCleanStale" -v`
Expected: FAIL — `renderHandoff`, `writeHandoffFile`, `cleanStaleHandoffs` not defined

- [ ] **Step 3: Write the implementation**

In `cmd/docket/handoff.go`:

```go
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sniffle6/claude-docket/internal/store"
)

func renderHandoff(data *store.HandoffData) string {
	var b strings.Builder
	f := data.Feature

	fmt.Fprintf(&b, "# Handoff: %s\n\n", f.Title)

	fmt.Fprintf(&b, "## Status\n")
	fmt.Fprintf(&b, "%s | Progress: %d/%d | Updated: %s\n\n",
		f.Status, data.Done, data.Total, f.UpdatedAt.Format("2006-01-02 15:04"))

	if f.LeftOff != "" {
		fmt.Fprintf(&b, "## Left Off\n%s\n\n", f.LeftOff)
	}

	if len(data.NextTasks) > 0 {
		b.WriteString("## Next Tasks\n")
		for _, task := range data.NextTasks {
			fmt.Fprintf(&b, "- [ ] %s\n", task)
		}
		b.WriteString("\n")
	}

	if len(f.KeyFiles) > 0 {
		b.WriteString("## Key Files\n")
		for _, kf := range f.KeyFiles {
			fmt.Fprintf(&b, "- %s\n", kf)
		}
		b.WriteString("\n")
	}

	if len(data.RecentSessions) > 0 {
		b.WriteString("## Recent Activity\n")
		for _, sess := range data.RecentSessions {
			line := fmt.Sprintf("- %s: %s", sess.CreatedAt.Format("2006-01-02"), sess.Summary)
			if len(sess.Commits) > 0 {
				line += fmt.Sprintf(" [%s]", strings.Join(sess.Commits, ", "))
			}
			fmt.Fprintf(&b, "%s\n", line)
		}
		b.WriteString("\n")
	}

	if len(data.SubtaskSummary) > 0 {
		b.WriteString("## Active Subtasks\n")
		for _, st := range data.SubtaskSummary {
			fmt.Fprintf(&b, "- %s [%d/%d]\n", st.Title, st.Done, st.Total)
		}
		b.WriteString("\n")
	}

	return b.String()
}

func writeHandoffFile(dir string, data *store.HandoffData) error {
	handoffDir := filepath.Join(dir, ".docket", "handoff")
	if err := os.MkdirAll(handoffDir, 0755); err != nil {
		return err
	}
	path := filepath.Join(handoffDir, data.Feature.ID+".md")
	return os.WriteFile(path, []byte(renderHandoff(data)), 0644)
}

func cleanStaleHandoffs(dir string, activeIDs map[string]bool) {
	handoffDir := filepath.Join(dir, ".docket", "handoff")
	entries, err := os.ReadDir(handoffDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".md")
		if !activeIDs[name] {
			os.Remove(filepath.Join(handoffDir, e.Name()))
		}
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/docket/ -run "TestRenderHandoff|TestWriteHandoff|TestCleanStale" -v`
Expected: all 4 tests PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/docket/handoff.go cmd/docket/handoff_test.go
git commit -m "feat: add handoff file renderer and writer"
```

---

### Task 3: Stop hook — write handoff files at session end

**Files:**
- Modify: `cmd/docket/hook.go` — `handleStop` function (lines 102-159)
- Test: `cmd/docket/hook_test.go` — update `TestStopWithCommitsAndFeature`, add new test

- [ ] **Step 1: Write the failing test**

Add to `cmd/docket/hook_test.go`:

```go
func TestStopWritesHandoffFile(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	f, err := s.AddFeature("Handoff Feature", "testing handoff generation")
	if err != nil {
		t.Fatal(err)
	}
	status := "in_progress"
	leftOff := "implementing the parser"
	s.UpdateFeature(f.ID, store.FeatureUpdate{Status: &status, LeftOff: &leftOff})
	s.Close()

	h := &hookInput{
		SessionID:     "test-session",
		CWD:           dir,
		HookEventName: "Stop",
	}

	var buf bytes.Buffer
	handleStop(h, &buf)

	// Verify handoff file was created
	handoffPath := filepath.Join(dir, ".docket", "handoff", f.ID+".md")
	content, err := os.ReadFile(handoffPath)
	if err != nil {
		t.Fatalf("handoff file not created: %v", err)
	}
	if !strings.Contains(string(content), "# Handoff: Handoff Feature") {
		t.Errorf("handoff missing title:\n%s", content)
	}
	if !strings.Contains(string(content), "implementing the parser") {
		t.Errorf("handoff missing left_off:\n%s", content)
	}
}

func TestStopCleansStaleHandoffs(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Create one active and one done feature
	s.AddFeature("Active Feature", "")
	s.UpdateFeature("active-feature", store.FeatureUpdate{Status: strPtr("in_progress")})
	s.AddFeature("Done Feature", "")
	s.UpdateFeature("done-feature", store.FeatureUpdate{Status: strPtr("done")})
	s.Close()

	// Create a stale handoff for the done feature
	handoffDir := filepath.Join(dir, ".docket", "handoff")
	os.MkdirAll(handoffDir, 0755)
	os.WriteFile(filepath.Join(handoffDir, "done-feature.md"), []byte("stale"), 0644)

	h := &hookInput{
		SessionID:     "test-session",
		CWD:           dir,
		HookEventName: "Stop",
	}

	var buf bytes.Buffer
	handleStop(h, &buf)

	// Active feature should have a handoff
	if _, err := os.Stat(filepath.Join(handoffDir, "active-feature.md")); err != nil {
		t.Error("active handoff should exist")
	}
	// Done feature handoff should be cleaned up
	if _, err := os.Stat(filepath.Join(handoffDir, "done-feature.md")); !os.IsNotExist(err) {
		t.Error("stale handoff should be deleted")
	}
}
```

Note: `strPtr` is not available in `hook_test.go` (it's in `feature_test.go` which is a different package). Add this helper at the top of `hook_test.go` if not already present:

```go
func strPtr(s string) *string { return &s }
```

This is needed by both `TestStopCleansStaleHandoffs` and `TestSessionStartSecondFeatureShowsPointer`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/docket/ -run "TestStopWritesHandoff|TestStopCleansStale" -v`
Expected: FAIL — handoff file not created (handleStop doesn't write them yet)

- [ ] **Step 3: Update handleStop to write handoff files**

In `cmd/docket/hook.go`, add handoff generation at the end of `handleStop`, after the commits.log cleanup and before the final `json.NewEncoder(w).Encode(out)`. Replace the function from line 102 onward:

Replace the existing `handleStop` function with:

```go
func handleStop(h *hookInput, w io.Writer) {
	out := hookOutput{Continue: true}

	// Read commits.log if it exists
	commitsPath := filepath.Join(h.CWD, ".docket", "commits.log")
	var commits []string
	if data, err := os.ReadFile(commitsPath); err == nil && len(strings.TrimSpace(string(data))) > 0 {
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			if line != "" {
				commits = append(commits, line)
			}
		}
	}

	// Find active feature
	s, err := store.Open(h.CWD)
	if err != nil {
		json.NewEncoder(w).Encode(out)
		return
	}
	defer s.Close()

	features, err := s.ListFeatures("in_progress")
	if err != nil {
		json.NewEncoder(w).Encode(out)
		return
	}

	// Log session directly if there's an active feature
	if len(features) > 0 && len(commits) > 0 {
		f := features[0]

		// Build mechanical summary from commits
		var summaryParts []string
		var commitHashes []string
		for _, c := range commits {
			parts := strings.SplitN(c, "|||", 2)
			if len(parts) == 2 {
				commitHashes = append(commitHashes, parts[0])
				summaryParts = append(summaryParts, parts[1])
			}
		}
		summary := fmt.Sprintf("%d commit(s): %s", len(commits), strings.Join(summaryParts, "; "))

		s.LogSession(store.SessionInput{
			FeatureID: f.ID,
			Summary:   summary,
			Commits:   commitHashes,
		})
	}

	// Clean up commits.log
	if len(commits) > 0 {
		os.Remove(commitsPath)
	}

	// Write handoff files for in_progress features, clean stale ones
	if len(features) > 0 {
		activeIDs := make(map[string]bool)
		for _, f := range features {
			activeIDs[f.ID] = true
			data, err := s.GetHandoffData(f.ID)
			if err == nil {
				writeHandoffFile(h.CWD, data)
			}
		}
		cleanStaleHandoffs(h.CWD, activeIDs)
	}

	json.NewEncoder(w).Encode(out)
}
```

- [ ] **Step 4: Run all Stop tests to verify they pass**

Run: `go test ./cmd/docket/ -run "TestStop" -v`
Expected: all Stop tests PASS (existing + new)

- [ ] **Step 5: Commit**

```bash
git add cmd/docket/hook.go cmd/docket/hook_test.go
git commit -m "feat: Stop hook writes handoff files for in_progress features"
```

---

### Task 4: SessionStart hook — inject handoff content

**Files:**
- Modify: `cmd/docket/hook.go` — `handleSessionStart` function (lines 49-99)
- Test: `cmd/docket/hook_test.go` — update existing, add new tests

- [ ] **Step 1: Write the failing test**

Add to `cmd/docket/hook_test.go`:

```go
func TestSessionStartInjectsHandoff(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	f, err := s.AddFeature("My Feature", "testing handoff injection")
	if err != nil {
		t.Fatal(err)
	}
	status := "in_progress"
	s.UpdateFeature(f.ID, store.FeatureUpdate{Status: &status})
	s.Close()

	// Write a handoff file
	handoffDir := filepath.Join(dir, ".docket", "handoff")
	os.MkdirAll(handoffDir, 0755)
	handoffContent := "# Handoff: My Feature\n\n## Status\nin_progress | Progress: 0/0\n"
	os.WriteFile(filepath.Join(handoffDir, f.ID+".md"), []byte(handoffContent), 0644)

	h := &hookInput{
		SessionID:     "test-session",
		CWD:           dir,
		HookEventName: "SessionStart",
	}

	var buf bytes.Buffer
	handleSessionStart(h, &buf)

	var out hookOutput
	json.Unmarshal(buf.Bytes(), &out)

	if !strings.Contains(out.SystemMessage, "# Handoff: My Feature") {
		t.Errorf("expected full handoff content in message, got: %s", out.SystemMessage)
	}
	if !strings.Contains(out.SystemMessage, "[docket] Session context:") {
		t.Errorf("expected session context prefix, got: %s", out.SystemMessage)
	}
}

func TestSessionStartFallsBackWithoutHandoff(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	f, err := s.AddFeature("Fallback Feature", "no handoff file")
	if err != nil {
		t.Fatal(err)
	}
	status := "in_progress"
	leftOff := "doing stuff"
	s.UpdateFeature(f.ID, store.FeatureUpdate{Status: &status, LeftOff: &leftOff})
	s.Close()

	h := &hookInput{
		SessionID:     "test-session",
		CWD:           dir,
		HookEventName: "SessionStart",
	}

	var buf bytes.Buffer
	handleSessionStart(h, &buf)

	var out hookOutput
	json.Unmarshal(buf.Bytes(), &out)

	// Should fall back to current behavior
	if !strings.Contains(out.SystemMessage, "Fallback Feature") {
		t.Errorf("expected feature title in fallback, got: %s", out.SystemMessage)
	}
	if !strings.Contains(out.SystemMessage, "doing stuff") {
		t.Errorf("expected left_off in fallback, got: %s", out.SystemMessage)
	}
}

func TestSessionStartSecondFeatureShowsPointer(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	s.AddFeature("Feature A", "first")
	s.UpdateFeature("feature-a", store.FeatureUpdate{Status: strPtr("in_progress")})
	s.AddFeature("Feature B", "second")
	s.UpdateFeature("feature-b", store.FeatureUpdate{Status: strPtr("in_progress")})
	s.Close()

	// Write handoff for both
	handoffDir := filepath.Join(dir, ".docket", "handoff")
	os.MkdirAll(handoffDir, 0755)
	os.WriteFile(filepath.Join(handoffDir, "feature-a.md"), []byte("# Handoff: Feature A\n"), 0644)
	os.WriteFile(filepath.Join(handoffDir, "feature-b.md"), []byte("# Handoff: Feature B\n"), 0644)

	h := &hookInput{
		SessionID:     "test-session",
		CWD:           dir,
		HookEventName: "SessionStart",
	}

	var buf bytes.Buffer
	handleSessionStart(h, &buf)

	var out hookOutput
	json.Unmarshal(buf.Bytes(), &out)

	// Second feature should be a pointer, not full content
	if strings.Contains(out.SystemMessage, "# Handoff: Feature B") {
		t.Errorf("second feature should be a pointer, not full content: %s", out.SystemMessage)
	}
	if !strings.Contains(out.SystemMessage, ".docket/handoff/feature-b.md") {
		t.Errorf("expected pointer to second feature handoff, got: %s", out.SystemMessage)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/docket/ -run "TestSessionStart(Injects|FallsBack|Second)" -v`
Expected: FAIL — current handleSessionStart doesn't read handoff files

- [ ] **Step 3: Replace handleSessionStart with handoff-aware version**

Replace the existing `handleSessionStart` function in `cmd/docket/hook.go`:

```go
func handleSessionStart(h *hookInput, w io.Writer) {
	s, err := store.Open(h.CWD)
	if err != nil {
		fmt.Fprintf(os.Stderr, "docket hook: open store: %v\n", err)
		return
	}
	defer s.Close()

	// Create/clear commits.log
	commitsPath := filepath.Join(h.CWD, ".docket", "commits.log")
	os.WriteFile(commitsPath, []byte{}, 0644)

	features, err := s.ListFeatures("in_progress")
	if err != nil {
		fmt.Fprintf(os.Stderr, "docket hook: list features: %v\n", err)
		return
	}

	out := hookOutput{Continue: true}

	if len(features) == 0 {
		out.SystemMessage = "[docket] No active features. Use docket MCP tools to create one."
		json.NewEncoder(w).Encode(out)
		return
	}

	var msg strings.Builder
	topFeature := features[0]
	handoffPath := filepath.Join(h.CWD, ".docket", "handoff", topFeature.ID+".md")

	if content, err := os.ReadFile(handoffPath); err == nil {
		msg.WriteString("[docket] Session context:\n\n")
		msg.Write(content)
	} else {
		// Fallback: list features with left_off and next task
		msg.WriteString("[docket] Active features:\n")
		msg.WriteString(fmt.Sprintf("- %s (id: %s)", topFeature.Title, topFeature.ID))
		if topFeature.LeftOff != "" {
			msg.WriteString(fmt.Sprintf(" — left off: %s", topFeature.LeftOff))
		}
		msg.WriteString("\n")

		subtasks, err := s.GetSubtasksForFeature(topFeature.ID, false)
		if err == nil {
			for _, st := range subtasks {
				for _, item := range st.Items {
					if !item.Checked {
						msg.WriteString(fmt.Sprintf("Next task: %s\n", item.Title))
						goto doneNextTask
					}
				}
			}
		}
	doneNextTask:
	}

	// Other features: pointers or one-liners
	for _, f := range features[1:] {
		otherHandoff := filepath.Join(h.CWD, ".docket", "handoff", f.ID+".md")
		if _, err := os.Stat(otherHandoff); err == nil {
			msg.WriteString(fmt.Sprintf("\n[docket] Handoff available: .docket/handoff/%s.md", f.ID))
		} else {
			msg.WriteString(fmt.Sprintf("\n[docket] Also active: %s (id: %s)", f.Title, f.ID))
		}
	}

	out.SystemMessage = msg.String()
	json.NewEncoder(w).Encode(out)
}
```

- [ ] **Step 4: Run ALL hook tests to verify nothing is broken**

Run: `go test ./cmd/docket/ -v`
Expected: all tests PASS

Note: `TestSessionStartWithFeature` will need updating — it currently checks for the feature title and left_off in the message, but there's no handoff file in that test, so it should still hit the fallback path. Verify it passes as-is. If not, the fallback output format changed slightly (from `"[docket] Active features:\n"` vs the old format) — update the test assertions to match the new fallback format.

- [ ] **Step 5: Update TestSessionStartWithFeature if needed**

The existing test checks for `"Auto Tracking Hooks"` and `"implementing session start"` in the message. The new fallback still includes both. The only difference is the prefix changed from `"[docket] Active features:\n"` to the same. Verify the test passes — if it does, no change needed.

- [ ] **Step 6: Commit**

```bash
git add cmd/docket/hook.go cmd/docket/hook_test.go
git commit -m "feat: SessionStart hook injects handoff file content"
```

---

### Task 5: Update board-manager agent instructions

**Files:**
- Modify: `plugin/agents/board-manager.md`

- [ ] **Step 1: Read current board-manager.md**

Verify current content at `plugin/agents/board-manager.md`.

- [ ] **Step 2: Add handoff enrichment instructions**

Add a new section after the "After a commit" section in the "How to handle each event" area. Add this block:

After the "After a commit" section's step 3, add step 4:

```markdown
4. Enrich the handoff file at `.docket/handoff/<feature-id>.md`:
   - Read the existing handoff file (the Stop hook writes the mechanical baseline).
   - Append these sections below the existing content (never modify the mechanical sections above):
     - **## Decisions & Context** — synthesize from session history: what approaches were tried, what worked, what was rejected.
     - **## Gotchas** — anything the next session should watch out for (edge cases found, fragile areas, things that almost broke).
     - **## Recommended Approach** — what to do next and why, based on the current state.
   - If these sections already exist in the file (from a previous enrichment), replace them with updated versions.
   - Keep it concise — 3-5 bullet points per section max.
```

Also add to the "Behavior rules" section:

```markdown
- **Enrich handoffs after commits.** Always read and update the handoff file when processing a commit. The mechanical baseline is written by the Stop hook — your job is to add synthesis.
```

- [ ] **Step 3: Commit**

```bash
git add plugin/agents/board-manager.md
git commit -m "feat: board-manager enriches handoff files after commits"
```

---

### Task 6: Documentation

**Files:**
- Create: `docs/handoff-files.md`

- [ ] **Step 1: Write the doc**

In `docs/handoff-files.md`:

```markdown
# Handoff Files

## What it does

Docket generates structured markdown files at session end so the next session can cold-start with full context. These live at `.docket/handoff/<feature-id>.md`.

## Why it exists

When a Claude Code session ends and a new one starts, the agent loses nuance — decisions made, approaches rejected, current progress. Handoff files give the fresh session everything it needs without re-reading code or re-discovering state.

## How it works

**Two-tier generation:**

1. **Stop hook (mechanical)** — every session end, the hook reads the database and writes a structured markdown file with: status, progress, left_off, next 3 tasks, key files, recent sessions, subtask progress. No LLM involved — instant and free.

2. **Board-manager (enriched)** — when dispatched after commits, the board-manager reads the mechanical handoff and appends synthesized sections: decisions & context, gotchas, and recommended approach.

**Session start injection:**

The SessionStart hook reads the top in-progress feature's handoff file and injects its full contents as the system message. Other in-progress features get one-line pointers to their handoff files.

**Cleanup:**

The Stop hook deletes handoff files for features that are no longer in_progress.

## Gotchas

- Handoff files are overwritten every session end. Agent-enriched sections are lost and re-generated. This is intentional — the mechanical baseline is always the source of truth.
- If no handoff file exists (first session for a feature), the SessionStart hook falls back to listing features with left_off text.
- Handoff files are in `.docket/` which should be in `.gitignore`.

## Key files

- `cmd/docket/handoff.go` — renderHandoff, writeHandoffFile, cleanStaleHandoffs
- `cmd/docket/hook.go` — Stop handler calls writeHandoffFile; SessionStart reads handoff files
- `internal/store/handoff.go` — HandoffData struct and GetHandoffData method
- `plugin/agents/board-manager.md` — agent instructions for enriching handoff files
```

- [ ] **Step 2: Commit**

```bash
git add docs/handoff-files.md
git commit -m "docs: add handoff files documentation"
```

---

### Task 7: Integration verification

- [ ] **Step 1: Run full test suite**

Run: `go test ./...`
Expected: all tests PASS

- [ ] **Step 2: Build the binary**

Run: `go build -ldflags="-s -w" -o docket.exe ./cmd/docket/`
Expected: builds without errors

- [ ] **Step 3: Install and verify**

Run: `bash install.sh`
Expected: installs updated binary and plugin
