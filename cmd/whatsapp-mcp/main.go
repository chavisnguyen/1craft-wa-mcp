package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/mark3labs/mcp-go/server"

	"whatsapp-mcp/internal/mcpserver"
	"whatsapp-mcp/internal/store"
	"whatsapp-mcp/internal/whatsapp"
)

const version = "2.0.0"

func main() {
	defaultPort := "3026"
	if p := os.Getenv("PORT"); p != "" {
		defaultPort = p
	}
	defaultHost := os.Getenv("HOST")

	sseMode := flag.Bool("sse", false, "Run in HTTP/SSE Server daemon mode (shared for multiple agents)")
	port := flag.String("port", defaultPort, "Port to bind HTTP/SSE server (e.g. 3026)")
	host := flag.String("host", defaultHost, "Specific host to bind (default empty: binds dual-stack 127.0.0.1 and [::1])")
	storeDir := flag.String("store-dir", "store", "Directory to store message database and session credentials")
	verbose := flag.Bool("verbose", false, "Enable verbose WhatsApp client logs")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "WhatsApp Model Context Protocol (MCP) Server (v%s)\n\n", version)
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  whatsapp-mcp [flags] [command]\n\n")
		fmt.Fprintf(os.Stderr, "Commands:\n")
		fmt.Fprintf(os.Stderr, "  serve       Run MCP server over stdio (default if -sse is omitted)\n")
		fmt.Fprintf(os.Stderr, "  daemon      Run in HTTP/SSE Server daemon mode on port %s\n", defaultPort)
		fmt.Fprintf(os.Stderr, "  login       Pair device interactively by scanning QR in terminal\n")
		fmt.Fprintf(os.Stderr, "  status      Check current login & session status\n")
		fmt.Fprintf(os.Stderr, "  doctor      Diagnose WhatsApp session, databases, daemon, and client configs\n")
		fmt.Fprintf(os.Stderr, "  version     Show application version\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	cmd := "serve"
	if flag.NArg() > 0 {
		cmd = flag.Arg(0)
	}

	if *sseMode || cmd == "daemon" {
		runDaemon(*port, *host, *storeDir, *verbose)
		return
	}

	switch cmd {
	case "version":
		fmt.Printf("whatsapp-mcp v%s\n", version)
		return

	case "login":
		runLogin(*storeDir, *verbose)
		return

	case "status":
		runStatus(*storeDir)
		return

	case "doctor":
		runDoctor(*port, *storeDir)
		return

	case "serve":
		runServe(*storeDir, *verbose)
		return

	default:
		flag.Usage()
		os.Exit(1)
	}
}

func runLogin(storeDir string, verbose bool) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nCancelled by user.")
		cancel()
	}()

	msgStore, err := store.NewStore(storeDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening message store: %v\n", err)
		os.Exit(1)
	}
	defer msgStore.Close()

	client, err := whatsapp.NewClient(ctx, storeDir, msgStore, verbose)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing WhatsApp client: %v\n", err)
		os.Exit(1)
	}
	defer client.Disconnect()

	if client.IsLoggedIn() {
		status := client.GetStatus()
		fmt.Printf("✓ Device is already authenticated with WhatsApp! (Account: %s, Phone: %s)\n", status.PushName, status.Phone)
		return
	}

	fmt.Println("Generating pairing QR code...")
	if err := client.Connect(ctx, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "Login error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✓ Login complete! You can now run the MCP server with Claude Desktop, Cursor, or AGY.")
}

func runStatus(storeDir string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	msgStore, err := store.NewStore(storeDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening store: %v\n", err)
		os.Exit(1)
	}
	defer msgStore.Close()

	client, err := whatsapp.NewClient(ctx, storeDir, msgStore, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error checking status: %v\n", err)
		os.Exit(1)
	}
	defer client.Disconnect()

	if client.IsLoggedIn() {
		status := client.GetStatus()
		fmt.Printf("Status: Authenticated (Phone: %s, Account: %s)\n", status.Phone, status.PushName)
	} else {
		fmt.Println("Status: Not authenticated. Run 'whatsapp-mcp login' to scan QR code.")
	}
}

