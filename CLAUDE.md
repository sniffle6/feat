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

Builds binary and installs plugin to `~/.claude/plugins/marketplaces/local/docket/`.

## Dev Workflow

For development (symlink-based — plugin edits are live without reinstalling):

```
bash install.sh --dev
```

This symlinks `~/.claude/plugins/marketplaces/local/docket/` to the source `plugin/` directory. One-time setup.

After Go code changes:

```
bash dev-build.sh
```

Then run `/reload-plugins` (restarts the MCP server with the new binary). Plugin file changes (hooks, skills, agents) also just need `/reload-plugins`.

## Docs

`docs/` contains project feature docs (tracked in git). `docs/superpowers/` contains working artifacts from brainstorming/planning skills (gitignored). Don't mix them.

## Key Files

- `cmd/docket/main.go` — entry point (serve, init, version commands)
- `cmd/docket/hook.go` — hook handlers (SessionStart, PreToolUse, PostToolUse, Stop, PreCompact, SessionEnd)
- `cmd/docket/handoff.go` — thin wrappers delegating to internal/handoff
- `cmd/docket/update.go` — CLAUDE.md snippet sync command
- `cmd/docket/export.go` — handoff file export for context resets
- `internal/mcp/tools.go` — tool registration (20 tools), handlers split across tools_*.go
- `internal/mcp/tools_note.go` — add_note MCP tool handler
- `internal/mcp/tools_checkpoint.go` — checkpoint MCP tool, transcript path finder
- `internal/mcp/tools_session.go` — session-related MCP tool handlers (compact_sessions)
- `internal/mcp/tools_search.go` — search MCP tool handler (FTS5 cross-feature search)
- `internal/store/store.go` — SQLite data layer, Feature/FeatureUpdate structs, completion gate
- `internal/store/search.go` — FTS5 search query methods (Search, RebuildSearchIndex)
- `internal/store/migrate.go` — schema migrations (v1-v17)
- `internal/store/checkpoint.go` — checkpoint job queue + observation CRUD
- `internal/store/worksession.go` — work session CRUD (open, close, get active)
- `internal/store/templates.go` — feature type templates (feature/bugfix/chore/spike)
- `internal/store/import.go` — plan file parser (regex-based markdown → subtasks)
- `internal/transcript/parse.go` — JSONL transcript parser (byte-offset delta extraction)
- `internal/transcript/types.go` — Delta struct, trivial message map
- `internal/checkpoint/worker.go` — background job queue worker (polls, summarizes, writes observations)
- `internal/checkpoint/anthropic.go` — Anthropic Messages API summarizer
- `internal/checkpoint/config.go` — checkpoint config from env vars
- `internal/handoff/render.go` — shared handoff rendering (used by hooks and MCP tools)
- `internal/dashboard/dashboard.go` — HTTP dashboard server, API endpoints, SSE events
- `internal/dashboard/launch.go` — launch prompt/script generation for dashboard play button
- `dashboard/index.html` — single-file frontend (embedded via Go embed)
- `plugin/` — Claude Code plugin (agent, skills, hooks, MCP config, binary at install time)
- `plugin/.mcp.json` — MCP server config using `${CLAUDE_PLUGIN_ROOT}/docket.exe`

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

- `commits.log` is written by PostToolUse hook (records commit hashes). Cleared by SessionEnd hook after handoff.
- `transcript-offset` file tracks byte offset into Claude's JSONL transcript. Reset at SessionStart, advanced after each checkpoint.
- Stop hook enqueues checkpoint jobs when transcript delta is meaningful (commits, errors, failed tests, 300+ chars, or non-trivial user input).
- PreCompact hook always checkpoints (no threshold — context compression means data loss).
- SessionEnd hook enqueues remaining delta, writes handoff files, closes work session.

## Adding Schema Migrations

Add a new `const schemaVN` in `migrate.go`, then `db.Exec(schemaVN)` in `migrate()`. Use `CREATE TABLE IF NOT EXISTS` or `ALTER TABLE` — errors are ignored for idempotency. No version tracking table.

## Completion Gate

`UpdateFeature` blocks `status=done` if unchecked task items or open issues exist. Pass `Force: true` + `ForceReason` to override (auto-logs a decision). Features with no subtasks pass the gate.

## Test Pattern

Store tests: `s, _ := Open(t.TempDir())` gives a fresh DB. No mocks, no cleanup needed.

## Release Versioning

After each commit, consider whether the accumulated changes since the last release tag warrant a version bump. Check `git log <last-tag>..HEAD --oneline` to see what's unreleased. Binary-affecting changes (Go code, schema migrations, new endpoints) should ship as a release. Docs-only or config-only changes can wait.

## Push Back

If there's a better approach than what's being asked for — say so. Explain why and propose the alternative. Don't just comply with a suboptimal request when you can see a clearly better path.

## Feature Tracking (docket)

This project uses `docket` for feature tracking. Dashboard: http://localhost:<port> (or run `/docket`). Active feature context is auto-injected at session start.

**Small tasks**: call `quick_track` — one call, no agent dispatch needed.

**Larger features**: call `get_ready`, then dispatch `board-manager` agent (model: sonnet) to create or find a card. Call `bind_session(feature_id, session_id)` to bind the session (session ID is in the session context message). Use `type` (feature/bugfix/chore/spike) for auto-generated subtask templates. Always pass `tags` when calling `add_feature` — use existing tags from `list_features`.

**Plan execution (superpowers):** When using executing-plans or subagent-driven-development, set up docket first — `get_ready`, create/find a card, `add_task_item` per plan task.

After a commit — use **direct MCP calls**, not agent dispatch:
- `update_feature` — left_off, key_files, status, tags. Completion gate blocks `done` with unchecked items — `force=true` + `force_reason` to override.
- `complete_task_item` — check off items with outcome and commit_hash (`items` JSON array for batch)
- `add_decision` — accepted/rejected with reason
- `add_note` — append findings, context, observations to a feature card
- `add_issue` / `resolve_issue` — track bugs found during work

Use `get_context` (not `get_feature`) for routine status checks (~15 lines, token-efficient).

Commit tracking, session context, and handoff files are automatic (hooks). Use `/checkpoint` for manual checkpoints, `/end-session` to close the work session without closing Claude.

**If user rejects a docket update**, fix the issue and retry — don't drop tracking.
