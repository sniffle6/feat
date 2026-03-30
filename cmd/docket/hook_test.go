package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sniffle6/claude-docket/internal/store"
)

func strPtr(s string) *string { return &s }

func TestSessionStartWithFeature(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	f, err := s.AddFeature("Auto Tracking Hooks", "hook system")
	if err != nil {
		t.Fatal(err)
	}
	status := "in_progress"
	leftOff := "implementing session start"
	s.UpdateFeature(f.ID, store.FeatureUpdate{Status: &status, LeftOff: &leftOff})
	s.Close()

	h := &hookInput{
		SessionID:     "test-session",
		CWD:           dir,
		HookEventName: "SessionStart",
	}

	var buf bytes.Buffer
	handleSessionStart(h, &buf)

	// Verify commits.log was created
	commitsPath := filepath.Join(dir, ".docket", "commits.log")
	if _, err := os.Stat(commitsPath); os.IsNotExist(err) {
		t.Error("commits.log was not created")
	}

	// Verify output
	var out hookOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if !out.Continue {
		t.Error("expected Continue to be true")
	}
	if !strings.Contains(out.SystemMessage, "Auto Tracking Hooks") {
		t.Errorf("expected feature title in message, got: %s", out.SystemMessage)
	}
	if !strings.Contains(out.SystemMessage, "implementing session start") {
		t.Errorf("expected left_off in message, got: %s", out.SystemMessage)
	}
}

func TestSessionStartNoFeatures(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	h := &hookInput{
		SessionID:     "test-session",
		CWD:           dir,
		HookEventName: "SessionStart",
	}

	var buf bytes.Buffer
	handleSessionStart(h, &buf)

	var out hookOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if !out.Continue {
		t.Error("expected Continue to be true")
	}
	if !strings.Contains(out.SystemMessage, "No active features") {
		t.Errorf("expected 'No active features' in message, got: %s", out.SystemMessage)
	}
}

func TestStopWithCommitsAndFeature(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	f, err := s.AddFeature("My Feature", "testing stop hook")
	if err != nil {
		t.Fatal(err)
	}
	status := "in_progress"
	s.UpdateFeature(f.ID, store.FeatureUpdate{Status: &status})
	s.Close()

	// Write a commits.log
	commitsPath := filepath.Join(dir, ".docket", "commits.log")
	os.WriteFile(commitsPath, []byte("abc123|||feat: add something\ndef456|||fix: broken thing\n"), 0644)

	h := &hookInput{
		SessionID:     "test-session",
		CWD:           dir,
		HookEventName: "Stop",
	}

	var buf bytes.Buffer
	handleStop(h, &buf)

	// First stop should block and prompt for rich summary
	var out stopHookOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if out.Decision != "block" {
		t.Errorf("expected decision=block, got: %s", out.Decision)
	}
	if !strings.Contains(out.Reason, f.ID) {
		t.Errorf("expected feature ID in reason, got: %s", out.Reason)
	}
	if !strings.Contains(out.Reason, "log_session") {
		t.Errorf("expected log_session instruction in reason, got: %s", out.Reason)
	}
	if !strings.Contains(out.Reason, "abc123") {
		t.Errorf("expected commit hashes in reason, got: %s", out.Reason)
	}

	// Commits.log should NOT be deleted yet (cleaned on re-trigger)
	if _, err := os.Stat(commitsPath); os.IsNotExist(err) {
		t.Error("expected commits.log to still exist after first stop")
	}

	// No session should be logged by the hook (Claude does it via MCP)
	s2, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	sessions, err := s2.GetSessionsForFeature(f.ID)
	if err != nil {
		t.Fatalf("get sessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions (Claude logs via MCP), got %d", len(sessions))
	}
}

