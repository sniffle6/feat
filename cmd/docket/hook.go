package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

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
	var buf bytes.Buffer
	if err := runHookFrom(os.Stdin, &buf); err != nil {
		fmt.Fprintf(os.Stderr, "docket hook: %v\n", err)
	}
	buf.WriteTo(os.Stdout)
	// Never exit non-zero — a hook failure is worse than doing nothing.
}

// safeDefault writes the appropriate fallback JSON for a hook event when
// stdin cannot be decoded. eventHint is best-effort (may be empty).
func safeDefault(w io.Writer, eventHint string) {
	switch eventHint {
	case "PreToolUse":
		json.NewEncoder(w).Encode(preToolUseOutput{
			HookSpecificOutput: &preToolUseDecision{PermissionDecision: "allow"},
		})
	case "Stop", "SessionEnd":
		json.NewEncoder(w).Encode(stopHookOutput{})
	default:
		json.NewEncoder(w).Encode(hookOutput{Continue: true})
	}
}

func runHookFrom(r io.Reader, w io.Writer) error {
	var h hookInput
	data, readErr := io.ReadAll(r)
	if readErr != nil || len(data) == 0 {
		fmt.Fprintf(os.Stderr, "docket hook: empty or unreadable stdin\n")
		safeDefault(w, "")
		return nil
	}

	if err := json.Unmarshal(data, &h); err != nil {
		eventHint := extractEventHint(data)
		fmt.Fprintf(os.Stderr, "docket hook: decode stdin: %v (fallback: %s)\n", err, eventHint)
		safeDefault(w, eventHint)
		return nil
	}

	// Check .docket/ exists — if not, project hasn't been initialized
	docketDir := filepath.Join(h.CWD, ".docket")
	if _, err := os.Stat(docketDir); os.IsNotExist(err) {
		// Not a docket project — pass through silently
		if h.HookEventName == "Stop" || h.HookEventName == "SessionEnd" {
			json.NewEncoder(w).Encode(stopHookOutput{})
		} else {
			json.NewEncoder(w).Encode(hookOutput{Continue: true})
		}
		return nil
	}

	switch h.HookEventName {
	case "PreToolUse":
		handlePreToolUse(&h, w)
	case "SessionStart":
		handleSessionStart(&h, w)
	case "PostToolUse":
		handlePostToolUse(&h, w)
	case "Stop":
		handleStop(&h, w)
	case "PreCompact":
		handlePreCompact(&h, w)
	case "SessionEnd":
		handleSessionEnd(&h, w)
	default:
		json.NewEncoder(w).Encode(hookOutput{Continue: true})
	}
	return nil
}

// extractEventHint tries to pull "hook_event_name" from partial/malformed JSON
// using simple string matching. Returns empty string if not found.
func extractEventHint(data []byte) string {
	s := string(data)
	key := `"hook_event_name"`
	idx := strings.Index(s, key)
	if idx < 0 {
		return ""
	}
	rest := s[idx+len(key):]
	rest = strings.TrimLeft(rest, " \t\n\r:")
	if len(rest) == 0 || rest[0] != '"' {
		return ""
	}
	rest = rest[1:]
	end := strings.IndexByte(rest, '"')
	if end < 0 {
		return rest
	}
	return rest[:end]
}

// agentPendingPath returns the per-session sentinel path for tracking pending subagents.
func agentPendingPath(cwd, sessionID string) string {
	return filepath.Join(cwd, ".docket", "agent-pending-"+sessionID)
}

// needsAttentionPath returns the per-session sentinel path for the needs-attention state.
func needsAttentionPath(cwd, sessionID string) string {
	return filepath.Join(cwd, ".docket", "needs-attention-"+sessionID)
}

func commitsLogPath(cwd, sessionID string) string {
	return filepath.Join(cwd, ".docket", "commits-"+sessionID+".log")
}

func transcriptOffsetPath(cwd, sessionID string) string {
	return filepath.Join(cwd, ".docket", "transcript-offset-"+sessionID)
}

