package providers

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// MinIOProvider implements the S3Provider interface for MinIO
type MinIOProvider struct {
	client *minio.Client
	config *S3Config
}

// NewMinIOProvider creates a new MinIO provider
func NewMinIOProvider(cfg *S3Config) (*MinIOProvider, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid MinIO config: %w", err)
	}

	// Extract endpoint without protocol for MinIO client
	endpoint := cfg.Endpoint
	if strings.HasPrefix(endpoint, "http://") {
		endpoint = strings.TrimPrefix(endpoint, "http://")
		cfg.UseSSL = false
	} else if strings.HasPrefix(endpoint, "https://") {
		endpoint = strings.TrimPrefix(endpoint, "https://")
		cfg.UseSSL = true
	}

	// Create MinIO client
	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, NewS3Error("minio", "configure", "", 0, err)
	}

	return &MinIOProvider{
		client: minioClient,
		config: cfg,
	}, nil
}

// Upload uploads data from a reader to the specified key
func (p *MinIOProvider) Upload(ctx context.Context, key string, reader io.Reader, size int64, opts UploadOptions) (*UploadResult, error) {
	startTime := time.Now()

	// Use multipart upload for large files
	if size >= p.config.MultipartThreshold {
		return p.MultipartUpload(ctx, key, reader, opts)
	}

	baseReader := reader
	seeker, isSeekable := baseReader.(io.ReadSeeker)

	// Prepare static upload options (without progress reader)
	baseOpts := minio.PutObjectOptions{
		ContentType: opts.ContentType,
	}

	// Add user metadata
	if len(opts.Metadata) > 0 {
		baseOpts.UserMetadata = opts.Metadata
	}

	// Set storage class
	if opts.StorageClass != "" {
		baseOpts.StorageClass = opts.StorageClass
	}

	// Perform upload with retry logic
	var info minio.UploadInfo
	var err error

	for attempt := 0; attempt <= p.config.RetryCount; attempt++ {
		// Reset reader if possible before each attempt (after the first)
		if attempt > 0 {
			if isSeekable {
				if _, seekErr := seeker.Seek(0, io.SeekStart); seekErr != nil {
					return nil, NewS3Error("minio", "upload", key, 0, fmt.Errorf("failed to reset reader: %w", seekErr))
				}
				if opts.ProgressCallback != nil {
					opts.ProgressCallback(0, size)
				}
			} else {
				return nil, NewS3Error("minio", "upload", key, 0, fmt.Errorf("reader is not seekable; cannot retry upload"))
			}
		}

		// Create per-attempt options to avoid sharing progress reader between attempts
		putOpts := baseOpts
		uploadCtx, cancel := context.WithTimeout(ctx, p.config.UploadTimeout)
		info, err = p.client.PutObject(uploadCtx, p.config.Bucket, key, baseReader, size, putOpts)
		cancel()

		if err == nil {
			break
		}

		if !IsRetryableError(err) || attempt == p.config.RetryCount {
			return nil, NewS3Error("minio", "upload", key, 0, err)
		}

		// Wait before retry
		select {
		case <-ctx.Done():
			return nil, NewS3Error("minio", "upload", key, 0, ctx.Err())
		case <-time.After(time.Duration(attempt+1) * time.Second):
			// Continue to next attempt
		}
	}

	// Build upload result
	uploadResult := &UploadResult{
		Key:            key,
		PublicURL:      p.GetPublicURL(key),
		Size:           info.Size,
		ETag:           info.ETag,
		Provider:       "minio",
		ProcessingTime: time.Since(startTime),
	}

	if info.VersionID != "" {
		uploadResult.VersionID = info.VersionID
	}

	// Set expiration if configured
	if opts.ExpirationDays > 0 {
		expiresAt := time.Now().AddDate(0, 0, opts.ExpirationDays)
		uploadResult.ExpiresAt = &expiresAt
	}

	return uploadResult, nil
}

