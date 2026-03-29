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

Run `/docket-init` in any project to set up tracking automatically, or add this to your project's CLAUDE.md manually:

    ## Feature Tracking (docket)

    This project uses `docket` for feature tracking. Dashboard: http://localhost:<port> (or run `/docket`).

    Dispatch the `board-manager` agent (model: sonnet) at these points:
    1. **Before writing any code for a new task** — if the user asks to build, fix, or add something, dispatch board-manager FIRST to create or find a feature card. Do not write code until the card exists. Skip only for questions, reviews, and lookups.
    2. **After a commit** — pass commit hash, message, files, feature ID
    3. **After subagent implementation work** — subagent commits bypass PostToolUse hooks. After an implementer subagent returns with commits, dispatch board-manager with all new commit hashes, messages, and files. Don't wait for per-commit dispatches — batch them.

    Session logging is handled automatically by the Stop hook (no agent dispatch needed).

    Carry the feature ID the agent returns across dispatches. `get_ready` stays in main session.

    **If user rejects a board-manager dispatch**, fix the issue (e.g., missing context) and retry — don't silently drop the dispatch for the rest of the session.
