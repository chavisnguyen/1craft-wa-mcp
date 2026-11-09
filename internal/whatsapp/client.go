// Add to internal/whatsapp/client.go: Status struct and helper
package whatsapp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/mdp/qrterminal"
	"github.com/rs/zerolog"
	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"

	"whatsapp-mcp/internal/store"
)

// ClientStatus represents runtime information of WhatsApp connection
type ClientStatus struct {
	Connected bool   `json:"connected"`
	LoggedIn  bool   `json:"logged_in"`
	UserJID   string `json:"user_jid,omitempty"`
	Phone     string `json:"phone,omitempty"`
	PushName  string `json:"push_name,omitempty"`
}

// Client wraps whatsmeow.Client with local message store and media handling
type Client struct {
	waClient *whatsmeow.Client
	store    *store.Store
	storeDir string
	logger   waLog.Logger
	mu       sync.RWMutex
}

// NewClient initializes whatsmeow client with device session store
func NewClient(ctx context.Context, storeDir string, msgStore *store.Store, verbose bool) (*Client, error) {
	if err := os.MkdirAll(storeDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create store dir: %w", err)
	}

	zlevel := zerolog.WarnLevel
	if verbose {
		zlevel = zerolog.InfoLevel
	}
	baseLog := zerolog.New(os.Stderr).With().Timestamp().Logger().Level(zlevel)
	logger := waLog.Zerolog(baseLog.With().Str("mod", "WhatsApp").Logger())
	dbLog := waLog.Zerolog(baseLog.With().Str("mod", "WhatsAppDB").Logger())

	sessionDbPath := filepath.Join(storeDir, "whatsapp.db")
	container, err := sqlstore.New(ctx, "sqlite3", fmt.Sprintf("file:%s?_foreign_keys=on", sessionDbPath), dbLog)
	if err != nil {
		return nil, fmt.Errorf("failed to open session database: %w", err)
	}

	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			deviceStore = container.NewDevice()
		} else {
			return nil, fmt.Errorf("failed to get device store: %w", err)
		}
	}

	waClient := whatsmeow.NewClient(deviceStore, logger)

	c := &Client{
		waClient: waClient,
		store:    msgStore,
		storeDir: storeDir,
		logger:   logger,
	}

	c.setupEventHandlers()
	return c, nil
}

// GetInnerClient returns underlying whatsmeow.Client
func (c *Client) GetInnerClient() *whatsmeow.Client {
	return c.waClient
}

// IsConnected returns whether client is connected to WhatsApp
func (c *Client) IsConnected() bool {
	return c.waClient != nil && c.waClient.IsConnected()
}

// IsLoggedIn checks if device has active credentials
func (c *Client) IsLoggedIn() bool {
	return c.waClient != nil && c.waClient.Store.ID != nil
}

// GetStatus returns the current status snapshot
func (c *Client) GetStatus() ClientStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()

	status := ClientStatus{
		Connected: c.IsConnected(),
		LoggedIn:  c.IsLoggedIn(),
	}

	if c.waClient != nil && c.waClient.Store.ID != nil {
		status.UserJID = c.waClient.Store.ID.String()
		status.Phone = c.waClient.Store.ID.User
		if c.waClient.Store.PushName != "" {
			status.PushName = c.waClient.Store.PushName
		}
	}
	return status
}

// Disconnect gracefully disconnects from WhatsApp
func (c *Client) Disconnect() {
	if c.waClient != nil {
		c.waClient.Disconnect()
	}
}

// Connect connects to WhatsApp. If not logged in, outputs QR to qrWriter.
func (c *Client) Connect(ctx context.Context, qrWriter io.Writer) error {
	if c.waClient.Store.ID == nil {
		// Needs QR code login
		qrChan, _ := c.waClient.GetQRChannel(ctx)
		if err := c.waClient.Connect(); err != nil {
			return fmt.Errorf("failed to initiate connection: %w", err)
		}

		fmt.Fprintln(qrWriter, "\n=== SCAN QR CODE WITH WHATSAPP ON YOUR PHONE ===")
		for evt := range qrChan {
			if evt.Event == "code" {
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, qrWriter)
			} else if evt.Event == "success" {
				fmt.Fprintln(qrWriter, "\n✓ Successfully authenticated with WhatsApp!")
				return nil
			} else if evt.Event == "timeout" {
				return fmt.Errorf("timed out waiting for QR code scan")
			}
		}
		return nil
	}

	// Already paired, direct connect
	if err := c.waClient.Connect(); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	return nil
}

