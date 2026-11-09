#!/bin/bash
set -euo pipefail

# Install WhatsApp MCP Server into AGY, Claude Desktop, Claude Code (CLI), and Kiro
# Usage:
#   ./install.sh sse    (Recommended for multiple agents: connects to http://localhost:3026/mcp)
#   ./install.sh stdio  (Standalone mode: spawns binary on demand)

MODE="${1:-sse}"
TOOL_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_PATH="$TOOL_DIR/bin/whatsapp-mcp"
SSE_URL="http://localhost:3026/mcp"
SERVER_NAME="whatsapp"

if [ ! -f "$BIN_PATH" ]; then
    echo "⚙️ Building binary first..."
    (cd "$TOOL_DIR" && go build -o "$BIN_PATH" ./cmd/whatsapp-mcp)
fi

echo "🔧 Configuring $SERVER_NAME MCP Server in [$MODE] mode..."
if [ "$MODE" = "sse" ]; then
    echo "   SSE URL: $SSE_URL (Run 'make daemon' to start server)"
else
    echo "   Binary: $BIN_PATH (stdio mode)"
fi
echo ""

# --- 1. AGY: ~/.gemini/config/mcp_config.json ---
AGY_CONFIG="$HOME/.gemini/config/mcp_config.json"
echo "📝 Updating AGY config: $AGY_CONFIG"

python3 - << PYEOF
import json, os

path = os.path.expanduser("~/.gemini/config/mcp_config.json")
if os.path.exists(path):
    with open(path) as f:
        try:
            data = json.load(f)
        except Exception:
            data = {}
else:
    os.makedirs(os.path.dirname(path), exist_ok=True)
    data = {}

data.setdefault("mcpServers", {})
if "$MODE" == "sse":
    data["mcpServers"]["$SERVER_NAME"] = {
        "disabled": False,
        "serverUrl": "$SSE_URL"
    }
else:
    data["mcpServers"]["$SERVER_NAME"] = {
        "command": "$BIN_PATH"
    }

with open(path, "w") as f:
    json.dump(data, f, indent=2)
    f.write("\n")

print("   ✅ AGY config updated")
PYEOF

# --- 2. Claude Desktop ---
CLAUDE_DIR="$HOME/Library/Application Support/Claude"
CLAUDE_CONFIG="$CLAUDE_DIR/claude_desktop_config.json"
echo "📝 Updating Claude Desktop config: $CLAUDE_CONFIG"

python3 - << PYEOF
import json, os

path = os.path.expanduser("~/Library/Application Support/Claude/claude_desktop_config.json")
if os.path.exists(path):
    with open(path) as f:
        try:
            data = json.load(f)
        except Exception:
            data = {}
else:
    os.makedirs(os.path.dirname(path), exist_ok=True)
    data = {}

data.setdefault("mcpServers", {})
if "$MODE" == "sse":
    data["mcpServers"]["$SERVER_NAME"] = {
        "endpoint": "$SSE_URL"
    }
else:
    data["mcpServers"]["$SERVER_NAME"] = {
        "command": "$BIN_PATH",
        "args": ["serve"]
    }

with open(path, "w") as f:
    json.dump(data, f, indent=2)
    f.write("\n")

print("   ✅ Claude Desktop config updated")
PYEOF

# --- 3. Claude Code (CLI): ~/.claude.json ---
CLAUDE_CLI_CONFIG="$HOME/.claude.json"
echo "📝 Updating Claude Code (CLI) config: $CLAUDE_CLI_CONFIG"

python3 - << PYEOF
import json, os

path = os.path.expanduser("~/.claude.json")
if os.path.exists(path):
    with open(path) as f:
        try:
            data = json.load(f)
        except Exception:
            data = {}
else:
    data = {}

data.setdefault("mcpServers", {})
if "$MODE" == "sse":
    data["mcpServers"]["$SERVER_NAME"] = {
        "type": "http",
        "url": "$SSE_URL"
    }
else:
    data["mcpServers"]["$SERVER_NAME"] = {
        "command": "$BIN_PATH",
        "args": ["serve"]
    }

with open(path, "w") as f:
    json.dump(data, f, indent=2)
    f.write("\n")

print("   ✅ Claude Code (CLI) config updated")
PYEOF

# --- 4. Kiro: ~/.kiro/settings/mcp.json ---
KIRO_CONFIG="$HOME/.kiro/settings/mcp.json"
echo "📝 Updating Kiro config: $KIRO_CONFIG"

python3 - << PYEOF
import json, os

path = os.path.expanduser("~/.kiro/settings/mcp.json")
if os.path.exists(path):
    with open(path) as f:
        try:
            data = json.load(f)
        except Exception:
            data = {}
else:
    os.makedirs(os.path.dirname(path), exist_ok=True)
    data = {}

data.setdefault("mcpServers", {})
if "$MODE" == "sse":
    data["mcpServers"]["$SERVER_NAME"] = {
        "url": "$SSE_URL",
        "disabled": False
    }
else:
    data["mcpServers"]["$SERVER_NAME"] = {
        "command": "$BIN_PATH",
        "args": ["serve"],
        "disabled": False
    }

with open(path, "w") as f:
    json.dump(data, f, indent=2)
    f.write("\n")

print("   ✅ Kiro config updated")
PYEOF

echo ""
echo "🎉 Setup complete! Supported AI clients configured for $SERVER_NAME MCP Server ($MODE mode)."
if [ "$MODE" = "sse" ]; then
    echo "👉 Run 'make daemon' to start the shared WhatsApp server daemon."
fi