func agentNudgedPath(cwd, sessionID string) string {
	return filepath.Join(cwd, ".docket", "agent-nudged-"+sessionID)
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

	// Create/clear session-scoped state files
	os.WriteFile(commitsLogPath(h.CWD, h.SessionID), []byte{}, 0644)
	os.Remove(agentNudgedPath(h.CWD, h.SessionID))
	os.Remove(agentPendingPath(h.CWD, h.SessionID))
	os.WriteFile(transcriptOffsetPath(h.CWD, h.SessionID), []byte("0"), 0644)
	os.WriteFile(filepath.Join(h.CWD, ".docket", "heartbeat-"+h.SessionID), []byte{}, 0644)

	features, err := s.ListFeatures("in_progress")
	if err != nil {
		fmt.Fprintf(os.Stderr, "docket hook: list features: %v\n", err)
		json.NewEncoder(w).Encode(hookOutput{Continue: true})
		return
	}

	// Also consider planned features — auto-promote to in_progress when binding
	if len(features) == 0 {
		planned, _ := s.ListFeatures("planned")
		features = planned
	}

	out := hookOutput{Continue: true}

	if len(features) == 0 {
		out.SystemMessage = archiveMsg + fmt.Sprintf("[docket] Session: %s | No active features. Use docket MCP tools to create one.", h.SessionID)
		json.NewEncoder(w).Encode(out)
		return
	}

	var topFeature store.Feature

	// Dashboard launch — bind to specific feature via env var
	if launchFeature := os.Getenv("DOCKET_LAUNCH_FEATURE"); launchFeature != "" {
		for _, f := range features {
			if f.ID == launchFeature {
				topFeature = f
				break
			}
		}
	}

	// Occupancy-aware selection for manual sessions (or if dashboard feature not found)
	if topFeature.ID == "" {
		for _, f := range features {
			openSess, _ := s.GetOpenWorkSessionForFeature(f.ID)
			if openSess == nil {
				topFeature = f
				break
			}
			// Reclaim zombie: no mcp_pid claim, not a placeholder, stale heartbeat >24h
			if openSess.McpPid == nil &&
				openSess.ClaudeSessionID != "dashboard-launch" &&
				openSess.LastHeartbeat != nil &&
				time.Since(*openSess.LastHeartbeat) > 24*time.Hour {
				topFeature = f
				break
			}
		}
	}

	// Supersession fallback — only supersede real sessions, never placeholders
	if topFeature.ID == "" {
		for _, f := range features {
			openSess, _ := s.GetOpenWorkSessionForFeature(f.ID)
			if openSess != nil && openSess.ClaudeSessionID != "dashboard-launch" {
				topFeature = f
				break
			}
		}
	}

	// All features occupied by placeholders
	if topFeature.ID == "" {
		out.SystemMessage = archiveMsg + fmt.Sprintf(
			"[docket] Session: %s | All in-progress features are occupied.\nUse get_ready to see features, then bind_session(feature_id=\"...\", session_id=\"%s\") to pick one.",
			h.SessionID, h.SessionID,
		)
		json.NewEncoder(w).Encode(out)
		return
	}

	ws, wsErr := s.OpenWorkSession(topFeature.ID, h.SessionID)
	if wsErr == nil {
		s.SetSessionState(ws.ID, "working")
	}

	// Auto-promote planned features to in_progress when a session binds
	if topFeature.Status == "planned" {
		inProgress := "in_progress"
		s.UpdateFeature(topFeature.ID, store.FeatureUpdate{Status: &inProgress})
		topFeature.Status = "in_progress"
	}

	var msg strings.Builder
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
		if f.ID == topFeature.ID {
			continue
		}
		otherHandoff := filepath.Join(h.CWD, ".docket", "handoff", f.ID+".md")
		if _, err := os.Stat(otherHandoff); err == nil {
			msg.WriteString(fmt.Sprintf("\n[docket] Handoff available: .docket/handoff/%s.md", f.ID))
		} else {
			msg.WriteString(fmt.Sprintf("\n[docket] Also active: %s (id: %s)", f.Title, f.ID))
		}
	}

	// Prepend session binding info
	bindInfo := fmt.Sprintf("[docket] Session: %s | Feature: %s (id: %s)\nBind docket: bind_session(feature_id=%q, session_id=%q)\n\n",
		h.SessionID, topFeature.Title, topFeature.ID, topFeature.ID, h.SessionID)

	out.SystemMessage = archiveMsg + bindInfo + msg.String()
	json.NewEncoder(w).Encode(out)
}

