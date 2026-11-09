package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"whatsapp-mcp/internal/audio"
	"whatsapp-mcp/internal/store"
	"whatsapp-mcp/internal/whatsapp"
)

// Server implements Model Context Protocol (MCP) server for WhatsApp
type Server struct {
	mcpServer *server.MCPServer
	waClient  *whatsapp.Client
	store     *store.Store
}

// NewServer registers all 12 MCP tools and initializes the MCP server
func NewServer(waClient *whatsapp.Client, msgStore *store.Store) *Server {
	s := &Server{
		mcpServer: server.NewMCPServer("whatsapp", "1.0.0"),
		waClient:  waClient,
		store:     msgStore,
	}

	s.registerTools()
	return s
}

// GetMCPServer returns the underlying MCPServer instance for SSE/HTTP serving
func (s *Server) GetMCPServer() *server.MCPServer {
	return s.mcpServer
}

// ServeStdio starts the MCP server over standard I/O (stdio)
func (s *Server) ServeStdio() error {
	return server.ServeStdio(s.mcpServer)
}

func (s *Server) registerTools() {
	// 1. search_contacts
	s.mcpServer.AddTool(
		mcp.NewTool("search_contacts",
			mcp.WithDescription("Search WhatsApp contacts by name or phone number."),
			mcp.WithString("query", mcp.Required(), mcp.Description("Search term to match against contact names or phone numbers")),
		),
		s.handleSearchContacts,
	)

	// 2. list_messages
	s.mcpServer.AddTool(
		mcp.NewTool("list_messages",
			mcp.WithDescription("Get WhatsApp messages matching specified criteria with optional context."),
			mcp.WithString("after", mcp.Description("Optional ISO-8601 formatted string to only return messages after this date")),
			mcp.WithString("before", mcp.Description("Optional ISO-8601 formatted string to only return messages before this date")),
			mcp.WithString("sender_phone_number", mcp.Description("Optional phone number to filter messages by sender")),
			mcp.WithString("chat_jid", mcp.Description("Optional chat JID to filter messages by chat")),
			mcp.WithString("query", mcp.Description("Optional search term to filter messages by content")),
			mcp.WithInteger("limit", mcp.Description("Maximum number of messages to return (default 20)")),
			mcp.WithInteger("page", mcp.Description("Page number for pagination (default 0)")),
			mcp.WithBoolean("include_context", mcp.Description("Whether to include messages before and after matches (default true)")),
			mcp.WithInteger("context_before", mcp.Description("Number of messages to include before each match (default 1)")),
			mcp.WithInteger("context_after", mcp.Description("Number of messages to include after each match (default 1)")),
		),
		s.handleListMessages,
	)

	// 3. list_chats
	s.mcpServer.AddTool(
		mcp.NewTool("list_chats",
			mcp.WithDescription("Get WhatsApp chats matching specified criteria."),
			mcp.WithString("query", mcp.Description("Optional search term to filter chats by name or JID")),
			mcp.WithInteger("limit", mcp.Description("Maximum number of chats to return (default 20)")),
			mcp.WithInteger("page", mcp.Description("Page number for pagination (default 0)")),
			mcp.WithBoolean("include_last_message", mcp.Description("Whether to include the last message in each chat (default true)")),
			mcp.WithString("sort_by", mcp.Description("Field to sort results by: 'last_active' or 'name' (default 'last_active')")),
		),
		s.handleListChats,
	)

	// 4. get_chat
	s.mcpServer.AddTool(
		mcp.NewTool("get_chat",
			mcp.WithDescription("Get WhatsApp chat metadata by JID."),
			mcp.WithString("chat_jid", mcp.Required(), mcp.Description("The JID of the chat to retrieve")),
			mcp.WithBoolean("include_last_message", mcp.Description("Whether to include the last message (default true)")),
		),
		s.handleGetChat,
	)

	// 5. get_direct_chat_by_contact
	s.mcpServer.AddTool(
		mcp.NewTool("get_direct_chat_by_contact",
			mcp.WithDescription("Get WhatsApp chat metadata by sender phone number."),
			mcp.WithString("sender_phone_number", mcp.Required(), mcp.Description("The phone number to search for")),
		),
		s.handleGetDirectChatByContact,
	)

	// 6. get_contact_chats
	s.mcpServer.AddTool(
		mcp.NewTool("get_contact_chats",
			mcp.WithDescription("Get all WhatsApp chats involving the contact."),
			mcp.WithString("jid", mcp.Required(), mcp.Description("The contact's JID to search for")),
			mcp.WithInteger("limit", mcp.Description("Maximum number of chats to return (default 20)")),
			mcp.WithInteger("page", mcp.Description("Page number for pagination (default 0)")),
		),
		s.handleGetContactChats,
	)

	// 7. get_last_interaction
	s.mcpServer.AddTool(
		mcp.NewTool("get_last_interaction",
			mcp.WithDescription("Get most recent WhatsApp message involving the contact."),
			mcp.WithString("jid", mcp.Required(), mcp.Description("The JID of the contact to search for")),
		),
		s.handleGetLastInteraction,
	)

	// 8. get_message_context
	s.mcpServer.AddTool(
		mcp.NewTool("get_message_context",
			mcp.WithDescription("Get context around a specific WhatsApp message."),
			mcp.WithString("message_id", mcp.Required(), mcp.Description("The ID of the message to get context for")),
			mcp.WithInteger("before", mcp.Description("Number of messages to include before the target message (default 5)")),
			mcp.WithInteger("after", mcp.Description("Number of messages to include after the target message (default 5)")),
		),
		s.handleGetMessageContext,
	)

	// 9. send_message
	s.mcpServer.AddTool(
		mcp.NewTool("send_message",
			mcp.WithDescription("Send a WhatsApp message to a person or group. For group chats use the JID."),
			mcp.WithString("recipient", mcp.Required(), mcp.Description("The recipient - either a phone number with country code but no + or other symbols, or a JID (e.g., '123456789@s.whatsapp.net' or a group JID like '123456789@g.us')")),
			mcp.WithString("message", mcp.Required(), mcp.Description("The message text to send")),
		),
		s.handleSendMessage,
	)

	// 10. send_file
	s.mcpServer.AddTool(
		mcp.NewTool("send_file",
			mcp.WithDescription("Send a file such as a picture, raw audio, video or document via WhatsApp to the specified recipient. For group messages use the JID."),
			mcp.WithString("recipient", mcp.Required(), mcp.Description("The recipient - phone number or JID")),
			mcp.WithString("media_path", mcp.Required(), mcp.Description("The absolute path to the media file to send (image, video, document)")),
		),
		s.handleSendFile,
	)

	// 11. send_audio_message
	s.mcpServer.AddTool(
		mcp.NewTool("send_audio_message",
			mcp.WithDescription("Send any audio file as a WhatsApp audio voice message to the specified recipient. For group messages use the JID. If it errors due to ffmpeg not being installed, use send_file instead."),
			mcp.WithString("recipient", mcp.Required(), mcp.Description("The recipient - phone number or JID")),
			mcp.WithString("media_path", mcp.Required(), mcp.Description("The absolute path to the audio file to send (will be converted to Opus .ogg if it's not a .ogg file)")),
		),
		s.handleSendAudioMessage,
	)

	// 12. download_media
	s.mcpServer.AddTool(
		mcp.NewTool("download_media",
			mcp.WithDescription("Download media from a WhatsApp message and get the local file path."),
			mcp.WithString("message_id", mcp.Required(), mcp.Description("The ID of the message containing the media")),
			mcp.WithString("chat_jid", mcp.Required(), mcp.Description("The JID of the chat containing the message")),
		),
		s.handleDownloadMedia,
	)

	// 13. get_whatsapp_status
	s.mcpServer.AddTool(
		mcp.NewTool("get_whatsapp_status",
			mcp.WithDescription("Check WhatsApp connection health, phone number, authentication status, and indexed messages/chats diagnostic."),
		),
		s.handleGetWhatsAppStatus,
	)
}

