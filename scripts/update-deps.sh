#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

cd "${ROOT_DIR}"

echo "==> Current whatsmeow version:"
go list -m go.mau.fi/whatsmeow || true

echo "==> Updating whatsmeow to latest version..."
go get -u go.mau.fi/whatsmeow@latest

echo "==> Running go mod tidy..."
go mod tidy

echo "==> Building whatsapp-mcp to verify compatibility..."
mkdir -p bin
go build -o bin/whatsapp-mcp ./cmd/whatsapp-mcp

echo "==> Updated whatsmeow version:"
go list -m go.mau.fi/whatsmeow

echo "==> Build successful! Binary is ready at bin/whatsapp-mcp"
