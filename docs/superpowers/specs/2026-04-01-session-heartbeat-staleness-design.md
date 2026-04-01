# Session Heartbeat & Staleness Detection

## Problem

When Claude Code crashes or the user kills the terminal, the work session stays `open` with `session_state=working` forever. The dashboard shows a green pulsing "Working" indicator on a dead session. There's no way to distinguish "actively working" from "session died."

## Approach

Hook-driven heartbeat with dashboard-side staleness evaluation. Every hook event updates a `last_heartbeat` timestamp on the work session. The dashboard compares heartbeat age against a threshold and renders a "Stale" indicator when it's too old.

The MCP server can't detect its own death, so eager detection (background ticker) doesn't solve the core problem. The dashboard is the natural evaluator — it runs independently of the Claude session.

### Why not reuse "Waiting"?

"Waiting" means Claude is alive and sitting in the terminal expecting input. "Stale" means the session is probably dead. Different user actions: go type in the terminal vs. start a new session. Collapsing them loses that distinction.

## Data Layer

### Schema migration (V13)

Add `last_heartbeat` column to `work_sessions` and add `stale` to the allowed `session_state` values. SQLite can't alter CHECK constraints in place, so this requires a table rebuild (same pattern as schemaV10 for checkpoint_jobs).

New allowed `session_state` values: `idle`, `working`, `needs_attention`, `stale`.

Note: `stale` is added to the CHECK constraint for completeness (other code paths may write it in the future), but the initial implementation treats staleness as a display-only concern — the dashboard evaluates it from `last_heartbeat` age, not from the stored `session_state` value.

### Store changes

- **`TouchHeartbeat(id int64)`** — updates `last_heartbeat = datetime('now')` on the work session. Single UPDATE, no return value needed.
- **`OpenWorkSession`** — sets `last_heartbeat` on INSERT (initial heartbeat = session start time).
- **`GetActiveSessionStates`** — returns `last_heartbeat` alongside `feature_id` and `session_state` so the dashboard API can pass it through.

### Staleness threshold

5 minutes, hardcoded. Hooks fire on every Stop and PostToolUse, so even slow sessions with long tool runs heartbeat multiple times within 5 minutes. Long enough to avoid false positives from a single long-running bash command.

## Hook Integration

| Hook | Heartbeat? | Reasoning |
|------|-----------|-----------|
| SessionStart | Yes | `OpenWorkSession` sets it on insert |
| PostToolUse | Yes | Fires on every tool call — most frequent heartbeat source |
| Stop | Yes | Proves session is alive even when waiting for input |
| PreCompact | Yes | Session is active during context compaction |
| SessionEnd | No | Session is closing, heartbeat irrelevant |

Implementation: add `s.TouchHeartbeat(ws.ID)` in 4 hook handlers. Each already has the work session loaded.

## Dashboard API

`GET /api/features` already returns `session_state` from `GetActiveSessionStates`. Extend to also return `last_heartbeat` as an ISO 8601 UTC string (e.g. `"2026-04-01T14:30:00Z"`). The staleness decision is made in JS:

```
if (state === 'working' || state === 'needs_attention') {
    if (now - last_heartbeat > 5 minutes) {
        render as "Stale"
    } else {
        render as current state
    }
}
```

Staleness is a display concern, not written back to the DB. The DB stays `working` or `needs_attention` — the dashboard overlays "Stale" when the heartbeat is too old.

## Dashboard UI

| State | Dot | Color | Text | Launch button |
|-------|-----|-------|------|--------------|
| Working | Pulsing | Green | "Working" | Disabled |
| Waiting | Pulsing (fast) | Yellow | "Waiting" | Enabled |
| Stale | Static | Gray | "Stale (12m)" | Enabled |
| Idle | None | — | — | Enabled |

- Static dot (no pulse) distinguishes stale from active states
- Relative time since last heartbeat shown in parentheses (e.g. "Stale (12m)", "Stale (3h)")
- Launch button enabled — starting a new session auto-closes the stale one via `OpenWorkSession`

## Cleanup

No new cleanup mechanism. `OpenWorkSession` already closes all other open sessions when a new one starts. A stale session gets auto-resolved the next time the user works on any feature.

Edge case: a stale session that nobody replaces (user abandons the project for days). Harmless — dashboard shows "Stale (3d)" which is accurate.

No `SessionEnd` hook fires for crashed sessions (the process is dead). No handoff file gets written. The checkpoint system already captured observations up to the last Stop hook, so data loss is limited to the final segment of work after the last checkpoint.

## Key Files

- `internal/store/migrate.go` — V13 migration (table rebuild + new column)
- `internal/store/worksession.go` — `TouchHeartbeat`, updated `OpenWorkSession` and `GetActiveSessionStates`
- `cmd/docket/hook.go` — heartbeat calls in 4 hook handlers
- `dashboard/index.html` — staleness evaluation and "Stale" indicator rendering
- `internal/dashboard/dashboard.go` — pass `last_heartbeat` through API response
