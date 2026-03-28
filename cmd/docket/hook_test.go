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

func TestPostToolUseIgnoresNonCommit(t *testing.T) {
	dir := t.TempDir()

	h := &hookInput{
		SessionID:     "test-session",
		CWD:           dir,
		HookEventName: "PostToolUse",
		ToolName:      "Bash",
		ToolInput:     toolInput{Command: "go test ./..."},
	}

	// Should not panic or produce any output
	handlePostToolUse(h)
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

	// Create .docket dir for commits.log
	os.MkdirAll(filepath.Join(dir, ".docket"), 0755)

	h := &hookInput{
		SessionID:     "test-session",
		CWD:           dir,
		HookEventName: "PostToolUse",
		ToolName:      "Bash",
		ToolInput:     toolInput{Command: "git commit -m 'test commit message'"},
	}

	handlePostToolUse(h)

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
}