func (c *Client) setupEventHandlers() {
	c.waClient.AddEventHandler(func(evt interface{}) {
		switch v := evt.(type) {
		case *events.Message:
			c.handleIncomingMessage(v)
		case *events.HistorySync:
			c.handleHistorySync(v)
		case *events.Connected:
			c.logger.Infof("Connected to WhatsApp server")
		case *events.LoggedOut:
			c.logger.Warnf("Logged out from WhatsApp. Re-authentication required.")
		}
	})
}

func (c *Client) handleIncomingMessage(msg *events.Message) {
	chatJID := msg.Info.Chat.String()
	sender := msg.Info.Sender.User

	name := c.resolveChatName(msg.Info.Chat, chatJID, nil, sender)
	_ = c.store.StoreChat(chatJID, name, msg.Info.Timestamp)

	content := ExtractTextContent(msg.Message)
	mediaType, filename, url, mediaKey, fileSHA256, fileEncSHA256, fileLength := ExtractMediaInfo(msg.Message)

	if content == "" && mediaType == "" {
		return
	}

	_ = c.store.StoreMessage(
		msg.Info.ID,
		chatJID,
		sender,
		content,
		msg.Info.Timestamp,
		msg.Info.IsFromMe,
		mediaType,
		filename,
		url,
		mediaKey,
		fileSHA256,
		fileEncSHA256,
		fileLength,
	)
}

func (c *Client) handleHistorySync(historySync *events.HistorySync) {
	c.logger.Infof("History sync received with %d conversations", len(historySync.Data.Conversations))
	for _, conv := range historySync.Data.Conversations {
		if conv.ID == nil {
			continue
		}
		chatJID := *conv.ID
		jid, err := types.ParseJID(chatJID)
		if err != nil {
			continue
		}

		name := c.resolveChatName(jid, chatJID, conv, "")
		for _, msgWrapper := range conv.Messages {
			if msgWrapper == nil || msgWrapper.Message == nil {
				continue
			}

			var timestamp time.Time
			if ts := msgWrapper.Message.GetMessageTimestamp(); ts != 0 {
				timestamp = time.Unix(int64(ts), 0)
			} else {
				continue
			}

			_ = c.store.StoreChat(chatJID, name, timestamp)

			var content string
			if msgWrapper.Message.Message != nil {
				content = ExtractTextContent(msgWrapper.Message.Message)
			}

			var mediaType, filename, url string
			var mediaKey, fileSHA256, fileEncSHA256 []byte
			var fileLength uint64
			if msgWrapper.Message.Message != nil {
				mediaType, filename, url, mediaKey, fileSHA256, fileEncSHA256, fileLength = ExtractMediaInfo(msgWrapper.Message.Message)
			}

			if content == "" && mediaType == "" {
				continue
			}

			isFromMe := false
			sender := jid.User
			if msgWrapper.Message.Key != nil {
				if msgWrapper.Message.Key.FromMe != nil {
					isFromMe = *msgWrapper.Message.Key.FromMe
				}
				if !isFromMe && msgWrapper.Message.Key.Participant != nil && *msgWrapper.Message.Key.Participant != "" {
					sender = *msgWrapper.Message.Key.Participant
				} else if isFromMe && c.waClient.Store.ID != nil {
					sender = c.waClient.Store.ID.User
				}
			}

			msgID := ""
			if msgWrapper.Message.Key != nil && msgWrapper.Message.Key.ID != nil {
				msgID = *msgWrapper.Message.Key.ID
			}

			_ = c.store.StoreMessage(
				msgID, chatJID, sender, content, timestamp, isFromMe,
				mediaType, filename, url, mediaKey, fileSHA256, fileEncSHA256, fileLength,
			)
		}
	}
}

