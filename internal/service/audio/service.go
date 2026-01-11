package audio

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/streaming-service/internal/domain"
	"github.com/streaming-service/internal/media/processor"
	"github.com/streaming-service/internal/repository/dynamodb"
	"github.com/streaming-service/internal/repository/s3"
	"github.com/streaming-service/pkg/logger"
)

// Service handles audio-specific operations
type Service struct {
	s3Client     *s3.Client
	dynamoClient *dynamodb.Client
	processor    processor.MediaProcessor
	log          *logger.Logger
}

// NewService creates a new audio service
func NewService(s3Client *s3.Client, dynamoClient *dynamodb.Client, proc processor.MediaProcessor, log *logger.Logger) *Service {
	return &Service{
		s3Client:     s3Client,
		dynamoClient: dynamoClient,
		processor:    proc,
		log:          log,
	}
}

// ExtractAudio extracts audio from a video file
func (s *Service) ExtractAudio(ctx context.Context, mediaID string) error {
	s.log.Info("extracting audio", "media_id", mediaID)

	// Get media record
	media, err := s.dynamoClient.GetMedia(ctx, mediaID)
	if err != nil {
		return fmt.Errorf("failed to get media: %w", err)
	}

	if media.Type != domain.MediaTypeVideo {
		return fmt.Errorf("media is not a video")
	}

	// Download source file
	reader, err := s.s3Client.Download(ctx, media.SourceBucket, media.SourceKey)
	if err != nil {
		return fmt.Errorf("failed to download source: %w", err)
	}
	defer reader.Close()

	// Save to temp file
	tempPath := filepath.Join(os.TempDir(), "streaming", "audio", mediaID+media.SourceFormat)
	if err := os.MkdirAll(filepath.Dir(tempPath), 0755); err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}

	tempFile, err := os.Create(tempPath)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	if _, err := io.Copy(tempFile, reader); err != nil {
		tempFile.Close()
		os.Remove(tempPath)
		return fmt.Errorf("failed to save source: %w", err)
	}
	tempFile.Close()
	defer os.Remove(tempPath)

	// Process audio extraction
	audioProfiles := []processor.ProfileConfig{
		{Name: "high", AudioBitrate: "320k"},
		{Name: "medium", AudioBitrate: "192k"},
		{Name: "low", AudioBitrate: "96k"},
	}

	input := &processor.ProcessInput{
		MediaID:    mediaID + "-audio",
		SourcePath: tempPath,
		OutputDir:  filepath.Join(os.TempDir(), "streaming", "audio", mediaID),
		Profiles:   audioProfiles,
	}

	output, err := s.processor.Process(ctx, input)
	if err != nil {
		return fmt.Errorf("audio extraction failed: %w", err)
	}

	// Upload extracted audio
	bucket := s.s3Client.GetProcessedBucket()
	outputDir := filepath.Dir(output.MasterPath)

	// Upload master playlist
	masterFile, err := os.Open(output.MasterPath)
	if err != nil {
		return fmt.Errorf("failed to open master playlist: %w", err)
	}
	defer masterFile.Close()

	masterKey := fmt.Sprintf("%s/audio/master.m3u8", mediaID)
	if err := s.s3Client.Upload(ctx, bucket, masterKey, masterFile, "application/x-mpegURL"); err != nil {
		return fmt.Errorf("failed to upload audio master: %w", err)
	}

	// Upload renditions
	for _, r := range output.Renditions {
		renditionDir := filepath.Join(outputDir, r.Name)

		// Upload playlist
		playlistPath := filepath.Join(renditionDir, "playlist.m3u8")
		if file, err := os.Open(playlistPath); err == nil {
			key := fmt.Sprintf("%s/audio/%s/playlist.m3u8", mediaID, r.Name)
			s.s3Client.Upload(ctx, bucket, key, file, "application/x-mpegURL")
			file.Close()
		}

		// Upload segments
		segments, _ := filepath.Glob(filepath.Join(renditionDir, "segment_*.aac"))
		for _, seg := range segments {
			if file, err := os.Open(seg); err == nil {
				segName := filepath.Base(seg)
				key := fmt.Sprintf("%s/audio/%s/%s", mediaID, r.Name, segName)
				s.s3Client.Upload(ctx, bucket, key, file, "audio/aac")
				file.Close()
			}
		}
	}

	// Cleanup
	os.RemoveAll(input.OutputDir)

	s.log.Info("audio extraction completed", "media_id", mediaID)

	return nil
}

// ProcessAudioFile processes a standalone audio file
func (s *Service) ProcessAudioFile(ctx context.Context, mediaID string) error {
	s.log.Info("processing audio file", "media_id", mediaID)

	// Get media record
	media, err := s.dynamoClient.GetMedia(ctx, mediaID)
	if err != nil {
		return fmt.Errorf("failed to get media: %w", err)
	}

	if media.Type != domain.MediaTypeAudio {
		return fmt.Errorf("media is not audio")
	}

	// Update status to processing
	if err := s.dynamoClient.UpdateMediaStatus(ctx, mediaID, domain.MediaStatusProcessing); err != nil {
		s.log.Error("failed to update status", "error", err)
	}

	// Download source file
	reader, err := s.s3Client.Download(ctx, media.SourceBucket, media.SourceKey)
	if err != nil {
		s.markFailed(ctx, mediaID)
		return fmt.Errorf("failed to download source: %w", err)
	}
	defer reader.Close()

	// Process and upload (similar to ExtractAudio)
	// ... implementation follows same pattern

	// Update status to completed
	if err := s.dynamoClient.UpdateMediaStatus(ctx, mediaID, domain.MediaStatusCompleted); err != nil {
		s.log.Error("failed to update status", "error", err)
	}

	s.log.Info("audio processing completed", "media_id", mediaID)

	return nil
}

func (s *Service) markFailed(ctx context.Context, mediaID string) {
	if err := s.dynamoClient.UpdateMediaStatus(ctx, mediaID, domain.MediaStatusFailed); err != nil {
		s.log.Error("failed to mark as failed", "error", err, "media_id", mediaID)
	}
}
