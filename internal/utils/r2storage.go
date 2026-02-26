package utils

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// R2Storage handles saving, deleting, and presigning files on Cloudflare R2.
type R2Storage struct {
	client     *s3.Client
	presigner  *s3.PresignClient
	bucketName string
}

// NewR2Storage creates an R2Storage client.
// endpoint should be "https://<account-id>.r2.cloudflarestorage.com".
func NewR2Storage(accessKeyID, secretAccessKey, endpoint, bucketName string) *R2Storage {
	cfg := aws.Config{
		Region: "auto",
		Credentials: credentials.NewStaticCredentialsProvider(
			accessKeyID,
			secretAccessKey,
			"", // session token — not used for R2
		),
		BaseEndpoint: aws.String(endpoint),
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		// R2 requires path-style addressing
		o.UsePathStyle = true
	})

	return &R2Storage{
		client:     client,
		presigner:  s3.NewPresignClient(client),
		bucketName: bucketName,
	}
}

// SaveFile uploads the contents of reader to R2 at <subDir>/<uniqueFilename>.
// It returns the object key (url_suffix) that can be stored in DB.
func (rs *R2Storage) SaveFile(subDir, originalFilename string, reader io.Reader) (string, error) {
	ext := filepath.Ext(originalFilename)
	uniqueName := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
	// Use forward slashes for the object key
	objectKey := subDir + "/" + uniqueName

	_, err := rs.client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket: aws.String(rs.bucketName),
		Key:    aws.String(objectKey),
		Body:   reader,
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload to R2: %w", err)
	}

	return objectKey, nil
}

// DeleteFile removes the object with the given key from R2.
// It is safe to call if the object does not exist.
func (rs *R2Storage) DeleteFile(objectKey string) error {
	_, err := rs.client.DeleteObject(context.TODO(), &s3.DeleteObjectInput{
		Bucket: aws.String(rs.bucketName),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		return fmt.Errorf("failed to delete from R2: %w", err)
	}
	return nil
}

// PresignGetObject generates a presigned GET URL for the given object key.
// The URL is valid for the specified duration.
func (rs *R2Storage) PresignGetObject(objectKey string, duration time.Duration) (string, error) {
	req, err := rs.presigner.PresignGetObject(context.TODO(), &s3.GetObjectInput{
		Bucket: aws.String(rs.bucketName),
		Key:    aws.String(objectKey),
	}, s3.WithPresignExpires(duration))
	if err != nil {
		return "", fmt.Errorf("failed to presign URL: %w", err)
	}
	return req.URL, nil
}
