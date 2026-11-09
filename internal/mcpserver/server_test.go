package mcpserver

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"whatsapp-mcp/internal/store"
)

func TestMCPServer_ToolsRegistrationAndExecution(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "wa-mcp-server-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	msgStore, err := store.NewStore(tempDir)
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}
	defer msgStore.Close()

	// Seed some test data
	now := time.Now()
	chatJID := "987654321@s.whatsapp.net"
	_ = msgStore.StoreChat(chatJID, "Bob Dylan", now)
	_ = msgStore.StoreMessage(
		"MSG_A", chatJID, "987654321", "The answer my friend is blowin in the wind",
		now.Add(-1*time.Minute), false, "", "", "", nil, nil, nil, 0,
	)

	// Create MCP Server without live WhatsApp client (nil for offline testing)
	s := NewServer(nil, msgStore)

	ctx := context.Background()

	// 1. Test search_contacts
	res, err := s.handleSearchContacts(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "search_contacts",
			Arguments: map[string]interface{}{
				"query": "Dylan",
			},
		},
	})
	if err != nil {
		t.Fatalf("handleSearchContacts error: %v", err)
	}
	if len(res.Content) == 0 {
		t.Fatalf("Expected content in result")
	}
	textResult := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(textResult, "Bob Dylan") {
		t.Errorf("Expected Bob Dylan in result, got: %s", textResult)
	}

	// 2. Test list_messages
	res, err = s.handleListMessages(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "list_messages",
			Arguments: map[string]interface{}{
				"chat_jid": chatJID,
			},
		},
	})
	if err != nil {
		t.Fatalf("handleListMessages error: %v", err)
	}
	textResult = res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(textResult, "blowin in the wind") {
		t.Errorf("Expected message content in result, got: %s", textResult)
	}

	// 3. Test list_chats
	res, err = s.handleListChats(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "list_chats",
			Arguments: map[string]interface{}{
				"query": "Bob",
			},
		},
	})
	if err != nil {
		t.Fatalf("handleListChats error: %v", err)
	}
	textResult = res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(textResult, "Bob Dylan") {
		t.Errorf("Expected Bob Dylan in chats, got: %s", textResult)
	}

	// 4. Test get_chat
	res, err = s.handleGetChat(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "get_chat",
			Arguments: map[string]interface{}{
				"chat_jid": chatJID,
			},
		},
	})
	if err != nil {
		t.Fatalf("handleGetChat error: %v", err)
	}
	textResult = res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(textResult, chatJID) {
		t.Errorf("Expected chat JID in result, got: %s", textResult)
	}

	// 5. Test get_last_interaction
	res, err = s.handleGetLastInteraction(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "get_last_interaction",
			Arguments: map[string]interface{}{
				"jid": chatJID,
			},
		},
	})
	if err != nil {
		t.Fatalf("handleGetLastInteraction error: %v", err)
	}
	textResult = res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(textResult, "blowin in the wind") {
		t.Errorf("Expected interaction message in result, got: %s", textResult)
	}
}