func runServe(storeDir string, verbose bool) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	msgStore, err := store.NewStore(storeDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal: failed to initialize message store: %v\n", err)
		os.Exit(1)
	}
	defer msgStore.Close()

	client, err := whatsapp.NewClient(ctx, storeDir, msgStore, verbose)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal: failed to initialize WhatsApp client: %v\n", err)
		os.Exit(1)
	}
	defer client.Disconnect()

	// Connect in background so MCP stdio server starts immediately
	go func() {
		// Output QR code to stderr if device needs pairing so stdout (JSON-RPC) is clean
		if err := client.Connect(ctx, os.Stderr); err != nil {
			fmt.Fprintf(os.Stderr, "[whatsapp-mcp] Connection error: %v\n", err)
		}
	}()

	srv := mcpserver.NewServer(client, msgStore)
	if err := srv.ServeStdio(); err != nil {
		fmt.Fprintf(os.Stderr, "[whatsapp-mcp] Server error: %v\n", err)
		os.Exit(1)
	}
}

func runDaemon(port, host, storeDir string, verbose bool) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	msgStore, err := store.NewStore(storeDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal: failed to initialize message store: %v\n", err)
		os.Exit(1)
	}
	defer msgStore.Close()

	client, err := whatsapp.NewClient(ctx, storeDir, msgStore, verbose)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal: failed to initialize WhatsApp client: %v\n", err)
		os.Exit(1)
	}
	defer client.Disconnect()

	// Connect to WhatsApp in background
	go func() {
		if err := client.Connect(ctx, os.Stderr); err != nil {
			fmt.Fprintf(os.Stderr, "[whatsapp-mcp] Background connection error: %v\n", err)
		}
	}()

	srv := mcpserver.NewServer(client, msgStore)
	streamableServer := server.NewStreamableHTTPServer(srv.GetMCPServer(),
		server.WithStreamableHTTPCORS(),
	)

	mux := http.NewServeMux()
	mux.Handle("/mcp", streamableServer)
	mux.Handle("/sse", streamableServer)
	mux.HandleFunc("/.well-known/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		status := client.GetStatus()
		chatCount, msgCount, _ := msgStore.GetStats()
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":           "ok",
			"version":          version,
			"whatsapp_online":  status.Connected,
			"whatsapp_auth":    status.LoggedIn,
			"phone":            status.Phone,
			"indexed_chats":    chatCount,
			"indexed_messages": msgCount,
		})
	})

	var listeners []net.Listener
	if host != "" {
		ln, err := net.Listen("tcp", net.JoinHostPort(host, port))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to bind %s:%s: %v\n", host, port, err)
			os.Exit(1)
		}
		listeners = append(listeners, ln)
	} else {
		// Dual-stack loopback binding: IPv4 and IPv6
		ln4, err4 := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", port))
		if err4 != nil {
			fmt.Fprintf(os.Stderr, "Failed to bind IPv4 loopback 127.0.0.1:%s: %v\n", port, err4)
		} else {
			listeners = append(listeners, ln4)
		}

		ln6, err6 := net.Listen("tcp6", net.JoinHostPort("::1", port))
		if err6 != nil {
			if err4 != nil {
				fmt.Fprintf(os.Stderr, "Failed to bind IPv6 loopback [::1]:%s: %v\n", port, err6)
			}
		} else {
			listeners = append(listeners, ln6)
		}

		if len(listeners) == 0 {
			fmt.Fprintf(os.Stderr, "Error: failed to bind any loopback interface on port %s\n", port)
			os.Exit(1)
		}
	}

	httpServer := &http.Server{
		Handler: mux,
	}

	fmt.Printf("🚀 Starting WhatsApp MCP Server (HTTP/SSE Shared Daemon) on port %s\n", port)
	for _, l := range listeners {
		fmt.Printf("   Listening on http://%s/mcp\n", l.Addr())
	}
	fmt.Printf("   Endpoints supported: /mcp, /sse, /health\n")

	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	for _, l := range listeners {
		go func(ln net.Listener) {
			if err := httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
				fmt.Fprintf(os.Stderr, "HTTP Server error on %s: %v\n", ln.Addr(), err)
			}
		}(l)
	}

	<-stopChan
	fmt.Println("\n🛑 Shutting down WhatsApp MCP Server...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer shutdownCancel()
	_ = httpServer.Shutdown(shutdownCtx)
	fmt.Println("👋 Goodbye!")
}

