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

func registerTools(srv *server.MCPServer, s *store.Store) {
	srv.AddTool(mcp.NewTool("add_feature",
		mcp.WithDescription("Create a new feature to track. Returns the generated slug ID."),
		mcp.WithString("title", mcp.Required(), mcp.Description("Feature title (e.g., 'Bluetooth Panel')")),
		mcp.WithString("description", mcp.Description("What the feature is")),
		mcp.WithString("status", mcp.Description("Initial status: planned (default), in_progress, blocked, dev_complete")),
		mcp.WithString("notes", mcp.Description("User notes — thoughts, ideas, context for Claude to read when picking up this feature")),
	), addFeatureHandler(s))

	srv.AddTool(mcp.NewTool("update_feature",
		mcp.WithDescription("Update a feature's status, description, left_off note, notes, worktree_path, or key_files."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Feature slug ID")),
		mcp.WithString("status", mcp.Description("New status: planned, in_progress, done, blocked, dev_complete")),
		mcp.WithString("title", mcp.Description("New title")),
		mcp.WithString("description", mcp.Description("New description")),
		mcp.WithString("left_off", mcp.Description("Where work stopped — free text")),
		mcp.WithString("notes", mcp.Description("User notes — thoughts, ideas, context for Claude")),
		mcp.WithString("worktree_path", mcp.Description("Absolute path to git worktree")),
		mcp.WithString("key_files", mcp.Description("Comma-separated list of key file paths for this feature")),
	), updateFeatureHandler(s))

	srv.AddTool(mcp.NewTool("list_features",
		mcp.WithDescription("List all features. Returns compact summaries: ID, title, status, left_off snippet. Filter by status optionally."),
		mcp.WithString("status", mcp.Description("Filter by status: planned, in_progress, done, blocked, dev_complete. Omit for all.")),
	), listFeaturesHandler(s))

	srv.AddTool(mcp.NewTool("get_feature",
		mcp.WithDescription("Get full detail for one feature including all linked sessions."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Feature slug ID")),
	), getFeatureHandler(s))

	srv.AddTool(mcp.NewTool("log_session",
		mcp.WithDescription("Record a session summary. Call this at end of session to log what was accomplished."),
		mcp.WithString("feature_id", mcp.Required(), mcp.Description("Feature slug ID this session was about.")),
		mcp.WithString("summary", mcp.Required(), mcp.Description("Brief summary of what was accomplished")),
		mcp.WithString("files_touched", mcp.Description("Comma-separated list of files modified this session")),
		mcp.WithString("commits", mcp.Description("Comma-separated list of commit hashes made this session")),
	), logSessionHandler(s))

	srv.AddTool(mcp.NewTool("get_context",
		mcp.WithDescription("Get a token-efficient briefing for a feature: status, where we left off, worktree path, recent sessions, key files. ~15-20 lines."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Feature slug ID")),
	), getContextHandler(s))

	srv.AddTool(mcp.NewTool("get_ready",
		mcp.WithDescription("List actionable features: in_progress first (resume), then planned (start new). Excludes blocked and done."),
	), getReadyHandler(s))

	srv.AddTool(mcp.NewTool("compact_sessions",
		mcp.WithDescription("Compact old sessions for a feature into a single summary. Keeps the last 3 sessions, replaces older ones with the provided summary. Call get_feature first to read the sessions, then write a summary."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Feature slug ID")),
		mcp.WithString("summary", mcp.Required(), mcp.Description("Summary of the compacted sessions (you write this after reading the old sessions)")),
	), compactSessionsHandler(s))

	srv.AddTool(mcp.NewTool("import_plan",
		mcp.WithDescription("Import a plan markdown file into subtasks and task items. Archives existing active subtasks first. Returns created subtask/item IDs."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Feature slug ID")),
		mcp.WithString("plan_path", mcp.Required(), mcp.Description("Absolute path to the plan markdown file")),
	), importPlanHandler(s))

	srv.AddTool(mcp.NewTool("complete_task_item",
		mcp.WithDescription("Mark a task item as done with outcome, commit hash, and key files."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Task item ID (number)")),
		mcp.WithString("outcome", mcp.Required(), mcp.Description("One-liner of what was accomplished")),
		mcp.WithString("commit_hash", mcp.Description("Git commit SHA")),
		mcp.WithString("key_files", mcp.Description("Comma-separated file paths modified")),
	), completeTaskItemHandler(s))

	srv.AddTool(mcp.NewTool("add_subtask",
		mcp.WithDescription("Manually add a subtask (phase) to a feature."),
		mcp.WithString("feature_id", mcp.Required(), mcp.Description("Feature slug ID")),
		mcp.WithString("title", mcp.Required(), mcp.Description("Subtask title")),
	), addSubtaskHandler(s))

	srv.AddTool(mcp.NewTool("add_task_item",
		mcp.WithDescription("Manually add a task item to a subtask."),
		mcp.WithString("subtask_id", mcp.Required(), mcp.Description("Parent subtask ID (number)")),
		mcp.WithString("title", mcp.Required(), mcp.Description("Task item title")),
	), addTaskItemHandler(s))

	srv.AddTool(mcp.NewTool("add_subtasks",
		mcp.WithDescription("Batch-add multiple subtasks (phases) to a feature in one call. More token-efficient than calling add_subtask repeatedly."),
		mcp.WithString("feature_id", mcp.Required(), mcp.Description("Feature slug ID")),
		mcp.WithString("titles", mcp.Required(), mcp.Description("Pipe-separated subtask titles (e.g., 'Phase 1|Phase 2|Phase 3')")),
	), addSubtasksHandler(s))

	srv.AddTool(mcp.NewTool("add_task_items",
		mcp.WithDescription("Batch-add multiple task items to a subtask in one call. More token-efficient than calling add_task_item repeatedly."),
		mcp.WithString("subtask_id", mcp.Required(), mcp.Description("Parent subtask ID (number)")),
		mcp.WithString("titles", mcp.Required(), mcp.Description("Pipe-separated task item titles (e.g., 'Write tests|Implement handler|Update docs')")),
	), addTaskItemsHandler(s))

	srv.AddTool(mcp.NewTool("complete_task_items",
		mcp.WithDescription("Batch-complete multiple task items in one call. More token-efficient than calling complete_task_item repeatedly. Each entry needs an ID and outcome; commit_hash and key_files are optional."),
		mcp.WithString("items", mcp.Required(), mcp.Description("JSON array of completions: [{\"id\":\"1\",\"outcome\":\"done\"},{\"id\":\"2\",\"outcome\":\"skipped\",\"commit_hash\":\"abc\",\"key_files\":\"a.go,b.go\"}]")),
	), completeTaskItemsHandler(s))

	srv.AddTool(mcp.NewTool("get_full_context",
		mcp.WithDescription("Get everything for a feature: all subtasks (including archived), all task items with outcomes and commits, all sessions. For subagent deep dives."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Feature slug ID")),
	), getFullContextHandler(s))

	srv.AddTool(mcp.NewTool("quick_track",
		mcp.WithDescription("Lightweight tracking for small, self-contained tasks (cosmetic changes, one-off fixes, config tweaks). One call — creates or updates a feature card with optional commit. Use instead of dispatching board-manager for simple work."),
		mcp.WithString("title", mcp.Required(), mcp.Description("What was done (e.g., 'Add logo to README and dashboard')")),
		mcp.WithString("commit_hash", mcp.Description("Git commit SHA to attach")),
		mcp.WithString("key_files", mcp.Description("Comma-separated file paths touched")),
		mcp.WithString("status", mcp.Description("Feature status: done (default), in_progress, planned")),
	), quickTrackHandler(s))

	srv.AddTool(mcp.NewTool("add_issue",
		mcp.WithDescription("Log a bug or issue on a feature card. Issues are visible on the dashboard and in get_feature output."),
		mcp.WithString("feature_id", mcp.Required(), mcp.Description("Feature slug ID")),
		mcp.WithString("description", mcp.Required(), mcp.Description("What's wrong — describe the bug")),
		mcp.WithString("task_item_id", mcp.Description("Optional task item ID this issue relates to")),
	), addIssueHandler(s))

	srv.AddTool(mcp.NewTool("resolve_issue",
		mcp.WithDescription("Mark an issue as resolved. Optionally attach the commit that fixed it."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Issue ID")),
		mcp.WithString("commit_hash", mcp.Description("Commit SHA that fixed the issue")),
	), resolveIssueHandler(s))

	srv.AddTool(mcp.NewTool("list_issues",
		mcp.WithDescription("List open issues. Filter by feature or list all open issues across all features."),
		mcp.WithString("feature_id", mcp.Description("Filter to one feature. Omit for all open issues.")),
	), listIssuesHandler(s))

	srv.AddTool(mcp.NewTool("add_decision",
		mcp.WithDescription("Log a decision on a feature. Records what approach was considered, whether it was accepted or rejected, and why. Prevents re-exploring dead ends across sessions."),
		mcp.WithString("feature_id", mcp.Required(), mcp.Description("Feature slug ID")),
		mcp.WithString("approach", mcp.Required(), mcp.Description("What was considered (e.g., 'Use websockets for real-time updates')")),
		mcp.WithString("outcome", mcp.Required(), mcp.Description("accepted or rejected")),
		mcp.WithString("reason", mcp.Required(), mcp.Description("Why — one-liner (e.g., 'Too complex for MVP, polling sufficient')")),
	), addDecisionHandler(s))
}

func addFeatureHandler(s *store.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		title := args["title"].(string)
		desc, _ := args["description"].(string)
		status, _ := args["status"].(string)
		notes, _ := args["notes"].(string)

		f, err := s.AddFeature(title, desc)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if status != "" && status != "planned" {
			s.UpdateFeature(f.ID, store.FeatureUpdate{Status: &status})
		}
		if notes != "" {
			s.UpdateFeature(f.ID, store.FeatureUpdate{Notes: &notes})
		}
		f, _ = s.GetFeature(f.ID)

		data, _ := json.MarshalIndent(f, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

func updateFeatureHandler(s *store.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		id := args["id"].(string)

		u := store.FeatureUpdate{}
		if v, ok := args["status"].(string); ok {
			u.Status = &v
		}
		if v, ok := args["title"].(string); ok {
			u.Title = &v
		}
		if v, ok := args["description"].(string); ok {
			u.Description = &v
		}
		if v, ok := args["left_off"].(string); ok {
			u.LeftOff = &v
		}
		if v, ok := args["notes"].(string); ok {
			u.Notes = &v
		}
		if v, ok := args["worktree_path"].(string); ok {
			u.WorktreePath = &v
		}
		if v, ok := args["key_files"].(string); ok && v != "" {
			files := strings.Split(v, ",")
			for i := range files {
				files[i] = strings.TrimSpace(files[i])
			}
			u.KeyFiles = &files
		}

		if err := s.UpdateFeature(id, u); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Updated feature %q", id)), nil
	}
}

func listFeaturesHandler(s *store.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		status, _ := req.GetArguments()["status"].(string)

		features, err := s.ListFeatures(status)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if len(features) == 0 {
			return mcp.NewToolResultText("No features found."), nil
		}

		var lines []string
		for _, f := range features {
			line := fmt.Sprintf("- **%s** [%s] %s", f.ID, f.Status, f.Title)
			if f.LeftOff != "" {
				snippet := f.LeftOff
				if len(snippet) > 60 {
					snippet = snippet[:60] + "..."
				}
				line += fmt.Sprintf(" — %s", snippet)
			}
			lines = append(lines, line)
		}
		return mcp.NewToolResultText(strings.Join(lines, "\n")), nil
	}
}

func getFeatureHandler(s *store.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id := req.GetArguments()["id"].(string)

		f, err := s.GetFeature(id)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		subtasks, _ := s.GetSubtasksForFeature(id, false)
		sessions, _ := s.GetSessionsForFeature(id)

		type fullFeature struct {
			store.Feature
			Subtasks  []store.Subtask  `json:"subtasks"`
			Sessions  []store.Session  `json:"sessions"`
			Decisions []store.Decision `json:"decisions"`
		}
		decisions, _ := s.GetDecisionsForFeature(id)
		if decisions == nil {
			decisions = []store.Decision{}
		}
		full := fullFeature{Feature: *f, Subtasks: subtasks, Sessions: sessions, Decisions: decisions}
		data, _ := json.MarshalIndent(full, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

func logSessionHandler(s *store.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		summary := args["summary"].(string)
		featureID := args["feature_id"].(string)

		if _, err := s.GetFeature(featureID); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("feature not found: %s", featureID)), nil
		}

		var filesTouched []string
		if v, ok := args["files_touched"].(string); ok && v != "" {
			for _, f := range strings.Split(v, ",") {
				filesTouched = append(filesTouched, strings.TrimSpace(f))
			}
		}

		var commits []string
		if v, ok := args["commits"].(string); ok && v != "" {
			for _, c := range strings.Split(v, ",") {
				commits = append(commits, strings.TrimSpace(c))
			}
		}

		sess, err := s.LogSession(store.SessionInput{
			FeatureID:    featureID,
			Summary:      summary,
			FilesTouched: filesTouched,
			Commits:      commits,
			AutoLinked:   featureID != "",
			LinkReason:   "provided by Claude at session end",
		})
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Session #%d logged.", sess.ID)), nil
	}
}

func getContextHandler(s *store.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id := req.GetArguments()["id"].(string)

		fc, err := s.GetContext(id)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		f := fc.Feature
		var b strings.Builder
		fmt.Fprintf(&b, "Title: %s\n", f.Title)
		fmt.Fprintf(&b, "Status: %s\n", f.Status)
		if f.WorktreePath != "" {
			fmt.Fprintf(&b, "Worktree: %s\n", f.WorktreePath)
		}
		if f.LeftOff != "" {
			fmt.Fprintf(&b, "Left off: %s\n", f.LeftOff)
		}
		if len(f.KeyFiles) > 0 {
			fmt.Fprintf(&b, "Key files: %s\n", strings.Join(f.KeyFiles, ", "))
		}
		if f.Notes != "" {
			fmt.Fprintf(&b, "User notes: %s\n", f.Notes)
		}

		done, total, _ := s.GetFeatureProgress(id)
		if total > 0 {
			fmt.Fprintf(&b, "Progress: %d/%d\n", done, total)
			subtasks, _ := s.GetSubtasksForFeature(id, false)
			var nextTask string
			for _, st := range subtasks {
				stDone := 0
				for _, item := range st.Items {
					if item.Checked {
						stDone++
					} else if nextTask == "" {
						nextTask = item.Title
					}
				}
				status := fmt.Sprintf("%d/%d", stDone, len(st.Items))
				if stDone == len(st.Items) {
					status += " ✓"
				}
				fmt.Fprintf(&b, "  %s [%s]\n", st.Title, status)
			}
			if nextTask != "" {
				fmt.Fprintf(&b, "Next: %s\n", nextTask)
			}
		}

		var rejected []store.Decision
		for _, d := range fc.Decisions {
			if d.Outcome == "rejected" {
				rejected = append(rejected, d)
			}
		}
		if len(rejected) > 0 {
			b.WriteString("Rejected approaches:\n")
			for _, d := range rejected {
				fmt.Fprintf(&b, "  - %s — %s\n", d.Approach, d.Reason)
			}
		}

		var allCommits []string
		if len(fc.RecentSessions) > 0 {
			b.WriteString("Recent sessions:\n")
			for _, sess := range fc.RecentSessions {
				fmt.Fprintf(&b, "  - %s: %s\n", sess.CreatedAt.Format("2006-01-02"), sess.Summary)
				allCommits = append(allCommits, sess.Commits...)
			}
		}

		// Collect commits from completed task items
		subtasks, _ := s.GetSubtasksForFeature(id, false)
		for _, st := range subtasks {
			for _, item := range st.Items {
				if item.CommitHash != "" {
					allCommits = append(allCommits, item.CommitHash)
				}
			}
		}

		// Deduplicate and surface commits
		if len(allCommits) > 0 {
			seen := make(map[string]bool)
			var unique []string
			for _, c := range allCommits {
				if !seen[c] {
					seen[c] = true
					unique = append(unique, c)
				}
			}
			b.WriteString("Commits: " + strings.Join(unique, ", ") + "\n")
			b.WriteString("Tip: run `git log --stat` on these commits to see which files this feature touches.\n")
		}

		return mcp.NewToolResultText(b.String()), nil
	}
}

func getReadyHandler(s *store.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		features, err := s.GetReadyFeatures()
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if len(features) == 0 {
			return mcp.NewToolResultText("Nothing ready — all features are done or blocked."), nil
		}

		var lines []string
		for _, f := range features {
			line := fmt.Sprintf("- **%s** [%s] %s", f.ID, f.Status, f.Title)
			if f.LeftOff != "" {
				snippet := f.LeftOff
				if len(snippet) > 60 {
					snippet = snippet[:60] + "..."
				}
				line += fmt.Sprintf(" — %s", snippet)
			}
			lines = append(lines, line)
		}
		return mcp.NewToolResultText(strings.Join(lines, "\n")), nil
	}
}

func compactSessionsHandler(s *store.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		id := args["id"].(string)
		summary := args["summary"].(string)

		n, err := s.CompactSessions(id, summary)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if n == 0 {
			return mcp.NewToolResultText("Nothing to compact (3 or fewer sessions)."), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Compacted %d sessions into 1 summary. Last 3 sessions preserved.", n)), nil
	}
}

func importPlanHandler(s *store.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		id := args["id"].(string)
		planPath := args["plan_path"].(string)

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
		id := parseInt64(args["id"].(string))
		outcome := args["outcome"].(string)
		commitHash, _ := args["commit_hash"].(string)

		var keyFiles []string
		if v, ok := args["key_files"].(string); ok && v != "" {
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

func completeTaskItemsHandler(s *store.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		itemsJSON := args["items"].(string)

		var entries []struct {
			ID        string `json:"id"`
			Outcome   string `json:"outcome"`
			CommitHash string `json:"commit_hash"`
			KeyFiles  string `json:"key_files"`
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
}

func addSubtaskHandler(s *store.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		featureID := args["feature_id"].(string)
		title := args["title"].(string)

		subtasks, _ := s.GetSubtasksForFeature(featureID, false)
		position := len(subtasks) + 1

		st, err := s.AddSubtask(featureID, title, position)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Subtask #%d created: %s", st.ID, st.Title)), nil
	}
}

func addTaskItemHandler(s *store.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		subtaskID := parseInt64(args["subtask_id"].(string))
		title := args["title"].(string)

		items, _ := s.GetTaskItemsForSubtask(subtaskID)
		position := len(items) + 1

		item, err := s.AddTaskItem(subtaskID, title, position)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Task item #%d created: %s", item.ID, item.Title)), nil
	}
}

func addSubtasksHandler(s *store.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		featureID := args["feature_id"].(string)
		titlesRaw := args["titles"].(string)

		titles := strings.Split(titlesRaw, "|")
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

		return mcp.NewToolResultText(fmt.Sprintf("Created %d subtasks:\n%s", len(lines), strings.Join(lines, "\n"))), nil
	}
}

func addTaskItemsHandler(s *store.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		subtaskID := parseInt64(args["subtask_id"].(string))
		titlesRaw := args["titles"].(string)

		titles := strings.Split(titlesRaw, "|")
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

		return mcp.NewToolResultText(fmt.Sprintf("Created %d task items:\n%s", len(lines), strings.Join(lines, "\n"))), nil
	}
}

func getFullContextHandler(s *store.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id := req.GetArguments()["id"].(string)

		f, err := s.GetFeature(id)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		subtasks, _ := s.GetSubtasksForFeature(id, true)
		sessions, _ := s.GetSessionsForFeature(id)

		type fullDump struct {
			Feature   store.Feature    `json:"feature"`
			Subtasks  []store.Subtask  `json:"subtasks"`
			Sessions  []store.Session  `json:"sessions"`
			Decisions []store.Decision `json:"decisions"`
		}
		decisions, _ := s.GetDecisionsForFeature(id)
		if decisions == nil {
			decisions = []store.Decision{}
		}
		data, _ := json.MarshalIndent(fullDump{
			Feature:   *f,
			Subtasks:  subtasks,
			Sessions:  sessions,
			Decisions: decisions,
		}, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

func quickTrackHandler(s *store.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		title := args["title"].(string)

		input := store.QuickTrackInput{Title: title}
		if v, ok := args["commit_hash"].(string); ok && v != "" {
			input.CommitHash = v
		}
		if v, ok := args["status"].(string); ok && v != "" {
			input.Status = v
		}
		if v, ok := args["key_files"].(string); ok && v != "" {
			for _, f := range strings.Split(v, ",") {
				input.KeyFiles = append(input.KeyFiles, strings.TrimSpace(f))
			}
		}

		result, err := s.QuickTrack(input)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		action := "Updated"
		if result.Created {
			action = "Created"
		}
		return mcp.NewToolResultText(fmt.Sprintf("%s feature %q [%s]", action, result.Feature.ID, result.Feature.Status)), nil
	}
}

func addIssueHandler(s *store.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		featureID := args["feature_id"].(string)
		description := args["description"].(string)

		var taskItemID *int64
		if v, ok := args["task_item_id"].(string); ok && v != "" {
			id := parseInt64(v)
			taskItemID = &id
		}

		issue, err := s.AddIssue(featureID, description, taskItemID)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		msg := fmt.Sprintf("Issue #%d logged on %s: %s", issue.ID, featureID, description)
		if taskItemID != nil {
			msg += fmt.Sprintf(" (linked to task item #%d)", *taskItemID)
		}
		return mcp.NewToolResultText(msg), nil
	}
}

func resolveIssueHandler(s *store.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		id := parseInt64(args["id"].(string))
		commitHash, _ := args["commit_hash"].(string)

		if err := s.ResolveIssue(id, commitHash); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		msg := fmt.Sprintf("Issue #%d resolved.", id)
		if commitHash != "" {
			msg += fmt.Sprintf(" Fix: %s", commitHash)
		}
		return mcp.NewToolResultText(msg), nil
	}
}

func listIssuesHandler(s *store.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		featureID, _ := req.GetArguments()["feature_id"].(string)

		var issues []store.Issue
		var err error
		if featureID != "" {
			issues, err = s.GetIssuesForFeature(featureID)
		} else {
			issues, err = s.GetAllOpenIssues()
		}
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if len(issues) == 0 {
			return mcp.NewToolResultText("No open issues."), nil
		}

		var lines []string
		for _, iss := range issues {
			line := fmt.Sprintf("- #%d [%s] %s", iss.ID, iss.FeatureID, iss.Description)
			if iss.Status == "resolved" {
				line += " (resolved"
				if iss.ResolvedCommit != "" {
					line += ": " + iss.ResolvedCommit
				}
				line += ")"
			}
			lines = append(lines, line)
		}
		return mcp.NewToolResultText(strings.Join(lines, "\n")), nil
	}
}

func addDecisionHandler(s *store.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		featureID := args["feature_id"].(string)
		approach := args["approach"].(string)
		outcome := args["outcome"].(string)
		reason := args["reason"].(string)

		if outcome != "accepted" && outcome != "rejected" {
			return mcp.NewToolResultError("outcome must be 'accepted' or 'rejected'"), nil
		}

		d, err := s.AddDecision(featureID, approach, outcome, reason)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Decision #%d logged: %s %s — %s", d.ID, outcome, approach, reason)), nil
	}
}

func parseInt64(s string) int64 {
	var n int64
	fmt.Sscanf(s, "%d", &n)
	return n
}
