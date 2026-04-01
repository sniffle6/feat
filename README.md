<p align="center">
  <img src="dashboard/docket-header.png" alt="docket logo" width="600">
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

## Install

### From marketplace (recommended)

```
/plugin marketplace add sniffle6/claude-docket
/plugin install docket@claude-docket
```

The binary downloads automatically on first session start. No build tools needed.

### From source

Requires [Go 1.21+](https://go.dev/dl/).

```bash
git clone https://github.com/sniffle6/claude-docket.git
cd claude-docket
bash install.sh
```

This builds the binary and copies it with the plugin files to `~/.claude/plugins/marketplaces/local/docket/`. All paths resolve via `${CLAUDE_PLUGIN_ROOT}` at runtime.

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

The plugin installs six lifecycle hooks:

| Hook | What it does |
|------|-------------|
| **SessionStart** | Opens a work session, injects active feature context and handoff files |
| **PreToolUse** (Agent) | Nudges Claude to set up docket tracking before dispatching subagents |
| **PostToolUse** (Bash) | Detects `git commit` commands, records commit hashes, auto-imports plan files |
| **Stop** | Parses transcript delta, enqueues a checkpoint if meaningful changes detected |
| **PreCompact** | Always checkpoints before context compression (no threshold) |
| **SessionEnd** | Enqueues remaining delta, writes handoff files, closes work session |

No manual tracking needed. See [docs/docket-hooks.md](docs/docket-hooks.md) for details.

### Board Manager Agent

The plugin includes a `board-manager` agent (runs on Sonnet) that autonomously:
- Creates and finds feature cards before work begins
- Matches commits to task items and marks them complete
- Imports plan files as structured subtasks
- Enriches handoff files with synthesized context

### Handoff Files

At session end, docket generates structured markdown files at `.docket/handoff/<feature-id>.md` with status, progress, next tasks, key files, and recent activity. A "Last Session" section includes observations extracted from conversation transcripts — summaries, blockers, dead ends, decisions, and gotchas. The next session gets this injected automatically — no re-discovery needed. See [docs/handoff-files.md](docs/handoff-files.md) and [docs/transcript-session-context.md](docs/transcript-session-context.md).

### Feature Templates

When creating a feature, pass a `type` to auto-generate a standard subtask structure:

- **feature** — Planning, Implementation, Polish
- **bugfix** — Investigation, Fix
- **chore** — Work (single phase)
- **spike** — Research (single phase)

Templates are fire-and-forget — once created, subtasks are fully independent. See [docs/feature-templates.md](docs/feature-templates.md).

### Completion Gate

Features can't be marked `done` until all task items are checked and all issues are resolved. If something is outstanding, the update is rejected with a clear message. Override with `force=true` to force-complete — this auto-logs a decision for audit.

### Decision Log

Each feature can have decisions logged against it — what was considered, whether it was accepted or rejected, and why. Rejected decisions are surfaced in context briefings so Claude doesn't re-explore dead ends. See [docs/decision-log.md](docs/decision-log.md).

## MCP Tools

| Tool | Purpose |
|------|---------|
| `add_feature` | Create a feature. Optional `type` (feature/bugfix/chore/spike) auto-generates subtasks from template. |
| `update_feature` | Update status, description, left_off, key_files, etc. Completion gate blocks `done` with unchecked items — use `force`/`force_reason` to override. |
| `list_features` | Compact feature list, filterable by status. |
| `get_feature` | Full feature detail with sessions, subtasks, decisions. |
| `get_context` | Token-efficient briefing (~15-20 lines). |
| `get_ready` | Actionable features (in_progress first, then planned). |
| `get_full_context` | Deep context dump for subagent research. |
| `checkpoint` | Force a checkpoint mid-session (enqueues transcript delta for summarization). |
| `compact_sessions` | Compress old sessions into a summary (keeps last 3). |
| `import_plan` | Import markdown plan file as subtasks/task items. |
| `add_subtask` | Add phase(s) — pipe-separated titles for batch. |
| `add_task_item` | Add task(s) to a subtask — pipe-separated titles for batch. |
| `complete_task_item` | Mark task(s) done — single or `items` JSON array for batch. |
| `add_decision` | Log an approach decision (accepted/rejected with reason). |
| `add_issue` | Log a bug/issue against a feature. |
| `resolve_issue` | Mark an issue as resolved with optional commit hash. |
| `list_issues` | List open issues, optionally filtered by feature. |
| `quick_track` | One-call tracking for small tasks (creates feature + logs session). |

## Dashboard

Run `/docket` to open the dashboard, or check `.docket/port` for the port number.

- **Kanban board**: Planned / In Progress / Blocked / Dev Complete / Done
- Click a card for full detail — session history, subtasks, decisions, issues, key files
- Edit notes inline per feature
- Create and resolve issues inline
- Issue badges on feature cards with open bug counts
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
| `/checkpoint` skill | Force a mid-session checkpoint |
| `/end-session` skill | Close work session without closing Claude (useful when switching features) |
| MCP server | Connects Claude Code to the docket binary |
| Hooks | SessionStart, PreToolUse, PostToolUse, Stop, PreCompact, SessionEnd |

## Documentation

- [INSTALL.md](INSTALL.md) — Installation, updating, troubleshooting
- [docs/docket-hooks.md](docs/docket-hooks.md) — How automatic session tracking works
- [docs/handoff-files.md](docs/handoff-files.md) — Cross-session context handoff
- [docs/transcript-session-context.md](docs/transcript-session-context.md) — Transcript parsing, checkpoints, and session observations
- [docs/decision-log.md](docs/decision-log.md) — Decision tracking per feature
- [docs/feature-templates.md](docs/feature-templates.md) — Feature types, templates, and completion gate
- [docs/tags-and-archival.md](docs/tags-and-archival.md) — Feature tags and auto-archival
- [docs/issue-tracking.md](docs/issue-tracking.md) — Bug/issue tracking per feature
- [docs/quick-track.md](docs/quick-track.md) — Lightweight one-call tracking for small tasks
- [docs/dashboard-session-control.md](docs/dashboard-session-control.md) — Session indicators and launch-from-dashboard
- [docs/session-heartbeat-staleness.md](docs/session-heartbeat-staleness.md) — Staleness detection for crashed sessions
- [docs/plugin-deployment-model.md](docs/plugin-deployment-model.md) — Plugin marketplace/cache architecture
- [docs/dev-workflow.md](docs/dev-workflow.md) — Dev build-deploy-reload cycle
- [docs/pretooluse-agent-nudge.md](docs/pretooluse-agent-nudge.md) — PreToolUse hook for subagent setup reminders
- [docs/mcp-stability.md](docs/mcp-stability.md) — SQLite contention and crash prevention
- [docs/dark-mode-toggle.md](docs/dark-mode-toggle.md) — Dashboard theming and dev mode
- [plugin/README.md](plugin/README.md) — Plugin setup and per-project CLAUDE.md snippet

## License

MIT