func TestStopRetriggerWritesHandoffAndCleans(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	f, err := s.AddFeature("My Feature", "testing re-trigger")
	if err != nil {
		t.Fatal(err)
	}
	status := "in_progress"
	leftOff := "wrote the parser"
	s.UpdateFeature(f.ID, store.FeatureUpdate{Status: &status, LeftOff: &leftOff})
	s.Close()

	// Write a commits.log (leftover from first stop)
	commitsPath := filepath.Join(dir, ".docket", "commits.log")
	os.WriteFile(commitsPath, []byte("abc123|||feat: add something\n"), 0644)

	h := &hookInput{
		SessionID:      "test-session",
		CWD:            dir,
		HookEventName:  "Stop",
		StopHookActive: true,
	}

	var buf bytes.Buffer
	handleStop(h, &buf)

	// Re-trigger should allow stop (no decision)
	var out stopHookOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if out.Decision != "" {
		t.Errorf("expected no decision on re-trigger, got: %s", out.Decision)
	}

	// Commits.log should be cleaned up
	if _, err := os.Stat(commitsPath); !os.IsNotExist(err) {
		t.Error("expected commits.log to be deleted on re-trigger")
	}

	// Handoff file should be written
	handoffPath := filepath.Join(dir, ".docket", "handoff", f.ID+".md")
	content, err := os.ReadFile(handoffPath)
	if err != nil {
		t.Fatalf("handoff file not created: %v", err)
	}
	if !strings.Contains(string(content), "# Handoff: My Feature") {
		t.Errorf("handoff missing title:\n%s", content)
	}
	if !strings.Contains(string(content), "wrote the parser") {
		t.Errorf("handoff missing left_off:\n%s", content)
	}

	// Fallback: session should be logged mechanically since Claude didn't call log_session
	s2, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	sessions, err := s2.GetSessionsForFeature(f.ID)
	if err != nil {
		t.Fatalf("get sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 fallback session, got %d", len(sessions))
	}
	if !strings.Contains(sessions[0].Summary, "feat: add something") {
		t.Errorf("expected mechanical summary, got: %s", sessions[0].Summary)
	}
}

func TestStopRetriggerNoDoubleLog(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	f, err := s.AddFeature("My Feature", "testing no double log")
	if err != nil {
		t.Fatal(err)
	}
	status := "in_progress"
	s.UpdateFeature(f.ID, store.FeatureUpdate{Status: &status})

	// Simulate Claude having called log_session with the commit hash
	s.LogSession(store.SessionInput{
		FeatureID: f.ID,
		Summary:   "Rich AI summary of the session",
		Commits:   []string{"abc123"},
	})
	s.MarkSessionLogged()
	s.Close()

	// Write a commits.log with the same commit
	commitsPath := filepath.Join(dir, ".docket", "commits.log")
	os.WriteFile(commitsPath, []byte("abc123|||feat: add something\n"), 0644)

	h := &hookInput{
		SessionID:      "test-session",
		CWD:            dir,
		HookEventName:  "Stop",
		StopHookActive: true,
	}

	var buf bytes.Buffer
	handleStop(h, &buf)

	// Should NOT double-log — Claude already logged the session
	s2, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	sessions, err := s2.GetSessionsForFeature(f.ID)
	if err != nil {
		t.Fatalf("get sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected exactly 1 session (no double-log), got %d", len(sessions))
	}
	if !strings.Contains(sessions[0].Summary, "Rich AI summary") {
		t.Errorf("expected Claude's rich summary, got: %s", sessions[0].Summary)
	}
}

