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
	"github.com/sniffle6/claude-docket/internal/transcript"
)

type hookInput struct {
	SessionID      string    `json:"session_id"`
	TranscriptPath string    `json:"transcript_path"`
	CWD            string    `json:"cwd"`
	HookEventName  string    `json:"hook_event_name"`
	ToolName       string    `json:"tool_name"`
	ToolInput toolInput `json:"tool_input"`
	Trigger   string    `json:"trigger"`
}

type toolInput struct {
	Command string `json:"command"`
}

type hookOutput struct {
	Continue      bool   `json:"continue"`
	SystemMessage string `json:"systemMessage,omitempty"`
}

type stopHookOutput struct {
	Decision string `json:"decision,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

type preToolUseOutput struct {
	HookSpecificOutput *preToolUseDecision `json:"hookSpecificOutput,omitempty"`
	SystemMessage      string              `json:"systemMessage,omitempty"`
}

type preToolUseDecision struct {
	PermissionDecision string `json:"permissionDecision"`
}

func runHook() {
	var h hookInput
	if err := json.NewDecoder(os.Stdin).Decode(&h); err != nil {
		fmt.Fprintf(os.Stderr, "docket hook: decode stdin: %v\n", err)
		os.Exit(1)
	}

	// Check .docket/ exists — if not, project hasn't been initialized
	docketDir := filepath.Join(h.CWD, ".docket")
	if _, err := os.Stat(docketDir); os.IsNotExist(err) {
		// Not a docket project — pass through silently
		if h.HookEventName == "Stop" || h.HookEventName == "SessionEnd" {
			json.NewEncoder(os.Stdout).Encode(stopHookOutput{})
		} else {
			json.NewEncoder(os.Stdout).Encode(hookOutput{Continue: true})
		}
		return
	}

	switch h.HookEventName {
	case "PreToolUse":
		handlePreToolUse(&h, os.Stdout)
	case "SessionStart":
		handleSessionStart(&h, os.Stdout)
	case "PostToolUse":
		handlePostToolUse(&h, os.Stdout)
	case "Stop":
		handleStop(&h, os.Stdout)
	case "PreCompact":
		handlePreCompact(&h, os.Stdout)
	case "SessionEnd":
		handleSessionEnd(&h, os.Stdout)
	default:
		json.NewEncoder(os.Stdout).Encode(hookOutput{Continue: true})
	}
}

func handleSessionStart(h *hookInput, w io.Writer) {
	s, err := store.Open(h.CWD)
	if err != nil {
		fmt.Fprintf(os.Stderr, "docket hook: open store: %v\n", err)
		json.NewEncoder(w).Encode(hookOutput{Continue: true})
		return
	}
	defer s.Close()

	// Auto-archive features done >7 days
	var archiveMsg string
	if archived, err := s.AutoArchiveStale(); err == nil && len(archived) > 0 {
		archiveMsg = fmt.Sprintf("[docket] Auto-archived %d features done >7 days: %s\n", len(archived), strings.Join(archived, ", "))
	}

	// Create/clear commits.log
	commitsPath := filepath.Join(h.CWD, ".docket", "commits.log")
	os.WriteFile(commitsPath, []byte{}, 0644)

	// Clear agent-nudged sentinel for new session
	sentinelPath := filepath.Join(h.CWD, ".docket", "agent-nudged")
	os.Remove(sentinelPath)

	// Reset transcript offset for new session
	offsetPath := filepath.Join(h.CWD, ".docket", "transcript-offset")
	os.WriteFile(offsetPath, []byte("0"), 0644)

	features, err := s.ListFeatures("in_progress")
	if err != nil {
		fmt.Fprintf(os.Stderr, "docket hook: list features: %v\n", err)
		json.NewEncoder(w).Encode(hookOutput{Continue: true})
		return
	}

	out := hookOutput{Continue: true}

	if len(features) == 0 {
		out.SystemMessage = archiveMsg + "[docket] No active features. Use docket MCP tools to create one."
		json.NewEncoder(w).Encode(out)
		return
	}

	var msg strings.Builder
	topFeature := features[0]

	s.OpenWorkSession(topFeature.ID, h.SessionID)

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

	out.SystemMessage = archiveMsg + msg.String()
	json.NewEncoder(w).Encode(out)
}

func handleStop(h *hookInput, w io.Writer) {
	s, err := store.Open(h.CWD)
	if err != nil {
		json.NewEncoder(w).Encode(stopHookOutput{})
		return
	}
	defer s.Close()

	ws, err := s.GetActiveWorkSession()
	if err != nil {
		json.NewEncoder(w).Encode(stopHookOutput{})
		return
	}

	delta := parseTranscriptDelta(h)
	if !isDeltaMeaningful(h.CWD, delta) {
		json.NewEncoder(w).Encode(stopHookOutput{})
		return
	}

	s.EnqueueCheckpointJob(store.CheckpointJobInput{
		WorkSessionID:         ws.ID,
		FeatureID:             ws.FeatureID,
		Reason:                "stop",
		TriggerType:           "auto",
		TranscriptStartOffset: getTranscriptOffset(h.CWD),
		TranscriptEndOffset:   delta.EndOffset,
		SemanticText:          delta.SemanticText,
		MechanicalFacts:       delta.MechanicalFacts,
	})

	saveTranscriptOffset(h.CWD, delta.EndOffset)
	json.NewEncoder(w).Encode(stopHookOutput{})
}

func handlePreCompact(h *hookInput, w io.Writer) {
	s, err := store.Open(h.CWD)
	if err != nil {
		json.NewEncoder(w).Encode(hookOutput{Continue: true})
		return
	}
	defer s.Close()

	ws, err := s.GetActiveWorkSession()
	if err != nil {
		json.NewEncoder(w).Encode(hookOutput{Continue: true})
		return
	}

	delta := parseTranscriptDelta(h)
	startOffset := getTranscriptOffset(h.CWD)

	s.EnqueueCheckpointJob(store.CheckpointJobInput{
		WorkSessionID:         ws.ID,
		FeatureID:             ws.FeatureID,
		Reason:                "precompact",
		TriggerType:           h.Trigger,
		TranscriptStartOffset: startOffset,
		TranscriptEndOffset:   delta.EndOffset,
		SemanticText:          delta.SemanticText,
		MechanicalFacts:       delta.MechanicalFacts,
	})

	saveTranscriptOffset(h.CWD, delta.EndOffset)
	json.NewEncoder(w).Encode(hookOutput{Continue: true})
}

func handleSessionEnd(h *hookInput, w io.Writer) {
	s, err := store.Open(h.CWD)
	if err != nil {
		json.NewEncoder(w).Encode(stopHookOutput{})
		return
	}
	defer s.Close()

	ws, err := s.GetActiveWorkSession()
	if err != nil {
		json.NewEncoder(w).Encode(stopHookOutput{})
		return
	}

	delta := parseTranscriptDelta(h)
	if delta.HasContent {
		s.EnqueueCheckpointJob(store.CheckpointJobInput{
			WorkSessionID:         ws.ID,
			FeatureID:             ws.FeatureID,
			Reason:                "session_end",
			TriggerType:           "auto",
			TranscriptStartOffset: getTranscriptOffset(h.CWD),
			TranscriptEndOffset:   delta.EndOffset,
			SemanticText:          delta.SemanticText,
			MechanicalFacts:       delta.MechanicalFacts,
		})
	}

	features, _ := s.ListFeatures("in_progress")
	if len(features) > 0 {
		activeIDs := make(map[string]bool)
		for _, f := range features {
			activeIDs[f.ID] = true
			data, err := s.GetHandoffData(f.ID)
			if err != nil {
				continue
			}

			var cpData *HandoffCheckpointData
			if f.ID == ws.FeatureID {
				obs, _ := s.GetObservationsForWorkSession(ws.ID)
				mf, _ := s.GetMechanicalFactsForWorkSession(ws.ID)
				if len(obs) > 0 || mf != nil {
					cpData = &HandoffCheckpointData{
						Observations:    obs,
						MechanicalFacts: mf,
					}
				}
			}

			if writeErr := writeHandoffFileWithCheckpoints(h.CWD, data, cpData); writeErr != nil {
				s.MarkHandoffStale(ws.ID)
			}
		}
		cleanStaleHandoffs(h.CWD, activeIDs)
	}

	commitsPath := filepath.Join(h.CWD, ".docket", "commits.log")
	os.Remove(commitsPath)

	s.CloseWorkSession(ws.ID)
	json.NewEncoder(w).Encode(stopHookOutput{})
}

func handlePreToolUse(h *hookInput, w io.Writer) {
	allow := preToolUseOutput{
		HookSpecificOutput: &preToolUseDecision{PermissionDecision: "allow"},
	}

	// Check sentinel — already nudged this session
	sentinelPath := filepath.Join(h.CWD, ".docket", "agent-nudged")
	if _, err := os.Stat(sentinelPath); err == nil {
		json.NewEncoder(w).Encode(allow)
		return
	}

	s, err := store.Open(h.CWD)
	if err != nil {
		json.NewEncoder(w).Encode(allow)
		return
	}
	defer s.Close()

	features, err := s.ListFeatures("in_progress")
	if err != nil {
		json.NewEncoder(w).Encode(allow)
		return
	}

	if len(features) == 0 {
		os.WriteFile(sentinelPath, []byte{}, 0644)
		allow.SystemMessage = "[docket] No active docket feature. Call get_ready to set up tracking before dispatching subagents."
		json.NewEncoder(w).Encode(allow)
		return
	}

	// Check if top feature has task items
	_, total, err := s.GetFeatureProgress(features[0].ID)
	if err != nil {
		json.NewEncoder(w).Encode(allow)
		return
	}

	if total == 0 {
		os.WriteFile(sentinelPath, []byte{}, 0644)
		allow.SystemMessage = fmt.Sprintf(
			"[docket] Active feature %q (id: %s) has no task items. Add task items from the plan before dispatching subagents.",
			features[0].Title, features[0].ID,
		)
		json.NewEncoder(w).Encode(allow)
		return
	}

	// Feature has task items — all good
	json.NewEncoder(w).Encode(allow)
}

// formatUncheckedTasks queries unchecked task items for a feature and formats
// them as lines for the system message. Returns empty string if no unchecked items.
func formatUncheckedTasks(s *store.Store, featureID string) string {
	subtasks, err := s.GetSubtasksForFeature(featureID, false)
	if err != nil {
		return ""
	}

	var unchecked []store.TaskItem
	for _, st := range subtasks {
		for _, item := range st.Items {
			if !item.Checked {
				unchecked = append(unchecked, item)
			}
		}
	}

	if len(unchecked) == 0 {
		return ""
	}

	var b strings.Builder
	maxItems := 10
	for i, item := range unchecked {
		if i >= maxItems {
			b.WriteString(fmt.Sprintf("\n  ... and %d more", len(unchecked)-maxItems))
			break
		}
		b.WriteString(fmt.Sprintf("\n  #%d: %s", item.ID, item.Title))
	}
	return b.String()
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

	if importMsg != "" {
		// Plan file imported — needs agent for structuring
		out.SystemMessage = fmt.Sprintf("[docket] Commit recorded: %s %s%s\nDispatch board-manager agent (model: sonnet) to structure imported plan: feature_id=\"%s\", commit %s.",
			hash, msg, importMsg, features[0].ID, hash)
	} else {
		// Normal commit — direct MCP calls only
		taskList := formatUncheckedTasks(s, features[0].ID)
		if taskList != "" {
			out.SystemMessage = fmt.Sprintf("[docket] Commit recorded: %s %s\nFeature %q — unchecked tasks:%s\nCall complete_task_item for any items this commit completes, then update_feature (left_off, key_files).",
				hash, msg, features[0].Title, taskList)
		} else {
			out.SystemMessage = fmt.Sprintf("[docket] Commit recorded: %s %s\nUpdate feature %q: update_feature (left_off, key_files).",
				hash, msg, features[0].ID)
		}
	}
	json.NewEncoder(w).Encode(out)
}

// --- Transcript helpers ---

func parseTranscriptDelta(h *hookInput) *transcript.Delta {
	if h.TranscriptPath == "" {
		return &transcript.Delta{}
	}
	offset := getTranscriptOffset(h.CWD)
	delta, err := transcript.Parse(h.TranscriptPath, offset)
	if err != nil {
		fmt.Fprintf(os.Stderr, "docket hook: parse transcript: %v\n", err)
		return &transcript.Delta{EndOffset: offset}
	}
	return delta
}

func getTranscriptOffset(cwd string) int64 {
	data, err := os.ReadFile(filepath.Join(cwd, ".docket", "transcript-offset"))
	if err != nil {
		return 0
	}
	var offset int64
	fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &offset)
	return offset
}

func saveTranscriptOffset(cwd string, offset int64) {
	os.WriteFile(
		filepath.Join(cwd, ".docket", "transcript-offset"),
		[]byte(fmt.Sprintf("%d", offset)),
		0644,
	)
}

func isDeltaMeaningful(cwd string, delta *transcript.Delta) bool {
	// Check commits.log (PostToolUse hook writes here)
	commitsPath := filepath.Join(cwd, ".docket", "commits.log")
	if data, err := os.ReadFile(commitsPath); err == nil && len(strings.TrimSpace(string(data))) > 0 {
		return true
	}
	// Transcript-detected commits
	if len(delta.MechanicalFacts.Commits) > 0 {
		return true
	}
	if len(delta.MechanicalFacts.Errors) > 0 {
		return true
	}
	for _, tr := range delta.MechanicalFacts.TestRuns {
		if !tr.Passed {
			return true
		}
	}
	// Substantial conversation volume
	if len(delta.SemanticText) >= 300 {
		return true
	}
	// Non-trivial user input that didn't meet the 300-char threshold
	// (e.g. a short but meaningful instruction like "try the other approach")
	if delta.HasContent {
		return true
	}
	return false
}

// --- Utility functions ---

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
