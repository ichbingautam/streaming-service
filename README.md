# Streaming Service

A high-performance video/audio streaming service built in Go, targeting 100M RPS with HLS adaptive streaming.

## Features

- **HLS Adaptive Bitrate Streaming**: Multiple quality levels (1080p, 720p, 480p, 360p)
- **FFMPEG Processing**: Video transcoding and audio extraction
- **AWS Integration**: S3 storage, DynamoDB metadata, CloudFront CDN
- **Design Patterns**: Factory, Strategy, and Pipeline patterns for extensibility

## Architecture

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Clients   │────▶│ API Gateway │────▶│  Services   │
└─────────────┘     └─────────────┘     └─────────────┘
                                              │
                    ┌─────────────────────────┼─────────────────────────┐
                    │                         │                         │
              ┌─────▼─────┐           ┌───────▼───────┐         ┌───────▼───────┐
              │  Upload   │           │  Transcode    │         │   Stream      │
              │  Service  │           │   Service     │         │   Service     │
              └─────┬─────┘           └───────┬───────┘         └───────┬───────┘
                    │                         │                         │
              ┌─────▼─────┐           ┌───────▼───────┐         ┌───────▼───────┐
              │    S3     │           │    FFMPEG     │         │  CloudFront   │
              └───────────┘           └───────────────┘         └───────────────┘
```

## Quick Start

```bash
# Build
make build

# Run API server
make run-api

# Run worker
make run-worker

# Run tests
make test
```

## Configuration

Set environment variables or use `config.yaml`:

```yaml
server:
  port: 8080

aws:
  region: us-east-1
  s3_bucket: streaming-media
  dynamodb_table: video-metadata
```

## License

MIT
