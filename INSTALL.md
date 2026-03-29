# Installing docket

## Prerequisites

- [Go 1.21+](https://go.dev/dl/) — used to build the binary
- [Python 3](https://www.python.org/) — used by the install script for JSON file manipulation
- [Claude Code](https://docs.anthropic.com/en/docs/claude-code) — must be initialized (run at least once so `~/.claude/` exists)

## Install

Run the install script from the docket repo root:

```bash
bash install.sh
```

This will:
1. Copy source to `~/.local/share/docket/` and build the binary
2. Install the Claude Code plugin to `~/.claude/plugins/marketplaces/local/docket/`
3. Generate `.mcp.json` with the absolute path to the binary
4. Install hooks (`SessionStart`, `PostToolUse`, `Stop`) with the binary path baked in
5. Register the plugin in `~/.claude/settings.json` (`enabledPlugins`)
6. Register the plugin in `~/.claude/plugins/installed_plugins.json`

## After install

1. Restart Claude Code (or run `/reload-plugins`)
2. In any project, run `/docket-init` to set up tracking — this creates `.docket/` and adds the dispatch snippet to `CLAUDE.md`
3. Remove any per-project copies of `.claude/agents/board-manager.md` and `.claude/skills/docket/` — the plugin provides these now
4. Remove the docket entry from any project-level `.mcp.json` files — the plugin handles MCP registration

## Updating

Pull the latest changes and re-run the install script:

```bash
cd /path/to/claude-docket
git pull
bash install.sh
```

This overwrites the binary and plugin files. Restart Claude Code after updating — hooks are loaded at session start and won't pick up changes mid-session.

To update the CLAUDE.md snippet in a specific project, run `/docket-update` in that project.

## Per-project overrides

If a project needs custom board-manager behavior, place a local `.claude/agents/board-manager.md` in that project. Local agents take precedence over plugin agents.

## Troubleshooting

- **"Go is not installed"** — Install Go from https://go.dev/dl/
- **Plugin not loading** — Check that `"docket@local": true` exists in `~/.claude/settings.json` under `enabledPlugins`
- **Dashboard blank** — The MCP server must be running. Check that the plugin's `.mcp.json` points to a valid binary path
- **Hooks not firing** — Restart Claude Code. Hooks are loaded once at session start
- **Old global MCP config** — If you previously had docket in `~/.claude/.mcp.json`, remove that entry. The plugin handles registration now
