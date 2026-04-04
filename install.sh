#!/usr/bin/env bash
set -e

# docket installer — builds binary, installs Claude Code plugin
# Usage: bash /path/to/docket/install.sh [--dev]
#   --dev   Symlink plugin dir instead of copying (for development)

SOURCE_DIR="$(cd "$(dirname "$0")" && pwd)"
PLUGIN_SOURCE="$SOURCE_DIR/plugin"
PLUGIN_INSTALL="$HOME/.claude/plugins/marketplaces/local/docket"
SETTINGS_FILE="$HOME/.claude/settings.json"
PLUGINS_FILE="$HOME/.claude/plugins/installed_plugins.json"

DEV_MODE=false
for arg in "$@"; do
    case "$arg" in
        --dev) DEV_MODE=true ;;
    esac
done

if [ "$DEV_MODE" = true ]; then
    echo "=== docket installer (DEV MODE) ==="
else
    echo "=== docket installer ==="
fi
echo "Source:  $SOURCE_DIR"
echo "Plugin:  $PLUGIN_INSTALL"

# Check Go is available
if ! command -v go &> /dev/null; then
    echo "ERROR: Go is not installed. Install Go first: https://go.dev/dl/"
    exit 1
fi

# --- Step 1: Build binary ---

if [ "$DEV_MODE" = true ]; then
    echo "Building docket into plugin/docket.exe..."
    cd "$SOURCE_DIR"
    go build -ldflags="-s -w" -o plugin/docket.exe ./cmd/docket/
    echo "Built: $PLUGIN_SOURCE/docket.exe"
    ./plugin/docket.exe version
else
    echo "Building docket..."
    cd "$SOURCE_DIR"
    go build -ldflags="-s -w" -o docket.exe ./cmd/docket/
    echo "Built: $SOURCE_DIR/docket.exe"
    ./docket.exe version
fi

# --- Step 2: Install plugin ---

if [ "$DEV_MODE" = true ]; then
    # Symlink: plugin install dir -> source plugin dir
    # Remove existing (whether it's a dir or symlink)
    rm -rf "$PLUGIN_INSTALL"
    # Ensure parent dir exists
    mkdir -p "$(dirname "$PLUGIN_INSTALL")"
    # Create symlink
    ln -sfn "$PLUGIN_SOURCE" "$PLUGIN_INSTALL"
    echo "Symlinked $PLUGIN_INSTALL -> $PLUGIN_SOURCE"
    echo "Plugin file edits take effect on /reload-plugins (no rebuild needed)"
    echo "Go code changes: run 'bash dev-build.sh' then restart session"
else
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
fi

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

# Kill running docket so file locks don't block cleanup (Windows)
if tasklist 2>/dev/null | grep -qi "docket.exe"; then
    taskkill //F //IM docket.exe >>/dev/null 2>&1 && echo "Stopped running docket.exe" || true
fi

# Remove old global MCP config entry BEFORE deleting old binary dir
# (otherwise the entry points at a deleted path if script aborts later)
MCP_FILE="$HOME/.claude/.mcp.json"
if [ -f "$MCP_FILE" ] && grep -q '"docket"' "$MCP_FILE" 2>/dev/null; then
    if command -v jq &> /dev/null; then
        tmp=$(mktemp)
        jq 'del(.mcpServers.docket)' "$MCP_FILE" > "$tmp" && mv "$tmp" "$MCP_FILE"
        echo "Removed docket from $MCP_FILE (old global config)"
    elif command -v python3 &> /dev/null; then
        python3 -c "
import json, sys
with open(sys.argv[1], 'r') as f:
    data = json.load(f)
data.get('mcpServers', {}).pop('docket', None)
with open(sys.argv[1], 'w') as f:
    json.dump(data, f, indent=2)
" "$MCP_FILE"
        echo "Removed docket from $MCP_FILE (old global config)"
    else
        echo "WARNING: Cannot auto-remove docket from $MCP_FILE (no jq or python3)."
        echo "Please remove the docket entry manually — the plugin handles MCP registration now."
    fi
fi

# Remove old pre-plugin install location (best-effort — don't abort on failure)
OLD_INSTALL="$HOME/.local/share/docket"
if [ -d "$OLD_INSTALL" ]; then
    echo "Removing old installation at $OLD_INSTALL..."
    rm -rf "$OLD_INSTALL" || echo "WARNING: Could not fully remove $OLD_INSTALL (files may be locked). Remove manually."
fi

# Invalidate Claude Code's plugin cache so it re-caches from marketplace
CACHE_DIR="$HOME/.claude/plugins/cache/local/docket"
if [ -d "$CACHE_DIR" ]; then
    rm -rf "$CACHE_DIR"
    echo "Deleted plugin cache at $CACHE_DIR (will be re-created on next session)"
fi

echo ""
echo "=== Done ==="
echo "Plugin:    $PLUGIN_INSTALL"
if [ "$DEV_MODE" = true ]; then
    echo "Mode:      SYMLINK (dev)"
    echo "Binary:    $PLUGIN_SOURCE/docket.exe"
    echo ""
    echo "Dev workflow:"
    echo "  Plugin file changes → /reload-plugins"
    echo "  Go code changes     → bash dev-build.sh, then restart session"
else
    echo "Mode:      COPY (production)"
    echo "Binary:    $PLUGIN_INSTALL/docket.exe"
fi
echo "Dashboard: run /docket in any project"
echo ""
echo "Next steps:"
echo "  1. Restart Claude Code (or /reload-plugins)"
echo "  2. Run /docket-init in your projects to add the CLAUDE.md snippet"
