package dashboard

import (
	"fmt"
	"strings"

	"github.com/sniffle6/claude-docket/internal/store"
)

// RenderLaunchPrompt generates a markdown prompt file for launching a new
// Claude session with full feature context.
func RenderLaunchPrompt(data *store.LaunchData) string {
	var b strings.Builder
	f := data.Feature

	fmt.Fprintf(&b, "# Resuming: %s\n\n", f.Title)

	if f.Description != "" {
		fmt.Fprintf(&b, "%s\n\n", f.Description)
	}

	fmt.Fprintf(&b, "**Status:** %s\n", f.Status)
	if f.LeftOff != "" {
		fmt.Fprintf(&b, "**Left off:** %s\n", f.LeftOff)
	}
	b.WriteString("\n")

	if len(data.TaskItems) > 0 {
		b.WriteString("## Remaining Tasks\n")
		for _, item := range data.TaskItems {
			fmt.Fprintf(&b, "- [ ] %s\n", item.Title)
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

	if len(data.Issues) > 0 {
		b.WriteString("## Open Issues\n")
		for _, issue := range data.Issues {
			fmt.Fprintf(&b, "- #%d: %s\n", issue.ID, issue.Description)
		}
		b.WriteString("\n")
	}

	if f.Notes != "" {
		b.WriteString("## Notes\n")
		fmt.Fprintf(&b, "%s\n", f.Notes)
	}

	return b.String()
}
