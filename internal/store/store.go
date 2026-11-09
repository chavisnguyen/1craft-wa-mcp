package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Store manages SQLite database for chats, messages, and contacts
type Store struct {
	db   *sql.DB
	path string
	mu   sync.RWMutex
}

// NewStore initializes the SQLite database with WAL mode and creates required tables
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create store directory: %w", err)
	}

	dbPath := filepath.Join(dir, "messages.db")
	// Open SQLite with foreign keys and WAL mode for high concurrency
	dsn := fmt.Sprintf("file:%s?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=5000", dbPath)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open message database: %w", err)
	}

	// Optimize connection pool for SQLite
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	// Create tables and indices
	schema := `
	CREATE TABLE IF NOT EXISTS chats (
		jid TEXT PRIMARY KEY,
		name TEXT,
		last_message_time TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS messages (
		id TEXT,
		chat_jid TEXT,
		sender TEXT,
		content TEXT,
		timestamp TIMESTAMP,
		is_from_me BOOLEAN,
		media_type TEXT,
		filename TEXT,
		url TEXT,
		media_key BLOB,
		file_sha256 BLOB,
		file_enc_sha256 BLOB,
		file_length INTEGER,
		PRIMARY KEY (id, chat_jid),
		FOREIGN KEY (chat_jid) REFERENCES chats(jid)
	);

	CREATE INDEX IF NOT EXISTS idx_messages_chat_jid_timestamp ON messages (chat_jid, timestamp DESC);
	CREATE INDEX IF NOT EXISTS idx_messages_sender ON messages (sender);
	CREATE INDEX IF NOT EXISTS idx_messages_timestamp ON messages (timestamp DESC);
	CREATE INDEX IF NOT EXISTS idx_chats_last_message_time ON chats (last_message_time DESC);
	`

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return &Store{
		db:   db,
		path: dbPath,
	}, nil
}

// Close closes the database connection
func (s *Store) Close() error {
	return s.db.Close()
}

// GetStats returns count of indexed chats and messages
func (s *Store) GetStats() (chatCount, messageCount int, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	err = s.db.QueryRow("SELECT COUNT(*) FROM chats").Scan(&chatCount)
	if err != nil {
		return 0, 0, err
	}
	err = s.db.QueryRow("SELECT COUNT(*) FROM messages").Scan(&messageCount)
	if err != nil {
		return chatCount, 0, err
	}
	return chatCount, messageCount, nil
}

// StoreChat upserts a chat into the database
func (s *Store) StoreChat(jid, name string, lastMessageTime time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`
		INSERT INTO chats (jid, name, last_message_time)
		VALUES (?, ?, ?)
		ON CONFLICT(jid) DO UPDATE SET
			name = CASE WHEN excluded.name != '' THEN excluded.name ELSE chats.name END,
			last_message_time = CASE 
				WHEN excluded.last_message_time > chats.last_message_time OR chats.last_message_time IS NULL 
				THEN excluded.last_message_time 
				ELSE chats.last_message_time 
			END
	`, jid, name, lastMessageTime)
	return err
}

