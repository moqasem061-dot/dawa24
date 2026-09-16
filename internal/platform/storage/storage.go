// Package storage provides an S3-compatible object storage client (supporting
// AWS S3 and MinIO).
//
// In multi-tenant systems, all tenant files are namespaced using KeyFor(orgID, path)
// so that objects are stored under "orgs/<orgID>/...".
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/muhiya/dawa24-store/internal/platform/config"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// ErrInvalidKey is returned when an object key is empty or malformed.
var ErrInvalidKey = errors.New("storage: key cannot be empty")

// Client wraps an S3 client and its presigner.
type Client struct {
	s3Client      *s3.Client
	presignClient *s3.PresignClient
	bucket        string
	publicBaseURL string
}

// New creates a new Storage Client configured from config.Storage.
func New(ctx context.Context, cfg config.Storage) (*Client, error) {
	if cfg.Bucket == "" {
		return nil, errors.New("storage: bucket is required")
	}

	optFns := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.Region),
	}

	if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" {
		optFns = append(optFns, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, optFns...)
	if err != nil {
		return nil, fmt.Errorf("storage: load aws config: %w", err)
	}

	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		o.UsePathStyle = cfg.UsePathStyle
	})

	presignClient := s3.NewPresignClient(s3Client)

	return &Client{
		s3Client:      s3Client,
		presignClient: presignClient,
		bucket:        cfg.Bucket,
		publicBaseURL: strings.TrimRight(cfg.PublicBaseURL, "/"),
	}, nil
}

// KeyFor returns a tenant-namespaced S3 key: "orgs/<orgID>/<sanitizedPath>".
func KeyFor(orgID int64, relativePath string) string {
	clean := strings.TrimPrefix(strings.TrimSpace(relativePath), "/")
	return fmt.Sprintf("orgs/%d/%s", orgID, clean)
}

// Bucket returns the configured storage bucket name.
func (c *Client) Bucket() string {
	return c.bucket
}

// Put uploads an object with the specified content type and body reader.
func (c *Client) Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error {
	if strings.TrimSpace(key) == "" {
		return ErrInvalidKey
	}

	input := &s3.PutObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
		Body:   body,
	}
	if size > 0 {
		input.ContentLength = aws.Int64(size)
	}
	if contentType != "" {
		input.ContentType = aws.String(contentType)
	}

	_, err := c.s3Client.PutObject(ctx, input)
	if err != nil {
		return fmt.Errorf("storage: put %s: %w", key, err)
	}
	return nil
}

// Get downloads an object, returning its body stream and content type.
// The caller is responsible for closing the returned ReadCloser.
func (c *Client) Get(ctx context.Context, key string) (io.ReadCloser, string, error) {
	if strings.TrimSpace(key) == "" {
		return nil, "", ErrInvalidKey
	}

	out, err := c.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var nsk *types.NoSuchKey
		if errors.As(err, &nsk) {
			return nil, "", apperr.NotFound("file")
		}
		return nil, "", fmt.Errorf("storage: get %s: %w", key, err)
	}

	contentType := ""
	if out.ContentType != nil {
		contentType = *out.ContentType
	}
	return out.Body, contentType, nil
}

// Delete removes an object from storage.
func (c *Client) Delete(ctx context.Context, key string) error {
	if strings.TrimSpace(key) == "" {
		return ErrInvalidKey
	}

	_, err := c.s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("storage: delete %s: %w", key, err)
	}
	return nil
}

// PresignGet generates a temporary presigned URL for downloading an object.
func (c *Client) PresignGet(ctx context.Context, key string, lifetime time.Duration) (string, error) {
	if strings.TrimSpace(key) == "" {
		return "", ErrInvalidKey
	}

	req, err := c.presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(lifetime))
	if err != nil {
		return "", fmt.Errorf("storage: presign get %s: %w", key, err)
	}
	return req.URL, nil
}

// PresignPut generates a temporary presigned URL for direct client upload.
func (c *Client) PresignPut(ctx context.Context, key string, contentType string, lifetime time.Duration) (string, error) {
	if strings.TrimSpace(key) == "" {
		return "", ErrInvalidKey
	}

	input := &s3.PutObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	}
	if contentType != "" {
		input.ContentType = aws.String(contentType)
	}

	req, err := c.presignClient.PresignPutObject(ctx, input, s3.WithPresignExpires(lifetime))
	if err != nil {
		return "", fmt.Errorf("storage: presign put %s: %w", key, err)
	}
	return req.URL, nil
}

// HeadObject checks if an object exists and returns its ContentLength and ContentType.
func (c *Client) HeadObject(ctx context.Context, key string) (int64, string, error) {
	if strings.TrimSpace(key) == "" {
		return 0, "", ErrInvalidKey
	}

	out, err := c.s3Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var nsk *types.NoSuchKey
		var nf *types.NotFound
		if errors.As(err, &nsk) || errors.As(err, &nf) {
			return 0, "", apperr.NotFound("file")
		}
		return 0, "", fmt.Errorf("storage: head %s: %w", key, err)
	}

	var size int64
	if out.ContentLength != nil {
		size = *out.ContentLength
	}
	var contentType string
	if out.ContentType != nil {
		contentType = *out.ContentType
	}
	return size, contentType, nil
}

// PublicURL returns the public URL for an object if PublicBaseURL is configured.
// If PublicBaseURL is empty, it returns an empty string.
func (c *Client) PublicURL(key string) string {
	if c.publicBaseURL == "" || strings.TrimSpace(key) == "" {
		return ""
	}
	clean := strings.TrimPrefix(strings.TrimSpace(key), "/")
	return fmt.Sprintf("%s/%s", c.publicBaseURL, url.PathEscape(clean))
}
