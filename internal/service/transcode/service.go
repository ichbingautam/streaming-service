package transcode

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/streaming-service/internal/domain"
	"github.com/streaming-service/internal/media/processor"
	"github.com/streaming-service/internal/queue"
	"github.com/streaming-service/internal/repository/dynamodb"
	"github.com/streaming-service/internal/repository/s3"
	"github.com/streaming-service/pkg/logger"
)

// Service handles transcoding operations
type Service struct {
	s3Client     *s3.Client
	dynamoClient *dynamodb.Client
	processor    processor.MediaProcessor
	log          *logger.Logger
}

// NewService creates a new transcode service
func NewService(s3Client *s3.Client, dynamoClient *dynamodb.Client, proc processor.MediaProcessor, log *logger.Logger) *Service {
	return &Service{
		s3Client:     s3Client,
		dynamoClient: dynamoClient,
		processor:    proc,
		log:          log,
	}
}

// ProcessMedia processes a media file
func (s *Service) ProcessMedia(ctx context.Context, mediaID string) error {
	s.log.Info("starting media processing", "media_id", mediaID)

	// Get media record
	media, err := s.dynamoClient.GetMedia(ctx, mediaID)
	if err != nil {
		return fmt.Errorf("failed to get media: %w", err)
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

	// Save to temp file
	tempPath := filepath.Join(os.TempDir(), "streaming", mediaID+media.SourceFormat)
	if err := os.MkdirAll(filepath.Dir(tempPath), 0755); err != nil {
		s.markFailed(ctx, mediaID)
		return fmt.Errorf("failed to create temp dir: %w", err)
	}

	tempFile, err := os.Create(tempPath)
	if err != nil {
		s.markFailed(ctx, mediaID)
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	if _, err := io.Copy(tempFile, reader); err != nil {
		tempFile.Close()
		os.Remove(tempPath)
		s.markFailed(ctx, mediaID)
		return fmt.Errorf("failed to save source: %w", err)
	}
	tempFile.Close()
	defer os.Remove(tempPath)

	// Configure processing profiles
	profiles := []processor.ProfileConfig{
		{Name: "1080p", Width: 1920, Height: 1080, VideoBitrate: "5000k", AudioBitrate: "192k", Codec: "h264"},
		{Name: "720p", Width: 1280, Height: 720, VideoBitrate: "2500k", AudioBitrate: "128k", Codec: "h264"},
		{Name: "480p", Width: 854, Height: 480, VideoBitrate: "1000k", AudioBitrate: "96k", Codec: "h264"},
		{Name: "360p", Width: 640, Height: 360, VideoBitrate: "500k", AudioBitrate: "64k", Codec: "h264"},
	}

	// Process media
	input := &processor.ProcessInput{
		MediaID:    mediaID,
		SourcePath: tempPath,
		OutputDir:  filepath.Join(os.TempDir(), "streaming", mediaID),
		Profiles:   profiles,
	}

	output, err := s.processor.Process(ctx, input)
	if err != nil {
		s.markFailed(ctx, mediaID)
		return fmt.Errorf("processing failed: %w", err)
	}

	// Upload processed files to S3
	if err := s.uploadProcessedFiles(ctx, mediaID, output); err != nil {
		s.markFailed(ctx, mediaID)
		return fmt.Errorf("failed to upload processed files: %w", err)
	}

	// Update media record with renditions
	for _, r := range output.Renditions {
		rendition := domain.Rendition{
			Name:        r.Name,
			Width:       r.Width,
			Height:      r.Height,
			Bitrate:     r.Bitrate,
			Codec:       r.Codec,
			PlaylistKey: fmt.Sprintf("%s/%s/playlist.m3u8", mediaID, r.Name),
		}
		if err := s.dynamoClient.AddRendition(ctx, mediaID, rendition); err != nil {
			s.log.Error("failed to add rendition", "error", err, "rendition", r.Name)
		}
	}

	// Update status to completed
	if err := s.dynamoClient.UpdateMediaStatus(ctx, mediaID, domain.MediaStatusCompleted); err != nil {
		s.log.Error("failed to update status", "error", err)
	}

	// Cleanup temp files
	os.RemoveAll(input.OutputDir)

	s.log.Info("media processing completed", "media_id", mediaID)

	return nil
}

// uploadProcessedFiles uploads all processed HLS files to S3
func (s *Service) uploadProcessedFiles(ctx context.Context, mediaID string, output *processor.ProcessOutput) error {
	bucket := s.s3Client.GetProcessedBucket()
	outputDir := filepath.Dir(output.MasterPath)

	// Upload master playlist
	masterFile, err := os.Open(output.MasterPath)
	if err != nil {
		return fmt.Errorf("failed to open master playlist: %w", err)
	}
	defer masterFile.Close()

	masterKey := mediaID + "/master.m3u8"
	if err := s.s3Client.Upload(ctx, bucket, masterKey, masterFile, "application/x-mpegURL"); err != nil {
		return fmt.Errorf("failed to upload master playlist: %w", err)
	}

	// Upload each rendition
	for _, r := range output.Renditions {
		renditionDir := filepath.Join(outputDir, r.Name)

		// Upload playlist
		playlistPath := filepath.Join(renditionDir, "playlist.m3u8")
		if err := s.uploadFile(ctx, bucket, fmt.Sprintf("%s/%s/playlist.m3u8", mediaID, r.Name), playlistPath, "application/x-mpegURL"); err != nil {
			s.log.Error("failed to upload playlist", "error", err, "rendition", r.Name)
			continue
		}

		// Upload segments
		segments, err := filepath.Glob(filepath.Join(renditionDir, "segment_*.ts"))
		if err != nil {
			s.log.Error("failed to find segments", "error", err)
			continue
		}

		for _, seg := range segments {
			segName := filepath.Base(seg)
			segKey := fmt.Sprintf("%s/%s/%s", mediaID, r.Name, segName)
			if err := s.uploadFile(ctx, bucket, segKey, seg, "video/MP2T"); err != nil {
				s.log.Error("failed to upload segment", "error", err, "segment", segName)
			}
		}
	}

	return nil
}

func (s *Service) uploadFile(ctx context.Context, bucket, key, path, contentType string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	return s.s3Client.Upload(ctx, bucket, key, file, contentType)
}

func (s *Service) markFailed(ctx context.Context, mediaID string) {
	if err := s.dynamoClient.UpdateMediaStatus(ctx, mediaID, domain.MediaStatusFailed); err != nil {
		s.log.Error("failed to mark as failed", "error", err, "media_id", mediaID)
	}
}

// Worker processes jobs from the queue
type Worker struct {
	queue       queue.Queue
	service     *Service
	concurrency int
	log         *logger.Logger
	wg          sync.WaitGroup
}

// NewWorker creates a new transcode worker
func NewWorker(q queue.Queue, svc *Service, concurrency int, log *logger.Logger) *Worker {
	return &Worker{
		queue:       q,
		service:     svc,
		concurrency: concurrency,
		log:         log,
	}
}

// Start begins processing jobs
func (w *Worker) Start(ctx context.Context) error {
	for i := 0; i < w.concurrency; i++ {
		w.wg.Add(1)
		go w.processLoop(ctx, i)
	}
	return nil
}

// Wait waits for all workers to finish
func (w *Worker) Wait() {
	w.wg.Wait()
}

func (w *Worker) processLoop(ctx context.Context, workerID int) {
	defer w.wg.Done()

	w.log.Info("worker started", "worker_id", workerID)

	for {
		select {
		case <-ctx.Done():
			w.log.Info("worker stopping", "worker_id", workerID)
			return
		default:
		}

		// Get next job
		job, err := w.queue.Dequeue(ctx, 5) // 5 second timeout
		if err != nil {
			w.log.Error("failed to dequeue job", "error", err)
			continue
		}

		if job == nil {
			continue // No jobs available
		}

		w.log.Info("processing job", "job_id", job.ID, "media_id", job.MediaID, "worker_id", workerID)

		// Process the job
		if err := w.service.ProcessMedia(ctx, job.MediaID); err != nil {
			w.log.Error("job processing failed", "error", err, "job_id", job.ID)
			if err := w.queue.Nack(ctx, job); err != nil {
				w.log.Error("failed to nack job", "error", err)
			}
			continue
		}

		// Acknowledge successful completion
		if err := w.queue.Ack(ctx, job); err != nil {
			w.log.Error("failed to ack job", "error", err, "job_id", job.ID)
		}

		w.log.Info("job completed", "job_id", job.ID, "media_id", job.MediaID)
	}
}
