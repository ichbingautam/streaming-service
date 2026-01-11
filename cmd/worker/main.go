package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/streaming-service/internal/config"
	"github.com/streaming-service/internal/media/ffmpeg"
	"github.com/streaming-service/internal/queue"
	"github.com/streaming-service/internal/repository/dynamodb"
	"github.com/streaming-service/internal/repository/s3"
	"github.com/streaming-service/internal/service/transcode"
	"github.com/streaming-service/pkg/logger"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	log := logger.New(cfg.Log.Level, cfg.Log.Format)
	log.Info("starting transcoding worker", "version", cfg.App.Version)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize AWS clients
	s3Client, err := s3.NewClient(ctx, cfg.AWS)
	if err != nil {
		log.Error("failed to initialize S3 client", "error", err)
		os.Exit(1)
	}

	dynamoClient, err := dynamodb.NewClient(ctx, cfg.AWS)
	if err != nil {
		log.Error("failed to initialize DynamoDB client", "error", err)
		os.Exit(1)
	}

	// Initialize job queue
	jobQueue, err := queue.NewRedisQueue(cfg.Redis)
	if err != nil {
		log.Error("failed to initialize job queue", "error", err)
		os.Exit(1)
	}

	// Initialize FFMPEG processor
	ffmpegProcessor := ffmpeg.NewProcessor(cfg.FFMPEG)

	// Initialize transcode service
	transcodeService := transcode.NewService(
		s3Client,
		dynamoClient,
		ffmpegProcessor,
		log,
	)

	// Create worker pool
	worker := transcode.NewWorker(
		jobQueue,
		transcodeService,
		cfg.Worker.Concurrency,
		log,
	)

	// Start worker
	go func() {
		log.Info("worker started", "concurrency", cfg.Worker.Concurrency)
		if err := worker.Start(ctx); err != nil {
			log.Error("worker error", "error", err)
			cancel()
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down worker...")
	cancel()

	// Wait for worker to finish current jobs
	worker.Wait()
	log.Info("worker stopped")
}
