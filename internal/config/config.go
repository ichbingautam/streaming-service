package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds all configuration for the application
type Config struct {
	App    AppConfig
	Server ServerConfig
	AWS    AWSConfig
	Redis  RedisConfig
	FFMPEG FFMPEGConfig
	Worker WorkerConfig
	Log    LogConfig
}

// AppConfig holds application metadata
type AppConfig struct {
	Name        string
	Version     string
	Environment string
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Port         int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

// AWSConfig holds AWS service configuration
type AWSConfig struct {
	Region            string
	AccessKeyID       string
	SecretAccessKey   string
	S3RawBucket       string
	S3ProcessedBucket string
	DynamoDBTable     string
	CloudFrontDomain  string
	CloudFrontKeyID   string
}

// RedisConfig holds Redis connection configuration
type RedisConfig struct {
	Host     string
	Port     int
	Password string
	DB       int
}

// FFMPEGConfig holds FFMPEG processing configuration
type FFMPEGConfig struct {
	BinaryPath      string
	TempDir         string
	SegmentDuration int
	Profiles        []TranscodeProfile
}

// TranscodeProfile defines a transcoding output profile
type TranscodeProfile struct {
	Name         string
	Width        int
	Height       int
	VideoBitrate string
	AudioBitrate string
	Codec        string
}

// WorkerConfig holds worker pool configuration
type WorkerConfig struct {
	Concurrency int
	JobTimeout  time.Duration
}

// LogConfig holds logging configuration
type LogConfig struct {
	Level  string
	Format string
}

// Load reads configuration from file and environment
func Load() (*Config, error) {
	v := viper.New()

	// Set config name and paths
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("./config")
	v.AddConfigPath("/etc/streaming-service")

	// Set defaults
	setDefaults(v)

	// Read config file (optional)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
		// Config file not found; continue with defaults and env vars
	}

	// Bind environment variables
	v.SetEnvPrefix("STREAM")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unable to unmarshal config: %w", err)
	}

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	// App defaults
	v.SetDefault("app.name", "streaming-service")
	v.SetDefault("app.version", "1.0.0")
	v.SetDefault("app.environment", "development")

	// Server defaults
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.readtimeout", 30*time.Second)
	v.SetDefault("server.writetimeout", 30*time.Second)
	v.SetDefault("server.idletimeout", 60*time.Second)

	// AWS defaults
	v.SetDefault("aws.region", "us-east-1")
	v.SetDefault("aws.s3rawbucket", "streaming-raw-media")
	v.SetDefault("aws.s3processedbucket", "streaming-processed-media")
	v.SetDefault("aws.dynamodbtable", "video-metadata")

	// Redis defaults
	v.SetDefault("redis.host", "localhost")
	v.SetDefault("redis.port", 6379)
	v.SetDefault("redis.db", 0)

	// FFMPEG defaults
	v.SetDefault("ffmpeg.binarypath", "ffmpeg")
	v.SetDefault("ffmpeg.tempdir", "/tmp/streaming")
	v.SetDefault("ffmpeg.segmentduration", 6)
	v.SetDefault("ffmpeg.profiles", []TranscodeProfile{
		{Name: "1080p", Width: 1920, Height: 1080, VideoBitrate: "5000k", AudioBitrate: "192k", Codec: "h264"},
		{Name: "720p", Width: 1280, Height: 720, VideoBitrate: "2500k", AudioBitrate: "128k", Codec: "h264"},
		{Name: "480p", Width: 854, Height: 480, VideoBitrate: "1000k", AudioBitrate: "96k", Codec: "h264"},
		{Name: "360p", Width: 640, Height: 360, VideoBitrate: "500k", AudioBitrate: "64k", Codec: "h264"},
	})

	// Worker defaults
	v.SetDefault("worker.concurrency", 4)
	v.SetDefault("worker.jobtimeout", 30*time.Minute)

	// Log defaults
	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "json")
}
