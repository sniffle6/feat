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

func addFeatureHandler(s *store.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		title, ok := argString(args, "title")
		if !ok || title == "" {
			return mcp.NewToolResultError("missing required parameter: title"), nil
		}
		desc, _ := argString(args, "description")
		status, _ := argString(args, "status")
		notes, _ := argString(args, "notes")
		typ, _ := argString(args, "type")

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
		if typ != "" {
			if err := s.ApplyTemplate(f.ID, typ); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("feature created but template failed: %v", err)), nil
			}
			s.UpdateFeature(f.ID, store.FeatureUpdate{Type: &typ})
		}
		var tagWarning string
		if tagStr, ok := argString(args, "tags"); ok && tagStr != "" {
			tags := strings.Split(tagStr, ",")
			for i := range tags {
				tags[i] = strings.TrimSpace(tags[i])
			}
			// Check for new tags BEFORE saving so they're still "unknown"
			newTags := s.CheckNewTags(tags)
			if len(newTags) > 0 {
				known, _ := s.GetKnownTags()
				tagWarning = fmt.Sprintf("\nNote: new tag(s) %q added. Existing tags: %s", strings.Join(newTags, ", "), strings.Join(known, ", "))
			}
			s.UpdateFeature(f.ID, store.FeatureUpdate{Tags: &tags})
		}
		f, _ = s.GetFeature(f.ID)

		data, _ := json.MarshalIndent(f, "", "  ")
		result := string(data)
		if tagWarning != "" {
			result += tagWarning
		}
		return mcp.NewToolResultText(result), nil
	}
}

func updateFeatureHandler(s *store.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		id, ok := argString(args, "id")
		if !ok || id == "" {
			return mcp.NewToolResultError("missing required parameter: id"), nil
		}

		u := store.FeatureUpdate{}
		if v, ok := argString(args, "status"); ok {
			u.Status = &v
		}
		if v, ok := argString(args, "title"); ok {
			u.Title = &v
		}
		if v, ok := argString(args, "description"); ok {
			u.Description = &v
		}
		if v, ok := argString(args, "left_off"); ok {
			u.LeftOff = &v
		}
		if v, ok := argString(args, "notes"); ok {
			u.Notes = &v
		}
		if v, ok := argString(args, "worktree_path"); ok {
			u.WorktreePath = &v
		}
		if v, ok := argString(args, "key_files"); ok && v != "" {
			files := strings.Split(v, ",")
			for i := range files {
				files[i] = strings.TrimSpace(files[i])
			}
			u.KeyFiles = &files
		}
		if v, ok := argString(args, "tags"); ok {
			if v == "" {
				empty := []string{}
				u.Tags = &empty
			} else {
				tags := strings.Split(v, ",")
				for i := range tags {
					tags[i] = strings.TrimSpace(tags[i])
				}
				u.Tags = &tags
			}
		}
		if v, ok := args["force"]; ok {
			if b, ok := v.(bool); ok && b {
				force := true
				u.Force = &force
			}
		}
		if v, ok := argString(args, "force_reason"); ok && v != "" {
			u.ForceReason = &v
		}

		// Check for new tags BEFORE saving so they're still "unknown"
		var tagWarning string
		if u.Tags != nil && len(*u.Tags) > 0 {
			newTags := s.CheckNewTags(*u.Tags)
			if len(newTags) > 0 {
				known, _ := s.GetKnownTags()
				tagWarning = fmt.Sprintf("\nNote: new tag(s) %q added. Existing tags: %s", strings.Join(newTags, ", "), strings.Join(known, ", "))
			}
		}

		if err := s.UpdateFeature(id, u); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		msg := fmt.Sprintf("Updated feature %q", id)
		if tagWarning != "" {
			msg += tagWarning
		}
		return mcp.NewToolResultText(msg), nil
	}
}

func listFeaturesHandler(s *store.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()
		status, _ := argString(args, "status")
		tag, _ := argString(args, "tag")

		var features []store.Feature
		var err error
		if tag != "" {
			features, err = s.ListFeaturesWithTag(status, tag)
		} else {
			features, err = s.ListFeatures(status)
		}
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if len(features) == 0 {
			return mcp.NewToolResultText("No features found."), nil
		}

		var lines []string
		for _, f := range features {
			line := fmt.Sprintf("- **%s** [%s] %s", f.ID, f.Status, f.Title)
			if len(f.Tags) > 0 {
				line += fmt.Sprintf(" {%s}", strings.Join(f.Tags, ", "))
			}
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
		id, ok := argString(req.GetArguments(), "id")
		if !ok || id == "" {
			return mcp.NewToolResultError("missing required parameter: id"), nil
		}

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

func getContextHandler(s *store.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, ok := argString(req.GetArguments(), "id")
		if !ok || id == "" {
			return mcp.NewToolResultError("missing required parameter: id"), nil
		}

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

func getFullContextHandler(s *store.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, ok := argString(req.GetArguments(), "id")
		if !ok || id == "" {
			return mcp.NewToolResultError("missing required parameter: id"), nil
		}

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
		title, ok := argString(args, "title")
		if !ok || title == "" {
			return mcp.NewToolResultError("missing required parameter: title"), nil
		}

		input := store.QuickTrackInput{Title: title}
		if v, ok := argString(args, "commit_hash"); ok && v != "" {
			input.CommitHash = v
		}
		if v, ok := argString(args, "status"); ok && v != "" {
			input.Status = v
		}
		if v, ok := argString(args, "key_files"); ok && v != "" {
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
