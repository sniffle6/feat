package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sniffle6/claude-docket/internal/store"
)

func renderHandoff(data *store.HandoffData) string {
	var b strings.Builder
	f := data.Feature

	fmt.Fprintf(&b, "# Handoff: %s\n\n", f.Title)

	fmt.Fprintf(&b, "## Status\n")
	fmt.Fprintf(&b, "%s | Progress: %d/%d | Updated: %s\n\n",
		f.Status, data.Done, data.Total, f.UpdatedAt.Format("2006-01-02 15:04"))

	if f.LeftOff != "" {
		fmt.Fprintf(&b, "## Left Off\n%s\n\n", f.LeftOff)
	}

	if len(data.NextTasks) > 0 {
		b.WriteString("## Next Tasks\n")
		for _, task := range data.NextTasks {
			fmt.Fprintf(&b, "- [ ] %s\n", task)
		}
		b.WriteString("\n")
	}

	if len(f.KeyFiles) > 0 {
		b.WriteString("## Key Files\n")
		for _, kf := range f.KeyFiles {
			fmt.Fprintf(&b, "- %s\n", kf)
		}
		b.WriteString("\n")
	}

	if len(data.RecentSessions) > 0 {
		b.WriteString("## Recent Activity\n")
		for _, sess := range data.RecentSessions {
			line := fmt.Sprintf("- %s: %s", sess.CreatedAt.Format("2006-01-02"), sess.Summary)
			if len(sess.Commits) > 0 {
				line += fmt.Sprintf(" [%s]", strings.Join(sess.Commits, ", "))
			}
			fmt.Fprintf(&b, "%s\n", line)
		}
		b.WriteString("\n")
	}

	if len(data.SubtaskSummary) > 0 {
		b.WriteString("## Active Subtasks\n")
		for _, st := range data.SubtaskSummary {
			fmt.Fprintf(&b, "- %s [%d/%d]\n", st.Title, st.Done, st.Total)
		}
		b.WriteString("\n")
	}

	return b.String()
}

func writeHandoffFile(dir string, data *store.HandoffData) error {
	handoffDir := filepath.Join(dir, ".docket", "handoff")
	if err := os.MkdirAll(handoffDir, 0755); err != nil {
		return err
	}
	path := filepath.Join(handoffDir, data.Feature.ID+".md")
	return os.WriteFile(path, []byte(renderHandoff(data)), 0644)
}

func cleanStaleHandoffs(dir string, activeIDs map[string]bool) {
	handoffDir := filepath.Join(dir, ".docket", "handoff")
	entries, err := os.ReadDir(handoffDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".md")
		if !activeIDs[name] {
			os.Remove(filepath.Join(handoffDir, e.Name()))
		}
	}
}
