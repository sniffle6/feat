<p align="center">
  <img src="assets/docket-logo.png" alt="docket logo" width="180">
</p>

# docket

Local feature tracker for [Claude Code](https://docs.anthropic.com/en/docs/claude-code) sessions. Tracks what you're building, what happened, and where you left off — across sessions, worktrees, and subagents.

MCP server + SQLite + web dashboard + Claude Code plugin.

## Why

Claude Code sessions are stateless. When a session ends, the next one starts fresh with no memory of what was built, what was tried, or what failed. Docket solves this by:

- **Tracking features** with status, progress, and session history
- **Logging decisions** so Claude doesn't re-explore rejected approaches
- **Generating handoff files** so fresh sessions cold-start with full context
- **Auto-tracking commits** via hooks — no manual bookkeeping

## Prerequisites

- [Go 1.21+](https://go.dev/dl/)
- [Claude Code](https://docs.anthropic.com/en/docs/claude-code)
- Python 3 (used by the install script for JSON manipulation)

## Install

```bash
git clone https://github.com/sniffle6/claude-docket.git
cd claude-docket
bash install.sh
```

This builds the binary to `~/.local/share/docket/docket.exe` and installs the Claude Code plugin to `~/.claude/plugins/marketplaces/local/docket/`.

See [INSTALL.md](INSTALL.md) for full details, updating, and troubleshooting.

## Quick Start

After installing, restart Claude Code (or run `/reload-plugins`), then in any project:

```
/docket-init
```

This creates the `.docket/` directory, adds tracking instructions to your project's `CLAUDE.md`, and you're set. The MCP server and dashboard start automatically when Claude Code loads the plugin.

Open the dashboard anytime with `/docket`.

## How It Works

docket runs two things in parallel:
1. **MCP server** on stdio — Claude Code talks to this via the plugin
2. **HTTP dashboard** on a per-project port — you open this in your browser

Both read/write the same SQLite database at `<project>/.docket/features.db`.

### Automatic Session Tracking

The plugin installs three lifecycle hooks:

| Hook | What it does |
|------|-------------|
| **SessionStart** | Injects active feature context and handoff files into the conversation |
| **PostToolUse** (Bash) | Detects `git commit` commands, records commit hashes to `.docket/commits.log`, auto-imports plan files |
| **Stop** | Logs the session to SQLite from commit messages, generates handoff files, cleans up |

No manual tracking needed. See [docs/docket-hooks.md](docs/docket-hooks.md) for details.

### Board Manager Agent

The plugin includes a `board-manager` agent (runs on Sonnet) that autonomously:
- Creates and finds feature cards before work begins
- Matches commits to task items and marks them complete
- Imports plan files as structured subtasks
- Enriches handoff files with synthesized context

### Handoff Files

At session end, docket generates structured markdown files at `.docket/handoff/<feature-id>.md` with status, progress, next tasks, key files, and recent activity. The next session gets this injected automatically — no re-discovery needed. See [docs/handoff-files.md](docs/handoff-files.md).

### Decision Log

Each feature can have decisions logged against it — what was considered, whether it was accepted or rejected, and why. Rejected decisions are surfaced in context briefings so Claude doesn't re-explore dead ends. See [docs/decision-log.md](docs/decision-log.md).

## MCP Tools

| Tool | Purpose |
|------|---------|
| `add_feature` | Create a feature. Returns slug ID. |
| `update_feature` | Update status, description, left_off, worktree_path, key_files. |
| `list_features` | Compact feature list, filterable by status. |
| `get_feature` | Full feature detail with sessions, subtasks, decisions. |
| `get_context` | Token-efficient briefing (~15-20 lines). |
| `get_ready` | Actionable features (in_progress first, then planned). |
| `get_full_context` | Deep context dump for subagent research. |
| `log_session` | Record what happened in a session. |
| `compact_sessions` | Compress old sessions into a summary (keeps last 3). |
| `import_plan` | Import markdown plan file as subtasks/task items. |
| `add_subtask` | Create a phase/milestone. |
| `add_subtasks` | Batch create phases (pipe-separated titles). |
| `add_task_item` | Add a task to a subtask. |
| `add_task_items` | Batch add tasks (pipe-separated titles). |
| `complete_task_item` | Mark a task done with outcome and commit hash. |
| `complete_task_items` | Batch complete tasks (JSON array). |
| `add_decision` | Log an approach decision (accepted/rejected with reason). |

## Dashboard

Run `/docket` to open the dashboard, or check `.docket/port` for the port number.

- **Kanban board**: Planned / In Progress / Blocked / Dev Complete / Done
- Click a card for full detail — session history, subtasks, decisions, key files
- Edit "left off" notes inline
- Reassign unlinked sessions to features
- Dark/light theme toggle

## Plugin Contents

The installed plugin provides:

| Component | Description |
|-----------|-------------|
| `board-manager` agent | Autonomous board management (creates features, tracks commits, enriches handoffs) |
| `/docket` skill | Opens the dashboard in your browser |
| `/docket-init` skill | Initializes docket in a new project |
| `/docket-update` skill | Syncs the CLAUDE.md snippet to the latest version |
| MCP server | Connects Claude Code to the docket binary |
| Hooks | SessionStart, PostToolUse, Stop — automatic session tracking |

## Documentation

- [INSTALL.md](INSTALL.md) — Installation, updating, troubleshooting
- [docs/docket-hooks.md](docs/docket-hooks.md) — How automatic session tracking works
- [docs/handoff-files.md](docs/handoff-files.md) — Cross-session context handoff
- [docs/decision-log.md](docs/decision-log.md) — Decision tracking per feature
- [docs/dark-mode-toggle.md](docs/dark-mode-toggle.md) — Dashboard theming and dev mode
- [plugin/README.md](plugin/README.md) — Plugin setup and per-project CLAUDE.md snippet

## License

MIT
