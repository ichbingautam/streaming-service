package processor

import (
	"context"
	"fmt"
	"io"

	"github.com/streaming-service/internal/domain"
)

// MediaProcessor defines the interface for processing media files
type MediaProcessor interface {
	// Process processes the input media and returns output paths
	Process(ctx context.Context, input *ProcessInput) (*ProcessOutput, error)
	// GetSupportedFormats returns the formats this processor can handle
	GetSupportedFormats() []string
	// GetType returns the media type this processor handles
	GetType() domain.MediaType
}

// ProcessInput represents input for media processing
type ProcessInput struct {
	MediaID      string
	SourcePath   string
	SourceReader io.Reader
	OutputDir    string
	Profiles     []ProfileConfig
}

// ProfileConfig defines a processing profile
type ProfileConfig struct {
	Name         string
	Width        int
	Height       int
	VideoBitrate string
	AudioBitrate string
	Codec        string
}

// ProcessOutput represents the output of media processing
type ProcessOutput struct {
	MediaID    string
	Renditions []RenditionOutput
	Duration   float64
	MasterPath string
	Metadata   map[string]interface{}
}

// RenditionOutput represents a single rendition output
type RenditionOutput struct {
	Name         string
	Width        int
	Height       int
	Bitrate      int
	Codec        string
	PlaylistPath string
	SegmentPaths []string
}

// Factory Pattern: ProcessorFactory creates appropriate processors based on media type
type ProcessorFactory struct {
	videoProcessor MediaProcessor
	audioProcessor MediaProcessor
}

// NewProcessorFactory creates a new processor factory
func NewProcessorFactory(videoProcessor, audioProcessor MediaProcessor) *ProcessorFactory {
	return &ProcessorFactory{
		videoProcessor: videoProcessor,
		audioProcessor: audioProcessor,
	}
}

// CreateProcessor returns the appropriate processor for the given media type
func (f *ProcessorFactory) CreateProcessor(mediaType domain.MediaType) (MediaProcessor, error) {
	switch mediaType {
	case domain.MediaTypeVideo:
		if f.videoProcessor == nil {
			return nil, fmt.Errorf("video processor not configured")
		}
		return f.videoProcessor, nil
	case domain.MediaTypeAudio:
		if f.audioProcessor == nil {
			return nil, fmt.Errorf("audio processor not configured")
		}
		return f.audioProcessor, nil
	default:
		return nil, fmt.Errorf("unsupported media type: %s", mediaType)
	}
}

// DetectMediaType detects the media type from file extension
func DetectMediaType(filename string) domain.MediaType {
	videoExtensions := map[string]bool{
		".mp4": true, ".mov": true, ".avi": true, ".mkv": true,
		".webm": true, ".flv": true, ".wmv": true, ".m4v": true,
	}
	audioExtensions := map[string]bool{
		".mp3": true, ".aac": true, ".wav": true, ".flac": true,
		".ogg": true, ".m4a": true, ".wma": true, ".opus": true,
	}

	ext := getExtension(filename)
	if videoExtensions[ext] {
		return domain.MediaTypeVideo
	}
	if audioExtensions[ext] {
		return domain.MediaTypeAudio
	}
	return domain.MediaTypeVideo // Default to video
}

func getExtension(filename string) string {
	for i := len(filename) - 1; i >= 0; i-- {
		if filename[i] == '.' {
			return filename[i:]
		}
	}
	return ""
}
