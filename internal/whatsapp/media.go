package whatsapp

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
)

// MediaDownloader implements whatsmeow.DownloadableMessage
type MediaDownloader struct {
	URL           string
	DirectPath    string
	MediaKey      []byte
	FileLength    uint64
	FileSHA256    []byte
	FileEncSHA256 []byte
	MediaType     whatsmeow.MediaType
}

func (d *MediaDownloader) GetDirectPath() string {
	return d.DirectPath
}

func (d *MediaDownloader) GetURL() string {
	return d.URL
}

func (d *MediaDownloader) GetMediaKey() []byte {
	return d.MediaKey
}

func (d *MediaDownloader) GetFileLength() uint64 {
	return d.FileLength
}

func (d *MediaDownloader) GetFileSHA256() []byte {
	return d.FileSHA256
}

func (d *MediaDownloader) GetFileEncSHA256() []byte {
	return d.FileEncSHA256
}

func (d *MediaDownloader) GetMediaType() whatsmeow.MediaType {
	return d.MediaType
}

// ExtractDirectPathFromURL extracts direct path from a WhatsApp media URL
func ExtractDirectPathFromURL(url string) string {
	parts := strings.SplitN(url, ".net/", 2)
	if len(parts) < 2 {
		return url
	}
	pathPart := strings.SplitN(parts[1], "?", 2)[0]
	return "/" + pathPart
}

// ExtractTextContent extracts text from a protobuf Message
func ExtractTextContent(msg *waProto.Message) string {
	if msg == nil {
		return ""
	}
	if text := msg.GetConversation(); text != "" {
		return text
	} else if extended := msg.GetExtendedTextMessage(); extended != nil {
		return extended.GetText()
	}
	return ""
}

// ExtractMediaInfo extracts media metadata from a protobuf Message
func ExtractMediaInfo(msg *waProto.Message) (mediaType, filename, url string, mediaKey, fileSHA256, fileEncSHA256 []byte, fileLength uint64) {
	if msg == nil {
		return "", "", "", nil, nil, nil, 0
	}

	timestampStr := time.Now().Format("20060102_150405")

	if img := msg.GetImageMessage(); img != nil {
		return "image", "image_" + timestampStr + ".jpg",
			img.GetURL(), img.GetMediaKey(), img.GetFileSHA256(), img.GetFileEncSHA256(), img.GetFileLength()
	}

	if vid := msg.GetVideoMessage(); vid != nil {
		return "video", "video_" + timestampStr + ".mp4",
			vid.GetURL(), vid.GetMediaKey(), vid.GetFileSHA256(), vid.GetFileEncSHA256(), vid.GetFileLength()
	}

	if aud := msg.GetAudioMessage(); aud != nil {
		return "audio", "audio_" + timestampStr + ".ogg",
			aud.GetURL(), aud.GetMediaKey(), aud.GetFileSHA256(), aud.GetFileEncSHA256(), aud.GetFileLength()
	}

	if doc := msg.GetDocumentMessage(); doc != nil {
		fname := doc.GetFileName()
		if fname == "" {
			fname = "document_" + timestampStr
		}
		return "document", fname,
			doc.GetURL(), doc.GetMediaKey(), doc.GetFileSHA256(), doc.GetFileEncSHA256(), doc.GetFileLength()
	}

	return "", "", "", nil, nil, nil, 0
}

// AnalyzeOggOpus extracts duration and generates waveform for voice notes
func AnalyzeOggOpus(data []byte) (duration uint32, waveform []byte, err error) {
	if len(data) < 4 || string(data[0:4]) != "OggS" {
		return 0, nil, fmt.Errorf("not a valid Ogg file (missing OggS signature)")
	}

	var lastGranule uint64
	var sampleRate uint32 = 48000
	var preSkip uint16 = 0
	var foundOpusHead bool

	for i := 0; i < len(data); {
		if i+27 >= len(data) {
			break
		}
		if string(data[i:i+4]) != "OggS" {
			i++
			continue
		}

		granulePos := binary.LittleEndian.Uint64(data[i+6 : i+14])
		pageSeqNum := binary.LittleEndian.Uint32(data[i+18 : i+22])
		numSegments := int(data[i+26])

		if i+27+numSegments >= len(data) {
			break
		}
		segmentTable := data[i+27 : i+27+numSegments]

		pageSize := 27 + numSegments
		for _, segLen := range segmentTable {
			pageSize += int(segLen)
		}

		if !foundOpusHead && pageSeqNum <= 1 && i+pageSize <= len(data) {
			pageData := data[i : i+pageSize]
			headPos := bytes.Index(pageData, []byte("OpusHead"))
			if headPos >= 0 && headPos+16 < len(pageData) {
				headPos += 8
				preSkip = binary.LittleEndian.Uint16(pageData[headPos+10 : headPos+12])
				sampleRate = binary.LittleEndian.Uint32(pageData[headPos+12 : headPos+16])
				foundOpusHead = true
			}
		}

		if granulePos != 0 {
			lastGranule = granulePos
		}
		i += pageSize
	}

	if lastGranule > 0 && sampleRate > 0 {
		durationSeconds := float64(lastGranule-uint64(preSkip)) / float64(sampleRate)
		duration = uint32(math.Ceil(durationSeconds))
	} else {
		duration = uint32(float64(len(data)) / 2000.0)
	}

	if duration < 1 {
		duration = 1
	} else if duration > 300 {
		duration = 300
	}

	waveform = generatePlaceholderWaveform(duration)
	return duration, waveform, nil
}

func generatePlaceholderWaveform(duration uint32) []byte {
	const waveformLength = 64
	waveform := make([]byte, waveformLength)

	r := rand.New(rand.NewSource(int64(duration)))
	baseAmplitude := 35.0
	durFactor := math.Min(float64(duration), 120.0) / 30.0

	for i := range waveform {
		pos := float64(i) / float64(waveformLength)
		val := baseAmplitude*math.Sin(pos*math.Pi*durFactor*8) + (baseAmplitude/2)*math.Sin(pos*math.Pi*durFactor*16)
		val += (r.Float64() - 0.5) * 15
		fadeInOut := math.Sin(pos * math.Pi)
		val = val * (0.7 + 0.3*fadeInOut)
		val = val + 50

		if val < 0 {
			val = 0
		} else if val > 100 {
			val = 100
		}
		waveform[i] = byte(val)
	}
	return waveform
}
