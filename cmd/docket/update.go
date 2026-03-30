package main

import (
	"fmt"
	"os"
	"strings"
)

const docketSectionHead = `## Feature Tracking (docket)

This project uses ` + "`docket`" + ` for feature tracking. Dashboard: http://localhost:<port> (or run ` + "`/docket`" + `).

**Small tasks** (cosmetic changes, one-off fixes, config tweaks): call ` + "`quick_track`" + ` directly — one call, no agent dispatch needed.

**Larger features** (multi-step, plan-driven, complex):

Start of work (after any brainstorming/planning) — call ` + "`get_ready`" + ` to find existing features, then dispatch ` + "`board-manager`" + ` agent (model: sonnet) to create or find a card. Use ` + "`type`" + ` param (feature/bugfix/chore/spike) to auto-generate subtask templates.

Use ` + "`tags`" + ` param (comma-separated) on ` + "`add_feature`" + `/` + "`update_feature`" + ` to categorize work. New tags warn about existing tags to prevent typos.

Done features are auto-archived after 7 days. Use ` + "`list_features(status=\"archived\")`" + ` to see them. ` + "`update_feature(status=\"planned\")`" + ` to unarchive.
`

const docketSectionSuperpowers = `
**Plan execution (superpowers):** When using executing-plans or subagent-driven-development, set up docket BEFORE dispatching the first task — call ` + "`get_ready`" + `, create/find a feature card, and use ` + "`add_task_item`" + ` for each plan task. A PreToolUse hook will remind you if you forget.
`

const docketSectionTail = `
After a commit — use **direct MCP calls**, not agent dispatch:
- ` + "`update_feature`" + ` — set left_off, key_files, status, tags. Completion gate blocks ` + "`done`" + ` with unchecked items — pass ` + "`force=true`" + ` + ` + "`force_reason`" + ` to override.
- ` + "`complete_task_item`" + ` — check off items with outcome and commit_hash (pass ` + "`items`" + ` JSON array for batch)
- ` + "`add_decision`" + ` — record notable decisions (accepted/rejected with reason)
- ` + "`add_issue`" + ` / ` + "`resolve_issue`" + ` — track bugs found during work

Plan files committed during work are auto-imported by hooks. Only dispatch board-manager when the update needs judgment (restructuring imported plans, creating new subtasks).

After subagent work — subagent commits bypass hooks. Use direct MCP calls to batch-update the feature.

Use ` + "`get_context`" + ` (not ` + "`get_feature`" + `) for routine status checks — it's token-efficient (~15 lines).

Session logging and handoff files are handled automatically by the Stop hook.

Carry the feature ID across the session.

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
