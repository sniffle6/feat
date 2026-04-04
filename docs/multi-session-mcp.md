# Multi-Session MCP Lifecycle

## What it does

Lets multiple Claude sessions (one per feature) share a single docket MCP server process. Each session is bound to a specific feature and gets its own session-scoped files (transcript offset, commits log). When a session ends, the server stays alive for other sessions.

## Why it exists

Before this, one Claude session owned the docket MCP server. If that session ended, the server shut down — killing tracking for all other open features. This became a problem once docket supported parallel feature work. Now each Claude session independently manages its own state, and the server only exits when no sessions with live MCP processes remain.

## How it works

### Session binding

When a Claude session starts, `SessionStart` hook calls `bind_session(feature_id, session_id)`. This creates or reopens a work session record in SQLite and writes the `mcp_pid` (the docket process PID) into the row. All subsequent hook calls use the `session_id` to find the right work session, so operations are scoped to the correct feature.

The `bind_session` MCP tool also accepts `feature_id` + `session_id` directly — the `board-manager` agent or user can call it manually to rebind a session to a different feature after calling `/end-session`.

### Leader election

The dashboard only runs from one docket process at a time. The process that successfully writes its PID to `.docket/dashboard.pid` (no-clobber) becomes the leader and starts the HTTP server. Other processes skip it. If the leader exits, the next process to start picks it up. The dashboard server uses SSE to push live state updates to the browser.

### Session liveness and shutdown

Each MCP server instance records its PID in the `mcp_pid` column of `work_sessions`. When deciding whether to exit, `SessionEnd` checks if any other open work sessions have live `mcp_pid` values (i.e., the PID is running). If none remain, the process exits. If other live sessions exist, it stays up.

Zombie sessions — open rows with no live `mcp_pid` and a heartbeat older than 24 hours — are cleaned up when a new session opens for the same feature. The zombie is closed and a fresh session is created.

### Session-scoped files

Each work session gets its own files under `.docket/sessions/<session_id>/`:
- `transcript-offset` — byte offset into the Claude JSONL transcript for this session
- `commits.log` — commit hashes recorded during this session

This prevents sessions from stepping on each other's checkpoint state.

## How to use

Normal operation is automatic — hooks handle binding, heartbeats, and shutdown. Manual use:

- `bind_session(feature_id="...", session_id="...")` — bind or rebind a session to a feature. Session ID is shown in the session context at startup.
- `/end-session` skill — closes the work session and writes the handoff file. After closing, call `bind_session` to start tracking a new feature in the same Claude session.

## Key files

- `internal/store/worksession.go` — work session CRUD, `OpenWorkSession`, `SetMcpPid`, `ExecRaw`, `CloseWorkSessionByFeature`
- `internal/store/migrate.go` — schema migrations including `mcp_pid` column (v14), session-scoped dirs (v15)
- `internal/mcp/server.go` — MCP server startup, bind state, shutdown logic
- `internal/mcp/tools_checkpoint.go` — checkpoint tool, session-scoped transcript path, `bind_session` tool
- `cmd/docket/hook.go` — `SessionStart` (calls `bind_session`), `SessionEnd` (handoff + conditional exit)
- `internal/dashboard/dashboard.go` — dashboard server, leader election via `dashboard.pid`
- `internal/dashboard/launch.go` — launch script generation, `DOCKET_FEATURE_ID` env var for pre-binding
- `plugin/skills/end-session/SKILL.md` — end-session skill, rebind instructions
