package domain

import (
	"time"
)

// MediaType represents the type of media content
type MediaType string

const (
	MediaTypeVideo MediaType = "video"
	MediaTypeAudio MediaType = "audio"
)

// MediaStatus represents the processing status of media
type MediaStatus string

const (
	MediaStatusPending    MediaStatus = "pending"
	MediaStatusProcessing MediaStatus = "processing"
	MediaStatusCompleted  MediaStatus = "completed"
	MediaStatusFailed     MediaStatus = "failed"
)

// Media represents a media item (video or audio)
type Media struct {
	ID          string      `json:"id" dynamodbav:"id"`
	Title       string      `json:"title" dynamodbav:"title"`
	Description string      `json:"description" dynamodbav:"description"`
	Type        MediaType   `json:"type" dynamodbav:"type"`
	Status      MediaStatus `json:"status" dynamodbav:"status"`

	// Source file info
	SourceKey    string `json:"source_key" dynamodbav:"source_key"`
	SourceBucket string `json:"source_bucket" dynamodbav:"source_bucket"`
	SourceSize   int64  `json:"source_size" dynamodbav:"source_size"`
	SourceFormat string `json:"source_format" dynamodbav:"source_format"`

	// Processed outputs
	Renditions []Rendition `json:"renditions" dynamodbav:"renditions"`

	// Metadata
	Duration float64           `json:"duration" dynamodbav:"duration"`
	Width    int               `json:"width,omitempty" dynamodbav:"width,omitempty"`
	Height   int               `json:"height,omitempty" dynamodbav:"height,omitempty"`
	Bitrate  int               `json:"bitrate,omitempty" dynamodbav:"bitrate,omitempty"`
	Codec    string            `json:"codec,omitempty" dynamodbav:"codec,omitempty"`
	Tags     map[string]string `json:"tags,omitempty" dynamodbav:"tags,omitempty"`

	// Timestamps
	CreatedAt   time.Time `json:"created_at" dynamodbav:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" dynamodbav:"updated_at"`
	ProcessedAt time.Time `json:"processed_at,omitempty" dynamodbav:"processed_at,omitempty"`

	// User info
	UserID string `json:"user_id" dynamodbav:"user_id"`
}

// Rendition represents a processed version of media
type Rendition struct {
	Name          string `json:"name" dynamodbav:"name"`
	Width         int    `json:"width,omitempty" dynamodbav:"width,omitempty"`
	Height        int    `json:"height,omitempty" dynamodbav:"height,omitempty"`
	Bitrate       int    `json:"bitrate" dynamodbav:"bitrate"`
	Codec         string `json:"codec" dynamodbav:"codec"`
	PlaylistKey   string `json:"playlist_key" dynamodbav:"playlist_key"`
	SegmentPrefix string `json:"segment_prefix" dynamodbav:"segment_prefix"`
}

// Video is a specialized Media type for video content
type Video struct {
	Media
	ThumbnailKey string  `json:"thumbnail_key,omitempty" dynamodbav:"thumbnail_key,omitempty"`
	AspectRatio  string  `json:"aspect_ratio,omitempty" dynamodbav:"aspect_ratio,omitempty"`
	FrameRate    float64 `json:"frame_rate,omitempty" dynamodbav:"frame_rate,omitempty"`
}

// Audio is a specialized Media type for audio content
type Audio struct {
	Media
	Artist      string `json:"artist,omitempty" dynamodbav:"artist,omitempty"`
	Album       string `json:"album,omitempty" dynamodbav:"album,omitempty"`
	Genre       string `json:"genre,omitempty" dynamodbav:"genre,omitempty"`
	SampleRate  int    `json:"sample_rate,omitempty" dynamodbav:"sample_rate,omitempty"`
	Channels    int    `json:"channels,omitempty" dynamodbav:"channels,omitempty"`
	CoverArtKey string `json:"cover_art_key,omitempty" dynamodbav:"cover_art_key,omitempty"`
}

// NewMedia creates a new Media with initialized fields
func NewMedia(id, title, userID string, mediaType MediaType) *Media {
	now := time.Now()
	return &Media{
		ID:        id,
		Title:     title,
		UserID:    userID,
		Type:      mediaType,
		Status:    MediaStatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// IsProcessed returns true if media has been successfully processed
func (m *Media) IsProcessed() bool {
	return m.Status == MediaStatusCompleted && len(m.Renditions) > 0
}

// GetMasterPlaylistKey returns the key for the master HLS playlist
func (m *Media) GetMasterPlaylistKey() string {
	return m.ID + "/master.m3u8"
}
