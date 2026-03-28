#!/usr/bin/env bash
set -e

# feat installer — builds binary, installs Claude Code plugin
# Usage: bash /path/to/feat/install.sh

SOURCE_DIR="$(cd "$(dirname "$0")" && pwd)"
INSTALL_DIR="$HOME/.local/share/feat"
FEAT_BIN="$INSTALL_DIR/feat.exe"
PLUGIN_INSTALL="$HOME/.claude/plugins/cache/local/feat/0.1.0"
SETTINGS_FILE="$HOME/.claude/settings.json"
PLUGINS_FILE="$HOME/.claude/plugins/installed_plugins.json"

echo "=== feat installer ==="
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

echo "Building feat..."
cd "$INSTALL_DIR"
go build -ldflags="-s -w" -o "$FEAT_BIN" ./cmd/feat/
echo "Built: $FEAT_BIN"
"$FEAT_BIN" version

# --- Step 2: Install plugin ---

echo "Installing plugin to $PLUGIN_INSTALL..."
rm -rf "$PLUGIN_INSTALL"
mkdir -p "$PLUGIN_INSTALL"
cp -r "$SOURCE_DIR/plugin/.claude-plugin" "$PLUGIN_INSTALL/"
cp -r "$SOURCE_DIR/plugin/agents" "$PLUGIN_INSTALL/"
cp -r "$SOURCE_DIR/plugin/skills" "$PLUGIN_INSTALL/"
cp "$SOURCE_DIR/plugin/README.md" "$PLUGIN_INSTALL/"

# Generate .mcp.json with absolute path to binary
FEAT_BIN_JSON=$(echo "$FEAT_BIN" | sed 's|\\|/|g')
cat > "$PLUGIN_INSTALL/.mcp.json" << MCPEOF
{
  "mcpServers": {
    "feat": {
      "command": "$FEAT_BIN_JSON",
      "args": ["serve"],
      "type": "stdio"
    }
  }
}
MCPEOF
echo "Generated $PLUGIN_INSTALL/.mcp.json"

# --- Step 3: Register plugin in settings.json ---

if [ -f "$SETTINGS_FILE" ]; then
    if grep -q '"feat@local"' "$SETTINGS_FILE" 2>/dev/null; then
        echo "feat@local already in settings.json — skipping"
    else
        # Insert feat@local into enabledPlugins object
        sed -i 's/"enabledPlugins": {/"enabledPlugins": {\n    "feat@local": true,/' "$SETTINGS_FILE"
        echo "Added feat@local to settings.json"
    fi
else
    echo "WARNING: $SETTINGS_FILE not found. Add \"feat@local\": true to enabledPlugins manually."
fi

# --- Step 4: Register plugin in installed_plugins.json ---

TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%S.000Z")
INSTALL_PATH_JSON=$(echo "$PLUGIN_INSTALL" | sed 's|/|\\\\|g')

if [ -f "$PLUGINS_FILE" ]; then
    if grep -q '"feat@local"' "$PLUGINS_FILE" 2>/dev/null; then
        echo "feat@local already in installed_plugins.json — skipping"
    else
        # Insert feat@local entry into plugins object
        sed -i "s/\"plugins\": {/\"plugins\": {\n    \"feat@local\": [\n      {\n        \"scope\": \"user\",\n        \"installPath\": \"$INSTALL_PATH_JSON\",\n        \"version\": \"0.1.0\",\n        \"installedAt\": \"$TIMESTAMP\",\n        \"lastUpdated\": \"$TIMESTAMP\"\n      }\n    ],/" "$PLUGINS_FILE"
        echo "Added feat@local to installed_plugins.json"
    fi
else
    echo "WARNING: $PLUGINS_FILE not found. Claude Code may not have been initialized yet."
fi

# --- Step 5: Clean up old global MCP config ---

MCP_FILE="$HOME/.claude/.mcp.json"
if [ -f "$MCP_FILE" ] && grep -q '"feat"' "$MCP_FILE" 2>/dev/null; then
    echo ""
    echo "NOTE: feat is still in $MCP_FILE (old global config)."
    echo "The plugin now handles MCP registration. You can remove the feat entry from $MCP_FILE."
fi

echo ""
echo "=== Done ==="
echo "Binary:    $FEAT_BIN"
echo "Plugin:    $PLUGIN_INSTALL"
echo "Dashboard: http://localhost:7890"
echo ""
echo "Next steps:"
echo "  1. Restart Claude Code (or /reload-plugins)"
echo "  2. Add the CLAUDE.md snippet to your projects (see plugin/README.md)"
echo "  3. Remove per-project .claude/agents/board-manager.md and .claude/skills/feat/ (plugin provides these now)"