func TestStopNoCommitsNoFeatures(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	h := &hookInput{
		SessionID:     "test-session",
		CWD:           dir,
		HookEventName: "Stop",
	}

	var buf bytes.Buffer
	handleStop(h, &buf)

	// Should allow stop with no decision
	var out stopHookOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if out.Decision != "" {
		t.Errorf("expected no decision, got: %s", out.Decision)
	}

	// Verify no sessions were logged
	s2, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	sessions, err := s2.GetUnlinkedSessions()
	if err != nil {
		t.Fatalf("get sessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestPostToolUseIgnoresNonCommit(t *testing.T) {
	dir := t.TempDir()

	h := &hookInput{
		SessionID:     "test-session",
		CWD:           dir,
		HookEventName: "PostToolUse",
		ToolName:      "Bash",
		ToolInput:     toolInput{Command: "go test ./..."},
	}

	var buf bytes.Buffer
	handlePostToolUse(h, &buf)

	var out hookOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if !out.Continue {
		t.Error("expected Continue to be true")
	}
	if out.SystemMessage != "" {
		t.Errorf("expected no systemMessage for non-commit, got: %s", out.SystemMessage)
	}
}

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

	// Handoff files are written on re-trigger (stop_hook_active=true)
	h := &hookInput{
		SessionID:      "test-session",
		CWD:            dir,
		HookEventName:  "Stop",
		StopHookActive: true,
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

	// Stale handoff cleanup happens on re-trigger
	h := &hookInput{
		SessionID:      "test-session",
		CWD:            dir,
		HookEventName:  "Stop",
		StopHookActive: true,
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
	s.AddFeature("Feature B", "second")
	s.UpdateFeature("feature-b", store.FeatureUpdate{Status: strPtr("in_progress")})
	s.UpdateFeature("feature-a", store.FeatureUpdate{Status: strPtr("in_progress")})
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

func TestPostToolUseAutoImportsPlan(t *testing.T) {
	dir := t.TempDir()

	// Init git repo
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
	}

	// Create a plan file and commit it
	planDir := filepath.Join(dir, "docs", "superpowers", "plans")
	os.MkdirAll(planDir, 0755)
	planContent := `# Test Plan

> For agentic workers

### Task 1: Add widget

**Files:**
- Create: ` + "`src/widget.go`" + `

- [ ] **Step 1: Write the test**
- [ ] **Step 2: Implement widget**
- [ ] **Step 3: Commit**

### Task 2: Add handler

- [ ] **Step 1: Write handler test**
- [ ] **Step 2: Implement handler**
`
	planPath := filepath.Join(planDir, "2026-03-28-test-plan.md")
	os.WriteFile(planPath, []byte(planContent), 0644)

	addCmd := exec.Command("git", "add", ".")
	addCmd.Dir = dir
	if out, err := addCmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v: %s", err, out)
	}

	commitCmd := exec.Command("git", "commit", "-m", "docs: add test plan")
	commitCmd.Dir = dir
	if out, err := commitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v: %s", err, out)
	}

	// Create an active feature
	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	f, err := s.AddFeature("Test Feature", "testing")
	if err != nil {
		t.Fatal(err)
	}
	status := "in_progress"
	s.UpdateFeature(f.ID, store.FeatureUpdate{Status: &status})
	s.Close()

	h := &hookInput{
		SessionID:     "test-session",
		CWD:           dir,
		HookEventName: "PostToolUse",
		ToolName:      "Bash",
		ToolInput:     toolInput{Command: "git commit -m 'docs: add test plan'"},
	}

	var buf bytes.Buffer
	handlePostToolUse(h, &buf)

	// Verify plan was auto-imported
	s2, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	subtasks, err := s2.GetSubtasksForFeature(f.ID, false)
	if err != nil {
		t.Fatalf("get subtasks: %v", err)
	}
	if len(subtasks) != 2 {
		t.Fatalf("expected 2 subtasks from plan import, got %d", len(subtasks))
	}
	if subtasks[0].Title != "Task 1: Add widget" {
		t.Errorf("subtask 0 title = %q, want %q", subtasks[0].Title, "Task 1: Add widget")
	}

	// Verify system message mentions import
	var out hookOutput
	json.Unmarshal(buf.Bytes(), &out)
	if !strings.Contains(out.SystemMessage, "imported") {
		t.Errorf("expected import mention in system message, got: %s", out.SystemMessage)
	}
}

func TestSessionStartAutoArchives(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Create a done feature backdated 8 days
	s.AddFeature("Old Done Feature", "")
	done := "done"
	s.UpdateFeature("old-done-feature", store.FeatureUpdate{Status: &done})
	s.DB().Exec(`UPDATE features SET updated_at = datetime('now', '-8 days') WHERE id = 'old-done-feature'`)

	// Create an active feature
	s.AddFeature("Active Feature", "")
	ip := "in_progress"
	s.UpdateFeature("active-feature", store.FeatureUpdate{Status: &ip})
	s.Close()

	h := &hookInput{
		SessionID:     "test-session",
		CWD:           dir,
		HookEventName: "SessionStart",
	}

	var buf bytes.Buffer
	handleSessionStart(h, &buf)

	// Verify old feature was archived
	s2, _ := store.Open(dir)
	defer s2.Close()
	f, _ := s2.GetFeature("old-done-feature")
	if f.Status != "archived" {
		t.Fatalf("expected archived, got %q", f.Status)
	}
}

func TestIsPlanFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"docs/superpowers/plans/2026-03-28-feature.md", true},
		{"plans/my-plan.md", true},
		{"docs/my-feature-plan.md", true},
		{"src/plans/config.go", false},        // not .md
		{"docs/migration-plans/notes.txt", false}, // not .md
		{"src/handler.go", false},
		{"README.md", false},
		{"plans/readme.txt", false}, // not .md
	}
	for _, tt := range tests {
		got := isPlanFile(tt.path)
		if got != tt.want {
			t.Errorf("isPlanFile(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestPostToolUseRecordsCommit(t *testing.T) {
	dir := t.TempDir()

	// Init git repo and make a commit
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
	}

	// Create a file and commit it
	testFile := filepath.Join(dir, "test.txt")
	os.WriteFile(testFile, []byte("hello"), 0644)

	addCmd := exec.Command("git", "add", ".")
	addCmd.Dir = dir
	if out, err := addCmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v: %s", err, out)
	}

	commitCmd := exec.Command("git", "commit", "-m", "test commit message")
	commitCmd.Dir = dir
	if out, err := commitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v: %s", err, out)
	}

	// Create .docket dir and an active feature
	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	f, err := s.AddFeature("Test Feature", "testing")
	if err != nil {
		t.Fatal(err)
	}
	status := "in_progress"
	s.UpdateFeature(f.ID, store.FeatureUpdate{Status: &status})
	s.Close()

	h := &hookInput{
		SessionID:     "test-session",
		CWD:           dir,
		HookEventName: "PostToolUse",
		ToolName:      "Bash",
		ToolInput:     toolInput{Command: "git commit -m 'test commit message'"},
	}

	var buf bytes.Buffer
	handlePostToolUse(h, &buf)

	// Verify commits.log has the commit
	commitsPath := filepath.Join(dir, ".docket", "commits.log")
	data, err := os.ReadFile(commitsPath)
	if err != nil {
		t.Fatalf("read commits.log: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "|||") {
		t.Errorf("expected hash|||message format, got: %s", content)
	}
	if !strings.Contains(content, "test commit message") {
		t.Errorf("expected commit message in log, got: %s", content)
	}

	// Verify systemMessage prompts direct MCP update
	var out hookOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if !strings.Contains(out.SystemMessage, "Update feature") {
		t.Errorf("expected direct update instruction, got: %s", out.SystemMessage)
	}
	if !strings.Contains(out.SystemMessage, f.ID) {
		t.Errorf("expected feature ID in message, got: %s", out.SystemMessage)
	}
}

