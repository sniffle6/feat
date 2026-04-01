# Session Heartbeat & Staleness Detection

Detects crashed or killed Claude Code sessions and shows them as "Stale" on the dashboard instead of falsely showing "Working."

## How It Works

Every hook event (Stop, PreCompact, PostToolUse with state flip or git commit) updates a `last_heartbeat` timestamp on the work session in SQLite. When the dashboard renders, it compares the heartbeat age against a 5-minute threshold. If the heartbeat is older than 5 minutes and the session is still marked as "working" or "needs_attention," the dashboard shows a gray "Stale (Xm)" indicator instead.

Staleness is a display-only concern — the DB stays in its last known state. This is because the MCP server (which writes to the DB) dies with the Claude session, so it can't update its own state when it crashes.

## Dashboard Indicators

| State | Indicator | Action |
|-------|-----------|--------|
| Working | Green pulsing dot | Session is live, launch disabled |
| Waiting | Yellow pulsing dot | Claude needs input, launch enabled |
| Stale | Gray static dot + "(Xm)" | Session probably dead, launch enabled |
| Idle | No indicator | No active session |

## Cleanup

Stale sessions auto-resolve when a new session starts — `OpenWorkSession` closes all other open sessions. No manual cleanup needed.

## Key Files

- `internal/store/migrate.go` — V13 migration (last_heartbeat column)
- `internal/store/worksession.go` — `TouchHeartbeat`, `SessionStateInfo`, updated `GetActiveSessionStates`
- `cmd/docket/hook.go` — heartbeat calls in Stop, PreCompact, PostToolUse hooks
- `dashboard/index.html` — staleness evaluation and rendering
- `internal/dashboard/dashboard.go` — passes `last_heartbeat` through API, stale-aware launch check
