package dashboard

import (
	"fmt"
	"os"
	"path/filepath"
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

// GetLaunchCmd reads the launch command template. Checks DOCKET_LAUNCH_CMD
// env var first, then .docket/launch.toml, returns empty string if neither set.
func GetLaunchCmd(projectDir string) string {
	if cmd := os.Getenv("DOCKET_LAUNCH_CMD"); cmd != "" {
		return cmd
	}
	tomlPath := filepath.Join(projectDir, ".docket", "launch.toml")
	data, err := os.ReadFile(tomlPath)
	if err != nil {
		return ""
	}
	// Simple format: first non-empty, non-comment line is the command template
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			return line
		}
	}
	return ""
}

// SubstituteLaunchCmd replaces template variables in the launch command.
func SubstituteLaunchCmd(template, handoffFile, featureTitle, featureID, projectDir string) string {
	r := strings.NewReplacer(
		"{{handoff_file}}", handoffFile,
		"{{feature_title}}", featureTitle,
		"{{feature_id}}", featureID,
		"{{project_dir}}", projectDir,
	)
	return r.Replace(template)
}

// renderLaunchExtras generates additional context sections (unchecked tasks,
// open issues, notes) to append when using an existing handoff file as base.
func renderLaunchExtras(data *store.LaunchData) string {
	var b strings.Builder

	if len(data.TaskItems) > 0 {
		b.WriteString("## Remaining Tasks (current)\n")
		for _, item := range data.TaskItems {
			fmt.Fprintf(&b, "- [ ] %s\n", item.Title)
		}
		b.WriteString("\n")
	}

	if len(data.Issues) > 0 {
		b.WriteString("## Open Issues (current)\n")
		for _, issue := range data.Issues {
			fmt.Fprintf(&b, "- #%d: %s\n", issue.ID, issue.Description)
		}
		b.WriteString("\n")
	}

	if data.Feature.Notes != "" {
		b.WriteString("## Notes\n")
		fmt.Fprintf(&b, "%s\n", data.Feature.Notes)
	}

	return b.String()
}
