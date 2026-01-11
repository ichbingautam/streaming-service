package ffmpeg

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/streaming-service/internal/config"
	"github.com/streaming-service/internal/domain"
	"github.com/streaming-service/internal/media/processor"
)

// AudioProcessor implements MediaProcessor for audio files
type AudioProcessor struct {
	binaryPath      string
	tempDir         string
	segmentDuration int
}

// NewAudioProcessor creates a new audio processor
func NewAudioProcessor(cfg config.FFMPEGConfig) *AudioProcessor {
	_ = os.MkdirAll(cfg.TempDir, 0755)

	return &AudioProcessor{
		binaryPath:      cfg.BinaryPath,
		tempDir:         cfg.TempDir,
		segmentDuration: cfg.SegmentDuration,
	}
}

// Process processes the input audio file
func (p *AudioProcessor) Process(ctx context.Context, input *processor.ProcessInput) (*processor.ProcessOutput, error) {
	// Create output directory
	outputDir := filepath.Join(p.tempDir, input.MediaID)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Create strategy executor
	executor := processor.NewStrategyExecutor()

	// Add audio-specific strategies
	audioProfiles := []processor.ProfileConfig{
		{Name: "high", AudioBitrate: "320k"},
		{Name: "medium", AudioBitrate: "192k"},
		{Name: "low", AudioBitrate: "96k"},
	}

	for _, profile := range audioProfiles {
		strategy := processor.NewAudioHLSTranscodeStrategy(profile, p.segmentDuration)
		executor.AddStrategy(strategy)
	}

	// Create command executor
	cmdExecutor := &ffmpegExecutor{binaryPath: p.binaryPath}

	// Execute all strategies
	renditions, err := executor.Execute(ctx, input.SourcePath, outputDir, cmdExecutor)
	if err != nil {
		return nil, fmt.Errorf("audio transcoding failed: %w", err)
	}

	// Generate master playlist
	masterPath := filepath.Join(outputDir, "master.m3u8")
	if err := p.generateAudioMasterPlaylist(masterPath, renditions); err != nil {
		return nil, fmt.Errorf("failed to generate master playlist: %w", err)
	}

	return &processor.ProcessOutput{
		MediaID:    input.MediaID,
		Renditions: renditions,
		MasterPath: masterPath,
	}, nil
}

// GetSupportedFormats returns supported audio formats
func (p *AudioProcessor) GetSupportedFormats() []string {
	return []string{
		"mp3", "aac", "wav", "flac", "ogg", "m4a", "wma", "opus",
	}
}

// GetType returns the media type this processor handles
func (p *AudioProcessor) GetType() domain.MediaType {
	return domain.MediaTypeAudio
}

// generateAudioMasterPlaylist creates the master HLS playlist for audio
func (p *AudioProcessor) generateAudioMasterPlaylist(path string, renditions []processor.RenditionOutput) error {
	var buf bytes.Buffer
	buf.WriteString("#EXTM3U\n")
	buf.WriteString("#EXT-X-VERSION:3\n")

	for _, r := range renditions {
		bandwidth := 320000 // Default
		switch r.Name {
		case "high":
			bandwidth = 320000
		case "medium":
			bandwidth = 192000
		case "low":
			bandwidth = 96000
		}

		buf.WriteString(fmt.Sprintf("#EXT-X-STREAM-INF:BANDWIDTH=%d\n", bandwidth))
		buf.WriteString(fmt.Sprintf("%s/playlist.m3u8\n", r.Name))
	}

	return os.WriteFile(path, buf.Bytes(), 0644)
}
