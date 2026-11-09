# 1craft WhatsApp MCP

[![Release](https://img.shields.io/github/v/release/chavisnguyen/1craft-wa-mcp?style=flat-square&color=blue)](https://github.com/chavisnguyen/1craft-wa-mcp/releases)
[![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat-square&logo=go)](https://golang.org)
[![MCP](https://img.shields.io/badge/MCP-Compatible-blue?style=flat-square)](https://modelcontextprotocol.io)
[![RAM](https://img.shields.io/badge/RAM-~15MB-brightgreen?style=flat-square)](README.md)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg?style=flat-square)](LICENSE)

Lightweight WhatsApp [Model Context Protocol (MCP)](https://modelcontextprotocol.io) server written in pure Go. Connects AI agents (**Antigravity**, **Claude Desktop & CLI**, **Kiro**) directly to personal WhatsApp accounts.

---

## Features

- **Pure Go (~15MB RAM):** Single binary, zero Python/node dependencies.
- **Shared Daemon (`:3026`):** One background process maintains the WhatsApp connection while serving multiple AI clients simultaneously without session drops or SQLite locks.
- **1-Click Auto Config:** Automatically detects and configures AGY, Claude Desktop, Claude Code (CLI), and Kiro.
- **Auto Updates (`make update`):** Keeps `whatsmeow` up-to-date with WhatsApp Web protocol changes.
- **Voice & Media Support:** Handles photos, videos, documents, and PTT voice notes (via FFmpeg).

---

## Quickstart

```bash
git clone https://github.com/chavisnguyen/1craft-wa-mcp.git
cd 1craft-wa-mcp

make setup   # Build binary and auto-configure all AI clients
make login   # Scan QR code in terminal (one-time)
make daemon  # Start background shared server on :3026
make doctor  # Verify connection and configuration health
```

---

## Commands

| Command | Action |
| :--- | :--- |
| `make setup` | Build and auto-configure all detected AI clients |
| `make login` | Pair WhatsApp account via terminal QR code |
| `make daemon` | Start shared background daemon on `http://localhost:3026/mcp` |
| `make stop` | Stop background daemon |
| `make restart` | Restart daemon |
| `make status` | Check daemon PID and WhatsApp session status |
| `make doctor` | Run full system diagnostic (`🟢/🟡/🔴`) |
| `make update` | Update `whatsmeow` to latest and rebuild |
| `make test` | Run unit tests |
| `make clean` | Clean build artifacts, PID, and log files |
| `make help` | Show available commands |

---

## Client Config (Manual)

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
      "command": "/ABSOLUTE/PATH/TO/1craft-wa-mcp/bin/whatsapp-mcp",
      "args": ["serve"]
    }
  }
}
```

---

## MCP Tools (13)

| Tool | Description |
| :--- | :--- |
| `get_whatsapp_status` | Check connection health, account info, and database stats |
| `search_contacts` | Search contacts by name or phone number |
| `list_messages` | Query messages with filters (time, sender, chat, query, context) |
| `list_chats` | List recent conversations and group chats |
| `get_chat` | Get chat metadata by JID |
| `get_direct_chat_by_contact` | Find chat by phone number |
| `get_contact_chats` | List all chats involving a specific contact |
| `get_last_interaction` | Get most recent message with a contact |
| `get_message_context` | Expand context before and after a specific message |
| `send_message` | Send text message to phone number or group JID |
| `send_file` | Send image, video, document, or audio file |
| `send_audio_message` | Send audio file formatted as WhatsApp voice memo |
| `download_media` | Download encrypted media attachment to local folder |

---

## License

MIT
