# Dev Workflow

## What it does

Switches the local plugin install from copy-based to symlink-based. The installed plugin directory (`~/.claude/plugins/marketplaces/local/docket/`) becomes a symlink to the source `plugin/` directory. Edits to plugin files are immediately live.

## Why it exists

The copy-based install (`install.sh` without flags) copies every file from `plugin/` into the install location. This means:
- Every plugin file edit (hooks, skills, agents, `.mcp.json`) requires re-running `install.sh`
- Every Go code change requires `go build` + `install.sh` (build + copy)
- Easy to forget the install step and debug stale code

With the symlink approach:
- Plugin file edits are live immediately (just `/reload-plugins`)
- Go code changes need only `bash dev-build.sh` (builds into `plugin/docket.exe`)
- No copy step, no forgetting

## How to use it

### One-time setup

```bash
bash install.sh --dev
```

This:
1. Builds the binary into `plugin/docket.exe`
2. Replaces the install dir with a symlink to `plugin/`
3. Registers the plugin in `settings.json` and `installed_plugins.json` (if not already there)

### Day-to-day

| What changed | What to run | When it takes effect |
|---|---|---|
| Plugin files (hooks, skills, agents) | Nothing (or `/reload-plugins`) | Next session (hooks), immediately (skills after reload) |
| Go code | `bash dev-build.sh` | Next session (MCP server restarts) |
| Both | `bash dev-build.sh` | Next session |

### Switching back to production install

```bash
bash install.sh
```

(No `--dev` flag.) This removes the symlink and copies files normally.

## Gotchas

- **Binary not hot-reloaded.** The MCP server (`docket.exe serve`) is started once per session. Rebuilding the binary only takes effect on the next session.
- **Hooks run the binary directly.** Same as above — a broken `go build` that produces a bad binary will break hooks for the next session. If this happens, fix the code, rebuild, and restart Claude Code.
- **`plugin/docket.exe` is gitignored.** Don't commit it. The `.gitignore` entry `plugin/docket.exe` prevents this.
- **Symlinks on Windows.** Git Bash supports `ln -sfn` on NTFS (creates a junction or symlink depending on privileges). If you get permission errors, run Git Bash as administrator once for the initial setup.
- **Production install is still copy-based.** `install.sh` without `--dev` works exactly as before. Marketplace users are unaffected — they use `ensure-binary.sh` to download from GitHub releases.

## Key files

- `install.sh` — main installer, `--dev` flag for symlink mode
- `dev-build.sh` — one-liner Go build into `plugin/docket.exe`
- `plugin/` — source plugin directory (symlink target in dev mode)
- `.gitignore` — contains `plugin/docket.exe` to prevent committing the dev binary