func (s *Server) handleSearchContacts(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := req.RequireString("query")
	if err != nil {
		return mcp.NewToolResultError("query is required"), nil
	}

	contacts, err := s.store.SearchContacts(query)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to search contacts: %v", err)), nil
	}

	data, _ := json.MarshalIndent(contacts, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) handleListMessages(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	filter := store.ListMessagesFilter{
		SenderPhoneNumber: req.GetString("sender_phone_number", ""),
		ChatJID:           req.GetString("chat_jid", ""),
		Query:             req.GetString("query", ""),
		Limit:             req.GetInt("limit", 20),
		Page:              req.GetInt("page", 0),
		IncludeContext:    req.GetBool("include_context", true),
		ContextBefore:     req.GetInt("context_before", 1),
		ContextAfter:      req.GetInt("context_after", 1),
	}

	if afterStr := req.GetString("after", ""); afterStr != "" {
		if t, err := time.Parse(time.RFC3339, afterStr); err == nil {
			filter.After = &t
		}
	}
	if beforeStr := req.GetString("before", ""); beforeStr != "" {
		if t, err := time.Parse(time.RFC3339, beforeStr); err == nil {
			filter.Before = &t
		}
	}

	messages, err := s.store.ListMessages(filter)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list messages: %v", err)), nil
	}

	var allMessages []store.Message
	senderNames := make(map[string]string)

	if filter.IncludeContext && len(messages) > 0 {
		seen := make(map[string]bool)
		for _, msg := range messages {
			context, err := s.store.GetMessageContext(msg.ID, filter.ContextBefore, filter.ContextAfter)
			if err == nil && context != nil {
				for _, b := range context.Before {
					if !seen[b.ID] {
						seen[b.ID] = true
						allMessages = append(allMessages, b)
					}
				}
				if !seen[context.Message.ID] {
					seen[context.Message.ID] = true
					allMessages = append(allMessages, context.Message)
				}
				for _, a := range context.After {
					if !seen[a.ID] {
						seen[a.ID] = true
						allMessages = append(allMessages, a)
					}
				}
			} else if !seen[msg.ID] {
				seen[msg.ID] = true
				allMessages = append(allMessages, msg)
			}
		}
	} else {
		allMessages = messages
	}

	for _, m := range allMessages {
		if _, ok := senderNames[m.Sender]; !ok {
			senderNames[m.Sender] = s.store.GetSenderName(m.Sender)
		}
	}

	formatted := store.FormatMessagesList(allMessages, senderNames, true)
	return mcp.NewToolResultText(formatted), nil
}

