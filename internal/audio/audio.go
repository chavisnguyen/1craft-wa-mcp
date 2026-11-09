package audio

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ConvertToOpusOgg converts any input audio file into WhatsApp-compatible Opus OGG format.
// Returns the path to the newly created temporary .ogg file.
func ConvertToOpusOgg(inputFile string) (string, error) {
	if _, err := os.Stat(inputFile); err != nil {
		return "", fmt.Errorf("input audio file not found: %w", err)
	}

	// Check if ffmpeg is installed
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return "", fmt.Errorf("ffmpeg is not installed or not found in PATH: %w", err)
	}

	// Create temporary output file
	tempDir := os.TempDir()
	baseName := strings.TrimSuffix(filepath.Base(inputFile), filepath.Ext(inputFile))
	outputFile := filepath.Join(tempDir, fmt.Sprintf("%s_converted_%d.ogg", baseName, os.Getpid()))

	// ffmpeg command optimized for WhatsApp Voice Note (PTT)
	cmd := exec.Command(ffmpegPath,
		"-i", inputFile,
		"-c:a", "libopus",
		"-b:a", "32k",
		"-ar", "24000",
		"-application", "voip",
		"-vbr", "on",
		"-compression_level", "10",
		"-frame_duration", "60",
		"-y",
		outputFile,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ffmpeg conversion failed: %s: %w", string(output), err)
	}

	return outputFile, nil
}
