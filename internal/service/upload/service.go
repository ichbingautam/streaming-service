package upload

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/streaming-service/internal/domain"
	"github.com/streaming-service/internal/media/processor"
	"github.com/streaming-service/internal/queue"
	"github.com/streaming-service/internal/repository/dynamodb"
	"github.com/streaming-service/internal/repository/s3"
	"github.com/streaming-service/pkg/logger"
)

// Service handles media upload operations
type Service struct {
	s3Client     *s3.Client
	dynamoClient *dynamodb.Client
	queue        queue.Queue
	log          *logger.Logger
}

// NewService creates a new upload service
func NewService(s3Client *s3.Client, dynamoClient *dynamodb.Client, log *logger.Logger) *Service {
	return &Service{
		s3Client:     s3Client,
		dynamoClient: dynamoClient,
		log:          log,
	}
}

// SetQueue sets the job queue for async processing
func (s *Service) SetQueue(q queue.Queue) {
	s.queue = q
}

// UploadRequest represents a media upload request
type UploadRequest struct {
	Title       string
	Description string
	UserID      string
	Filename    string
	ContentType string
	Body        io.Reader
}

// UploadResponse contains upload result
type UploadResponse struct {
	MediaID   string             `json:"media_id"`
	Status    domain.MediaStatus `json:"status"`
	UploadURL string             `json:"upload_url,omitempty"`
}

// Upload handles direct file upload
func (s *Service) Upload(ctx context.Context, req *UploadRequest) (*UploadResponse, error) {
	// Generate unique ID
	mediaID := uuid.New().String()

	// Detect media type
	mediaType := processor.DetectMediaType(req.Filename)

	// Create S3 key
	ext := filepath.Ext(req.Filename)
	s3Key := fmt.Sprintf("raw/%s%s", mediaID, ext)

	// Upload to S3
	if err := s.s3Client.UploadRaw(ctx, s3Key, req.Body, req.ContentType); err != nil {
		s.log.Error("failed to upload to S3", "error", err, "media_id", mediaID)
		return nil, fmt.Errorf("upload failed: %w", err)
	}

	// Create media record
	media := domain.NewMedia(mediaID, req.Title, req.UserID, mediaType)
	media.Description = req.Description
	media.SourceKey = s3Key
	media.SourceBucket = s.s3Client.GetRawBucket()
	media.SourceFormat = ext

	if err := s.dynamoClient.CreateMedia(ctx, media); err != nil {
		s.log.Error("failed to create media record", "error", err, "media_id", mediaID)
		// Clean up S3 on failure
		_ = s.s3Client.Delete(ctx, s.s3Client.GetRawBucket(), s3Key)
		return nil, fmt.Errorf("failed to create media record: %w", err)
	}

	// Queue transcoding job
	if s.queue != nil {
		job := &queue.Job{
			ID:       uuid.New().String(),
			Type:     queue.JobTypeTranscode,
			MediaID:  mediaID,
			Priority: 1,
			Payload: map[string]string{
				"source_key":    s3Key,
				"source_bucket": s.s3Client.GetRawBucket(),
			},
		}
		if err := s.queue.Enqueue(ctx, job); err != nil {
			s.log.Error("failed to enqueue job", "error", err, "media_id", mediaID)
			// Don't fail the upload, processing can be retried
		}
	}

	s.log.Info("media uploaded", "media_id", mediaID, "type", mediaType)

	return &UploadResponse{
		MediaID: mediaID,
		Status:  domain.MediaStatusPending,
	}, nil
}

// GetPresignedUploadURL generates a presigned URL for client-side upload
func (s *Service) GetPresignedUploadURL(ctx context.Context, userID, filename, contentType string) (*UploadResponse, error) {
	mediaID := uuid.New().String()
	ext := filepath.Ext(filename)
	s3Key := fmt.Sprintf("raw/%s%s", mediaID, ext)

	// Generate presigned URL (valid for 1 hour)
	url, err := s.s3Client.GetPresignedUploadURL(ctx, s3Key, contentType, time.Hour)
	if err != nil {
		return nil, fmt.Errorf("failed to generate upload URL: %w", err)
	}

	return &UploadResponse{
		MediaID:   mediaID,
		Status:    domain.MediaStatusPending,
		UploadURL: url,
	}, nil
}

// ConfirmUpload confirms a presigned URL upload and triggers processing
func (s *Service) ConfirmUpload(ctx context.Context, req *UploadRequest, mediaID string) (*UploadResponse, error) {
	mediaType := processor.DetectMediaType(req.Filename)
	ext := filepath.Ext(req.Filename)
	s3Key := fmt.Sprintf("raw/%s%s", mediaID, ext)

	// Create media record
	media := domain.NewMedia(mediaID, req.Title, req.UserID, mediaType)
	media.Description = req.Description
	media.SourceKey = s3Key
	media.SourceBucket = s.s3Client.GetRawBucket()
	media.SourceFormat = ext

	if err := s.dynamoClient.CreateMedia(ctx, media); err != nil {
		return nil, fmt.Errorf("failed to create media record: %w", err)
	}

	// Queue transcoding job
	if s.queue != nil {
		job := &queue.Job{
			ID:       uuid.New().String(),
			Type:     queue.JobTypeTranscode,
			MediaID:  mediaID,
			Priority: 1,
			Payload: map[string]string{
				"source_key":    s3Key,
				"source_bucket": s.s3Client.GetRawBucket(),
			},
		}
		if err := s.queue.Enqueue(ctx, job); err != nil {
			s.log.Error("failed to enqueue job", "error", err, "media_id", mediaID)
		}
	}

	return &UploadResponse{
		MediaID: mediaID,
		Status:  domain.MediaStatusPending,
	}, nil
}
