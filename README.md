# WhatsApp MCP Server (v2.0.0 — Pure Go & Shared Daemon)

An ultra-lightweight, high-performance [Model Context Protocol (MCP)](https://modelcontextprotocol.io) server for WhatsApp written in **pure Golang**.

Connect your **personal WhatsApp account** directly to LLM agents (**Antigravity / AGY**, **Claude Code**, **Claude Desktop**, **Cursor**, **OpenCode**) with **dual-transport support** (stdio or HTTP/SSE shared daemon).

![WhatsApp MCP](./example-use.png)

> **Security Warning (Lethal Trifecta):** Beware of prompt injection attacks from untrusted incoming messages. For sensitive actions like sending messages, review them or keep user approval active in your client.

---

## ⚡ Highlights

* **Pure Go & Single Binary:** No Python, no UV, no heavy runtimes. Consumes ~15MB RAM.
* **Dual-Transport Architecture:**
  * **HTTP/SSE Shared Daemon (`:3026`):** Exactly **ONE** process maintains the WhatsApp connection while serving multiple AI agents simultaneously (AGY + Cursor + Claude Desktop) without connection drops or SQLite database locking.
  * **stdio Mode:** Traditional direct stdio invocation when preferred.
* **1-Click Auto Configuration (`make setup` / `./install.sh`):** Instantly configures AGY, Claude Desktop, Cursor, Claude Code, and OpenCode with zero manual JSON editing.
* **Doctor Diagnostic Command (`make doctor`):** Instant visual status (`🟢 / 🟡 / 🔴`) checking WhatsApp session, SQLite store, FFmpeg installation, daemon health, and AI client configs.
* **Continuous Updates (`make update`):** One-command update script and GitHub Actions cron workflow to keep `whatsmeow` compatible with WhatsApp Web protocol changes.
* **Rich Media & Voice Notes:** Send & receive photos, videos, documents, and PTT voice notes with automatic Opus audio encoding via FFmpeg.

---

## 🚀 Quickstart (1-Click Setup)

```bash
git clone https://github.com/lharries/whatsapp-mcp.git
cd whatsapp-mcp

# 1. 1-click build and auto-configure all AI clients (AGY, Claude Desktop, Cursor, OpenCode)
make setup

# 2. Pair your WhatsApp account (scan QR code in terminal)
make login

# 3. Start the shared background daemon
make daemon
```

Now open **Antigravity**, **Cursor**, or **Claude Desktop** — WhatsApp tools are immediately ready to use!

---

## 🩺 System Diagnostic (`make doctor`)

Run anytime to check system health and configuration status:

```bash
make doctor
```

Output:
```
🩺 WhatsApp MCP Server Doctor (v2.0.0)

🟢 SQLite Store: OK in 'store' (12 chats, 280 messages indexed)
🟢 WhatsApp Session: Authenticated (Phone: 84901234567, Account: Chavis)
🟢 FFmpeg: Installed at /usr/local/bin/ffmpeg (Voice messages / Opus PTT enabled)
🟢 Daemon (: 3026): Running and reachable at http://localhost:3026/mcp

🔍 AI Client Configuration Status:
  🟢 Antigravity (AGY): Configured in ~/.gemini/config/mcp_config.json
  🟢 Claude Desktop: Configured in ~/Library/Application Support/Claude/claude_desktop_config.json
  🟢 Cursor: Configured in ~/.cursor/mcp.json
```

---

## 🛠️ Makefile Commands

| Command | Description |
| :--- | :--- |
| `make setup` | 1-Click build, auto-configure clients into SSE mode, and print instructions |
| `make login` | Pair your WhatsApp account interactively via terminal QR code |
| `make daemon` | Start the HTTP/SSE shared server daemon in the background (`:3026`) |
| `make stop` | Stop the background daemon |
| `make restart` | Restart the background daemon |
| `make status` | Check running daemon PID and WhatsApp session status |
| `make doctor` | Full health diagnostic for sessions, databases, and client configs |
| `make update` | Update `whatsmeow` to the latest version and verify build |
| `make test` | Run all unit tests (`go test -v ./...`) |

---

## 📦 Manual Client Configuration

If you prefer configuring clients manually, add the following snippet:

### HTTP/SSE Shared Mode (Recommended)
```json
{
  "mcpServers": {
    "whatsapp": {
      "url": "http://localhost:3026/mcp"
    }
  }
}
```

### stdio Mode (Direct binary)
```json
{
  "mcpServers": {
    "whatsapp": {
      "command": "/ABSOLUTE/PATH/TO/whatsapp-mcp/bin/whatsapp-mcp",
      "args": ["serve"]
    }
  }
}
```

---

## 🔧 Available MCP Tools (13 Tools)

| Tool | Description |
| :--- | :--- |
| `get_whatsapp_status` | Check WhatsApp connection health, phone number, and database statistics |
| `search_contacts` | Search WhatsApp contacts by name or phone number |
| `list_messages` | Retrieve messages with filters (time, sender, chat, query) and optional context |
| `list_chats` | List chats with metadata and latest message preview |
| `get_chat` | Get metadata for a specific chat by JID |
| `get_direct_chat_by_contact` | Get direct chat metadata by phone number |
| `get_contact_chats` | List all chats involving a specific contact |
| `get_last_interaction` | Get the most recent message with a contact |
| `get_message_context` | Retrieve messages immediately before and after a specific message |
| `send_message` | Send a text message to a phone number or group JID |
| `send_file` | Send an image, video, document, or raw audio file |
| `send_audio_message` | Send an audio file formatted as a WhatsApp voice memo (uses FFmpeg) |
| `download_media` | Download media attached to a message and get local file path |

---

## License

MIT
