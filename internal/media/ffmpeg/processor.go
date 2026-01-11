package ffmpeg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/streaming-service/internal/config"
	"github.com/streaming-service/internal/domain"
	"github.com/streaming-service/internal/media/processor"
)

// Processor implements MediaProcessor using FFMPEG
type Processor struct {
	binaryPath      string
	probePath       string
	tempDir         string
	segmentDuration int
	profiles        []config.TranscodeProfile
}

// NewProcessor creates a new FFMPEG processor
func NewProcessor(cfg config.FFMPEGConfig) *Processor {
	// Ensure temp directory exists
	_ = os.MkdirAll(cfg.TempDir, 0755)

	return &Processor{
		binaryPath:      cfg.BinaryPath,
		probePath:       strings.Replace(cfg.BinaryPath, "ffmpeg", "ffprobe", 1),
		tempDir:         cfg.TempDir,
		segmentDuration: cfg.SegmentDuration,
		profiles:        cfg.Profiles,
	}
}

// Process processes the input media file
func (p *Processor) Process(ctx context.Context, input *processor.ProcessInput) (*processor.ProcessOutput, error) {
	// Create output directory
	outputDir := filepath.Join(p.tempDir, input.MediaID)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Get media info
	info, err := p.probe(ctx, input.SourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to probe media: %w", err)
	}

	// Create strategy executor
	executor := processor.NewStrategyExecutor()

	// Add strategies based on profiles
	for _, profile := range input.Profiles {
		strategy := processor.NewHLSTranscodeStrategy(profile, p.segmentDuration)
		executor.AddStrategy(strategy)
	}

	// Create command executor
	cmdExecutor := &ffmpegExecutor{binaryPath: p.binaryPath}

	// Execute all strategies
	renditions, err := executor.Execute(ctx, input.SourcePath, outputDir, cmdExecutor)
	if err != nil {
		return nil, fmt.Errorf("transcoding failed: %w", err)
	}

	// Generate master playlist
	masterPath := filepath.Join(outputDir, "master.m3u8")
	if err := p.generateMasterPlaylist(masterPath, renditions); err != nil {
		return nil, fmt.Errorf("failed to generate master playlist: %w", err)
	}

	return &processor.ProcessOutput{
		MediaID:    input.MediaID,
		Renditions: renditions,
		Duration:   info.Duration,
		MasterPath: masterPath,
		Metadata: map[string]interface{}{
			"width":      info.Width,
			"height":     info.Height,
			"bitrate":    info.Bitrate,
			"codec":      info.Codec,
			"frame_rate": info.FrameRate,
		},
	}, nil
}

// GetSupportedFormats returns supported input formats
func (p *Processor) GetSupportedFormats() []string {
	return []string{
		"mp4", "mov", "avi", "mkv", "webm", "flv", "wmv", "m4v",
	}
}

// GetType returns the media type this processor handles
func (p *Processor) GetType() domain.MediaType {
	return domain.MediaTypeVideo
}

// MediaInfo contains probe results
type MediaInfo struct {
	Duration  float64
	Width     int
	Height    int
	Bitrate   int
	Codec     string
	FrameRate float64
}

// probe gets media information using ffprobe
func (p *Processor) probe(ctx context.Context, path string) (*MediaInfo, error) {
	args := []string{
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		path,
	}

	cmd := exec.CommandContext(ctx, p.probePath, args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe failed: %w", err)
	}

	var probeResult struct {
		Streams []struct {
			CodecType  string `json:"codec_type"`
			CodecName  string `json:"codec_name"`
			Width      int    `json:"width"`
			Height     int    `json:"height"`
			RFrameRate string `json:"r_frame_rate"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
			BitRate  string `json:"bit_rate"`
		} `json:"format"`
	}

	if err := json.Unmarshal(output, &probeResult); err != nil {
		return nil, fmt.Errorf("failed to parse probe result: %w", err)
	}

	info := &MediaInfo{}

	// Parse duration
	if dur, err := strconv.ParseFloat(probeResult.Format.Duration, 64); err == nil {
		info.Duration = dur
	}

	// Parse bitrate
	if br, err := strconv.Atoi(probeResult.Format.BitRate); err == nil {
		info.Bitrate = br
	}

	// Find video stream
	for _, stream := range probeResult.Streams {
		if stream.CodecType == "video" {
			info.Width = stream.Width
			info.Height = stream.Height
			info.Codec = stream.CodecName

			// Parse frame rate (format: "30000/1001" or "30/1")
			if parts := strings.Split(stream.RFrameRate, "/"); len(parts) == 2 {
				num, _ := strconv.ParseFloat(parts[0], 64)
				den, _ := strconv.ParseFloat(parts[1], 64)
				if den > 0 {
					info.FrameRate = num / den
				}
			}
			break
		}
	}

	return info, nil
}

// generateMasterPlaylist creates the master HLS playlist
func (p *Processor) generateMasterPlaylist(path string, renditions []processor.RenditionOutput) error {
	var buf bytes.Buffer
	buf.WriteString("#EXTM3U\n")
	buf.WriteString("#EXT-X-VERSION:3\n")

	for _, r := range renditions {
		bandwidth := r.Bitrate
		if bandwidth == 0 {
			// Estimate bandwidth from name
			switch r.Name {
			case "1080p":
				bandwidth = 5000000
			case "720p":
				bandwidth = 2500000
			case "480p":
				bandwidth = 1000000
			case "360p":
				bandwidth = 500000
			default:
				bandwidth = 1000000
			}
		}

		buf.WriteString(fmt.Sprintf("#EXT-X-STREAM-INF:BANDWIDTH=%d,RESOLUTION=%dx%d\n",
			bandwidth, r.Width, r.Height))
		buf.WriteString(fmt.Sprintf("%s/playlist.m3u8\n", r.Name))
	}

	return os.WriteFile(path, buf.Bytes(), 0644)
}

// ffmpegExecutor implements CommandExecutor for FFMPEG
type ffmpegExecutor struct {
	binaryPath string
}

func (e *ffmpegExecutor) Execute(ctx context.Context, args []string) error {
	cmd := exec.CommandContext(ctx, e.binaryPath, args...)
	cmd.Stderr = os.Stderr // Log FFMPEG errors

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg command failed: %w", err)
	}
	return nil
}
