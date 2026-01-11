package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/streaming-service/internal/domain"
	"github.com/streaming-service/internal/service/stream"
	"github.com/streaming-service/internal/service/upload"
	"github.com/streaming-service/pkg/logger"
)

// Upload request body
type uploadRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

// Presign request body
type presignRequest struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
}

// uploadHandler handles direct file uploads
func uploadHandler(svc *upload.Service, log *logger.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse multipart form (max 100MB)
		if err := r.ParseMultipartForm(100 << 20); err != nil {
			respondError(w, http.StatusBadRequest, "failed to parse form")
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			respondError(w, http.StatusBadRequest, "file is required")
			return
		}
		defer file.Close()

		title := r.FormValue("title")
		if title == "" {
			title = header.Filename
		}

		// Get user ID from context (set by auth middleware)
		userID := getUserID(r)

		req := &upload.UploadRequest{
			Title:       title,
			Description: r.FormValue("description"),
			UserID:      userID,
			Filename:    header.Filename,
			ContentType: header.Header.Get("Content-Type"),
			Body:        file,
		}

		resp, err := svc.Upload(r.Context(), req)
		if err != nil {
			log.Error("upload failed", "error", err)
			respondError(w, http.StatusInternalServerError, "upload failed")
			return
		}

		respondJSON(w, http.StatusCreated, resp)
	}
}

// presignHandler generates presigned upload URLs
func presignHandler(svc *upload.Service, log *logger.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req presignRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if req.Filename == "" || req.ContentType == "" {
			respondError(w, http.StatusBadRequest, "filename and content_type are required")
			return
		}

		userID := getUserID(r)

		resp, err := svc.GetPresignedUploadURL(r.Context(), userID, req.Filename, req.ContentType)
		if err != nil {
			log.Error("failed to generate presigned URL", "error", err)
			respondError(w, http.StatusInternalServerError, "failed to generate upload URL")
			return
		}

		respondJSON(w, http.StatusOK, resp)
	}
}

// confirmUploadHandler confirms a presigned URL upload
func confirmUploadHandler(svc *upload.Service, log *logger.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mediaID := chi.URLParam(r, "mediaID")
		if mediaID == "" {
			respondError(w, http.StatusBadRequest, "media ID is required")
			return
		}

		var body uploadRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		userID := getUserID(r)

		req := &upload.UploadRequest{
			Title:       body.Title,
			Description: body.Description,
			UserID:      userID,
		}

		resp, err := svc.ConfirmUpload(r.Context(), req, mediaID)
		if err != nil {
			log.Error("failed to confirm upload", "error", err)
			respondError(w, http.StatusInternalServerError, "failed to confirm upload")
			return
		}

		respondJSON(w, http.StatusOK, resp)
	}
}

// getMediaHandler retrieves media information
func getMediaHandler(svc *stream.Service, log *logger.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mediaID := chi.URLParam(r, "mediaID")
		if mediaID == "" {
			respondError(w, http.StatusBadRequest, "media ID is required")
			return
		}

		info, err := svc.GetMedia(r.Context(), mediaID)
		if err != nil {
			if err == domain.ErrMediaNotFound {
				respondError(w, http.StatusNotFound, "media not found")
				return
			}
			log.Error("failed to get media", "error", err)
			respondError(w, http.StatusInternalServerError, "failed to get media")
			return
		}

		respondJSON(w, http.StatusOK, info)
	}
}

// listMediaHandler lists media for a user
func listMediaHandler(svc *stream.Service, log *logger.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := getUserID(r)

		media, err := svc.ListMedia(r.Context(), userID, 100)
		if err != nil {
			log.Error("failed to list media", "error", err)
			respondError(w, http.StatusInternalServerError, "failed to list media")
			return
		}

		respondJSON(w, http.StatusOK, map[string]interface{}{
			"items": media,
			"count": len(media),
		})
	}
}

// deleteMediaHandler deletes a media item
func deleteMediaHandler(svc *stream.Service, log *logger.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mediaID := chi.URLParam(r, "mediaID")
		if mediaID == "" {
			respondError(w, http.StatusBadRequest, "media ID is required")
			return
		}

		userID := getUserID(r)

		if err := svc.DeleteMedia(r.Context(), mediaID, userID); err != nil {
			if err == domain.ErrMediaNotFound {
				respondError(w, http.StatusNotFound, "media not found")
				return
			}
			if err == domain.ErrUnauthorized {
				respondError(w, http.StatusForbidden, "unauthorized")
				return
			}
			log.Error("failed to delete media", "error", err)
			respondError(w, http.StatusInternalServerError, "failed to delete media")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// playbackHandler returns playback URLs
func playbackHandler(svc *stream.Service, log *logger.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mediaID := chi.URLParam(r, "mediaID")
		if mediaID == "" {
			respondError(w, http.StatusBadRequest, "media ID is required")
			return
		}

		url, err := svc.GetPlaybackURL(r.Context(), mediaID)
		if err != nil {
			if err == domain.ErrMediaNotFound {
				respondError(w, http.StatusNotFound, "media not found")
				return
			}
			log.Error("failed to get playback URL", "error", err)
			respondError(w, http.StatusInternalServerError, "failed to get playback URL")
			return
		}

		respondJSON(w, http.StatusOK, map[string]string{
			"playback_url": url,
		})
	}
}

// getUserID extracts user ID from request context
// In production, this would come from auth middleware
func getUserID(r *http.Request) string {
	// Placeholder - should come from JWT or session
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		userID = "anonymous"
	}
	return userID
}
