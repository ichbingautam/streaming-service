package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/streaming-service/internal/service/stream"
	"github.com/streaming-service/internal/service/upload"
	"github.com/streaming-service/pkg/logger"
)

// RouterConfig contains router dependencies
type RouterConfig struct {
	UploadService *upload.Service
	StreamService *stream.Service
	Logger        *logger.Logger
}

// NewRouter creates a new HTTP router
func NewRouter(cfg RouterConfig) *chi.Mux {
	r := chi.NewRouter()

	// Middleware stack
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))
	r.Use(requestLogger(cfg.Logger))
	r.Use(corsMiddleware)

	// Health check
	r.Get("/health", healthHandler)
	r.Get("/ready", readyHandler)

	// API routes
	r.Route("/api/v1", func(r chi.Router) {
		// Upload routes
		r.Route("/upload", func(r chi.Router) {
			r.Post("/", uploadHandler(cfg.UploadService, cfg.Logger))
			r.Post("/presign", presignHandler(cfg.UploadService, cfg.Logger))
			r.Post("/{mediaID}/confirm", confirmUploadHandler(cfg.UploadService, cfg.Logger))
		})

		// Media routes
		r.Route("/media", func(r chi.Router) {
			r.Get("/", listMediaHandler(cfg.StreamService, cfg.Logger))
			r.Get("/{mediaID}", getMediaHandler(cfg.StreamService, cfg.Logger))
			r.Delete("/{mediaID}", deleteMediaHandler(cfg.StreamService, cfg.Logger))
			r.Get("/{mediaID}/playback", playbackHandler(cfg.StreamService, cfg.Logger))
		})
	})

	return r
}

// JSON response helpers
func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		_ = json.NewEncoder(w).Encode(data)
	}
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}

// Health check handlers
func healthHandler(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{
		"status": "healthy",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

func readyHandler(w http.ResponseWriter, r *http.Request) {
	// TODO: Check dependencies (DB, S3, Redis)
	respondJSON(w, http.StatusOK, map[string]string{
		"status": "ready",
	})
}

// CORS middleware
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Request logger middleware
func requestLogger(log *logger.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			log.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"duration", time.Since(start).String(),
				"bytes", ww.BytesWritten(),
			)
		})
	}
}
