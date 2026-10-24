package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

type Client struct {
	client    *s3.Client
	preSginer *s3.PresignClient
	bucket    string
}

type R2Config struct {
	AccountId       string
	AccessKeyID     string
	AccessKeySecret string
	Bucket          string
}

var ErrNotFound = errors.New("storage: object not found")

func NewR2(ctx context.Context, cfg R2Config) (*Client, error) {
	r2Cfg, err := config.LoadDefaultConfig(ctx, config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.AccessKeySecret, "")),
		config.WithRegion("auto"))
	if err != nil {
		return nil, fmt.Errorf("Storage: Failed to load storage config : %w", err)
	}
	client := s3.NewFromConfig(r2Cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(fmt.Sprintf("https://%s.r2.cloudflarestorage.com", cfg.AccountId))
	})

	preSignClient := s3.NewPresignClient(client)

	return &Client{
		client:    client,
		preSginer: preSignClient,
		bucket:    cfg.Bucket,
	}, nil
}

func (c *Client) PreSignUpload(ctx context.Context, key, contentType string, contentLength int64, expiry time.Duration) (string, error) {
	req, err := c.preSginer.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(c.bucket),
		Key:           aws.String(key),
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(contentLength),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = expiry
	})
	if err != nil {
		return "", fmt.Errorf("Storage: PreSignUpload: Failed to presign upload request :key-%s,  error-%w", key, err)
	}
	return req.URL, nil
}

func (c *Client) Head(ctx context.Context, key string) (contentLength int64, contentType string, err error) {
	out, err := c.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isNotFoundError(err) {
			return 0, "", ErrNotFound
		}
		return 0, "", fmt.Errorf("Storage: Head: Failed to head object :key-%q,  error-%w", key, err)
	}
	if out.ContentLength != nil {
		contentLength = *out.ContentLength
	}
	if out.ContentType != nil {
		contentType = *out.ContentType
	}
	return contentLength, contentType, nil
}

func isNotFoundError(err error) bool {
	var ae smithy.APIError
	if errors.As(err, &ae) {
		switch ae.ErrorCode() {
		case "NotFound", "NoSuchKey":
			return true
		}
	}
	return false
}