func TestPreToolUseNoFeatures(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	h := &hookInput{
		SessionID:     "test-session",
		CWD:           dir,
		HookEventName: "PreToolUse",
		ToolName:      "Agent",
	}

	var buf bytes.Buffer
	handlePreToolUse(h, &buf)

	var out preToolUseOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if out.HookSpecificOutput == nil || out.HookSpecificOutput.PermissionDecision != "allow" {
		t.Error("expected permissionDecision=allow")
	}
	if !strings.Contains(out.SystemMessage, "No active docket feature") {
		t.Errorf("expected nudge message, got: %s", out.SystemMessage)
	}
	if !strings.Contains(out.SystemMessage, "get_ready") {
		t.Errorf("expected get_ready instruction, got: %s", out.SystemMessage)
	}

	// Verify sentinel was written
	sentinel := filepath.Join(dir, ".docket", "agent-nudged")
	if _, err := os.Stat(sentinel); os.IsNotExist(err) {
		t.Error("expected agent-nudged sentinel to be created")
	}
}

func TestPreToolUseFeatureNoTaskItems(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	f, err := s.AddFeature("My Feature", "testing")
	if err != nil {
		t.Fatal(err)
	}
	status := "in_progress"
	s.UpdateFeature(f.ID, store.FeatureUpdate{Status: &status})
	s.Close()

	h := &hookInput{
		SessionID:     "test-session",
		CWD:           dir,
		HookEventName: "PreToolUse",
		ToolName:      "Agent",
	}

	var buf bytes.Buffer
	handlePreToolUse(h, &buf)

	var out preToolUseOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if out.HookSpecificOutput == nil || out.HookSpecificOutput.PermissionDecision != "allow" {
		t.Error("expected permissionDecision=allow")
	}
	if !strings.Contains(out.SystemMessage, "no task items") {
		t.Errorf("expected task items nudge, got: %s", out.SystemMessage)
	}
	if !strings.Contains(out.SystemMessage, f.ID) {
		t.Errorf("expected feature ID in message, got: %s", out.SystemMessage)
	}
}

func TestPreToolUseFeatureWithTaskItems(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	f, err := s.AddFeature("My Feature", "testing")
	if err != nil {
		t.Fatal(err)
	}
	status := "in_progress"
	s.UpdateFeature(f.ID, store.FeatureUpdate{Status: &status})

	// Add a subtask with a task item
	st, err := s.AddSubtask(f.ID, "Task 1", 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.AddTaskItem(st.ID, "Step 1: do something", 0)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	h := &hookInput{
		SessionID:     "test-session",
		CWD:           dir,
		HookEventName: "PreToolUse",
		ToolName:      "Agent",
	}

	var buf bytes.Buffer
	handlePreToolUse(h, &buf)

	var out preToolUseOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if out.HookSpecificOutput == nil || out.HookSpecificOutput.PermissionDecision != "allow" {
		t.Error("expected permissionDecision=allow")
	}
	if out.SystemMessage != "" {
		t.Errorf("expected no systemMessage when task items exist, got: %s", out.SystemMessage)
	}

	// Verify no sentinel written
	sentinel := filepath.Join(dir, ".docket", "agent-nudged")
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Error("sentinel should NOT be written when feature has task items")
	}
}
