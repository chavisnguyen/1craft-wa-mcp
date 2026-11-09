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

## setup: 1-click build và tự động cấu hình vào tất cả AI clients (AGY, Claude Desktop, Cursor, OpenCode)
setup: build
	@./install.sh sse
	@echo ""
	@echo "========================================================"
	@echo "🎉 SETUP HOÀN TẤT!"
	@echo "========================================================"
	@echo "1. Quét QR đăng nhập: make login"
	@echo "2. Khởi động daemon nền: make daemon"
	@echo "3. Kiểm tra chẩn đoán hệ thống: make doctor"
	@echo "========================================================"

## build: Biên dịch binary thuần Golang
build:
	@mkdir -p $(BIN_DIR)
	@go build -o $(BIN_PATH) ./cmd/whatsapp-mcp
	@echo "✅ Build success: $(BIN_PATH)"

## install-sse: Cấu hình MCP server chế độ HTTP/SSE daemon (Khuyến nghị cho nhiều AI Agent)
install-sse: build
	@./install.sh sse

## install-stdio: Cấu hình MCP server chế độ stdio (Standalone binary)
install-stdio: build
	@./install.sh stdio

## install: Cài đặt binary vào $GOPATH/bin
install:
	go install ./cmd/whatsapp-mcp

## daemon: Chạy HTTP/SSE Daemon nền trên port $(PORT) (dành cho nhiều Agent dùng đồng thời)
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

## stop: Dừng tiến trình daemon nền
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

## restart: Khởi động lại daemon
restart: stop daemon

## status: Kiểm tra trạng thái Daemon và WhatsApp
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

## doctor: Chẩn đoán toàn diện sức khỏe hệ thống
doctor: build
	@$(BIN_PATH) -port $(PORT) doctor

## login: Quét mã QR liên kết tài khoản WhatsApp trên terminal
login: build
	@$(BIN_PATH) login

## run: Chạy MCP stdio server trực tiếp trên terminal
run: build
	@$(BIN_PATH) serve

## update: Tự động cập nhật whatsmeow lên bản mới nhất
update:
	@./scripts/update-deps.sh

## test: Chạy toàn bộ Unit Tests
test:
	@go test -v ./...

## clean: Dọn dẹp thư mục bin và file log/pid
clean:
	@rm -rf $(BIN_DIR) $(PID_FILE) $(LOG_FILE)
	@echo "✅ Cleaned."
