package aws

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3PutObjectAPI is the minimal S3 surface PutObject depends on. *s3.Client
// satisfies it; tests provide a fake.
type S3PutObjectAPI interface {
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
}

// InitS3Client loads AWS configuration from the environment (credentials,
// AWS_REGION) and returns an S3 client.
func InitS3Client(ctx context.Context) (*s3.Client, error) {
	region := os.Getenv("AWS_REGION")
	if region == "" {
		slog.Warn("AWS_REGION not set, falling back to SDK default")
	}
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("unable to load AWS SDK config: %w", err)
	}
	return s3.NewFromConfig(cfg), nil
}

// PutObject uploads body to bucket/key. contentType and contentDisposition are
// set on the object when non-empty (e.g. "application/json" and
// `attachment; filename="export.json"`). Uploading to an existing key
// overwrites it.
func PutObject(ctx context.Context, client S3PutObjectAPI, bucket, key string, body []byte, contentType, contentDisposition string) error {
	input := &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(body),
	}
	if contentType != "" {
		input.ContentType = aws.String(contentType)
	}
	if contentDisposition != "" {
		input.ContentDisposition = aws.String(contentDisposition)
	}

	if _, err := client.PutObject(ctx, input); err != nil {
		slog.Error("S3 PutObject failed", slog.String("bucket", bucket), slog.String("key", key), slog.Any("error", err))
		return fmt.Errorf("s3 put object %s/%s: %w", bucket, key, err)
	}
	return nil
}
