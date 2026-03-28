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

	var out hookOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if !out.Continue {
		t.Error("expected Continue to be true")
	}
	if out.SystemMessage != "" {
		t.Errorf("expected no systemMessage, got: %s", out.SystemMessage)
	}

	// Verify session was logged directly to the database
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
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	sess := sessions[0]
	if !strings.Contains(sess.Summary, "feat: add something") {
		t.Errorf("expected commit message in summary, got: %s", sess.Summary)
	}
	if len(sess.Commits) != 2 || sess.Commits[0] != "abc123" {
		t.Errorf("expected commit hashes [abc123, def456], got: %v", sess.Commits)
	}

	// Verify commits.log was deleted
	if _, err := os.Stat(commitsPath); !os.IsNotExist(err) {
		t.Error("expected commits.log to be deleted")
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

	var out hookOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if !out.Continue {
		t.Error("expected Continue to be true")
	}
	if out.SystemMessage != "" {
		t.Errorf("expected no systemMessage, got: %s", out.SystemMessage)
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

	// Verify systemMessage prompts board-manager dispatch
	var out hookOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if !strings.Contains(out.SystemMessage, "board-manager") {
		t.Errorf("expected board-manager dispatch instruction, got: %s", out.SystemMessage)
	}
	if !strings.Contains(out.SystemMessage, f.ID) {
		t.Errorf("expected feature ID in message, got: %s", out.SystemMessage)
	}
}
