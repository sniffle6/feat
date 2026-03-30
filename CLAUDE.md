# docket

Local feature tracker for Claude Code sessions. MCP server + SQLite + web dashboard.

## Build

```
go build -ldflags="-s -w" -o docket.exe ./cmd/docket/
```

## Test

```
go test ./...
```

## Install

```
bash install.sh
```

Builds binary to `~/.local/share/docket/docket.exe`, installs plugin to `~/.claude/plugins/marketplaces/local/docket/`.

## Key Files

- `cmd/docket/main.go` — entry point (serve, init, version commands)
- `cmd/docket/hook.go` — SessionStart/PostToolUse/Stop hook handlers
- `cmd/docket/handoff.go` — handoff file renderer and writer
- `cmd/docket/update.go` — CLAUDE.md snippet sync command
- `cmd/docket/export.go` — handoff file export for context resets
- `internal/mcp/tools.go` — tool registration (18 tools), handlers split across tools_*.go
- `internal/store/store.go` — SQLite data layer, Feature/FeatureUpdate structs, completion gate
- `internal/store/migrate.go` — schema migrations (v1-v7)
- `internal/store/templates.go` — feature type templates (feature/bugfix/chore/spike)
- `internal/store/import.go` — plan file parser (regex-based markdown → subtasks)
- `dashboard/index.html` — single-file frontend (embedded via Go embed)
- `plugin/` — Claude Code plugin (agent, skills, hooks, MCP config)

## Dashboard

http://localhost:<port> (port is per-project, see `.docket/port`) (runs while MCP server is active)

## Architecture

docket.exe runs two things in parallel:
1. MCP server on stdio (Claude Code talks to this)
2. HTTP dashboard on a per-project port (user opens in browser)

Both read/write the same SQLite database at `<project>/.docket/features.db`.

## SQLite Gotchas

- `datetime('now')` has second-level precision — use `ORDER BY id DESC` not `ORDER BY created_at DESC` when insertion order matters within the same second.

## Hook / MCP IPC

- `log_session` MCP handler writes `.docket/session-logged` sentinel file. Stop hook checks for it to avoid double-logging. Cleared after each stop cycle.
- `commits.log` is written by PostToolUse hook, read by Stop hook. Cleared after handoff.

## Adding Schema Migrations

Add a new `const schemaVN` in `migrate.go`, then `db.Exec(schemaVN)` in `migrate()`. Use `CREATE TABLE IF NOT EXISTS` or `ALTER TABLE` — errors are ignored for idempotency. No version tracking table.

## Completion Gate

`UpdateFeature` blocks `status=done` if unchecked task items or open issues exist. Pass `Force: true` + `ForceReason` to override (auto-logs a decision). Features with no subtasks pass the gate.

## Test Pattern

Store tests: `s, _ := Open(t.TempDir())` gives a fresh DB. No mocks, no cleanup needed.

## Feature Tracking (docket)

This project uses `docket` for feature tracking. Dashboard: http://localhost:<port> (or run `/docket`).

**Small tasks** (cosmetic changes, one-off fixes, config tweaks): call `quick_track` directly — one call, no agent dispatch needed.

**Larger features** (multi-step, plan-driven, complex):

Start of work (after any brainstorming/planning) — call `get_ready` to find existing features, then dispatch `board-manager` agent (model: sonnet) to create or find a card. Use `type` param (feature/bugfix/chore/spike) to auto-generate subtask templates.

Use `tags` param (comma-separated) on `add_feature`/`update_feature` to categorize work. New tags warn about existing tags to prevent typos.

Done features are auto-archived after 7 days. Use `list_features(status="archived")` to see them. `update_feature(status="planned")` to unarchive.

After a commit — use **direct MCP calls**, not agent dispatch:
- `update_feature` — set left_off, key_files, status, tags. Completion gate blocks `done` with unchecked items — pass `force=true` + `force_reason` to override.
- `complete_task_item` — check off items with outcome and commit_hash (pass `items` JSON array for batch)
- `add_decision` — record notable decisions (accepted/rejected with reason)
- `add_issue` / `resolve_issue` — track bugs found during work

Plan files committed during work are auto-imported by hooks. Only dispatch board-manager when the update needs judgment (restructuring imported plans, creating new subtasks).

After subagent work — subagent commits bypass hooks. Use direct MCP calls to batch-update the feature.

Use `get_context` (not `get_feature`) for routine status checks — it's token-efficient (~15 lines).

Session logging and handoff files are handled automatically by the Stop hook.

Carry the feature ID across the session.

**If user rejects a docket update**, fix the issue and retry — don't drop tracking.
