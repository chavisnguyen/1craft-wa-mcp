package whatsapp

import (
	"testing"

	waProto "go.mau.fi/whatsmeow/binary/proto"
	"google.golang.org/protobuf/proto"
)

func TestExtractDirectPathFromURL(t *testing.T) {
	url := "https://mmg.whatsapp.net/v/t62.7118-24/13812002_698058036224062_3424455886509161511_n.enc?ccb=11-4&oh=abc"
	directPath := ExtractDirectPathFromURL(url)
	expected := "/v/t62.7118-24/13812002_698058036224062_3424455886509161511_n.enc"
	if directPath != expected {
		t.Errorf("Expected %s, got %s", expected, directPath)
	}
}

func TestExtractTextContent(t *testing.T) {
	msg1 := &waProto.Message{
		Conversation: proto.String("Simple conversation"),
	}
	if text := ExtractTextContent(msg1); text != "Simple conversation" {
		t.Errorf("Expected 'Simple conversation', got '%s'", text)
	}

	msg2 := &waProto.Message{
		ExtendedTextMessage: &waProto.ExtendedTextMessage{
			Text: proto.String("Extended message text"),
		},
	}
	if text := ExtractTextContent(msg2); text != "Extended message text" {
		t.Errorf("Expected 'Extended message text', got '%s'", text)
	}
}

func TestExtractMediaInfo(t *testing.T) {
	imgMsg := &waProto.Message{
		ImageMessage: &waProto.ImageMessage{
			URL:           proto.String("https://mmg.whatsapp.net/v/test.enc"),
			MediaKey:      []byte{1, 2, 3},
			FileSHA256:    []byte{4, 5, 6},
			FileEncSHA256: []byte{7, 8, 9},
			FileLength:    proto.Uint64(2048),
		},
	}

	mediaType, filename, url, mediaKey, sha, encSha, length := ExtractMediaInfo(imgMsg)
	if mediaType != "image" {
		t.Errorf("Expected mediaType 'image', got '%s'", mediaType)
	}
	if filename == "" {
		t.Errorf("Expected non-empty filename")
	}
	if url != "https://mmg.whatsapp.net/v/test.enc" || length != 2048 {
		t.Errorf("Unexpected media info values: url=%s, length=%d", url, length)
	}
	if len(mediaKey) != 3 || len(sha) != 3 || len(encSha) != 3 {
		t.Errorf("Unexpected byte slice lengths")
	}
}

func TestGeneratePlaceholderWaveform(t *testing.T) {
	waveform := generatePlaceholderWaveform(10)
	if len(waveform) != 64 {
		t.Errorf("Expected waveform length 64, got %d", len(waveform))
	}
	for i, v := range waveform {
		if v > 100 {
			t.Errorf("Waveform byte %d exceeds 100: %d", i, v)
		}
	}
}
