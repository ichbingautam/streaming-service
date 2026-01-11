package stream

import (
	"context"
	"fmt"
	"time"

	"github.com/streaming-service/internal/domain"
	"github.com/streaming-service/internal/repository/dynamodb"
	"github.com/streaming-service/internal/repository/s3"
	"github.com/streaming-service/pkg/logger"
)

// Service handles streaming operations
type Service struct {
	s3Client         *s3.Client
	dynamoClient     *dynamodb.Client
	cloudFrontDomain string
	log              *logger.Logger
}

// NewService creates a new streaming service
func NewService(s3Client *s3.Client, dynamoClient *dynamodb.Client, cloudFrontDomain string, log *logger.Logger) *Service {
	return &Service{
		s3Client:         s3Client,
		dynamoClient:     dynamoClient,
		cloudFrontDomain: cloudFrontDomain,
		log:              log,
	}
}

// MediaInfo contains media information for playback
type MediaInfo struct {
	ID          string             `json:"id"`
	Title       string             `json:"title"`
	Description string             `json:"description"`
	Type        domain.MediaType   `json:"type"`
	Status      domain.MediaStatus `json:"status"`
	Duration    float64            `json:"duration"`
	Renditions  []RenditionInfo    `json:"renditions,omitempty"`
	PlaybackURL string             `json:"playback_url,omitempty"`
	CreatedAt   time.Time          `json:"created_at"`
}

// RenditionInfo contains rendition details
type RenditionInfo struct {
	Name      string `json:"name"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
	Bitrate   int    `json:"bitrate"`
	StreamURL string `json:"stream_url"`
}

// GetMedia retrieves media information
func (s *Service) GetMedia(ctx context.Context, mediaID string) (*MediaInfo, error) {
	media, err := s.dynamoClient.GetMedia(ctx, mediaID)
	if err != nil {
		return nil, err
	}

	info := &MediaInfo{
		ID:          media.ID,
		Title:       media.Title,
		Description: media.Description,
		Type:        media.Type,
		Status:      media.Status,
		Duration:    media.Duration,
		CreatedAt:   media.CreatedAt,
	}

	// Add playback URL if processed
	if media.IsProcessed() {
		info.PlaybackURL = s.buildPlaybackURL(media.GetMasterPlaylistKey())

		for _, r := range media.Renditions {
			info.Renditions = append(info.Renditions, RenditionInfo{
				Name:      r.Name,
				Width:     r.Width,
				Height:    r.Height,
				Bitrate:   r.Bitrate,
				StreamURL: s.buildPlaybackURL(r.PlaylistKey),
			})
		}
	}

	return info, nil
}

// GetPlaybackURL returns the playback URL for a media item
func (s *Service) GetPlaybackURL(ctx context.Context, mediaID string) (string, error) {
	media, err := s.dynamoClient.GetMedia(ctx, mediaID)
	if err != nil {
		return "", err
	}

	if !media.IsProcessed() {
		return "", fmt.Errorf("media not yet processed")
	}

	return s.buildPlaybackURL(media.GetMasterPlaylistKey()), nil
}

// ListMedia lists media for a user
func (s *Service) ListMedia(ctx context.Context, userID string, limit int32) ([]*MediaInfo, error) {
	mediaList, err := s.dynamoClient.ListMediaByUser(ctx, userID, limit)
	if err != nil {
		return nil, err
	}

	result := make([]*MediaInfo, 0, len(mediaList))
	for _, media := range mediaList {
		info := &MediaInfo{
			ID:          media.ID,
			Title:       media.Title,
			Description: media.Description,
			Type:        media.Type,
			Status:      media.Status,
			Duration:    media.Duration,
			CreatedAt:   media.CreatedAt,
		}

		if media.IsProcessed() {
			info.PlaybackURL = s.buildPlaybackURL(media.GetMasterPlaylistKey())
		}

		result = append(result, info)
	}

	return result, nil
}

// DeleteMedia deletes a media item
func (s *Service) DeleteMedia(ctx context.Context, mediaID, userID string) error {
	// Get media to verify ownership
	media, err := s.dynamoClient.GetMedia(ctx, mediaID)
	if err != nil {
		return err
	}

	if media.UserID != userID {
		return domain.ErrUnauthorized
	}

	// Delete from DynamoDB
	if err := s.dynamoClient.DeleteMedia(ctx, mediaID); err != nil {
		return fmt.Errorf("failed to delete media record: %w", err)
	}

	// Delete source file from S3
	if media.SourceKey != "" {
		if err := s.s3Client.Delete(ctx, media.SourceBucket, media.SourceKey); err != nil {
			s.log.Error("failed to delete source file", "error", err, "key", media.SourceKey)
		}
	}

	// Delete processed files
	processedBucket := s.s3Client.GetProcessedBucket()
	objects, err := s.s3Client.ListObjects(ctx, processedBucket, mediaID+"/")
	if err == nil {
		for _, obj := range objects {
			_ = s.s3Client.Delete(ctx, processedBucket, *obj.Key)
		}
	}

	s.log.Info("media deleted", "media_id", mediaID)

	return nil
}

// buildPlaybackURL constructs the CloudFront playback URL
func (s *Service) buildPlaybackURL(key string) string {
	if s.cloudFrontDomain == "" {
		return "" // No CDN configured
	}
	return fmt.Sprintf("https://%s/%s", s.cloudFrontDomain, key)
}