func handleStop(h *hookInput, w io.Writer) {
	s, err := store.Open(h.CWD)
	if err != nil {
		json.NewEncoder(w).Encode(stopHookOutput{})
		return
	}
	defer s.Close()

	ws, err := s.GetWorkSessionByClaudeSession(h.SessionID)
	if err != nil {
		json.NewEncoder(w).Encode(stopHookOutput{})
		return
	}

	// Check if a subagent is running — if so, stay in "subagent" state
	// instead of "needs_attention" (which shows "Waiting" on dashboard).
	apPath := agentPendingPath(h.CWD, h.SessionID)
	if pidData, err := os.ReadFile(apPath); err == nil {
		pid := strings.TrimSpace(string(pidData))
		if isPIDAlive(pid) {
			s.SetSessionState(ws.ID, "subagent")
			s.TouchHeartbeat(ws.ID)

			delta := parseTranscriptDelta(h)
			if isDeltaMeaningful(h.CWD, h.SessionID, delta) {
				s.EnqueueCheckpointJob(store.CheckpointJobInput{
					WorkSessionID:         ws.ID,
					FeatureID:             ws.FeatureID,
					Reason:                "stop",
					TriggerType:           "auto",
					TranscriptStartOffset: getTranscriptOffset(h.CWD, h.SessionID),
					TranscriptEndOffset:   delta.EndOffset,
					SemanticText:          delta.SemanticText,
					MechanicalFacts:       delta.MechanicalFacts,
				})
				saveTranscriptOffset(h.CWD, h.SessionID, delta.EndOffset)
			}

			json.NewEncoder(w).Encode(stopHookOutput{})
			return
		}
		// PID dead — stale sentinel, clean up and fall through to needs_attention
		os.Remove(apPath)
	}

	s.SetSessionState(ws.ID, "needs_attention")
	s.TouchHeartbeat(ws.ID)

	// Write sentinel so PostToolUse can skip SQLite open in the common case
	sentinelPath := needsAttentionPath(h.CWD, h.SessionID)
	os.WriteFile(sentinelPath, []byte{}, 0644)

	delta := parseTranscriptDelta(h)
	if !isDeltaMeaningful(h.CWD, h.SessionID, delta) {
		json.NewEncoder(w).Encode(stopHookOutput{})
		return
	}

	s.EnqueueCheckpointJob(store.CheckpointJobInput{
		WorkSessionID:         ws.ID,
		FeatureID:             ws.FeatureID,
		Reason:                "stop",
		TriggerType:           "auto",
		TranscriptStartOffset: getTranscriptOffset(h.CWD, h.SessionID),
		TranscriptEndOffset:   delta.EndOffset,
		SemanticText:          delta.SemanticText,
		MechanicalFacts:       delta.MechanicalFacts,
	})

	saveTranscriptOffset(h.CWD, h.SessionID, delta.EndOffset)
	json.NewEncoder(w).Encode(stopHookOutput{})
}

func handlePreCompact(h *hookInput, w io.Writer) {
	s, err := store.Open(h.CWD)
	if err != nil {
		json.NewEncoder(w).Encode(hookOutput{Continue: true})
		return
	}
	defer s.Close()

	ws, err := s.GetWorkSessionByClaudeSession(h.SessionID)
	if err != nil {
		json.NewEncoder(w).Encode(hookOutput{Continue: true})
		return
	}

	s.TouchHeartbeat(ws.ID)

	delta := parseTranscriptDelta(h)
	startOffset := getTranscriptOffset(h.CWD, h.SessionID)

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

	saveTranscriptOffset(h.CWD, h.SessionID, delta.EndOffset)
	json.NewEncoder(w).Encode(hookOutput{Continue: true})
}

