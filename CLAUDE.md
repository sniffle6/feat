# feat

Local feature tracker for Claude Code sessions. MCP server + SQLite + web dashboard.

## Build

```
go build -ldflags="-s -w" -o feat.exe ./cmd/feat/
```

## Test

```
go test ./...
```

## Install

```
bash install.sh
```

Builds binary to `~/.local/share/feat/feat.exe`, installs plugin to `~/.claude/plugins/cache/local/feat/0.1.0/`.

## Key Files

- `cmd/feat/main.go` — entry point (serve, init, version commands)
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

http://localhost:7890 (runs while MCP server is active)

## Architecture

feat.exe runs two things in parallel:
1. MCP server on stdio (Claude Code talks to this)
2. HTTP dashboard on :7890 (user opens in browser)

Both read/write the same SQLite database at `<project>/.feat/features.db`.