// StoreMessage stores or updates a message
func (s *Store) StoreMessage(
	id, chatJID, sender, content string,
	timestamp time.Time, isFromMe bool,
	mediaType, filename, url string,
	mediaKey, fileSHA256, fileEncSHA256 []byte,
	fileLength uint64,
) error {
	if content == "" && mediaType == "" {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`
		INSERT INTO messages (
			id, chat_jid, sender, content, timestamp, is_from_me,
			media_type, filename, url, media_key, file_sha256, file_enc_sha256, file_length
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id, chat_jid) DO UPDATE SET
			content = CASE WHEN excluded.content != '' THEN excluded.content ELSE messages.content END,
			timestamp = excluded.timestamp,
			is_from_me = excluded.is_from_me,
			media_type = CASE WHEN excluded.media_type != '' THEN excluded.media_type ELSE messages.media_type END,
			filename = CASE WHEN excluded.filename != '' THEN excluded.filename ELSE messages.filename END,
			url = CASE WHEN excluded.url != '' THEN excluded.url ELSE messages.url END,
			media_key = CASE WHEN length(excluded.media_key) > 0 THEN excluded.media_key ELSE messages.media_key END,
			file_sha256 = CASE WHEN length(excluded.file_sha256) > 0 THEN excluded.file_sha256 ELSE messages.file_sha256 END,
			file_enc_sha256 = CASE WHEN length(excluded.file_enc_sha256) > 0 THEN excluded.file_enc_sha256 ELSE messages.file_enc_sha256 END,
			file_length = CASE WHEN excluded.file_length > 0 THEN excluded.file_length ELSE messages.file_length END
	`, id, chatJID, sender, content, timestamp, isFromMe,
		mediaType, filename, url, mediaKey, fileSHA256, fileEncSHA256, fileLength)

	return err
}

// GetSenderName resolves sender JID to name if known
func (s *Store) GetSenderName(senderJID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var name string
	err := s.db.QueryRow("SELECT name FROM chats WHERE jid = ? LIMIT 1", senderJID).Scan(&name)
	if err == nil && name != "" {
		return name
	}

	phonePart := senderJID
	if strings.Contains(senderJID, "@") {
		phonePart = strings.Split(senderJID, "@")[0]
	}

	err = s.db.QueryRow("SELECT name FROM chats WHERE jid LIKE ? AND name != '' LIMIT 1", "%"+phonePart+"%").Scan(&name)
	if err == nil && name != "" {
		return name
	}

	return senderJID
}

// SearchContacts searches contacts by name or phone number
func (s *Store) SearchContacts(query string) ([]Contact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	searchPattern := "%" + strings.TrimSpace(query) + "%"
	rows, err := s.db.Query(`
		SELECT DISTINCT jid, name
		FROM chats
		WHERE (LOWER(name) LIKE LOWER(?) OR LOWER(jid) LIKE LOWER(?))
		  AND jid NOT LIKE '%@g.us'
		ORDER BY name, jid
		LIMIT 50
	`, searchPattern, searchPattern)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contacts []Contact
	for rows.Next() {
		var jid, name string
		if err := rows.Scan(&jid, &name); err != nil {
			return nil, err
		}
		phone := jid
		if strings.Contains(jid, "@") {
			phone = strings.Split(jid, "@")[0]
		}
		contacts = append(contacts, Contact{
			PhoneNumber: phone,
			Name:        name,
			JID:         jid,
		})
	}
	return contacts, nil
}

// ListMessagesFilter options for querying messages
type ListMessagesFilter struct {
	After             *time.Time
	Before            *time.Time
	SenderPhoneNumber string
	ChatJID           string
	Query             string
	Limit             int
	Page              int
	IncludeContext    bool
	ContextBefore     int
	ContextAfter      int
}

