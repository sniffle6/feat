# feat plugin

Claude Code plugin for the feat feature tracker.

## What it provides

- **board-manager agent** — autonomous agent that handles all feat board operations (create features, update status, log sessions, auto-compact)
- **/feat skill** — opens the feat dashboard (http://localhost:7890) in the default browser
- **MCP server** — connects Claude Code to the feat binary for feature tracking tools

## Setup

Run `install.sh` from the feat repo root. It builds the binary and installs this plugin.

## Per-project setup

Add this to your project's CLAUDE.md:

    ## Feature Tracking (feat)

    This project uses `feat` for feature tracking. Dashboard: http://localhost:7890 (or run `/feat`).

    Dispatch the `board-manager` agent (model: sonnet) at these points:
    1. **Start of implementation work** — skip for questions/reviews/lookups
    2. **After a commit** — pass commit hash, message, files, feature ID
    3. **Session ending** — pass summary, commits, files, feature ID

    Carry the feature ID the agent returns across dispatches. `get_ready` stays in main session.
