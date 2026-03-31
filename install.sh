#!/usr/bin/env bash
set -e

# docket installer — builds binary, installs Claude Code plugin
# Usage: bash /path/to/docket/install.sh

SOURCE_DIR="$(cd "$(dirname "$0")" && pwd)"
PLUGIN_SOURCE="$SOURCE_DIR/plugin"
PLUGIN_INSTALL="$HOME/.claude/plugins/marketplaces/local/docket"
SETTINGS_FILE="$HOME/.claude/settings.json"
PLUGINS_FILE="$HOME/.claude/plugins/installed_plugins.json"

echo "=== docket installer ==="
echo "Source:  $SOURCE_DIR"
echo "Plugin:  $PLUGIN_INSTALL"

# Check Go is available
if ! command -v go &> /dev/null; then
    echo "ERROR: Go is not installed. Install Go first: https://go.dev/dl/"
    exit 1
fi

# --- Step 1: Build binary in source dir ---

echo "Building docket..."
cd "$SOURCE_DIR"
go build -ldflags="-s -w" -o docket.exe ./cmd/docket/
echo "Built: $SOURCE_DIR/docket.exe"
./docket.exe version

# --- Step 2: Install plugin (copy plugin dir + binary) ---

echo "Installing plugin to $PLUGIN_INSTALL..."
rm -rf "$PLUGIN_INSTALL"
mkdir -p "$PLUGIN_INSTALL"

# Copy plugin components
cp -r "$PLUGIN_SOURCE/.claude-plugin" "$PLUGIN_INSTALL/"
cp -r "$PLUGIN_SOURCE/agents" "$PLUGIN_INSTALL/"
cp -r "$PLUGIN_SOURCE/skills" "$PLUGIN_INSTALL/"
cp -r "$PLUGIN_SOURCE/hooks" "$PLUGIN_INSTALL/"
cp -r "$PLUGIN_SOURCE/scripts" "$PLUGIN_INSTALL/"
cp "$PLUGIN_SOURCE/.mcp.json" "$PLUGIN_INSTALL/"
cp "$PLUGIN_SOURCE/README.md" "$PLUGIN_INSTALL/"

# Copy binary into plugin dir (${CLAUDE_PLUGIN_ROOT}/docket.exe)
cp "$SOURCE_DIR/docket.exe" "$PLUGIN_INSTALL/docket.exe"
echo "Binary installed to $PLUGIN_INSTALL/docket.exe"

# --- Step 3: Register plugin in settings.json ---

if [ -f "$SETTINGS_FILE" ]; then
    if grep -q '"docket@local"' "$SETTINGS_FILE" 2>/dev/null; then
        echo "docket@local already in settings.json — skipping"
    elif command -v jq &> /dev/null; then
        # Use jq if available
        tmp=$(mktemp)
        jq '.enabledPlugins["docket@local"] = true' "$SETTINGS_FILE" > "$tmp" && mv "$tmp" "$SETTINGS_FILE"
        echo "Added docket@local to settings.json (via jq)"
    elif command -v python3 &> /dev/null; then
        python3 -c "
import json, sys
with open(sys.argv[1], 'r') as f:
    data = json.load(f)
data.setdefault('enabledPlugins', {})['docket@local'] = True
with open(sys.argv[1], 'w') as f:
    json.dump(data, f, indent=2)
" "$SETTINGS_FILE"
        echo "Added docket@local to settings.json (via python3)"
    else
        echo "WARNING: Neither jq nor python3 found. Add manually to $SETTINGS_FILE:"
        echo '  "enabledPlugins": { "docket@local": true }'
    fi
else
    echo "WARNING: $SETTINGS_FILE not found. Add \"docket@local\": true to enabledPlugins manually."
fi

# --- Step 4: Register plugin in installed_plugins.json ---

TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%S.000Z")
# Convert install path for JSON
if command -v cygpath &> /dev/null; then
    INSTALL_PATH_JSON=$(cygpath -w "$PLUGIN_INSTALL")
else
    INSTALL_PATH_JSON="$PLUGIN_INSTALL"
fi

if [ -f "$PLUGINS_FILE" ]; then
    if grep -q '"docket@local"' "$PLUGINS_FILE" 2>/dev/null; then
        echo "docket@local already in installed_plugins.json — skipping"
    elif command -v jq &> /dev/null; then
        tmp=$(mktemp)
        jq --arg path "$INSTALL_PATH_JSON" --arg ts "$TIMESTAMP" \
            '.plugins["docket@local"] = [{"scope": "user", "installPath": $path, "version": "0.1.0", "installedAt": $ts, "lastUpdated": $ts}]' \
            "$PLUGINS_FILE" > "$tmp" && mv "$tmp" "$PLUGINS_FILE"
        echo "Added docket@local to installed_plugins.json (via jq)"
    elif command -v python3 &> /dev/null; then
        python3 -c "
import json, sys
path, ts, pf = sys.argv[1], sys.argv[2], sys.argv[3]
with open(pf, 'r') as f:
    data = json.load(f)
data['plugins']['docket@local'] = [{'scope': 'user', 'installPath': path, 'version': '0.1.0', 'installedAt': ts, 'lastUpdated': ts}]
with open(pf, 'w') as f:
    json.dump(data, f, indent=2)
" "$INSTALL_PATH_JSON" "$TIMESTAMP" "$PLUGINS_FILE"
        echo "Added docket@local to installed_plugins.json (via python3)"
    else
        echo "WARNING: Neither jq nor python3 found. Register plugin manually in $PLUGINS_FILE"
    fi
else
    echo "WARNING: $PLUGINS_FILE not found. Claude Code may not have been initialized yet."
fi

# --- Step 5: Clean up old installations ---

OLD_INSTALL="$HOME/.local/share/docket"
if [ -d "$OLD_INSTALL" ]; then
    echo ""
    echo "NOTE: Old installation found at $OLD_INSTALL"
    echo "The binary now lives inside the plugin dir. You can safely remove it:"
    echo "  rm -rf $OLD_INSTALL"
fi

MCP_FILE="$HOME/.claude/.mcp.json"
if [ -f "$MCP_FILE" ] && grep -q '"docket"' "$MCP_FILE" 2>/dev/null; then
    echo ""
    echo "NOTE: docket is still in $MCP_FILE (old global config)."
    echo "The plugin now handles MCP registration. You can remove the docket entry from $MCP_FILE."
fi

echo ""
echo "=== Done ==="
echo "Plugin:    $PLUGIN_INSTALL"
echo "Binary:    $PLUGIN_INSTALL/docket.exe"
echo "Dashboard: run /docket in any project"
echo ""
echo "Next steps:"
echo "  1. Restart Claude Code (or /reload-plugins)"
echo "  2. Run /docket-init in your projects to add the CLAUDE.md snippet"
