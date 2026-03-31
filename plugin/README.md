# docket plugin

Claude Code plugin for the docket feature tracker.

## What it provides

- **board-manager agent** — autonomous agent that creates features, updates status, completes tasks, and enriches handoff files
- **/docket skill** — opens the docket dashboard in the default browser
- **/docket-init skill** — initializes docket in a new project
- **/docket-update skill** — syncs the CLAUDE.md snippet to the latest version
- **/checkpoint skill** — force a mid-session checkpoint (transcript delta summarization)
- **/end-session skill** — close work session without closing Claude (useful when switching features)
- **MCP server** — connects Claude Code to the docket binary for feature tracking tools
- **Hooks** — SessionStart (context injection), PreToolUse (subagent nudge), PostToolUse (commit tracking), Stop (checkpoint on meaningful delta), PreCompact (always checkpoint), SessionEnd (handoff generation)

## Setup

**Marketplace install** (recommended):
```
/plugin marketplace add sniffle6/claude-docket
/plugin install docket@claude-docket
```
The binary downloads automatically on first session start.

**Source install**: Run `install.sh` from the docket repo root. Requires Go 1.21+.

All paths (hooks, MCP server, skills) use `${CLAUDE_PLUGIN_ROOT}` — no hardcoded paths. The binary lives at `${CLAUDE_PLUGIN_ROOT}/docket.exe`.

## Per-project setup

Run `/docket-init` in any project to set up tracking automatically. To update the snippet in an existing project, run `/docket-update`.

The snippet added to CLAUDE.md instructs Claude to use board-manager for feature creation and direct MCP calls (`update_feature`, `complete_task_item`, `add_decision`) for post-commit updates. Board-manager is only dispatched after commits when judgment is needed (plan imports, new subtasks, restructuring). Session tracking and handoff files are handled automatically by the lifecycle hooks.
