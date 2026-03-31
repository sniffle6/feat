#!/usr/bin/env bash
# Ensures the docket binary exists in CLAUDE_PLUGIN_ROOT.
# Downloads from GitHub releases if missing. Called by SessionStart hook.
set -e

REPO="sniffle6/claude-docket"
BINARY="$CLAUDE_PLUGIN_ROOT/docket.exe"

# Already installed — nothing to do
if [ -x "$BINARY" ] || [ -f "$BINARY" ]; then
    exit 0
fi

# Detect platform
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$OS" in
    mingw*|msys*|cygwin*) OS="windows" ;;
    linux)                OS="linux" ;;
    darwin)               OS="darwin" ;;
    *)
        echo "Unsupported OS: $OS" >&2
        exit 1
        ;;
esac

case "$ARCH" in
    x86_64|amd64)  ARCH="amd64" ;;
    aarch64|arm64)  ARCH="arm64" ;;
    *)
        echo "Unsupported architecture: $ARCH" >&2
        exit 1
        ;;
esac

# Build asset name
if [ "$OS" = "windows" ]; then
    ASSET="docket-${OS}-${ARCH}.exe"
else
    ASSET="docket-${OS}-${ARCH}"
fi

# Get latest release download URL
DOWNLOAD_URL="https://github.com/$REPO/releases/latest/download/$ASSET"

echo "docket binary not found. Downloading from $DOWNLOAD_URL ..." >&2

# Download
if command -v curl &> /dev/null; then
    curl -fSL -o "$BINARY" "$DOWNLOAD_URL"
elif command -v wget &> /dev/null; then
    wget -q -O "$BINARY" "$DOWNLOAD_URL"
else
    echo "ERROR: Neither curl nor wget found. Download manually from:" >&2
    echo "  $DOWNLOAD_URL" >&2
    echo "Place at: $BINARY" >&2
    exit 1
fi

chmod +x "$BINARY" 2>/dev/null || true
echo "docket binary installed to $BINARY" >&2
