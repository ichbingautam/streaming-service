package s3

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	appconfig "github.com/streaming-service/internal/config"
)

// Client wraps the AWS S3 client
type Client struct {
	client          *s3.Client
	presignClient   *s3.PresignClient
	rawBucket       string
	processedBucket string
}

// NewClient creates a new S3 client
func NewClient(ctx context.Context, cfg appconfig.AWSConfig) (*Client, error) {
	// Build AWS config
	var opts []func(*config.LoadOptions) error
	opts = append(opts, config.WithRegion(cfg.Region))

	// Add credentials if provided
	if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" {
		opts = append(opts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				cfg.AccessKeyID,
				cfg.SecretAccessKey,
				"",
			),
		))
	}

	awsCfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg)
	presignClient := s3.NewPresignClient(client)

	return &Client{
		client:          client,
		presignClient:   presignClient,
		rawBucket:       cfg.S3RawBucket,
		processedBucket: cfg.S3ProcessedBucket,
	}, nil
}

// Upload uploads a file to S3
func (c *Client) Upload(ctx context.Context, bucket, key string, body io.Reader, contentType string) error {
	_, err := c.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        body,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("failed to upload to S3: %w", err)
	}
	return nil
}

// UploadRaw uploads a file to the raw media bucket
func (c *Client) UploadRaw(ctx context.Context, key string, body io.Reader, contentType string) error {
	return c.Upload(ctx, c.rawBucket, key, body, contentType)
}

// UploadProcessed uploads a file to the processed media bucket
func (c *Client) UploadProcessed(ctx context.Context, key string, body io.Reader, contentType string) error {
	return c.Upload(ctx, c.processedBucket, key, body, contentType)
}

// Download downloads a file from S3
func (c *Client) Download(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	result, err := c.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to download from S3: %w", err)
	}
	return result.Body, nil
}

// DownloadRaw downloads a file from the raw media bucket
func (c *Client) DownloadRaw(ctx context.Context, key string) (io.ReadCloser, error) {
	return c.Download(ctx, c.rawBucket, key)
}

// DownloadProcessed downloads a file from the processed media bucket
func (c *Client) DownloadProcessed(ctx context.Context, key string) (io.ReadCloser, error) {
	return c.Download(ctx, c.processedBucket, key)
}

// Delete removes a file from S3
func (c *Client) Delete(ctx context.Context, bucket, key string) error {
	_, err := c.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to delete from S3: %w", err)
	}
	return nil
}

// GetPresignedUploadURL generates a presigned URL for uploading
func (c *Client) GetPresignedUploadURL(ctx context.Context, key string, contentType string, expiresIn time.Duration) (string, error) {
	result, err := c.presignClient.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(c.rawBucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
	}, s3.WithPresignExpires(expiresIn))
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}
	return result.URL, nil
}

// GetPresignedDownloadURL generates a presigned URL for downloading
func (c *Client) GetPresignedDownloadURL(ctx context.Context, bucket, key string, expiresIn time.Duration) (string, error) {
	result, err := c.presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(expiresIn))
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}
	return result.URL, nil
}

// ListObjects lists objects in a bucket with a given prefix
func (c *Client) ListObjects(ctx context.Context, bucket, prefix string) ([]types.Object, error) {
	var objects []types.Object
	paginator := s3.NewListObjectsV2Paginator(c.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(prefix),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list objects: %w", err)
		}
		objects = append(objects, page.Contents...)
	}

	return objects, nil
}

// CopyObject copies an object within S3
func (c *Client) CopyObject(ctx context.Context, srcBucket, srcKey, dstBucket, dstKey string) error {
	_, err := c.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(dstBucket),
		Key:        aws.String(dstKey),
		CopySource: aws.String(fmt.Sprintf("%s/%s", srcBucket, srcKey)),
	})
	if err != nil {
		return fmt.Errorf("failed to copy object: %w", err)
	}
	return nil
}

// GetRawBucket returns the raw bucket name
func (c *Client) GetRawBucket() string {
	return c.rawBucket
}

// GetProcessedBucket returns the processed bucket name
func (c *Client) GetProcessedBucket() string {
	return c.processedBucket
}
