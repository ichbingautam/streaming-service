package processor

import (
	"context"
	"fmt"
)

// TranscodeStrategy defines the interface for transcoding strategies
// Strategy Pattern: Different strategies for different output formats/quality levels
type TranscodeStrategy interface {
	// GetName returns the strategy name
	GetName() string
	// GetProfile returns the profile configuration
	GetProfile() ProfileConfig
	// BuildCommand builds the FFMPEG command arguments
	BuildCommand(input, outputDir string) []string
}

// HLSTranscodeStrategy implements transcoding to HLS format
type HLSTranscodeStrategy struct {
	profile         ProfileConfig
	segmentDuration int
}

// NewHLSTranscodeStrategy creates a new HLS transcoding strategy
func NewHLSTranscodeStrategy(profile ProfileConfig, segmentDuration int) *HLSTranscodeStrategy {
	return &HLSTranscodeStrategy{
		profile:         profile,
		segmentDuration: segmentDuration,
	}
}

func (s *HLSTranscodeStrategy) GetName() string {
	return s.profile.Name
}

func (s *HLSTranscodeStrategy) GetProfile() ProfileConfig {
	return s.profile
}

func (s *HLSTranscodeStrategy) BuildCommand(input, outputDir string) []string {
	playlistPath := fmt.Sprintf("%s/%s/playlist.m3u8", outputDir, s.profile.Name)
	segmentPath := fmt.Sprintf("%s/%s/segment_%%04d.ts", outputDir, s.profile.Name)

	return []string{
		"-i", input,
		"-vf", fmt.Sprintf("scale=%d:%d", s.profile.Width, s.profile.Height),
		"-c:v", s.profile.Codec,
		"-b:v", s.profile.VideoBitrate,
		"-c:a", "aac",
		"-b:a", s.profile.AudioBitrate,
		"-hls_time", fmt.Sprintf("%d", s.segmentDuration),
		"-hls_list_size", "0",
		"-hls_segment_filename", segmentPath,
		"-f", "hls",
		playlistPath,
	}
}

// AudioTranscodeStrategy implements transcoding for audio-only content
type AudioTranscodeStrategy struct {
	profile ProfileConfig
}

// NewAudioTranscodeStrategy creates a new audio transcoding strategy
func NewAudioTranscodeStrategy(profile ProfileConfig) *AudioTranscodeStrategy {
	return &AudioTranscodeStrategy{
		profile: profile,
	}
}

func (s *AudioTranscodeStrategy) GetName() string {
	return s.profile.Name
}

func (s *AudioTranscodeStrategy) GetProfile() ProfileConfig {
	return s.profile
}

func (s *AudioTranscodeStrategy) BuildCommand(input, outputDir string) []string {
	outputPath := fmt.Sprintf("%s/%s/audio.m4a", outputDir, s.profile.Name)

	return []string{
		"-i", input,
		"-vn", // No video
		"-c:a", "aac",
		"-b:a", s.profile.AudioBitrate,
		outputPath,
	}
}

// AudioHLSTranscodeStrategy implements HLS transcoding for audio
type AudioHLSTranscodeStrategy struct {
	profile         ProfileConfig
	segmentDuration int
}

// NewAudioHLSTranscodeStrategy creates a new audio HLS transcoding strategy
func NewAudioHLSTranscodeStrategy(profile ProfileConfig, segmentDuration int) *AudioHLSTranscodeStrategy {
	return &AudioHLSTranscodeStrategy{
		profile:         profile,
		segmentDuration: segmentDuration,
	}
}

func (s *AudioHLSTranscodeStrategy) GetName() string {
	return s.profile.Name
}

func (s *AudioHLSTranscodeStrategy) GetProfile() ProfileConfig {
	return s.profile
}

func (s *AudioHLSTranscodeStrategy) BuildCommand(input, outputDir string) []string {
	playlistPath := fmt.Sprintf("%s/%s/playlist.m3u8", outputDir, s.profile.Name)
	segmentPath := fmt.Sprintf("%s/%s/segment_%%04d.aac", outputDir, s.profile.Name)

	return []string{
		"-i", input,
		"-vn", // No video
		"-c:a", "aac",
		"-b:a", s.profile.AudioBitrate,
		"-hls_time", fmt.Sprintf("%d", s.segmentDuration),
		"-hls_list_size", "0",
		"-hls_segment_filename", segmentPath,
		"-f", "hls",
		playlistPath,
	}
}

// StrategyExecutor manages and executes transcoding strategies
type StrategyExecutor struct {
	strategies []TranscodeStrategy
}

// NewStrategyExecutor creates a new strategy executor
func NewStrategyExecutor() *StrategyExecutor {
	return &StrategyExecutor{
		strategies: make([]TranscodeStrategy, 0),
	}
}

// AddStrategy adds a transcoding strategy
func (e *StrategyExecutor) AddStrategy(strategy TranscodeStrategy) {
	e.strategies = append(e.strategies, strategy)
}

// GetStrategies returns all registered strategies
func (e *StrategyExecutor) GetStrategies() []TranscodeStrategy {
	return e.strategies
}

// Execute runs all strategies in sequence (can be parallelized)
func (e *StrategyExecutor) Execute(ctx context.Context, input string, outputDir string, executor CommandExecutor) ([]RenditionOutput, error) {
	results := make([]RenditionOutput, 0, len(e.strategies))

	for _, strategy := range e.strategies {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		args := strategy.BuildCommand(input, outputDir)
		if err := executor.Execute(ctx, args); err != nil {
			return nil, fmt.Errorf("strategy %s failed: %w", strategy.GetName(), err)
		}

		profile := strategy.GetProfile()
		result := RenditionOutput{
			Name:         profile.Name,
			Width:        profile.Width,
			Height:       profile.Height,
			Codec:        profile.Codec,
			PlaylistPath: fmt.Sprintf("%s/%s/playlist.m3u8", outputDir, profile.Name),
		}
		results = append(results, result)
	}

	return results, nil
}

// CommandExecutor interface for executing commands
type CommandExecutor interface {
	Execute(ctx context.Context, args []string) error
}
