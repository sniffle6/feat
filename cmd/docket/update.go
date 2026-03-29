package main

import (
	"fmt"
	"os"
	"strings"
)

const docketSection = `## Feature Tracking (docket)

This project uses ` + "`docket`" + ` for feature tracking. Dashboard: http://localhost:<port> (or run ` + "`/docket`" + `).

**Small tasks** (cosmetic changes, one-off fixes, config tweaks): call ` + "`quick_track`" + ` MCP tool directly — one call, no agent dispatch needed. Pass title, commit_hash, and key_files.

**Larger features** (multi-step, plan-driven, complex):

Start of work — dispatch ` + "`board-manager`" + ` agent (model: sonnet) to create or find a feature card. Do not write code until the card exists. Skip only for questions, reviews, and lookups.

After a commit — use **direct MCP calls** by default, not agent dispatch:
- ` + "`update_feature`" + ` — set left_off, key_files, status on the existing feature
- ` + "`complete_task_item`" + ` — check off task items with outcome and commit_hash (pass ` + "`items`" + ` JSON array for batch)
- ` + "`add_decision`" + ` — record a notable decision

Only dispatch board-manager after a commit when the update requires judgment:
- The commit adds a plan file that needs importing and structuring
- The feature needs new subtasks or task items created
- Significant status change or handoff enrichment is needed

After subagent implementation work — subagent commits bypass PostToolUse hooks. Use direct MCP calls to batch-update the feature with all new commit hashes and complete relevant task items. Dispatch board-manager only if restructuring is needed.

Session logging and handoff files are handled automatically by the Stop hook (no agent dispatch needed).

Carry the feature ID across the session. ` + "`get_ready`" + ` stays in main session.

**If user rejects a docket update**, fix the issue (e.g., missing context) and retry — don't silently drop tracking for the rest of the session.
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
