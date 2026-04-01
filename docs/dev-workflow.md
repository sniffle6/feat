# Dev Workflow

## What it does

Provides a fast build-deploy-reload cycle for developing docket. The `dev-build.sh` script builds the Go binary, kills the running MCP server, deploys the binary to all install locations, and tells you to `/reload-plugins`.

## Why it exists

The binary runs as an MCP server (started by Claude Code) and as a hook handler. After Go code changes, the new binary needs to replace the old one in all locations Claude Code might look for it. Manually copying to 3 directories and restarting is tedious.

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
3. Copies the binary to all known install locations:
   - `~/.claude/plugins/marketplaces/local/docket/docket.exe`
   - `~/.claude/plugins/cache/local/docket/0.1.0/docket.exe`
   - `~/.local/share/docket/docket.exe` (legacy)
4. Prints "Run /reload-plugins to restart the MCP server."

After running, do `/reload-plugins` in Claude Code — this restarts the MCP server with the new binary.

### Switching back to production install

```bash
bash install.sh
```

(No `--dev` flag.) This copies files normally.

## Gotchas

- **Multiple binary locations.** Claude Code may load the binary from the cache path (`plugins/cache/...`) rather than the marketplace path. `dev-build.sh` copies to all known locations to handle this.
- **`plugin/docket.exe` is gitignored.** Don't commit it.
- **Hooks and MCP share the same binary.** A broken build will break both hooks and the MCP server.

## Key files

- `install.sh` — main installer, `--dev` flag for symlink mode
- `dev-build.sh` — build, kill, deploy, prompt for reload
- `plugin/` — source plugin directory
- `.gitignore` — contains `plugin/docket.exe`
