package domain

import "errors"

// Common domain errors.
var (
	ErrMediaNotFound      = errors.New("media not found")
	ErrMediaAlreadyExists = errors.New("media already exists")
	ErrInvalidMediaType   = errors.New("invalid media type")
	ErrInvalidMediaStatus = errors.New("invalid media status")
	ErrProcessingFailed   = errors.New("media processing failed")
	ErrUploadFailed       = errors.New("media upload failed")
	ErrStorageError       = errors.New("storage error")
	ErrDatabaseError      = errors.New("database error")
	ErrUnauthorized       = errors.New("unauthorized access")
	ErrInvalidInput       = errors.New("invalid input")
)
