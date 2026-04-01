# Plugin Deployment Model

## How Claude Code plugins work

Claude Code plugins live in a **marketplace directory** and are cached in a **cache directory**.

### Marketplace directory

```
~/.claude/plugins/marketplaces/local/docket/
```

This is where the plugin is installed. It contains the plugin manifest, MCP config, hooks, skills, agents, and the binary.

The `.mcp.json` uses a variable for the binary path:

```json
{"mcpServers":{"docket":{"command":"${CLAUDE_PLUGIN_ROOT}/docket.exe","args":["serve"]}}}
```

`${CLAUDE_PLUGIN_ROOT}` is resolved by Claude Code at cache time to the absolute path of the plugin directory.

### Cache directory

```
~/.claude/plugins/cache/local/docket/<version>/
```

Claude Code creates this by copying from the marketplace directory. At cache time, `${CLAUDE_PLUGIN_ROOT}` in `.mcp.json` is resolved to the actual path and written as a literal string in the cached copy.

**The cache is NOT automatically refreshed.** It persists until:
- The cache directory is manually deleted
- The plugin version changes (new cache key)

This means if the plugin is moved or reinstalled to a different path, the cache still points to the old path.

### Dev workflow implications

- **`dev-build.sh`** deploys only to the marketplace directory and deletes the cache. This forces Claude Code to re-cache from marketplace on next session/reload.
- **`install.sh`** does the same cache invalidation.
- **Never copy directly to the cache directory.** It will be overwritten on next cache refresh, and the `.mcp.json` inside it has resolved (hardcoded) paths that may be wrong.

## Troubleshooting

### Two docket.exe processes running

**Cause:** Stale cache `.mcp.json` pointing to an old binary path.

**Fix:**
```bash
rm -rf ~/.claude/plugins/cache/local/docket
# Restart Claude Code
```

### Binary not found after install

**Cause:** Cache still points to old path.

**Fix:** Same as above — delete cache and restart.

## Key files

- `plugin/.mcp.json` — MCP server config (uses `${CLAUDE_PLUGIN_ROOT}`)
- `dev-build.sh` — builds binary, deploys to marketplace, invalidates cache
- `install.sh` — full install (build, plugin registration, cleanup, cache invalidation)
