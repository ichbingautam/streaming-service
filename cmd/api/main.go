package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/streaming-service/internal/api"
	"github.com/streaming-service/internal/config"
	"github.com/streaming-service/internal/repository/dynamodb"
	"github.com/streaming-service/internal/repository/s3"
	"github.com/streaming-service/internal/service/stream"
	"github.com/streaming-service/internal/service/upload"
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
	log.Info("starting streaming service api", "version", cfg.App.Version)

	// Initialize AWS clients
	ctx := context.Background()

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

	// Initialize services
	uploadService := upload.NewService(s3Client, dynamoClient, log)
	streamService := stream.NewService(s3Client, dynamoClient, cfg.AWS.CloudFrontDomain, log)

	// Initialize HTTP router
	router := api.NewRouter(api.RouterConfig{
		UploadService: uploadService,
		StreamService: streamService,
		Logger:        log,
	})

	// Create HTTP server
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	// Start server in goroutine
	go func() {
		log.Info("server listening", "port", cfg.Server.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down server...")

	// Graceful shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error("server forced to shutdown", "error", err)
		os.Exit(1)
	}

	log.Info("server stopped")
}
