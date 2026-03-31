# Installing docket

## From marketplace (recommended)

No build tools required. In Claude Code:

```
/plugin marketplace add sniffle6/claude-docket
/plugin install docket@claude-docket
```

Restart Claude Code. On first session start, the binary downloads automatically from GitHub releases for your platform (Windows, macOS, Linux — amd64 and arm64).

## From source

### Prerequisites

- [Go 1.21+](https://go.dev/dl/) — used to build the binary
- [Claude Code](https://docs.anthropic.com/en/docs/claude-code) — must be initialized (run at least once so `~/.claude/` exists)
- Optional: [jq](https://jqlang.github.io/jq/) or [Python 3](https://www.python.org/) — for automatic plugin registration in settings files. Without either, the script tells you what to add manually.

### Install

Run the install script from the docket repo root:

```bash
git clone https://github.com/sniffle6/claude-docket.git
cd claude-docket
bash install.sh
```

This will:
1. Build the binary in the source directory
2. Copy the binary and plugin files to `~/.claude/plugins/marketplaces/local/docket/`
3. Register the plugin in `~/.claude/settings.json` (`enabledPlugins`)
4. Register the plugin in `~/.claude/plugins/installed_plugins.json`

The binary lives inside the plugin directory. All hooks, MCP config, and skills reference it via `${CLAUDE_PLUGIN_ROOT}/docket.exe` — no hardcoded paths or install-time substitutions.

## After install

1. Restart Claude Code (or run `/reload-plugins`)
2. In any project, run `/docket-init` to set up tracking — this creates `.docket/` and adds the dispatch snippet to `CLAUDE.md`
3. Remove any per-project copies of `.claude/agents/board-manager.md` and `.claude/skills/docket/` — the plugin provides these now
4. Remove the docket entry from any project-level `.mcp.json` files — the plugin handles MCP registration
5. If upgrading from an older install, you can remove `~/.local/share/docket/` — the binary now lives inside the plugin directory

## Updating

### Marketplace install

```
/plugin marketplace update claude-docket
```

The binary re-downloads on next session start if the version changed.

### Source install

Pull the latest changes and re-run the install script:

```bash
cd /path/to/claude-docket
git pull
bash install.sh
```

This rebuilds the binary and overwrites the plugin files. Restart Claude Code after updating — hooks are loaded at session start and won't pick up changes mid-session.

To update the CLAUDE.md snippet in a specific project, run `/docket-update` in that project.

## Per-project overrides

If a project needs custom board-manager behavior, place a local `.claude/agents/board-manager.md` in that project. Local agents take precedence over plugin agents.

## Troubleshooting

- **Binary not downloading** — Check internet access. The SessionStart hook downloads from `https://github.com/sniffle6/claude-docket/releases/latest/download/`. Requires `curl` or `wget`.
- **"Go is not installed"** — Only applies to source install. Install Go from https://go.dev/dl/
- **Plugin not loading** — Check that the plugin is enabled: run `/plugin` and check the Installed tab
- **Dashboard blank** — The MCP server must be running. Verify `docket.exe` exists in the plugin directory
- **Hooks not firing** — Restart Claude Code. Hooks are loaded once at session start
- **Old global MCP config** — If you previously had docket in `~/.claude/.mcp.json`, remove that entry. The plugin handles registration now
- **Old binary at `~/.local/share/docket/`** — Safe to remove. The binary now lives inside the plugin directory