func (s *Server) handleListChats(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query := req.GetString("query", "")
	limit := req.GetInt("limit", 20)
	page := req.GetInt("page", 0)
	includeLastMessage := req.GetBool("include_last_message", true)
	sortBy := req.GetString("sort_by", "last_active")

	chats, err := s.store.ListChats(query, limit, page, includeLastMessage, sortBy)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to list chats: %v", err)), nil
	}

	data, _ := json.MarshalIndent(chats, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) handleGetChat(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	chatJID, err := req.RequireString("chat_jid")
	if err != nil {
		return mcp.NewToolResultError("chat_jid is required"), nil
	}
	includeLastMessage := req.GetBool("include_last_message", true)

	chat, err := s.store.GetChat(chatJID, includeLastMessage)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Database error: %v", err)), nil
	}
	if chat == nil {
		return mcp.NewToolResultError("Chat not found"), nil
	}

	data, _ := json.MarshalIndent(chat, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) handleGetDirectChatByContact(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	phone, err := req.RequireString("sender_phone_number")
	if err != nil {
		return mcp.NewToolResultError("sender_phone_number is required"), nil
	}

	chat, err := s.store.GetDirectChatByContact(phone)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Database error: %v", err)), nil
	}
	if chat == nil {
		return mcp.NewToolResultError("Direct chat not found for contact"), nil
	}

	data, _ := json.MarshalIndent(chat, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) handleGetContactChats(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	jid, err := req.RequireString("jid")
	if err != nil {
		return mcp.NewToolResultError("jid is required"), nil
	}
	limit := req.GetInt("limit", 20)
	page := req.GetInt("page", 0)

	chats, err := s.store.GetContactChats(jid, limit, page)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get contact chats: %v", err)), nil
	}

	data, _ := json.MarshalIndent(chats, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) handleGetLastInteraction(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	jid, err := req.RequireString("jid")
	if err != nil {
		return mcp.NewToolResultError("jid is required"), nil
	}

	msg, err := s.store.GetLastInteraction(jid)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Database error: %v", err)), nil
	}
	if msg == nil {
		return mcp.NewToolResultText("No interactions found."), nil
	}

	senderName := s.store.GetSenderName(msg.Sender)
	formatted := store.FormatMessage(*msg, senderName, true)
	return mcp.NewToolResultText(formatted), nil
}

