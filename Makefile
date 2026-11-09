# ========================================================
# 1craft WhatsApp MCP Server (Pure Go Edition)
# ========================================================

BIN_DIR := bin
BIN_NAME := whatsapp-mcp
BIN_PATH := $(BIN_DIR)/$(BIN_NAME)
PID_FILE := daemon.pid
LOG_FILE := daemon.log
PORT ?= 3026

.PHONY: all build install setup install-sse install-stdio daemon stop restart status doctor login run update test clean help

all: build

## setup: 1-click build and auto-configure supported AI clients (AGY, Claude Desktop, Claude Code, Kiro)
setup: build
	@./install.sh sse
	@echo ""
	@echo "========================================================"
	@echo "🎉 SETUP COMPLETE!"
	@echo "========================================================"
	@echo "1. Scan QR code to log in: make login"
	@echo "2. Start background daemon: make daemon"
	@echo "3. Run system diagnostics: make doctor"
	@echo "========================================================"

## build: Compile pure Go binary
build:
	@mkdir -p $(BIN_DIR)
	@go build -o $(BIN_PATH) ./cmd/whatsapp-mcp
	@echo "✅ Build success: $(BIN_PATH)"

## install-sse: Configure MCP server in HTTP/SSE daemon mode (Recommended for multiple AI agents)
install-sse: build
	@./install.sh sse

## install-stdio: Configure MCP server in stdio mode (Standalone binary)
install-stdio: build
	@./install.sh stdio

## install: Install binary to $GOPATH/bin
install:
	go install ./cmd/whatsapp-mcp

## daemon: Start background HTTP/SSE daemon on port $(PORT) (for concurrent agents)
daemon: build
	@if [ -f $(PID_FILE) ] && kill -0 $$(cat $(PID_FILE)) 2>/dev/null; then \
		echo "⚠️ Daemon already running (PID: $$(cat $(PID_FILE))) on port $(PORT)"; \
	else \
		echo "🚀 Starting WhatsApp HTTP/SSE Daemon on http://localhost:$(PORT)/mcp ..."; \
		nohup $(BIN_PATH) -sse -port $(PORT) > $(LOG_FILE) 2>&1 & echo $$! > $(PID_FILE); \
		sleep 1; \
		if kill -0 $$(cat $(PID_FILE)) 2>/dev/null; then \
			echo "✅ Daemon started successfully (PID: $$(cat $(PID_FILE)))"; \
			echo "   Endpoint: http://localhost:$(PORT)/mcp"; \
			echo "   Health:   http://localhost:$(PORT)/health"; \
		else \
			echo "❌ Failed to start daemon. Check $(LOG_FILE)"; \
			cat $(LOG_FILE); \
		fi \
	fi

## stop: Stop background daemon process
stop:
	@if [ -f $(PID_FILE) ]; then \
		echo "🛑 Stopping daemon (PID: $$(cat $(PID_FILE)))..."; \
		kill $$(cat $(PID_FILE)) 2>/dev/null || true; \
		rm -f $(PID_FILE); \
		echo "✅ Daemon stopped."; \
	else \
		pkill -f "$(BIN_PATH) -sse" 2>/dev/null || true; \
		echo "ℹ️ No PID file found, checked running processes."; \
	fi

## restart: Restart daemon
restart: stop daemon

## status: Check Daemon and WhatsApp session status
status: build
	@echo "--- WhatsApp Daemon Status ---"
	@if [ -f $(PID_FILE) ] && kill -0 $$(cat $(PID_FILE)) 2>/dev/null; then \
		echo "🟢 Daemon is RUNNING (PID: $$(cat $(PID_FILE))) on http://localhost:$(PORT)/mcp"; \
	else \
		echo "⚪ Daemon is NOT running (Run 'make daemon' to start)"; \
	fi
	@echo ""
	@echo "--- WhatsApp Session Status ---"
	@$(BIN_PATH) status

## doctor: Run full system diagnostics
doctor: build
	@$(BIN_PATH) -port $(PORT) doctor

## login: Scan QR code in terminal to link WhatsApp account
login: build
	@$(BIN_PATH) login

## run: Run MCP stdio server directly in terminal
run: build
	@$(BIN_PATH) serve

## update: Automatically update whatsmeow to latest version
update:
	@./scripts/update-deps.sh

## test: Run all unit tests
test:
	@go test -v ./...

## clean: Clean bin directory, PID, and log files
clean:
	@rm -rf $(BIN_DIR) $(PID_FILE) $(LOG_FILE)
	@echo "✅ Cleaned."

## help: Show this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed -e 's/## //' | awk -F': ' '{printf "  %-15s %s\n", $$1, $$2}'
