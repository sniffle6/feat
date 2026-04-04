#!/usr/bin/env bash
set -e

# Rebuilds the docket binary and deploys it to the plugin install location.
# Usage: bash dev-build.sh

SOURCE_DIR="$(cd "$(dirname "$0")" && pwd)"
PLUGIN_INSTALL="$HOME/.claude/plugins/marketplaces/local/docket"

echo "Building docket..."
cd "$SOURCE_DIR"
VERSION=$(grep '"version"' plugin/.claude-plugin/plugin.json | sed 's/.*: *"\(.*\)".*/\1/')
[ -n "$VERSION" ] || { echo "ERROR: could not extract version from plugin.json"; exit 1; }
go build -ldflags="-s -w -X main.version=$VERSION" -o plugin/docket.exe ./cmd/docket/
echo "Done. $(./plugin/docket.exe version)"

# Kill running docket MCP server so Claude Code restarts it with the new binary
if tasklist 2>/dev/null | grep -qi "docket.exe"; then
  taskkill //F //IM docket.exe >>/dev/null 2>&1 && echo "Killed running docket.exe" || true
fi

# Deploy binary to marketplace (the ONE correct location)
DEPLOYED=false
if [ -d "$PLUGIN_INSTALL" ] || [ -L "$PLUGIN_INSTALL" ]; then
  cp plugin/docket.exe "$PLUGIN_INSTALL/docket.exe" && { echo "Deployed to $PLUGIN_INSTALL/docket.exe"; DEPLOYED=true; }
else
  echo "WARNING: Plugin install dir not found at $PLUGIN_INSTALL"
  echo "Run 'bash install.sh --dev' first."
fi

# Only invalidate cache if deploy succeeded — otherwise we'd break the existing cached version
if [ "$DEPLOYED" = true ]; then
  CACHE_DIR="$HOME/.claude/plugins/cache/local/docket"
  if [ -d "$CACHE_DIR" ]; then
    rm -rf "$CACHE_DIR"
    echo "Deleted plugin cache at $CACHE_DIR (Claude Code will re-cache on restart)"
  fi
fi

echo "Run /reload-plugins to restart the MCP server."
