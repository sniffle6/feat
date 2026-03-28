# feat

Local feature tracker for AI coding agents. MCP server + web dashboard.

## Install

```bash
bash install.sh
```

See `INSTALL.md` for full setup instructions.

## Quick Start

```bash
cd your-project
feat init          # creates .feat/ directory
feat serve         # starts MCP server + dashboard on localhost:7890
```

## Claude Code Setup

Run `bash install.sh` to build and install the plugin. See `INSTALL.md` for details.

The plugin provides:
- **board-manager agent** — handles all feat board operations autonomously
- **/feat skill** — opens the dashboard in a browser
- **MCP server config** — connects Claude Code to the feat binary

Add the CLAUDE.md dispatch snippet to each project (see `plugin/README.md`).

## MCP Tools

| Tool | Purpose |
|---|---|
| `add_feature` | Create a feature. Returns slug ID. |
| `update_feature` | Update status, description, left_off, worktree_path, key_files. |
| `list_features` | Compact feature list, filterable by status. |
| `get_feature` | Full feature detail with all sessions. |
| `get_context` | Token-efficient briefing (~15-20 lines). |
| `get_ready` | Actionable features (in_progress first, then planned). |
| `get_full_context` | Deep context for subagent research. |
| `log_session` | Record what happened in a session. |
| `compact_sessions` | Compress old sessions into a summary. |
| `import_plan` | Import markdown plan as subtasks/task items. |
| `add_subtask` | Create a phase manually. |
| `add_task_item` | Add a task to a subtask. |
| `complete_task_item` | Mark a task item done with outcome and commit hash. |

## Dashboard

Open `http://localhost:7890` while `feat serve` is running (or use `/feat` skill).

- Kanban board: Planned / In Progress / Blocked / Done
- Click a card for full detail, session history, key files
- Edit "left off" notes inline
- Reassign unlinked sessions to features
