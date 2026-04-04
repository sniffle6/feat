package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/sniffle6/claude-docket/internal/handoff"
	"github.com/sniffle6/claude-docket/internal/store"
	"github.com/sniffle6/claude-docket/internal/transcript"
)

func checkpointHandler(s *store.Store, projectDir string, onCheckpoint func(), binding *Binding) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		endSession := false
		if v, ok := args["end_session"]; ok {
			if b, ok := v.(bool); ok {
				endSession = b
			}
		}

		// Get cached identity
		wsID, featureID, claudeSessionID, bound := binding.Get()
		if !bound {
			// Try env var auto-bind for dashboard launches
			if lf := os.Getenv("DOCKET_LAUNCH_FEATURE"); lf != "" {
				ws, err := s.GetOpenWorkSessionForFeature(lf)
				if err == nil && ws != nil {
					pid := int64(os.Getpid())
					s.SetMcpPid(ws.ID, &pid)
					binding.Set(ws.ID, ws.FeatureID, ws.ClaudeSessionID)
					wsID, featureID, claudeSessionID, bound = ws.ID, ws.FeatureID, ws.ClaudeSessionID, true
				}
			}
			if !bound {
				return mcp.NewToolResultError(
					"No session bound. Call bind_session(feature_id=\"...\", session_id=\"...\") first — see your session context for the IDs.",
				), nil
			}
		}

		// Read transcript offset — scoped by claudeSessionID
		offsetPath := filepath.Join(projectDir, ".docket", "transcript-offset-"+claudeSessionID)
		// Fallback to unscoped path for backwards compatibility
		if _, err := os.Stat(offsetPath); os.IsNotExist(err) {
			offsetPath = filepath.Join(projectDir, ".docket", "transcript-offset")
		}
		var startOffset int64
		if data, err := os.ReadFile(offsetPath); err == nil {
			fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &startOffset)
		}

		// Find transcript path
		transcriptPath := findTranscriptPath(claudeSessionID)

		var delta *transcript.Delta
		if transcriptPath != "" {
			var parseErr error
			delta, parseErr = transcript.Parse(transcriptPath, startOffset)
			if parseErr != nil {
				return mcp.NewToolResultError(fmt.Sprintf("parse transcript: %v", parseErr)), nil
			}
		} else {
			delta = &transcript.Delta{EndOffset: startOffset}
		}

		reason := "manual_checkpoint"
		if endSession {
			reason = "manual_end_session"
		}

		job, err := s.EnqueueCheckpointJob(store.CheckpointJobInput{
			WorkSessionID:         wsID,
			FeatureID:             featureID,
			Reason:                reason,
			TriggerType:           "manual",
			TranscriptStartOffset: startOffset,
			TranscriptEndOffset:   delta.EndOffset,
			SemanticText:          delta.SemanticText,
			MechanicalFacts:       delta.MechanicalFacts,
		})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("enqueue checkpoint: %v", err)), nil
		}
		if onCheckpoint != nil {
			onCheckpoint()
		}

		// Update offset — write to session-scoped path
		scopedOffsetPath := filepath.Join(projectDir, ".docket", "transcript-offset-"+claudeSessionID)
		os.WriteFile(scopedOffsetPath, []byte(fmt.Sprintf("%d", delta.EndOffset)), 0644)

		if endSession {
			data, err := s.GetHandoffData(featureID)
			if err == nil {
				handoff.WriteFile(projectDir, data, nil)
			}
			s.SetMcpPid(wsID, nil)
			s.CloseWorkSession(wsID)
			binding.Clear()

			return mcp.NewToolResultText(fmt.Sprintf(
				"Work session closed for feature %q. Checkpoint #%d enqueued. Handoff written. Call bind_session to start a new session.",
				featureID, job.ID,
			)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf(
			"Checkpoint #%d enqueued for feature %q. %d chars semantic text, %d files edited.",
			job.ID, featureID, len(delta.SemanticText), len(delta.MechanicalFacts.FilesEdited),
		)), nil
	}
}

func findTranscriptPath(claudeSessionID string) string {
	if claudeSessionID == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	projectsDir := filepath.Join(home, ".claude", "projects")
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		candidate := filepath.Join(projectsDir, entry.Name(), claudeSessionID+".jsonl")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}
