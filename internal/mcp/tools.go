package mcp

import (
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/sniffle6/claude-docket/internal/store"
)

func registerTools(srv *server.MCPServer, s *store.Store, projectDir string, onCheckpoint func(), binding *Binding, launchFeature string) {
	srv.AddTool(mcp.NewTool("add_feature",
		mcp.WithDescription("Create a new feature to track. Returns the generated slug ID."),
		mcp.WithString("title", mcp.Required(), mcp.Description("Feature title (e.g., 'Bluetooth Panel')")),
		mcp.WithString("description", mcp.Description("What the feature is")),
		mcp.WithString("status", mcp.Description("Initial status: planned (default), in_progress, blocked, dev_complete")),
		mcp.WithString("notes", mcp.Description("User notes — thoughts, ideas, context for Claude to read when picking up this feature")),
		mcp.WithString("type", mcp.Description("Feature type: feature, bugfix, chore, spike. Auto-creates subtasks from template when set.")),
		mcp.WithString("tags", mcp.Description("Comma-separated tags (e.g., 'auth,frontend'). New tags trigger a warning listing existing tags.")),
		mcp.WithString("spec_path", mcp.Description("Relative path to the design spec file (e.g., 'docs/superpowers/specs/2026-04-01-foo-design.md')")),
	), addFeatureHandler(s))

	srv.AddTool(mcp.NewTool("update_feature",
		mcp.WithDescription("Update a feature's status, description, left_off note, notes, worktree_path, or key_files."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Feature slug ID")),
		mcp.WithString("status", mcp.Description("New status: planned, in_progress, done, blocked, dev_complete, archived")),
		mcp.WithString("title", mcp.Description("New title")),
		mcp.WithString("description", mcp.Description("New description")),
		mcp.WithString("left_off", mcp.Description("Where work stopped — free text")),
		mcp.WithString("notes", mcp.Description("User notes — thoughts, ideas, context for Claude")),
		mcp.WithString("worktree_path", mcp.Description("Absolute path to git worktree")),
		mcp.WithString("key_files", mcp.Description("Comma-separated list of key file paths for this feature")),
		mcp.WithString("tags", mcp.Description("Comma-separated tags — replaces all existing tags. New tags trigger a warning.")),
		mcp.WithString("spec_path", mcp.Description("Relative path to the design spec file (e.g., 'docs/superpowers/specs/2026-04-01-foo-design.md')")),
		mcp.WithBoolean("force", mcp.Description("Force status=done even with unchecked task items or open issues. Logs a decision.")),
		mcp.WithString("force_reason", mcp.Description("Reason for force-completing (logged as a decision)")),
	), updateFeatureHandler(s))

	srv.AddTool(mcp.NewTool("delete_feature",
		mcp.WithDescription("Permanently delete a feature and all its data (subtasks, decisions, notes, issues, sessions). Irreversible."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Feature slug ID")),
		mcp.WithBoolean("confirm", mcp.Required(), mcp.Description("Must be true to confirm deletion")),
	), deleteFeatureHandler(s))

	srv.AddTool(mcp.NewTool("list_features",
		mcp.WithDescription("List features. Returns compact summaries: ID, title, status, tags, left_off snippet. Excludes archived by default."),
		mcp.WithString("status", mcp.Description("Filter by status: planned, in_progress, done, blocked, dev_complete, archived. Omit for all (excluding archived).")),
		mcp.WithString("tag", mcp.Description("Filter to features with this tag. Combines with status filter.")),
	), listFeaturesHandler(s))

	srv.AddTool(mcp.NewTool("get_feature",
		mcp.WithDescription("Get full detail for one feature including all linked sessions."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Feature slug ID")),
	), getFeatureHandler(s))

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

	srv.AddTool(mcp.NewTool("add_subtask",
		mcp.WithDescription("Add subtask(s) to a feature. Use pipe-separated titles for multiple (e.g., 'Phase 1|Phase 2|Phase 3')."),
		mcp.WithString("feature_id", mcp.Required(), mcp.Description("Feature slug ID")),
		mcp.WithString("title", mcp.Required(), mcp.Description("Subtask title, or pipe-separated titles for batch")),
	), addSubtaskHandler(s))

	srv.AddTool(mcp.NewTool("add_task_item",
		mcp.WithDescription("Add task item(s) to a subtask. Use pipe-separated titles for multiple (e.g., 'Write tests|Implement handler')."),
		mcp.WithString("subtask_id", mcp.Required(), mcp.Description("Parent subtask ID (number)")),
		mcp.WithString("title", mcp.Required(), mcp.Description("Task item title, or pipe-separated titles for batch")),
	), addTaskItemHandler(s))

	srv.AddTool(mcp.NewTool("complete_task_item",
		mcp.WithDescription("Mark task item(s) as done. Pass id+outcome for one, or items JSON array for batch: [{\"id\":\"1\",\"outcome\":\"done\",\"commit_hash\":\"abc\",\"key_files\":\"a.go\"}]"),
		mcp.WithString("id", mcp.Description("Task item ID (number) — for single completion")),
		mcp.WithString("outcome", mcp.Description("One-liner of what was accomplished — for single completion")),
		mcp.WithString("commit_hash", mcp.Description("Git commit SHA")),
		mcp.WithString("key_files", mcp.Description("Comma-separated file paths modified")),
		mcp.WithString("items", mcp.Description("JSON array for batch: [{\"id\":\"1\",\"outcome\":\"done\"}]. Overrides id/outcome params.")),
	), completeTaskItemHandler(s))

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

	srv.AddTool(mcp.NewTool("add_note",
		mcp.WithDescription("Append a note to a feature card — findings, context, observations discovered during work. Use when told to 'add findings', 'note this', 'save context', or when you discover something worth recording for future sessions."),
		mcp.WithString("feature_id", mcp.Required(), mcp.Description("Feature slug ID")),
		mcp.WithString("content", mcp.Required(), mcp.Description("The note content — what was found, observed, or worth remembering")),
	), addNoteHandler(s))

	srv.AddTool(mcp.NewTool("add_decision",
		mcp.WithDescription("Log a decision on a feature. Records what approach was considered, whether it was accepted or rejected, and why. Prevents re-exploring dead ends across sessions."),
		mcp.WithString("feature_id", mcp.Required(), mcp.Description("Feature slug ID")),
		mcp.WithString("approach", mcp.Required(), mcp.Description("What was considered (e.g., 'Use websockets for real-time updates')")),
		mcp.WithString("outcome", mcp.Required(), mcp.Description("accepted or rejected")),
		mcp.WithString("reason", mcp.Required(), mcp.Description("Why — one-liner (e.g., 'Too complex for MVP, polling sufficient')")),
	), addDecisionHandler(s))

	srv.AddTool(mcp.NewTool("checkpoint",
		mcp.WithDescription("Force a checkpoint of the current session's semantic and mechanical state. Enqueues a background summarization job. Pass end_session=true to also close the work session and write the handoff file."),
		mcp.WithBoolean("end_session", mcp.Description("If true, close the work session and write handoff after checkpointing. Default: false.")),
	), checkpointHandler(s, projectDir, onCheckpoint, binding))

	srv.AddTool(mcp.NewTool("bind_session",
		mcp.WithDescription("Bind this MCP server to a specific work session. Call at session start with the session_id and feature_id from the system message. Required before checkpoint can work. Safe to call multiple times — idempotent if already bound."),
		mcp.WithString("feature_id", mcp.Required(), mcp.Description("Feature slug ID to bind to")),
		mcp.WithString("session_id", mcp.Required(), mcp.Description("Claude session ID from the session context system message")),
	), bindSessionHandler(s, binding, launchFeature))

	srv.AddTool(mcp.NewTool("search",
		mcp.WithDescription("Search across all feature content: descriptions, decisions, issues, notes, sessions, tasks, and checkpoint observations. Supports FTS5 syntax: plain words, \"phrase match\", prefix*, AND/OR operators."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Search text — supports FTS5 syntax")),
		mcp.WithString("scope", mcp.Description("Comma-separated entity types to search: feature, decision, issue, note, session, subtask, task_item, observation. Omit for all.")),
		mcp.WithString("feature_id", mcp.Description("Limit search to one feature")),
		mcp.WithBoolean("verbose", mcp.Description("Return full field values instead of snippets (default: false)")),
		mcp.WithString("limit", mcp.Description("Max results to return (default: 20)")),
	), searchHandler(s))
}

func parseInt64(s string) int64 {
	var n int64
	fmt.Sscanf(s, "%d", &n)
	return n
}

// argString safely extracts a string argument, handling nil and non-string types
// without panicking. Returns empty string and false if missing or wrong type.
func argString(args map[string]interface{}, key string) (string, bool) {
	v, ok := args[key]
	if !ok || v == nil {
		return "", false
	}
	s, ok := v.(string)
	if !ok {
		// Handle numeric values sent as float64 (JSON numbers)
		if f, ok := v.(float64); ok {
			return fmt.Sprintf("%d", int64(f)), true
		}
		return fmt.Sprintf("%v", v), true
	}
	return s, true
}