func runDoctor(port, storeDir string) {
	fmt.Printf("🩺 WhatsApp MCP Server Doctor (v%s)\n\n", version)

	// 1. WhatsApp Authentication
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	msgStore, err := store.NewStore(storeDir)
	if err != nil {
		fmt.Printf("🔴 SQLite Store: Error opening store in '%s': %v\n", storeDir, err)
	} else {
		chatCount, msgCount, _ := msgStore.GetStats()
		fmt.Printf("🟢 SQLite Store: OK in '%s' (%d chats, %d messages indexed)\n", storeDir, chatCount, msgCount)
		client, clientErr := whatsapp.NewClient(ctx, storeDir, msgStore, false)
		if clientErr == nil {
			if client.IsLoggedIn() {
				status := client.GetStatus()
				fmt.Printf("🟢 WhatsApp Session: Authenticated (Phone: %s, Account: %s)\n", status.Phone, status.PushName)
			} else {
				fmt.Printf("🟡 WhatsApp Session: Not authenticated yet. Run 'whatsapp-mcp login' to scan QR code.\n")
			}
			client.Disconnect()
		}
		msgStore.Close()
	}

	// 2. FFmpeg check
	if ffmpegPath, err := exec.LookPath("ffmpeg"); err == nil {
		fmt.Printf("🟢 FFmpeg: Installed at %s (Voice messages / Opus PTT enabled)\n", ffmpegPath)
	} else {
		fmt.Printf("🟡 FFmpeg: Not found in PATH (Voice audio will fall back to send_file)\n")
	}

	// 3. HTTP/SSE Daemon status
	daemonURL := fmt.Sprintf("http://127.0.0.1:%s/health", port)
	httpClient := &http.Client{Timeout: 1200 * time.Millisecond}
	resp, err := httpClient.Get(daemonURL)
	if err != nil {
		fmt.Printf("⚪ Daemon (: %s): Not running (Run 'make daemon' to start background shared server)\n", port)
	} else {
		_ = resp.Body.Close()
		fmt.Printf("🟢 Daemon (: %s): Running and reachable at http://localhost:%s/mcp\n", port, port)
	}

	// 4. AI Client Configuration Checks
	home, _ := os.UserHomeDir()
	fmt.Println("\n🔍 AI Client Configuration Status:")

	// AGY
	checkClientConfig("Antigravity (AGY)", filepath.Join(home, ".gemini/config/mcp_config.json"))

	// Claude Desktop
	checkClientConfig("Claude Desktop", filepath.Join(home, "Library/Application Support/Claude/claude_desktop_config.json"))

	// Cursor
	checkClientConfig("Cursor", filepath.Join(home, ".cursor/mcp.json"))

	// Claude Code
	checkClientConfig("Claude Code", filepath.Join(home, ".claude/claude_code_config.json"))

	// OpenCode
	checkClientConfig("OpenCode", filepath.Join(home, ".config/opencode/opencode.json"))

	fmt.Printf("\n✨ Doctor check complete! Run './install.sh' to configure clients automatically.\n")
}

func checkClientConfig(name, path string) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		fmt.Printf("  ⚪ %s: Config file not found at %s\n", name, path)
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Printf("  🔴 %s: Cannot read %s: %v\n", name, path, err)
		return
	}

	content := string(data)
	if strings.Contains(content, "whatsapp") || strings.Contains(content, "1craft-wa-mcp") {
		fmt.Printf("  🟢 %s: Configured in %s\n", name, path)
	} else {
		fmt.Printf("  🟡 %s: File exists but whatsapp server is not configured yet\n", name)
	}
}