func handleSessionEnd(h *hookInput, w io.Writer) {
	s, err := store.Open(h.CWD)
	if err != nil {
		json.NewEncoder(w).Encode(stopHookOutput{})
		return
	}
	defer s.Close()

	ws, err := s.GetWorkSessionByClaudeSession(h.SessionID)
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
			TranscriptStartOffset: getTranscriptOffset(h.CWD, h.SessionID),
			TranscriptEndOffset:   delta.EndOffset,
			SemanticText:          delta.SemanticText,
			MechanicalFacts:       delta.MechanicalFacts,
		})
	}

	// Only write handoff for this session's feature
	data, hdErr := s.GetHandoffData(ws.FeatureID)
	if hdErr == nil {
		var cpData *HandoffCheckpointData
		obs, _ := s.GetObservationsForWorkSession(ws.ID)
		mf, _ := s.GetMechanicalFactsForWorkSession(ws.ID)
		if len(obs) > 0 || mf != nil {
			cpData = &HandoffCheckpointData{
				Observations:    obs,
				MechanicalFacts: mf,
			}
		}
		if writeErr := writeHandoffFileWithCheckpoints(h.CWD, data, cpData); writeErr != nil {
			s.MarkHandoffStale(ws.ID)
		}
	}

	os.Remove(commitsLogPath(h.CWD, h.SessionID))
	os.Remove(needsAttentionPath(h.CWD, h.SessionID))
	os.Remove(agentPendingPath(h.CWD, h.SessionID))
	os.Remove(filepath.Join(h.CWD, ".docket", "heartbeat-"+h.SessionID))

	s.SetSessionState(ws.ID, "idle")
	s.CloseWorkSession(ws.ID)
	json.NewEncoder(w).Encode(stopHookOutput{})
}

