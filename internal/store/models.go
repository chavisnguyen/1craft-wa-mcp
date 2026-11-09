package store

import (
	"fmt"
	"strings"
	"time"
)

// Message represents a chat message
type Message struct {
	ID        string    `json:"id"`
	ChatJID   string    `json:"chat_jid"`
	Sender    string    `json:"sender"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
	IsFromMe  bool      `json:"is_from_me"`
	MediaType string    `json:"media_type,omitempty"`
	Filename  string    `json:"filename,omitempty"`
	ChatName  string    `json:"chat_name,omitempty"`
}

// Chat represents a chat session/group
type Chat struct {
	JID             string     `json:"jid"`
	Name            string     `json:"name"`
	LastMessageTime *time.Time `json:"last_message_time,omitempty"`
	LastMessage     string     `json:"last_message,omitempty"`
	LastSender      string     `json:"last_sender,omitempty"`
	LastIsFromMe    *bool      `json:"last_is_from_me,omitempty"`
}

// IsGroup checks if a chat JID belongs to a group
func (c *Chat) IsGroup() bool {
	return strings.HasSuffix(c.JID, "@g.us")
}

// Contact represents a WhatsApp contact
type Contact struct {
	PhoneNumber string `json:"phone_number"`
	Name        string `json:"name"`
	JID         string `json:"jid"`
}

// MessageContext holds a message and its preceding/following messages
type MessageContext struct {
	Message Message   `json:"message"`
	Before  []Message `json:"before"`
	After   []Message `json:"after"`
}

// FormatMessage formats a single message to human-readable text for AI consumption
func FormatMessage(msg Message, senderName string, showChatInfo bool) string {
	var sb strings.Builder
	timeStr := msg.Timestamp.Format("2006-01-02 15:04:05")

	if showChatInfo && msg.ChatName != "" {
		sb.WriteString(fmt.Sprintf("[%s] Chat: %s ", timeStr, msg.ChatName))
	} else {
		sb.WriteString(fmt.Sprintf("[%s] ", timeStr))
	}

	contentPrefix := ""
	if msg.MediaType != "" {
		contentPrefix = fmt.Sprintf("[%s - Message ID: %s - Chat JID: %s] ", msg.MediaType, msg.ID, msg.ChatJID)
	}

	sender := senderName
	if msg.IsFromMe {
		sender = "Me"
	} else if sender == "" {
		sender = msg.Sender
	}

	sb.WriteString(fmt.Sprintf("From %s: %s%s\n", sender, contentPrefix, msg.Content))
	return sb.String()
}

// FormatMessagesList formats a list of messages into a single text block
func FormatMessagesList(messages []Message, senderNames map[string]string, showChatInfo bool) string {
	if len(messages) == 0 {
		return "No messages to display."
	}
	var sb strings.Builder
	for _, msg := range messages {
		senderName := senderNames[msg.Sender]
		sb.WriteString(FormatMessage(msg, senderName, showChatInfo))
	}
	return sb.String()
}
