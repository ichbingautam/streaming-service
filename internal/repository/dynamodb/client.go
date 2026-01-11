package dynamodb

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	appconfig "github.com/streaming-service/internal/config"
	"github.com/streaming-service/internal/domain"
)

// Client wraps the AWS DynamoDB client
type Client struct {
	client    *dynamodb.Client
	tableName string
}

// NewClient creates a new DynamoDB client
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

	client := dynamodb.NewFromConfig(awsCfg)

	return &Client{
		client:    client,
		tableName: cfg.DynamoDBTable,
	}, nil
}

// CreateMedia creates a new media record
func (c *Client) CreateMedia(ctx context.Context, media *domain.Media) error {
	av, err := attributevalue.MarshalMap(media)
	if err != nil {
		return fmt.Errorf("failed to marshal media: %w", err)
	}

	_, err = c.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(c.tableName),
		Item:                av,
		ConditionExpression: aws.String("attribute_not_exists(id)"),
	})
	if err != nil {
		return fmt.Errorf("failed to create media: %w", err)
	}

	return nil
}

// GetMedia retrieves a media record by ID
func (c *Client) GetMedia(ctx context.Context, id string) (*domain.Media, error) {
	result, err := c.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(c.tableName),
		Key: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: id},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get media: %w", err)
	}

	if result.Item == nil {
		return nil, domain.ErrMediaNotFound
	}

	var media domain.Media
	if err := attributevalue.UnmarshalMap(result.Item, &media); err != nil {
		return nil, fmt.Errorf("failed to unmarshal media: %w", err)
	}

	return &media, nil
}

// UpdateMedia updates an existing media record
func (c *Client) UpdateMedia(ctx context.Context, media *domain.Media) error {
	media.UpdatedAt = time.Now()

	av, err := attributevalue.MarshalMap(media)
	if err != nil {
		return fmt.Errorf("failed to marshal media: %w", err)
	}

	_, err = c.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(c.tableName),
		Item:      av,
	})
	if err != nil {
		return fmt.Errorf("failed to update media: %w", err)
	}

	return nil
}

// UpdateMediaStatus updates only the status and timestamp
func (c *Client) UpdateMediaStatus(ctx context.Context, id string, status domain.MediaStatus) error {
	update := expression.Set(
		expression.Name("status"),
		expression.Value(status),
	).Set(
		expression.Name("updated_at"),
		expression.Value(time.Now()),
	)

	if status == domain.MediaStatusCompleted {
		update = update.Set(
			expression.Name("processed_at"),
			expression.Value(time.Now()),
		)
	}

	expr, err := expression.NewBuilder().WithUpdate(update).Build()
	if err != nil {
		return fmt.Errorf("failed to build expression: %w", err)
	}

	_, err = c.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(c.tableName),
		Key: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: id},
		},
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		UpdateExpression:          expr.Update(),
	})
	if err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	return nil
}

// DeleteMedia removes a media record
func (c *Client) DeleteMedia(ctx context.Context, id string) error {
	_, err := c.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(c.tableName),
		Key: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: id},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to delete media: %w", err)
	}

	return nil
}

// ListMediaByUser retrieves all media for a user
func (c *Client) ListMediaByUser(ctx context.Context, userID string, limit int32) ([]*domain.Media, error) {
	keyExpr := expression.Key("user_id").Equal(expression.Value(userID))
	expr, err := expression.NewBuilder().WithKeyCondition(keyExpr).Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build expression: %w", err)
	}

	result, err := c.client.Query(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(c.tableName),
		IndexName:                 aws.String("user_id-index"),
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		Limit:                     aws.Int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query media: %w", err)
	}

	var mediaList []*domain.Media
	for _, item := range result.Items {
		var media domain.Media
		if err := attributevalue.UnmarshalMap(item, &media); err != nil {
			return nil, fmt.Errorf("failed to unmarshal media: %w", err)
		}
		mediaList = append(mediaList, &media)
	}

	return mediaList, nil
}

// ListMediaByStatus retrieves media by processing status
func (c *Client) ListMediaByStatus(ctx context.Context, status domain.MediaStatus, limit int32) ([]*domain.Media, error) {
	keyExpr := expression.Key("status").Equal(expression.Value(string(status)))
	expr, err := expression.NewBuilder().WithKeyCondition(keyExpr).Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build expression: %w", err)
	}

	result, err := c.client.Query(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(c.tableName),
		IndexName:                 aws.String("status-index"),
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		Limit:                     aws.Int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query media: %w", err)
	}

	var mediaList []*domain.Media
	for _, item := range result.Items {
		var media domain.Media
		if err := attributevalue.UnmarshalMap(item, &media); err != nil {
			return nil, fmt.Errorf("failed to unmarshal media: %w", err)
		}
		mediaList = append(mediaList, &media)
	}

	return mediaList, nil
}

// AddRendition adds a rendition to a media record
func (c *Client) AddRendition(ctx context.Context, id string, rendition domain.Rendition) error {
	update := expression.Set(
		expression.Name("renditions"),
		expression.ListAppend(
			expression.Name("renditions"),
			expression.Value([]domain.Rendition{rendition}),
		),
	).Set(
		expression.Name("updated_at"),
		expression.Value(time.Now()),
	)

	expr, err := expression.NewBuilder().WithUpdate(update).Build()
	if err != nil {
		return fmt.Errorf("failed to build expression: %w", err)
	}

	_, err = c.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(c.tableName),
		Key: map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{Value: id},
		},
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		UpdateExpression:          expr.Update(),
	})
	if err != nil {
		return fmt.Errorf("failed to add rendition: %w", err)
	}

	return nil
}
