#!/usr/bin/env bash
set -e

# docket installer — builds binary, installs Claude Code plugin
# Usage: bash /path/to/docket/install.sh

SOURCE_DIR="$(cd "$(dirname "$0")" && pwd)"
INSTALL_DIR="$HOME/.local/share/docket"
DOCKET_BIN="$INSTALL_DIR/docket.exe"
PLUGIN_INSTALL="$HOME/.claude/plugins/marketplaces/local/docket"
SETTINGS_FILE="$HOME/.claude/settings.json"
PLUGINS_FILE="$HOME/.claude/plugins/installed_plugins.json"

echo "=== docket installer ==="
echo "Source:  $SOURCE_DIR"
echo "Install: $INSTALL_DIR"
echo "Plugin:  $PLUGIN_INSTALL"

# Check Go is available
if ! command -v go &> /dev/null; then
    echo "ERROR: Go is not installed. Install Go first: https://go.dev/dl/"
    exit 1
fi

# --- Step 1: Copy source and build binary ---

echo "Copying source to $INSTALL_DIR..."
mkdir -p "$INSTALL_DIR"
cp -r "$SOURCE_DIR/cmd" "$INSTALL_DIR/"
cp -r "$SOURCE_DIR/internal" "$INSTALL_DIR/"
cp -r "$SOURCE_DIR/dashboard" "$INSTALL_DIR/"
cp "$SOURCE_DIR/go.mod" "$SOURCE_DIR/go.sum" "$SOURCE_DIR/README.md" "$INSTALL_DIR/"
cp "$SOURCE_DIR/.gitignore" "$INSTALL_DIR/" 2>/dev/null || true

echo "Building docket..."
cd "$INSTALL_DIR"
go build -ldflags="-s -w" -o "$DOCKET_BIN" ./cmd/docket/
echo "Built: $DOCKET_BIN"
"$DOCKET_BIN" version

# --- Step 2: Install plugin ---

echo "Installing plugin to $PLUGIN_INSTALL..."
rm -rf "$PLUGIN_INSTALL"
mkdir -p "$PLUGIN_INSTALL"
cp -r "$SOURCE_DIR/plugin/.claude-plugin" "$PLUGIN_INSTALL/"
cp -r "$SOURCE_DIR/plugin/agents" "$PLUGIN_INSTALL/"
cp -r "$SOURCE_DIR/plugin/skills" "$PLUGIN_INSTALL/"
cp "$SOURCE_DIR/plugin/README.md" "$PLUGIN_INSTALL/"

# Generate .mcp.json with absolute path to binary
# Convert to forward-slash Windows path for JSON (C:/Users/... not /c/Users/...)
if command -v cygpath &> /dev/null; then
    DOCKET_BIN_JSON=$(cygpath -m "$DOCKET_BIN")
else
    DOCKET_BIN_JSON=$(echo "$DOCKET_BIN" | sed 's|\\|/|g')
fi
cat > "$PLUGIN_INSTALL/.mcp.json" << MCPEOF
{
  "mcpServers": {
    "docket": {
      "command": "$DOCKET_BIN_JSON",
      "args": ["serve"],
      "type": "stdio"
    }
  }
}
MCPEOF
echo "Generated $PLUGIN_INSTALL/.mcp.json"

# Copy hooks and replace binary path placeholder
if [ -d "$SOURCE_DIR/plugin/hooks" ]; then
    cp -r "$SOURCE_DIR/plugin/hooks" "$PLUGIN_INSTALL/"
    sed -i "s|DOCKET_EXE_PATH|$DOCKET_BIN_JSON|g" "$PLUGIN_INSTALL/hooks/hooks.json"
    echo "Installed hooks with binary path: $DOCKET_BIN_JSON"
fi

# --- Step 3: Register plugin in settings.json ---

if [ -f "$SETTINGS_FILE" ]; then
    if grep -q '"docket@local"' "$SETTINGS_FILE" 2>/dev/null; then
        echo "docket@local already in settings.json — skipping"
    else
        # Insert docket@local into enabledPlugins object
        sed -i 's/"enabledPlugins": {/"enabledPlugins": {\n    "docket@local": true,/' "$SETTINGS_FILE"
        echo "Added docket@local to settings.json"
    fi
else
    echo "WARNING: $SETTINGS_FILE not found. Add \"docket@local\": true to enabledPlugins manually."
fi

# --- Step 4: Register plugin in installed_plugins.json ---

TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%S.000Z")
# Convert to Windows path for JSON
if command -v cygpath &> /dev/null; then
    INSTALL_PATH_WIN=$(cygpath -w "$PLUGIN_INSTALL")
else
    INSTALL_PATH_WIN="$PLUGIN_INSTALL"
fi

if [ -f "$PLUGINS_FILE" ]; then
    if grep -q '"docket@local"' "$PLUGINS_FILE" 2>/dev/null; then
        echo "docket@local already in installed_plugins.json — skipping"
    else
        # Use python3 with sys.argv to avoid backslash escaping issues
        python3 -c "
import json, sys
path, ts, pf = sys.argv[1], sys.argv[2], sys.argv[3]
with open(pf, 'r') as f:
    data = json.load(f)
data['plugins']['docket@local'] = [{
    'scope': 'user',
    'installPath': path,
    'version': '0.1.0',
    'installedAt': ts,
    'lastUpdated': ts
}]
with open(pf, 'w') as f:
    json.dump(data, f, indent=2)
" "$INSTALL_PATH_WIN" "$TIMESTAMP" "$PLUGINS_FILE"
        echo "Added docket@local to installed_plugins.json"
    fi
else
    echo "WARNING: $PLUGINS_FILE not found. Claude Code may not have been initialized yet."
fi

# --- Step 5: Clean up old global MCP config ---

MCP_FILE="$HOME/.claude/.mcp.json"
if [ -f "$MCP_FILE" ] && grep -q '"docket"' "$MCP_FILE" 2>/dev/null; then
    echo ""
    echo "NOTE: docket is still in $MCP_FILE (old global config)."
    echo "The plugin now handles MCP registration. You can remove the docket entry from $MCP_FILE."
fi

echo ""
echo "=== Done ==="
echo "Binary:    $DOCKET_BIN"
echo "Plugin:    $PLUGIN_INSTALL"
echo "Dashboard: run /docket in any project (port is per-project)"
echo ""
echo "Next steps:"
echo "  1. Restart Claude Code (or /reload-plugins)"
echo "  2. Add the CLAUDE.md snippet to your projects (see plugin/README.md)"
echo "  3. Remove per-project .claude/agents/board-manager.md and .claude/skills/docket/ (plugin provides these now)"
