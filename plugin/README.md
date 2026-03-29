# docket plugin

Claude Code plugin for the docket feature tracker.

## What it provides

- **board-manager agent** — autonomous agent that creates features, updates status, completes tasks, and enriches handoff files
- **/docket skill** — opens the docket dashboard in the default browser
- **/docket-init skill** — initializes docket in a new project
- **/docket-update skill** — syncs the CLAUDE.md snippet to the latest version
- **MCP server** — connects Claude Code to the docket binary for feature tracking tools
- **Hooks** — SessionStart (context injection + handoff files), PostToolUse (commit tracking + auto plan import), Stop (session logging + handoff generation)

## Setup

Run `install.sh` from the docket repo root. It builds the binary and installs this plugin.

## Per-project setup

Run `/docket-init` in any project to set up tracking automatically. To update the snippet in an existing project, run `/docket-update`.

The snippet added to CLAUDE.md instructs Claude to use board-manager for feature creation and direct MCP calls (`update_feature`, `complete_task_item`, `add_decision`) for post-commit updates. Board-manager is only dispatched after commits when judgment is needed (plan imports, new subtasks, restructuring). Session logging and handoff files are handled automatically by the Stop hook.