func (c *Client) resolveChatName(jid types.JID, chatJID string, conversation interface{}, sender string) string {
	existingName := c.store.GetSenderName(chatJID)
	if existingName != chatJID && existingName != "" {
		return existingName
	}

	if jid.Server == "g.us" {
		if conversation != nil {
			v := reflect.ValueOf(conversation)
			if v.Kind() == reflect.Ptr && !v.IsNil() {
				v = v.Elem()
				if dnField := v.FieldByName("DisplayName"); dnField.IsValid() && dnField.Kind() == reflect.Ptr && !dnField.IsNil() {
					return dnField.Elem().String()
				}
				if nameField := v.FieldByName("Name"); nameField.IsValid() && nameField.Kind() == reflect.Ptr && !nameField.IsNil() {
					return nameField.Elem().String()
				}
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		groupInfo, err := c.waClient.GetGroupInfo(ctx, jid)
		if err == nil && groupInfo.Name != "" {
			return groupInfo.Name
		}
		return fmt.Sprintf("Group %s", jid.User)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	contact, err := c.waClient.Store.Contacts.GetContact(ctx, jid)
	if err == nil && contact.FullName != "" {
		return contact.FullName
	}
	if sender != "" {
		return sender
	}
	return jid.User
}

// SendMessage sends text, file or voice audio to recipient
func (c *Client) SendMessage(ctx context.Context, recipient, message, mediaPath string) (bool, string, error) {
	if !c.IsConnected() {
		return false, "Not connected to WhatsApp", errors.New("whatsapp client not connected")
	}

	var recipientJID types.JID
	var err error
	if strings.Contains(recipient, "@") {
		recipientJID, err = types.ParseJID(recipient)
		if err != nil {
			return false, fmt.Sprintf("Invalid recipient JID: %v", err), err
		}
	} else {
		recipientJID = types.JID{
			User:   strings.TrimPrefix(recipient, "+"),
			Server: "s.whatsapp.net",
		}
	}

	msg := &waProto.Message{}

	if mediaPath != "" {
		mediaData, err := os.ReadFile(mediaPath)
		if err != nil {
			return false, fmt.Sprintf("Failed to read media file: %v", err), err
		}

		ext := strings.ToLower(filepath.Ext(mediaPath))
		var mediaType whatsmeow.MediaType
		var mimeType string

		switch ext {
		case ".jpg", ".jpeg":
			mediaType = whatsmeow.MediaImage
			mimeType = "image/jpeg"
		case ".png":
			mediaType = whatsmeow.MediaImage
			mimeType = "image/png"
		case ".gif":
			mediaType = whatsmeow.MediaImage
			mimeType = "image/gif"
		case ".webp":
			mediaType = whatsmeow.MediaImage
			mimeType = "image/webp"
		case ".ogg":
			mediaType = whatsmeow.MediaAudio
			mimeType = "audio/ogg; codecs=opus"
		case ".mp4":
			mediaType = whatsmeow.MediaVideo
			mimeType = "video/mp4"
		case ".avi":
			mediaType = whatsmeow.MediaVideo
			mimeType = "video/avi"
		case ".mov":
			mediaType = whatsmeow.MediaVideo
			mimeType = "video/quicktime"
		default:
			mediaType = whatsmeow.MediaDocument
			mimeType = "application/octet-stream"
		}

		resp, err := c.waClient.Upload(ctx, mediaData, mediaType)
		if err != nil {
			return false, fmt.Sprintf("Failed to upload media to WhatsApp: %v", err), err
		}

		switch mediaType {
		case whatsmeow.MediaImage:
			msg.ImageMessage = &waProto.ImageMessage{
				Caption:       proto.String(message),
				Mimetype:      proto.String(mimeType),
				URL:           &resp.URL,
				DirectPath:    &resp.DirectPath,
				MediaKey:      resp.MediaKey,
				FileEncSHA256: resp.FileEncSHA256,
				FileSHA256:    resp.FileSHA256,
				FileLength:    &resp.FileLength,
			}
		case whatsmeow.MediaAudio:
			var seconds uint32 = 30
			var waveform []byte
			if strings.Contains(mimeType, "ogg") {
				if analyzedSec, analyzedWaveform, err := AnalyzeOggOpus(mediaData); err == nil {
					seconds = analyzedSec
					waveform = analyzedWaveform
				}
			}
			msg.AudioMessage = &waProto.AudioMessage{
				Mimetype:      proto.String(mimeType),
				URL:           &resp.URL,
				DirectPath:    &resp.DirectPath,
				MediaKey:      resp.MediaKey,
				FileEncSHA256: resp.FileEncSHA256,
				FileSHA256:    resp.FileSHA256,
				FileLength:    &resp.FileLength,
				Seconds:       proto.Uint32(seconds),
				PTT:           proto.Bool(true),
				Waveform:      waveform,
			}
		case whatsmeow.MediaVideo:
			msg.VideoMessage = &waProto.VideoMessage{
				Caption:       proto.String(message),
				Mimetype:      proto.String(mimeType),
				URL:           &resp.URL,
				DirectPath:    &resp.DirectPath,
				MediaKey:      resp.MediaKey,
				FileEncSHA256: resp.FileEncSHA256,
				FileSHA256:    resp.FileSHA256,
				FileLength:    &resp.FileLength,
			}
		case whatsmeow.MediaDocument:
			msg.DocumentMessage = &waProto.DocumentMessage{
				Title:         proto.String(filepath.Base(mediaPath)),
				Caption:       proto.String(message),
				Mimetype:      proto.String(mimeType),
				URL:           &resp.URL,
				DirectPath:    &resp.DirectPath,
				MediaKey:      resp.MediaKey,
				FileEncSHA256: resp.FileEncSHA256,
				FileSHA256:    resp.FileSHA256,
				FileLength:    &resp.FileLength,
			}
		}
	} else {
		msg.Conversation = proto.String(message)
	}

	_, err = c.waClient.SendMessage(ctx, recipientJID, msg)
	if err != nil {
		return false, fmt.Sprintf("Error sending WhatsApp message: %v", err), err
	}

	return true, fmt.Sprintf("Message sent to %s", recipient), nil
}

// DownloadMedia downloads attachment for a message
func (c *Client) DownloadMedia(ctx context.Context, messageID, chatJID string) (string, error) {
	info, err := c.store.GetMediaInfo(messageID, chatJID)
	if err != nil {
		return "", fmt.Errorf("failed to get media info: %w", err)
	}
	if info.MediaType == "" {
		return "", errors.New("not a media message")
	}

	chatDir := filepath.Join(c.storeDir, strings.ReplaceAll(chatJID, ":", "_"))
	if err := os.MkdirAll(chatDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create chat dir: %w", err)
	}

	localPath := filepath.Join(chatDir, info.Filename)
	absPath, err := filepath.Abs(localPath)
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(localPath); err == nil {
		return absPath, nil
	}

	if info.URL == "" || len(info.MediaKey) == 0 {
		return "", errors.New("incomplete media information for download")
	}

	var waMediaType whatsmeow.MediaType
	switch info.MediaType {
	case "image":
		waMediaType = whatsmeow.MediaImage
	case "video":
		waMediaType = whatsmeow.MediaVideo
	case "audio":
		waMediaType = whatsmeow.MediaAudio
	case "document":
		waMediaType = whatsmeow.MediaDocument
	default:
		return "", fmt.Errorf("unsupported media type: %s", info.MediaType)
	}

	directPath := ExtractDirectPathFromURL(info.URL)
	downloader := &MediaDownloader{
		URL:           info.URL,
		DirectPath:    directPath,
		MediaKey:      info.MediaKey,
		FileLength:    info.FileLength,
		FileSHA256:    info.FileSHA256,
		FileEncSHA256: info.FileEncSHA256,
		MediaType:     waMediaType,
	}

	mediaData, err := c.waClient.Download(ctx, downloader)
	if err != nil {
		return "", fmt.Errorf("failed to download media from WhatsApp: %w", err)
	}

	if err := os.WriteFile(localPath, mediaData, 0644); err != nil {
		return "", fmt.Errorf("failed to save downloaded file: %w", err)
	}

	return absPath, nil
}
