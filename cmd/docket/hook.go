package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sniffle6/claude-docket/internal/store"
)

type hookInput struct {
	SessionID     string    `json:"session_id"`
	CWD           string    `json:"cwd"`
	HookEventName string    `json:"hook_event_name"`
	ToolName      string    `json:"tool_name"`
	ToolInput     toolInput `json:"tool_input"`
}

type toolInput struct {
	Command string `json:"command"`
}

type hookOutput struct {
	Continue      bool   `json:"continue"`
	SystemMessage string `json:"systemMessage,omitempty"`
}

func runHook() {
	var h hookInput
	if err := json.NewDecoder(os.Stdin).Decode(&h); err != nil {
		fmt.Fprintf(os.Stderr, "docket hook: decode stdin: %v\n", err)
		os.Exit(1)
	}

	switch h.HookEventName {
	case "SessionStart":
		handleSessionStart(&h, os.Stdout)
	case "PostToolUse":
		handlePostToolUse(&h, os.Stdout)
	case "Stop":
		handleStop(&h, os.Stdout)
	}
}

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

func handlePostToolUse(h *hookInput, w io.Writer) {
	out := hookOutput{Continue: true}

	if !strings.Contains(h.ToolInput.Command, "git commit") {
		json.NewEncoder(w).Encode(out)
		return
	}

	cmd := exec.Command("git", "log", "-1", "--format=%H|||%s")
	cmd.Dir = h.CWD
	output, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "docket hook: git log: %v\n", err)
		json.NewEncoder(w).Encode(out)
		return
	}

	line := strings.TrimSpace(string(output))

	// Append to commits.log
	commitsPath := filepath.Join(h.CWD, ".docket", "commits.log")
	f, err := os.OpenFile(commitsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "docket hook: open commits.log: %v\n", err)
		json.NewEncoder(w).Encode(out)
		return
	}
	f.WriteString(line + "\n")
	f.Close()

	// Find active feature to prompt board-manager dispatch
	s, err := store.Open(h.CWD)
	if err != nil {
		json.NewEncoder(w).Encode(out)
		return
	}
	defer s.Close()

	features, err := s.ListFeatures("in_progress")
	if err != nil || len(features) == 0 {
		json.NewEncoder(w).Encode(out)
		return
	}

	parts := strings.SplitN(line, "|||", 2)
	hash := parts[0]
	msg := ""
	if len(parts) == 2 {
		msg = parts[1]
	}

	// Auto-import plan files from this commit
	var importMsg string
	changedFiles := getCommitFiles(h.CWD, hash)
	for _, cf := range changedFiles {
		if isPlanFile(cf) {
			absPath := filepath.Join(h.CWD, cf)
			if _, statErr := os.Stat(absPath); statErr == nil {
				result, importErr := s.ImportPlan(features[0].ID, absPath)
				if importErr == nil {
					importMsg = fmt.Sprintf("\n[docket] Auto-imported plan: %d subtasks, %d items from %s", result.SubtaskCount, result.TaskItemCount, cf)
				}
				break // only import first plan file found
			}
		}
	}

	out.SystemMessage = fmt.Sprintf("[docket] Commit recorded: %s %s\nDispatch board-manager agent (model: sonnet) with: commit %s, message \"%s\", feature_id=\"%s\".%s",
		hash, msg, hash, msg, features[0].ID, importMsg)
	json.NewEncoder(w).Encode(out)
}

func getCommitFiles(dir, hash string) []string {
	cmd := exec.Command("git", "diff-tree", "--root", "--no-commit-id", "--name-only", "-r", hash)
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return nil
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line != "" {
			files = append(files, line)
		}
	}
	return files
}

func isPlanFile(path string) bool {
	if !strings.HasSuffix(path, ".md") {
		return false
	}
	return strings.Contains(path, "plans/") || strings.HasSuffix(path, "-plan.md")
}
