# Dev Workflow

## What it does

Provides a fast build-deploy-reload cycle for developing docket. The `dev-build.sh` script builds the Go binary, kills the running MCP server, deploys to the marketplace directory, invalidates the plugin cache, and tells you to `/reload-plugins`.

## Why it exists

The binary runs as an MCP server (started by Claude Code) and as a hook handler. After Go code changes, the new binary needs to replace the old one in the marketplace directory and the plugin cache needs invalidating so Claude Code re-caches it.

## How to use it

### Initial setup

```bash
bash install.sh --dev
```

This builds the binary, symlinks the plugin directory (so plugin file edits are live), and registers the plugin.

**Note:** On Windows, the symlink may not work (requires admin/developer mode). In that case, `install.sh --dev` copies files instead, and `dev-build.sh` handles deploying the binary to the right places.

### Day-to-day

| What changed | What to run | When it takes effect |
|---|---|---|
| Plugin files (hooks, skills, agents) | `/reload-plugins` | Immediately |
| Go code | `bash dev-build.sh` then `/reload-plugins` | Immediately (MCP server restarts on reload) |
| Both | `bash dev-build.sh` then `/reload-plugins` | Immediately |

### What dev-build.sh does

1. Builds `plugin/docket.exe` via `go build`
2. Kills any running `docket.exe` process (the MCP server)
3. Copies the binary to `~/.claude/plugins/marketplaces/local/docket/docket.exe`
4. **Only if deploy succeeded:** deletes the plugin cache at `~/.claude/plugins/cache/local/docket` (forces re-cache on next reload). If the plugin dir doesn't exist (deploy skipped), the cache is left alone to avoid breaking a working install.
5. Prints "Run /reload-plugins to restart the MCP server."

After running, do `/reload-plugins` in Claude Code — this restarts the MCP server with the new binary.

### Switching back to production install

```bash
bash install.sh
```

(No `--dev` flag.) This copies files normally.

## Gotchas

- **Cache invalidation.** Claude Code loads plugins from its cache, not the marketplace directory. `dev-build.sh` deletes the cache so Claude Code re-caches from marketplace on next reload. If the cache isn't deleted, the old binary keeps running.
- **`plugin/docket.exe` is gitignored.** Don't commit it.
- **Hooks and MCP share the same binary.** A broken build will break both hooks and the MCP server.
- **Legacy cleanup ordering (install.sh).** Step 5 kills running docket, removes the old `.mcp.json` entry, then deletes the old binary dir — in that order. The `.mcp.json` cleanup comes first so a mid-run abort doesn't leave stale config pointing at a deleted path. The `rm -rf` is best-effort to handle Windows file locks without aborting.

## Key files

- `install.sh` — main installer, `--dev` flag for symlink mode
- `dev-build.sh` — build, kill, deploy, prompt for reload
- `plugin/` — source plugin directory
- `.gitignore` — contains `plugin/docket.exe`
