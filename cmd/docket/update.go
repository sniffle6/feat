package main

import (
	"fmt"
	"os"
	"strings"
)

const docketSection = `## Feature Tracking (docket)

This project uses ` + "`docket`" + ` for feature tracking. Dashboard: http://localhost:<port> (or run ` + "`/docket`" + `).

Dispatch the ` + "`board-manager`" + ` agent (model: sonnet) at these points:
1. **Before writing any code for a new task** — if the user asks to build, fix, or add something, dispatch board-manager FIRST to create or find a feature card. Do not write code until the card exists. Skip only for questions, reviews, and lookups.
2. **After a commit** — pass commit hash, message, files, feature ID

Session logging is handled automatically by the Stop hook (no agent dispatch needed).

Carry the feature ID the agent returns across dispatches. ` + "`get_ready`" + ` stays in main session.
`

const sectionHeading = "## Feature Tracking (docket)"

func runUpdate() {
	data, err := os.ReadFile("CLAUDE.md")
	if err != nil {
		fmt.Fprintln(os.Stderr, "No CLAUDE.md found in current directory.")
		os.Exit(1)
	}

	content := string(data)
	updated := updateDocketSection(content)

	if updated == content {
		fmt.Println("CLAUDE.md docket section is already up to date.")
		return
	}

	if err := os.WriteFile("CLAUDE.md", []byte(updated), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write CLAUDE.md: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Updated CLAUDE.md with latest docket section.")
}

func updateDocketSection(content string) string {
	// If section exists, replace it in place
	idx := strings.Index(content, sectionHeading)
	if idx >= 0 {
		// Find the end of this section (next ## heading or EOF)
		rest := content[idx+len(sectionHeading):]
		endIdx := strings.Index(rest, "\n## ")
		if endIdx >= 0 {
			// Replace section, keep everything after
			return content[:idx] + docketSection + "\n" + rest[endIdx+1:]
		}
		// Section goes to EOF
		return content[:idx] + docketSection
	}

	// Not found — insert after the first section
	lines := strings.Split(content, "\n")
	var result []string
	inserted := false
	passedFirstHeading := false

	for i, line := range lines {
		if !inserted && strings.HasPrefix(line, "## ") {
			if !passedFirstHeading {
				passedFirstHeading = true
				result = append(result, line)
				continue
			}
			// This is the second ## heading — insert before it
			result = append(result, "")
			result = append(result, strings.Split(strings.TrimRight(docketSection, "\n"), "\n")...)
			result = append(result, "")
			inserted = true
		}
		result = append(result, line)
		_ = i
	}

	if !inserted {
		// No second heading found — append at end
		result = append(result, "")
		result = append(result, strings.Split(strings.TrimRight(docketSection, "\n"), "\n")...)
		result = append(result, "")
	}

	return strings.Join(result, "\n")
}