func handlePreToolUse(h *hookInput, w io.Writer) {
	allow := preToolUseOutput{
		HookSpecificOutput: &preToolUseDecision{PermissionDecision: "allow"},
	}

	// Mark that a subagent is pending so Stop hook can distinguish
	// "waiting for user" from "waiting for subagent".
	apPath := agentPendingPath(h.CWD, h.SessionID)
	os.WriteFile(apPath, []byte(fmt.Sprintf("%d", os.Getppid())), 0644)

	s, err := store.Open(h.CWD)
	if err != nil {
		json.NewEncoder(w).Encode(allow)
		return
	}
	defer s.Close()

	// Check sentinel — already nudged this session
	sentinelPath := agentNudgedPath(h.CWD, h.SessionID)
	if _, err := os.Stat(sentinelPath); err == nil {
		json.NewEncoder(w).Encode(allow)
		return
	}

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

	// Clean up agent-pending sentinel — subagent has returned
	apPath := agentPendingPath(h.CWD, h.SessionID)
	hadAgentPending := false
	if _, statErr := os.Stat(apPath); statErr == nil {
		os.Remove(apPath)
		hadAgentPending = true
	}

	// Flip session state back to working if Claude resumes after a stop or subagent.
	// Check sentinel file first to avoid opening SQLite on every tool call.
	sentinelPath := needsAttentionPath(h.CWD, h.SessionID)
	needsStateFlip := false
	if _, statErr := os.Stat(sentinelPath); statErr == nil || hadAgentPending {
		os.Remove(sentinelPath)
		needsStateFlip = true
	}

	// Periodic heartbeat — throttled via file timestamp to avoid opening
	// SQLite on every tool call. Without this, active sessions show "Stale"
	// on the dashboard after 5 min of normal (non-commit) tool use.
	needsHeartbeat := false
	hbPath := filepath.Join(h.CWD, ".docket", "heartbeat-"+h.SessionID)
	if info, err := os.Stat(hbPath); err != nil || time.Since(info.ModTime()) > 2*time.Minute {
		needsHeartbeat = true
	}

	if needsStateFlip || needsHeartbeat {
		if s, err := store.Open(h.CWD); err == nil {
			if ws, wsErr := s.GetWorkSessionByClaudeSession(h.SessionID); wsErr == nil {
				if needsStateFlip {
					s.SetSessionState(ws.ID, "working")
				}
				s.TouchHeartbeat(ws.ID)
			}
			s.Close()
		}
		os.WriteFile(hbPath, []byte{}, 0644)
	}

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

	// Append to session-scoped commits log
	commitsPath := commitsLogPath(h.CWD, h.SessionID)
	f, err := os.OpenFile(commitsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "docket hook: open commits.log: %v\n", err)
		json.NewEncoder(w).Encode(out)
		return
	}
	f.WriteString(line + "\n")
	f.Close()

	// Find the feature this session is working on
	s, err := store.Open(h.CWD)
	if err != nil {
		json.NewEncoder(w).Encode(out)
		return
	}
	defer s.Close()

	// Use the work session to find the bound feature — more reliable than
	// listing in_progress features and guessing which one is ours.
	var featureID, featureTitle string
	if ws, wsErr := s.GetWorkSessionByClaudeSession(h.SessionID); wsErr == nil {
		s.TouchHeartbeat(ws.ID)
		featureID = ws.FeatureID
		if f, fErr := s.GetFeature(featureID); fErr == nil {
			featureTitle = f.Title
		}
	}
	if featureID == "" {
		// Fallback: first in_progress feature
		features, fErr := s.ListFeatures("in_progress")
		if fErr != nil || len(features) == 0 {
			json.NewEncoder(w).Encode(out)
			return
		}
		featureID = features[0].ID
		featureTitle = features[0].Title
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
				result, importErr := s.ImportPlan(featureID, absPath)
				if importErr == nil {
					importMsg = fmt.Sprintf("\n[docket] Auto-imported plan: %d subtasks, %d items from %s", result.SubtaskCount, result.TaskItemCount, cf)
				}
				break // only import first plan file found
			}
		}
	}

	if importMsg != "" {
		// Plan file imported — show unchecked tasks (includes newly imported ones)
		taskList := formatUncheckedTasks(s, featureID)
		if taskList != "" {
			out.SystemMessage = fmt.Sprintf("[docket] Commit recorded: %s %s%s\nFeature %q — unchecked tasks:%s\nDispatch board-manager agent (model: sonnet) to structure imported plan: feature_id=\"%s\", commit %s.",
				hash, msg, importMsg, featureTitle, taskList, featureID, hash)
		} else {
			out.SystemMessage = fmt.Sprintf("[docket] Commit recorded: %s %s%s\nDispatch board-manager agent (model: sonnet) to structure imported plan: feature_id=\"%s\", commit %s.",
				hash, msg, importMsg, featureID, hash)
		}
	} else {
		// Normal commit — direct MCP calls only
		taskList := formatUncheckedTasks(s, featureID)
		if taskList != "" {
			out.SystemMessage = fmt.Sprintf("[docket] Commit recorded: %s %s\nFeature %q — unchecked tasks:%s\nCall complete_task_item for any items this commit completes, then update_feature (left_off, key_files).",
				hash, msg, featureTitle, taskList)
		} else {
			out.SystemMessage = fmt.Sprintf("[docket] Commit recorded: %s %s\nUpdate feature %q: update_feature (left_off, key_files).",
				hash, msg, featureID)
		}
	}
	json.NewEncoder(w).Encode(out)
}

// --- Transcript helpers ---

func parseTranscriptDelta(h *hookInput) *transcript.Delta {
	if h.TranscriptPath == "" {
		return &transcript.Delta{}
	}
	offset := getTranscriptOffset(h.CWD, h.SessionID)
	delta, err := transcript.Parse(h.TranscriptPath, offset)
	if err != nil {
		fmt.Fprintf(os.Stderr, "docket hook: parse transcript: %v\n", err)
		return &transcript.Delta{EndOffset: offset}
	}
	return delta
}

func getTranscriptOffset(cwd, sessionID string) int64 {
	data, err := os.ReadFile(transcriptOffsetPath(cwd, sessionID))
	if err != nil {
		return 0
	}
	var offset int64
	fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &offset)
	return offset
}

func saveTranscriptOffset(cwd, sessionID string, offset int64) {
	os.WriteFile(transcriptOffsetPath(cwd, sessionID), []byte(fmt.Sprintf("%d", offset)), 0644)
}

func isDeltaMeaningful(cwd, sessionID string, delta *transcript.Delta) bool {
	// Check session-scoped commits log (PostToolUse hook writes here)
	commitsPath := commitsLogPath(cwd, sessionID)
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
