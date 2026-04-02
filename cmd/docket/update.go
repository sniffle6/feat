package main

import (
	"fmt"
	"os"
	"strings"
)

const docketSectionHead = `## Feature Tracking (docket)

This project uses ` + "`docket`" + ` for feature tracking. Dashboard: http://localhost:<port> (or run ` + "`/docket`" + `). Active feature context is auto-injected at session start.

**Small tasks**: call ` + "`quick_track`" + ` — one call, no agent dispatch needed.

**Larger features**: call ` + "`get_ready`" + `, then dispatch ` + "`board-manager`" + ` agent (model: sonnet) to create or find a card. Use ` + "`type`" + ` (feature/bugfix/chore/spike) for auto-generated subtask templates.
`

const docketSectionSuperpowers = `
**Plan execution (superpowers):** When using executing-plans or subagent-driven-development, set up docket first — ` + "`get_ready`" + `, create/find a card, ` + "`add_task_item`" + ` per plan task.
`

const docketSectionTail = `
After a commit — use **direct MCP calls**, not agent dispatch:
- ` + "`update_feature`" + ` — left_off, key_files, status, tags. Completion gate blocks ` + "`done`" + ` with unchecked items — ` + "`force=true`" + ` + ` + "`force_reason`" + ` to override.
- ` + "`complete_task_item`" + ` — check off items with outcome and commit_hash (` + "`items`" + ` JSON array for batch)
- ` + "`add_decision`" + ` — accepted/rejected with reason
- ` + "`add_issue`" + ` / ` + "`resolve_issue`" + ` — track bugs found during work

Use ` + "`get_context`" + ` (not ` + "`get_feature`" + `) for routine status checks (~15 lines, token-efficient).

Commit tracking, session context, and handoff files are automatic (hooks). Use ` + "`/checkpoint`" + ` for manual checkpoints, ` + "`/end-session`" + ` to close the work session without closing Claude.

**If user rejects a docket update**, fix the issue and retry — don't drop tracking.
`

const sectionHeading = "## Feature Tracking (docket)"

func buildDocketSection(hasSuperpowers bool) string {
	if hasSuperpowers {
		return docketSectionHead + docketSectionSuperpowers + docketSectionTail
	}
	return docketSectionHead + docketSectionTail
}

func detectSuperpowers() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	data, err := os.ReadFile(home + "/.claude/plugins/installed_plugins.json")
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "superpowers")
}

func runUpdate() {
	data, err := os.ReadFile("CLAUDE.md")
	if err != nil {
		fmt.Fprintln(os.Stderr, "No CLAUDE.md found in current directory.")
		os.Exit(1)
	}

	content := string(data)
	section := buildDocketSection(detectSuperpowers())
	updated := updateDocketSection(content, section)

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

func updateDocketSection(content string, section string) string {
	// If section exists, replace it in place
	idx := strings.Index(content, sectionHeading)
	if idx >= 0 {
		// Find the end of this section (next ## heading or EOF)
		rest := content[idx+len(sectionHeading):]
		endIdx := strings.Index(rest, "\n## ")
		if endIdx >= 0 {
			// Replace section, keep everything after
			return content[:idx] + section + "\n" + rest[endIdx+1:]
		}
		// Section goes to EOF
		return content[:idx] + section
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
			result = append(result, strings.Split(strings.TrimRight(section, "\n"), "\n")...)
			result = append(result, "")
			inserted = true
		}
		result = append(result, line)
		_ = i
	}

	if !inserted {
		// No second heading found — append at end
		result = append(result, "")
		result = append(result, strings.Split(strings.TrimRight(section, "\n"), "\n")...)
		result = append(result, "")
	}

	return strings.Join(result, "\n")
}
