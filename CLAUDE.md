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

Builds binary to `~/.local/share/docket/docket.exe`, installs plugin to `~/.claude/plugins/cache/local/docket/0.1.0/`.

## Key Files

- `cmd/docket/main.go` — entry point (serve, init, version commands)
- `internal/mcp/server.go` — MCP server setup
- `internal/mcp/tools.go` — all 14 MCP tool implementations
- `internal/store/store.go` — SQLite data layer, Feature/Session structs
- `internal/store/migrate.go` — schema migrations (3 versions)
- `internal/store/subtask.go` — subtask/task item operations
- `internal/store/import.go` — plan file parser
- `internal/dashboard/dashboard.go` — HTTP handler for web UI
- `dashboard/index.html` — frontend (embedded in binary)
- `plugin/` — Claude Code plugin (agent, skill, MCP config)

## Dashboard

http://localhost:<port> (port is per-project, see `.docket/port`) (runs while MCP server is active)

## Architecture

docket.exe runs two things in parallel:
1. MCP server on stdio (Claude Code talks to this)
2. HTTP dashboard on a per-project port (user opens in browser)

Both read/write the same SQLite database at `<project>/.docket/features.db`.

## Feature Tracking (docket)

This project uses `docket` for feature tracking. Dashboard: http://localhost:7890 (or run `/docket`).

Dispatch the `board-manager` agent (model: sonnet) at these points:
1. **Start of implementation work** — skip for questions/reviews/lookups
2. **After a commit** — pass commit hash, message, files, feature ID

Session logging is handled automatically by the Stop hook (no agent dispatch needed).

Carry the feature ID the agent returns across dispatches. `get_ready` stays in main session.
