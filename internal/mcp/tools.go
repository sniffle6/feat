package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/sniffyanimal/feat/internal/store"
)

func registerTools(srv *server.MCPServer, s *store.Store) {
	srv.AddTool(mcp.NewTool("add_feature",
		mcp.WithDescription("Create a new feature to track. Returns the generated slug ID."),
		mcp.WithString("title", mcp.Required(), mcp.Description("Feature title (e.g., 'Bluetooth Panel')")),
		mcp.WithString("description", mcp.Description("What the feature is")),
	), addFeatureHandler(s))

	srv.AddTool(mcp.NewTool("update_feature",
		mcp.WithDescription("Update a feature's status, description, left_off note, worktree_path, or key_files."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Feature slug ID")),
		mcp.WithString("status", mcp.Description("New status: planned, in_progress, done, blocked")),
		mcp.WithString("title", mcp.Description("New title")),
		mcp.WithString("description", mcp.Description("New description")),
		mcp.WithString("left_off", mcp.Description("Where work stopped — free text")),
		mcp.WithString("worktree_path", mcp.Description("Absolute path to git worktree")),
	), updateFeatureHandler(s))

	srv.AddTool(mcp.NewTool("list_features",
		mcp.WithDescription("List all features. Returns compact summaries: ID, title, status, left_off snippet. Filter by status optionally."),
		mcp.WithString("status", mcp.Description("Filter by status: planned, in_progress, done, blocked. Omit for all.")),
	), listFeaturesHandler(s))

	srv.AddTool(mcp.NewTool("get_feature",
		mcp.WithDescription("Get full detail for one feature including all linked sessions."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Feature slug ID")),
	), getFeatureHandler(s))

	srv.AddTool(mcp.NewTool("log_session",
		mcp.WithDescription("Record a session summary. Call this at end of session to log what was accomplished."),
		mcp.WithString("feature_id", mcp.Description("Feature slug ID this session was about. Omit if unlinked.")),
		mcp.WithString("summary", mcp.Required(), mcp.Description("Brief summary of what was accomplished")),
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
}

func addFeatureHandler(s *store.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		title := req.GetArguments()["title"].(string)
		desc, _ := req.GetArguments()["description"].(string)

		f, err := s.AddFeature(title, desc)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

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
		if v, ok := args["worktree_path"].(string); ok {
			u.WorktreePath = &v
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

		sessions, _ := s.GetSessionsForFeature(id)
		type fullFeature struct {
			store.Feature
			Sessions []store.Session `json:"sessions"`
		}
		full := fullFeature{Feature: *f, Sessions: sessions}
		data, _ := json.MarshalIndent(full, "", "  ")
		return mcp.NewToolResultText(string(data)), nil
	}
}

func logSessionHandler(s *store.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		summary := args["summary"].(string)
		featureID, _ := args["feature_id"].(string)

		sess, err := s.LogSession(store.SessionInput{
			FeatureID:  featureID,
			Summary:    summary,
			AutoLinked: featureID != "",
			LinkReason: "provided by Claude at session end",
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
		if len(fc.RecentSessions) > 0 {
			b.WriteString("Recent sessions:\n")
			for _, sess := range fc.RecentSessions {
				fmt.Fprintf(&b, "  - %s: %s\n", sess.CreatedAt.Format("2006-01-02"), sess.Summary)
			}
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