func (s *Server) handleGetMessageContext(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	messageID, err := req.RequireString("message_id")
	if err != nil {
		return mcp.NewToolResultError("message_id is required"), nil
	}
	before := req.GetInt("before", 5)
	after := req.GetInt("after", 5)

	mContext, err := s.store.GetMessageContext(messageID, before, after)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to get message context: %v", err)), nil
	}

	data, _ := json.MarshalIndent(mContext, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) handleSendMessage(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	recipient, err := req.RequireString("recipient")
	if err != nil || recipient == "" {
		return mcp.NewToolResultError("recipient is required"), nil
	}
	message, err := req.RequireString("message")
	if err != nil || message == "" {
		return mcp.NewToolResultError("message text is required"), nil
	}

	success, statusMsg, err := s.waClient.SendMessage(ctx, recipient, message, "")
	res := map[string]interface{}{
		"success": success,
		"message": statusMsg,
	}
	data, _ := json.MarshalIndent(res, "", "  ")
	if err != nil || !success {
		return mcp.NewToolResultError(string(data)), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) handleSendFile(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	recipient, err := req.RequireString("recipient")
	if err != nil || recipient == "" {
		return mcp.NewToolResultError("recipient is required"), nil
	}
	mediaPath, err := req.RequireString("media_path")
	if err != nil || mediaPath == "" {
		return mcp.NewToolResultError("media_path is required"), nil
	}

	if _, err := os.Stat(mediaPath); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Media file not found: %s", mediaPath)), nil
	}

	success, statusMsg, err := s.waClient.SendMessage(ctx, recipient, "", mediaPath)
	res := map[string]interface{}{
		"success": success,
		"message": statusMsg,
	}
	data, _ := json.MarshalIndent(res, "", "  ")
	if err != nil || !success {
		return mcp.NewToolResultError(string(data)), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) handleSendAudioMessage(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	recipient, err := req.RequireString("recipient")
	if err != nil || recipient == "" {
		return mcp.NewToolResultError("recipient is required"), nil
	}
	mediaPath, err := req.RequireString("media_path")
	if err != nil || mediaPath == "" {
		return mcp.NewToolResultError("media_path is required"), nil
	}

	if _, err := os.Stat(mediaPath); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Audio file not found: %s", mediaPath)), nil
	}

	finalPath := mediaPath
	if !strings.HasSuffix(strings.ToLower(mediaPath), ".ogg") {
		converted, err := audio.ConvertToOpusOgg(mediaPath)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to convert audio to Opus OGG: %v. You likely need ffmpeg installed.", err)), nil
		}
		finalPath = converted
		defer os.Remove(converted)
	}

	success, statusMsg, err := s.waClient.SendMessage(ctx, recipient, "", finalPath)
	res := map[string]interface{}{
		"success": success,
		"message": statusMsg,
	}
	data, _ := json.MarshalIndent(res, "", "  ")
	if err != nil || !success {
		return mcp.NewToolResultError(string(data)), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) handleDownloadMedia(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	messageID, err := req.RequireString("message_id")
	if err != nil {
		return mcp.NewToolResultError("message_id is required"), nil
	}
	chatJID, err := req.RequireString("chat_jid")
	if err != nil {
		return mcp.NewToolResultError("chat_jid is required"), nil
	}

	filePath, err := s.waClient.DownloadMedia(ctx, messageID, chatJID)
	if err != nil {
		res := map[string]interface{}{
			"success": false,
			"message": fmt.Sprintf("Failed to download media: %v", err),
		}
		data, _ := json.MarshalIndent(res, "", "  ")
		return mcp.NewToolResultError(string(data)), nil
	}

	res := map[string]interface{}{
		"success":   true,
		"message":   "Media downloaded successfully",
		"file_path": filePath,
	}
	data, _ := json.MarshalIndent(res, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) handleGetWhatsAppStatus(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var waStatus whatsapp.ClientStatus
	if s.waClient != nil {
		waStatus = s.waClient.GetStatus()
	}

	chatCount, msgCount, _ := s.store.GetStats()
	_, ffmpegErr := exec.LookPath("ffmpeg")

	report := map[string]interface{}{
		"whatsapp_connected": waStatus.Connected,
		"whatsapp_logged_in": waStatus.LoggedIn,
		"user_phone":         waStatus.Phone,
		"user_jid":           waStatus.UserJID,
		"user_name":          waStatus.PushName,
		"indexed_chats":      chatCount,
		"indexed_messages":   msgCount,
		"ffmpeg_installed":   ffmpegErr == nil,
	}

	data, _ := json.MarshalIndent(report, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}