// MultipartUpload handles large file uploads using multipart upload
func (p *MinIOProvider) MultipartUpload(ctx context.Context, key string, reader io.Reader, opts UploadOptions) (*UploadResult, error) {
	startTime := time.Now()

	// Prepare upload options for multipart
	putOpts := minio.PutObjectOptions{
		ContentType: opts.ContentType,
		PartSize:    uint64(opts.ChunkSize),
	}

	if opts.ChunkSize == 0 {
		putOpts.PartSize = uint64(p.config.ChunkSize)
	}

	// Add user metadata
	if len(opts.Metadata) > 0 {
		putOpts.UserMetadata = opts.Metadata
	}

	// Set storage class
	if opts.StorageClass != "" {
		putOpts.StorageClass = opts.StorageClass
	}

	// Add progress callback if provided
	if opts.ProgressCallback != nil {
		putOpts.Progress = &progressReader{
			reader:   reader,
			callback: opts.ProgressCallback,
			total:    -1, // Unknown size for multipart
		}
		reader = putOpts.Progress
	}

	// Perform multipart upload (MinIO handles this automatically)
	info, err := p.client.PutObject(ctx, p.config.Bucket, key, reader, -1, putOpts)
	if err != nil {
		return nil, NewS3Error("minio", "multipart_upload", key, 0, err)
	}

	// Build upload result
	uploadResult := &UploadResult{
		Key:            key,
		PublicURL:      p.GetPublicURL(key),
		Size:           info.Size,
		ETag:           info.ETag,
		Provider:       "minio",
		ProcessingTime: time.Since(startTime),
	}

	if info.VersionID != "" {
		uploadResult.VersionID = info.VersionID
	}

	// Set expiration if configured
	if opts.ExpirationDays > 0 {
		expiresAt := time.Now().AddDate(0, 0, opts.ExpirationDays)
		uploadResult.ExpiresAt = &expiresAt
	}

	return uploadResult, nil
}

// UploadBase64 uploads base64-encoded data to the specified key
func (p *MinIOProvider) UploadBase64(ctx context.Context, key string, data string, opts UploadOptions) (*UploadResult, error) {
	// Parse data URL if present (data:mime/type;base64,xxxxx)
	var base64Data string
	var detectedContentType string

	if strings.HasPrefix(data, "data:") {
		parts := strings.Split(data, ",")
		if len(parts) != 2 {
			return nil, NewS3Error("minio", "parse_base64", key, 0, ErrInvalidBase64)
		}

		// Extract content type from data URL
		header := parts[0]
		if strings.Contains(header, ";base64") {
			contentTypePart := strings.Split(header, ";")[0]
			detectedContentType = strings.TrimPrefix(contentTypePart, "data:")
		}

		base64Data = parts[1]
	} else {
		base64Data = data
	}

	// Decode base64 data
	decodedData, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return nil, NewS3Error("minio", "decode_base64", key, 0, ErrInvalidBase64)
	}

	// Use detected content type if not provided
	if opts.ContentType == "" && detectedContentType != "" {
		opts.ContentType = detectedContentType
	}

	// Upload decoded data
	reader := strings.NewReader(string(decodedData))
	return p.Upload(ctx, key, reader, int64(len(decodedData)), opts)
}

// GetPublicURL returns the public URL for accessing the uploaded object
func (p *MinIOProvider) GetPublicURL(key string) string {
	return p.config.GetPublicURL(key)
}

// SetExpiration sets expiration for an object
func (p *MinIOProvider) SetExpiration(key string, days int) error {
	// MinIO supports object expiration through lifecycle policies
	// This is a placeholder for future implementation
	return ErrFeatureNotSupported
}

// HealthCheck verifies the provider connection and configuration
func (p *MinIOProvider) HealthCheck(ctx context.Context) error {
	// Check if bucket exists and is accessible
	exists, err := p.client.BucketExists(ctx, p.config.Bucket)
	if err != nil {
		return NewS3Error("minio", "health_check", "", 0, err)
	}

	if !exists {
		return NewS3Error("minio", "health_check", "", 0, ErrBucketNotFound)
	}

	return nil
}

// DeleteObject removes an object from storage
func (p *MinIOProvider) DeleteObject(ctx context.Context, key string) error {
	err := p.client.RemoveObject(ctx, p.config.Bucket, key, minio.RemoveObjectOptions{})
	if err != nil {
		return NewS3Error("minio", "delete", key, 0, err)
	}

	return nil
}

// GetObjectInfo retrieves metadata about an object
func (p *MinIOProvider) GetObjectInfo(ctx context.Context, key string) (*ObjectInfo, error) {
	objInfo, err := p.client.StatObject(ctx, p.config.Bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return nil, NewS3Error("minio", "stat_object", key, 0, err)
	}

	info := &ObjectInfo{
		Key:          key,
		Size:         objInfo.Size,
		ETag:         objInfo.ETag,
		ContentType:  objInfo.ContentType,
		LastModified: objInfo.LastModified,
		Metadata:     objInfo.UserMetadata,
		StorageClass: objInfo.StorageClass,
	}

	if objInfo.VersionID != "" {
		info.VersionID = objInfo.VersionID
	}

	return info, nil
}

// progressReader wraps an io.Reader to provide progress callbacks
type progressReader struct {
	reader   io.Reader
	callback func(bytesTransferred, totalBytes int64)
	total    int64
	read     int64
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	if n > 0 {
		pr.read += int64(n)
		if pr.callback != nil {
			pr.callback(pr.read, pr.total)
		}
	}
	return n, err
}
