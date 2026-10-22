package storage

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type Client struct {
	client    *s3.Client
	preSginer *s3.PresignClient
}

type R2Config struct {
	AccountId       string
	AccessKeyID     string
	AccessKeySecret string
	Bucket          string
}

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
	}, nil
}
