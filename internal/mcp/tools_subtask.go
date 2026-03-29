package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/sniffle6/claude-docket/internal/store"
)

func importPlanHandler(s *store.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		id, ok := argString(args, "id")
		if !ok || id == "" {
			return mcp.NewToolResultError("missing required parameter: id"), nil
		}
		planPath, ok := argString(args, "plan_path")
		if !ok || planPath == "" {
			return mcp.NewToolResultError("missing required parameter: plan_path"), nil
		}

		result, err := s.ImportPlan(id, planPath)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		var b strings.Builder
		fmt.Fprintf(&b, "Imported %d subtasks, %d task items for %s:\n", result.SubtaskCount, result.TaskItemCount, id)
		for _, st := range result.Subtasks {
			if len(st.ItemIDs) > 0 {
				fmt.Fprintf(&b, "  Subtask: %s (items #%d-#%d)\n", st.Title, st.ItemIDs[0], st.ItemIDs[len(st.ItemIDs)-1])
			} else {
				fmt.Fprintf(&b, "  Subtask: %s (no items)\n", st.Title)
			}
		}
		return mcp.NewToolResultText(b.String()), nil
	}
}

func completeTaskItemHandler(s *store.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()

		// Batch mode: items JSON array
		if itemsJSON, ok := argString(args, "items"); ok && itemsJSON != "" {
			var entries []struct {
				ID         string `json:"id"`
				Outcome    string `json:"outcome"`
				CommitHash string `json:"commit_hash"`
				KeyFiles   string `json:"key_files"`
			}
			if err := json.Unmarshal([]byte(itemsJSON), &entries); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("invalid JSON: %v", err)), nil
			}

			var results []string
			for _, e := range entries {
				id := parseInt64(e.ID)
				var keyFiles []string
				if e.KeyFiles != "" {
					for _, f := range strings.Split(e.KeyFiles, ",") {
						keyFiles = append(keyFiles, strings.TrimSpace(f))
					}
				}

				if err := s.CompleteTaskItem(id, store.TaskItemCompletion{
					Outcome:    e.Outcome,
					CommitHash: e.CommitHash,
					KeyFiles:   keyFiles,
				}); err != nil {
					results = append(results, fmt.Sprintf("#%d: error: %s", id, err))
					continue
				}
				results = append(results, fmt.Sprintf("#%d: done", id))
			}

			return mcp.NewToolResultText(fmt.Sprintf("Completed %d items: %s", len(entries), strings.Join(results, ", "))), nil
		}

		// Single mode: id + outcome
		idStr, ok := argString(args, "id")
		if !ok || idStr == "" {
			return mcp.NewToolResultError("missing required parameter: id (or provide items JSON array for batch)"), nil
		}
		id := parseInt64(idStr)
		outcome, ok := argString(args, "outcome")
		if !ok || outcome == "" {
			return mcp.NewToolResultError("missing required parameter: outcome"), nil
		}
		commitHash, _ := argString(args, "commit_hash")

		var keyFiles []string
		if v, ok := argString(args, "key_files"); ok && v != "" {
			for _, f := range strings.Split(v, ",") {
				keyFiles = append(keyFiles, strings.TrimSpace(f))
			}
		}

		if err := s.CompleteTaskItem(id, store.TaskItemCompletion{
			Outcome:    outcome,
			CommitHash: commitHash,
			KeyFiles:   keyFiles,
		}); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		item, _ := s.GetTaskItem(id)
		if item != nil {
			st, _ := s.GetSubtask(item.SubtaskID)
			if st != nil {
				items, _ := s.GetTaskItemsForSubtask(st.ID)
				done := 0
				for _, i := range items {
					if i.Checked {
						done++
					}
				}
				return mcp.NewToolResultText(fmt.Sprintf("Task #%d done. %s: %d/%d", id, st.Title, done, len(items))), nil
			}
		}

		return mcp.NewToolResultText(fmt.Sprintf("Task #%d done.", id)), nil
	}
}

func addSubtaskHandler(s *store.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		featureID, ok := argString(args, "feature_id")
		if !ok || featureID == "" {
			return mcp.NewToolResultError("missing required parameter: feature_id"), nil
		}
		titleRaw, ok := argString(args, "title")
		if !ok || titleRaw == "" {
			return mcp.NewToolResultError("missing required parameter: title"), nil
		}

		titles := strings.Split(titleRaw, "|")
		subtasks, _ := s.GetSubtasksForFeature(featureID, false)
		position := len(subtasks) + 1

		var lines []string
		for _, title := range titles {
			title = strings.TrimSpace(title)
			if title == "" {
				continue
			}
			st, err := s.AddSubtask(featureID, title, position)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed on %q: %v", title, err)), nil
			}
			lines = append(lines, fmt.Sprintf("Subtask #%d: %s", st.ID, st.Title))
			position++
		}

		if len(lines) == 1 {
			return mcp.NewToolResultText(lines[0] + " created"), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Created %d subtasks:\n%s", len(lines), strings.Join(lines, "\n"))), nil
	}
}

func addTaskItemHandler(s *store.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		subtaskIDStr, ok := argString(args, "subtask_id")
		if !ok || subtaskIDStr == "" {
			return mcp.NewToolResultError("missing required parameter: subtask_id"), nil
		}
		subtaskID := parseInt64(subtaskIDStr)
		titleRaw, ok := argString(args, "title")
		if !ok || titleRaw == "" {
			return mcp.NewToolResultError("missing required parameter: title"), nil
		}

		titles := strings.Split(titleRaw, "|")
		items, _ := s.GetTaskItemsForSubtask(subtaskID)
		position := len(items) + 1

		var lines []string
		for _, title := range titles {
			title = strings.TrimSpace(title)
			if title == "" {
				continue
			}
			item, err := s.AddTaskItem(subtaskID, title, position)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed on %q: %v", title, err)), nil
			}
			lines = append(lines, fmt.Sprintf("Task item #%d: %s", item.ID, item.Title))
			position++
		}

		if len(lines) == 1 {
			return mcp.NewToolResultText(lines[0] + " created"), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Created %d task items:\n%s", len(lines), strings.Join(lines, "\n"))), nil
	}
}
