package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/streaming-service/internal/config"
)

// JobType represents the type of processing job.
type JobType string

// ErrNoJobAvailable is returned when no jobs are available in the queue.
var ErrNoJobAvailable = errors.New("no job available")

const (
	JobTypeTranscode JobType = "transcode"
	JobTypeAudio     JobType = "audio"
	JobTypeThumbnail JobType = "thumbnail"
)

// Job represents a processing job
type Job struct {
	ID        string            `json:"id"`
	Type      JobType           `json:"type"`
	MediaID   string            `json:"media_id"`
	Priority  int               `json:"priority"`
	Payload   map[string]string `json:"payload"`
	CreatedAt time.Time         `json:"created_at"`
	Attempts  int               `json:"attempts"`
}

// Queue defines the interface for a job queue
type Queue interface {
	Enqueue(ctx context.Context, job *Job) error
	Dequeue(ctx context.Context, timeout time.Duration) (*Job, error)
	Ack(ctx context.Context, job *Job) error
	Nack(ctx context.Context, job *Job) error
	Len(ctx context.Context) (int64, error)
}

// RedisQueue implements Queue using Redis
type RedisQueue struct {
	client        *redis.Client
	queueKey      string
	processingKey string
}

const (
	defaultQueueKey      = "streaming:jobs:pending"
	defaultProcessingKey = "streaming:jobs:processing"
)

// NewRedisQueue creates a new Redis-based job queue
func NewRedisQueue(cfg config.RedisConfig) (*RedisQueue, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &RedisQueue{
		client:        client,
		queueKey:      defaultQueueKey,
		processingKey: defaultProcessingKey,
	}, nil
}

// Enqueue adds a job to the queue
func (q *RedisQueue) Enqueue(ctx context.Context, job *Job) error {
	job.CreatedAt = time.Now()

	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	// Use ZADD with priority as score (lower priority = higher score for processing first)
	score := float64(time.Now().Unix()) - float64(job.Priority*1000)

	if err := q.client.ZAdd(ctx, q.queueKey, redis.Z{
		Score:  score,
		Member: string(data),
	}).Err(); err != nil {
		return fmt.Errorf("failed to enqueue job: %w", err)
	}

	return nil
}

// Dequeue removes and returns the next job from the queue
func (q *RedisQueue) Dequeue(ctx context.Context, timeout time.Duration) (*Job, error) {
	// Use BZPOPMIN for blocking pop from sorted set
	result, err := q.client.BZPopMin(ctx, timeout, q.queueKey).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrNoJobAvailable
		}
		return nil, fmt.Errorf("failed to dequeue job: %w", err)
	}

	data, ok := result.Member.(string)
	if !ok {
		return nil, fmt.Errorf("unexpected member type: %T", result.Member)
	}

	var job Job
	if err := json.Unmarshal([]byte(data), &job); err != nil {
		return nil, fmt.Errorf("failed to unmarshal job: %w", err)
	}

	// Move to processing set
	if err := q.client.SAdd(ctx, q.processingKey, data).Err(); err != nil {
		// Re-enqueue if we can't track processing - log but don't fail
		if enqErr := q.Enqueue(ctx, &job); enqErr != nil {
			return nil, fmt.Errorf("failed to re-enqueue job: %w", enqErr)
		}
		return nil, fmt.Errorf("failed to track processing job: %w", err)
	}

	return &job, nil
}

// Ack acknowledges successful job completion
func (q *RedisQueue) Ack(ctx context.Context, job *Job) error {
	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	if err := q.client.SRem(ctx, q.processingKey, string(data)).Err(); err != nil {
		return fmt.Errorf("failed to ack job: %w", err)
	}

	return nil
}

// Nack re-queues a failed job for retry
func (q *RedisQueue) Nack(ctx context.Context, job *Job) error {
	// Remove from processing
	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	if err := q.client.SRem(ctx, q.processingKey, string(data)).Err(); err != nil {
		return fmt.Errorf("failed to remove from processing: %w", err)
	}

	// Re-enqueue with incremented attempts
	job.Attempts++
	if job.Attempts < 3 { // Max 3 attempts
		return q.Enqueue(ctx, job)
	}

	// Move to dead letter queue after max attempts
	deadLetterKey := "streaming:jobs:dead"
	if err := q.client.SAdd(ctx, deadLetterKey, string(data)).Err(); err != nil {
		return fmt.Errorf("failed to add to dead letter queue: %w", err)
	}

	return nil
}

// Len returns the number of pending jobs
func (q *RedisQueue) Len(ctx context.Context) (int64, error) {
	return q.client.ZCard(ctx, q.queueKey).Result()
}

// Close closes the Redis connection
func (q *RedisQueue) Close() error {
	return q.client.Close()
}
