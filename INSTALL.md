# Installing feat

Run the install script:

```bash
bash install.sh
```

This will:
1. Build the feat binary to `~/.local/share/feat/feat.exe`
2. Install the Claude Code plugin to `~/.claude/plugins/cache/local/feat/0.1.0/`
3. Register the plugin in Claude Code settings

## After install

1. Restart Claude Code (or run `/reload-plugins`)
2. Add the CLAUDE.md snippet to your projects — see `plugin/README.md` for the copy-paste block
3. Remove any per-project copies of `.claude/agents/board-manager.md` and `.claude/skills/feat/` — the plugin provides these now
4. Remove the feat entry from any project-level `.mcp.json` files — the plugin handles MCP registration

## Updating

Re-run `bash install.sh` after pulling changes. It overwrites the binary and plugin files.

## Per-project overrides

If a project needs custom board-manager behavior, place a local `.claude/agents/board-manager.md` in that project. Local agents take precedence over plugin agents.