// ListMessages retrieves messages matching filters, optionally formatting with context
func (s *Store) ListMessages(filter ListMessagesFilter) ([]Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if filter.Limit <= 0 {
		filter.Limit = 20
	}

	baseQuery := `
		SELECT m.timestamp, m.sender, COALESCE(c.name, ''), m.content, m.is_from_me, m.chat_jid, m.id, COALESCE(m.media_type, '')
		FROM messages m
		LEFT JOIN chats c ON m.chat_jid = c.jid
	`

	var whereClauses []string
	var args []interface{}

	if filter.After != nil {
		whereClauses = append(whereClauses, "m.timestamp > ?")
		args = append(args, *filter.After)
	}
	if filter.Before != nil {
		whereClauses = append(whereClauses, "m.timestamp < ?")
		args = append(args, *filter.Before)
	}
	if filter.SenderPhoneNumber != "" {
		whereClauses = append(whereClauses, "(m.sender = ? OR m.sender LIKE ?)")
		args = append(args, filter.SenderPhoneNumber, "%"+filter.SenderPhoneNumber+"%")
	}
	if filter.ChatJID != "" {
		whereClauses = append(whereClauses, "m.chat_jid = ?")
		args = append(args, filter.ChatJID)
	}
	if filter.Query != "" {
		whereClauses = append(whereClauses, "LOWER(m.content) LIKE LOWER(?)")
		args = append(args, "%"+filter.Query+"%")
	}

	if len(whereClauses) > 0 {
		baseQuery += " WHERE " + strings.Join(whereClauses, " AND ")
	}

	baseQuery += " ORDER BY m.timestamp DESC LIMIT ? OFFSET ?"
	args = append(args, filter.Limit, filter.Page*filter.Limit)

	rows, err := s.db.Query(baseQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var msg Message
		if err := rows.Scan(&msg.Timestamp, &msg.Sender, &msg.ChatName, &msg.Content, &msg.IsFromMe, &msg.ChatJID, &msg.ID, &msg.MediaType); err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	return messages, nil
}

// GetMessageContext gets context around a specific message
func (s *Store) GetMessageContext(messageID string, before, after int) (*MessageContext, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var target Message
	err := s.db.QueryRow(`
		SELECT m.timestamp, m.sender, COALESCE(c.name, ''), m.content, m.is_from_me, m.chat_jid, m.id, COALESCE(m.media_type, '')
		FROM messages m
		LEFT JOIN chats c ON m.chat_jid = c.jid
		WHERE m.id = ?
		LIMIT 1
	`, messageID).Scan(&target.Timestamp, &target.Sender, &target.ChatName, &target.Content, &target.IsFromMe, &target.ChatJID, &target.ID, &target.MediaType)
	if err != nil {
		return nil, fmt.Errorf("message with ID %s not found: %w", messageID, err)
	}

	if before <= 0 {
		before = 5
	}
	if after <= 0 {
		after = 5
	}

	// Messages before
	rowsBefore, err := s.db.Query(`
		SELECT m.timestamp, m.sender, COALESCE(c.name, ''), m.content, m.is_from_me, m.chat_jid, m.id, COALESCE(m.media_type, '')
		FROM messages m
		LEFT JOIN chats c ON m.chat_jid = c.jid
		WHERE m.chat_jid = ? AND m.timestamp < ?
		ORDER BY m.timestamp DESC
		LIMIT ?
	`, target.ChatJID, target.Timestamp, before)
	if err != nil {
		return nil, err
	}
	defer rowsBefore.Close()

	var beforeMsgs []Message
	for rowsBefore.Next() {
		var msg Message
		if err := rowsBefore.Scan(&msg.Timestamp, &msg.Sender, &msg.ChatName, &msg.Content, &msg.IsFromMe, &msg.ChatJID, &msg.ID, &msg.MediaType); err != nil {
			return nil, err
		}
		beforeMsgs = append([]Message{msg}, beforeMsgs...) // maintain chronological order
	}

	// Messages after
	rowsAfter, err := s.db.Query(`
		SELECT m.timestamp, m.sender, COALESCE(c.name, ''), m.content, m.is_from_me, m.chat_jid, m.id, COALESCE(m.media_type, '')
		FROM messages m
		LEFT JOIN chats c ON m.chat_jid = c.jid
		WHERE m.chat_jid = ? AND m.timestamp > ?
		ORDER BY m.timestamp ASC
		LIMIT ?
	`, target.ChatJID, target.Timestamp, after)
	if err != nil {
		return nil, err
	}
	defer rowsAfter.Close()

	var afterMsgs []Message
	for rowsAfter.Next() {
		var msg Message
		if err := rowsAfter.Scan(&msg.Timestamp, &msg.Sender, &msg.ChatName, &msg.Content, &msg.IsFromMe, &msg.ChatJID, &msg.ID, &msg.MediaType); err != nil {
			return nil, err
		}
		afterMsgs = append(afterMsgs, msg)
	}

	return &MessageContext{
		Message: target,
		Before:  beforeMsgs,
		After:   afterMsgs,
	}, nil
}

// ListChats retrieves chats matching criteria
func (s *Store) ListChats(query string, limit, page int, includeLastMessage bool, sortBy string) ([]Chat, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 20
	}

	baseQuery := `
		SELECT 
			c.jid,
			COALESCE(c.name, ''),
			c.last_message_time,
			COALESCE(m.content, ''),
			COALESCE(m.sender, ''),
			m.is_from_me
		FROM chats c
	`

	if includeLastMessage {
		baseQuery += `
			LEFT JOIN messages m ON c.jid = m.chat_jid 
			AND c.last_message_time = m.timestamp
		`
	} else {
		baseQuery += `
			LEFT JOIN messages m ON 1=0
		`
	}

	var whereClauses []string
	var args []interface{}

	if query != "" {
		whereClauses = append(whereClauses, "(LOWER(c.name) LIKE LOWER(?) OR c.jid LIKE ?)")
		args = append(args, "%"+query+"%", "%"+query+"%")
	}

	if len(whereClauses) > 0 {
		baseQuery += " WHERE " + strings.Join(whereClauses, " AND ")
	}

	orderBy := "c.last_message_time DESC NULLS LAST"
	if sortBy == "name" {
		orderBy = "c.name ASC"
	}
	baseQuery += fmt.Sprintf(" ORDER BY %s LIMIT ? OFFSET ?", orderBy)
	args = append(args, limit, page*limit)

	rows, err := s.db.Query(baseQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chats []Chat
	for rows.Next() {
		var c Chat
		var lastTime sql.NullTime
		var isFromMe sql.NullBool
		if err := rows.Scan(&c.JID, &c.Name, &lastTime, &c.LastMessage, &c.LastSender, &isFromMe); err != nil {
			return nil, err
		}
		if lastTime.Valid {
			c.LastMessageTime = &lastTime.Time
		}
		if isFromMe.Valid {
			c.LastIsFromMe = &isFromMe.Bool
		}
		chats = append(chats, c)
	}
	return chats, nil
}

// GetChat gets chat metadata by JID
func (s *Store) GetChat(chatJID string, includeLastMessage bool) (*Chat, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT 
			c.jid,
			COALESCE(c.name, ''),
			c.last_message_time,
			COALESCE(m.content, ''),
			COALESCE(m.sender, ''),
			m.is_from_me
		FROM chats c
	`
	if includeLastMessage {
		query += `
			LEFT JOIN messages m ON c.jid = m.chat_jid 
			AND c.last_message_time = m.timestamp
		`
	} else {
		query += `
			LEFT JOIN messages m ON 1=0
		`
	}
	query += " WHERE c.jid = ? LIMIT 1"

	var c Chat
	var lastTime sql.NullTime
	var isFromMe sql.NullBool
	err := s.db.QueryRow(query, chatJID).Scan(&c.JID, &c.Name, &lastTime, &c.LastMessage, &c.LastSender, &isFromMe)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if lastTime.Valid {
		c.LastMessageTime = &lastTime.Time
	}
	if isFromMe.Valid {
		c.LastIsFromMe = &isFromMe.Bool
	}
	return &c, nil
}

// GetDirectChatByContact gets chat by sender phone number
func (s *Store) GetDirectChatByContact(phoneNumber string) (*Chat, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cleanPhone := strings.TrimPrefix(phoneNumber, "+")
	query := `
		SELECT 
			c.jid,
			COALESCE(c.name, ''),
			c.last_message_time,
			COALESCE(m.content, ''),
			COALESCE(m.sender, ''),
			m.is_from_me
		FROM chats c
		LEFT JOIN messages m ON c.jid = m.chat_jid 
			AND c.last_message_time = m.timestamp
		WHERE c.jid LIKE ? AND c.jid NOT LIKE '%@g.us'
		LIMIT 1
	`
	var c Chat
	var lastTime sql.NullTime
	var isFromMe sql.NullBool
	err := s.db.QueryRow(query, "%"+cleanPhone+"%").Scan(&c.JID, &c.Name, &lastTime, &c.LastMessage, &c.LastSender, &isFromMe)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if lastTime.Valid {
		c.LastMessageTime = &lastTime.Time
	}
	if isFromMe.Valid {
		c.LastIsFromMe = &isFromMe.Bool
	}
	return &c, nil
}

// GetContactChats gets all chats involving a specific contact
func (s *Store) GetContactChats(jid string, limit, page int) ([]Chat, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 20
	}

	phonePart := jid
	if strings.Contains(jid, "@") {
		phonePart = strings.Split(jid, "@")[0]
	}

	query := `
		SELECT DISTINCT
			c.jid,
			COALESCE(c.name, ''),
			c.last_message_time,
			COALESCE(m.content, ''),
			COALESCE(m.sender, ''),
			m.is_from_me
		FROM chats c
		JOIN messages m ON c.jid = m.chat_jid
		WHERE m.sender = ? OR m.sender LIKE ? OR c.jid = ?
		ORDER BY c.last_message_time DESC NULLS LAST
		LIMIT ? OFFSET ?
	`
	rows, err := s.db.Query(query, jid, "%"+phonePart+"%", jid, limit, page*limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chats []Chat
	for rows.Next() {
		var c Chat
		var lastTime sql.NullTime
		var isFromMe sql.NullBool
		if err := rows.Scan(&c.JID, &c.Name, &lastTime, &c.LastMessage, &c.LastSender, &isFromMe); err != nil {
			return nil, err
		}
		if lastTime.Valid {
			c.LastMessageTime = &lastTime.Time
		}
		if isFromMe.Valid {
			c.LastIsFromMe = &isFromMe.Bool
		}
		chats = append(chats, c)
	}
	return chats, nil
}

// GetLastInteraction gets the most recent message involving a contact
func (s *Store) GetLastInteraction(jid string) (*Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	phonePart := jid
	if strings.Contains(jid, "@") {
		phonePart = strings.Split(jid, "@")[0]
	}

	query := `
		SELECT 
			m.timestamp,
			m.sender,
			COALESCE(c.name, ''),
			m.content,
			m.is_from_me,
			m.chat_jid,
			m.id,
			COALESCE(m.media_type, '')
		FROM messages m
		JOIN chats c ON m.chat_jid = c.jid
		WHERE m.sender = ? OR m.sender LIKE ? OR c.jid = ?
		ORDER BY m.timestamp DESC
		LIMIT 1
	`
	var msg Message
	err := s.db.QueryRow(query, jid, "%"+phonePart+"%", jid).Scan(
		&msg.Timestamp, &msg.Sender, &msg.ChatName, &msg.Content, &msg.IsFromMe, &msg.ChatJID, &msg.ID, &msg.MediaType,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &msg, nil
}

// MediaInfo represents media metadata for downloading
type MediaInfo struct {
	MediaType     string
	Filename      string
	URL           string
	MediaKey      []byte
	FileSHA256    []byte
	FileEncSHA256 []byte
	FileLength    uint64
}

// GetMediaInfo gets full media download info for a message
func (s *Store) GetMediaInfo(messageID, chatJID string) (*MediaInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var info MediaInfo
	err := s.db.QueryRow(`
		SELECT COALESCE(media_type, ''), COALESCE(filename, ''), COALESCE(url, ''),
		       COALESCE(media_key, X''), COALESCE(file_sha256, X''), COALESCE(file_enc_sha256, X''), COALESCE(file_length, 0)
		FROM messages
		WHERE id = ? AND chat_jid = ?
		LIMIT 1
	`, messageID, chatJID).Scan(&info.MediaType, &info.Filename, &info.URL, &info.MediaKey, &info.FileSHA256, &info.FileEncSHA256, &info.FileLength)

	if err != nil {
		return nil, err
	}
	return &info, nil
}
