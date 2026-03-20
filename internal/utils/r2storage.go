package utils

import (
	"context"
	"fmt"
	"io"
	"mime"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// R2Storage handles saving and deleting files on Cloudflare R2.
type R2Storage struct {
	client     *s3.Client
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
		bucketName: bucketName,
	}
}

// SaveFile uploads the contents of reader to R2 at <subDir>/<uniqueFilename>.
// It returns the object key (url_suffix) that can be stored in DB.
func (rs *R2Storage) SaveFile(subDir, originalFilename string, reader io.Reader) (string, error) {
	ext := strings.ToLower(filepath.Ext(originalFilename))
	uniqueName := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
	// Use forward slashes for the object key
	objectKey := subDir + "/" + uniqueName

	// Detect MIME type from file extension so browsers display images inline
	contentType := mime.TypeByExtension(ext)
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	_, err := rs.client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:      aws.String(rs.bucketName),
		Key:         aws.String(objectKey),
		Body:        reader,
		ContentType: aws.String(contentType),
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


