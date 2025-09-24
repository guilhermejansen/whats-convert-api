package providers

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// BackblazeProvider implements the S3Provider interface for Backblaze B2
type BackblazeProvider struct {
	client *s3.Client
	config *S3Config
}

// NewBackblazeProvider creates a new Backblaze B2 provider
func NewBackblazeProvider(cfg *S3Config) (*BackblazeProvider, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid Backblaze B2 config: %w", err)
	}

	// Backblaze B2 requires specific configuration
	if cfg.Region == "" {
		cfg.Region = "us-west-000" // Default Backblaze region
	}

	// Create AWS config for Backblaze B2 (S3-compatible)
	awsConfig, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(cfg.Region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AccessKey,
			cfg.SecretKey,
			"",
		)),
	)
	if err != nil {
		return nil, NewS3Error("backblaze", "configure", "", 0, err)
	}

	// Create S3 client with Backblaze B2 endpoint
	s3Client := s3.NewFromConfig(awsConfig, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(cfg.Endpoint)
		o.UsePathStyle = true // Backblaze B2 requires path-style URLs
	})

	return &BackblazeProvider{
		client: s3Client,
		config: cfg,
	}, nil
}

// Upload uploads data from a reader to the specified key
func (p *BackblazeProvider) Upload(ctx context.Context, key string, reader io.Reader, size int64, opts UploadOptions) (*UploadResult, error) {
	startTime := time.Now()

	// Use multipart upload for large files
	if size >= p.config.MultipartThreshold {
		return p.MultipartUpload(ctx, key, reader, opts)
	}

	// Prepare upload input
	input := &s3.PutObjectInput{
		Bucket:      aws.String(p.config.Bucket),
		Key:         aws.String(key),
		Body:        reader,
		ContentType: aws.String(opts.ContentType),
	}

	// Backblaze B2 doesn't support ACLs in the same way as AWS S3
	// Public access is controlled at the bucket level

	// Add metadata
	if len(opts.Metadata) > 0 {
		input.Metadata = opts.Metadata
	}

	// Backblaze B2 storage classes
	if opts.StorageClass != "" {
		// Map to Backblaze B2 storage classes
		switch opts.StorageClass {
		case "STANDARD":
			input.StorageClass = types.StorageClassStandard
		case "REDUCED_REDUNDANCY":
			input.StorageClass = types.StorageClassReducedRedundancy
		default:
			input.StorageClass = types.StorageClassStandard
		}
	}

	// Perform upload with retry logic
	var result *s3.PutObjectOutput
	var err error

	for attempt := 0; attempt <= p.config.RetryCount; attempt++ {
		uploadCtx, cancel := context.WithTimeout(ctx, p.config.UploadTimeout)
		result, err = p.client.PutObject(uploadCtx, input)
		cancel()

		if err == nil {
			break
		}

		if !IsRetryableError(err) || attempt == p.config.RetryCount {
			return nil, NewS3Error("backblaze", "upload", key, 0, err)
		}

		// Wait before retry
		select {
		case <-ctx.Done():
			return nil, NewS3Error("backblaze", "upload", key, 0, ctx.Err())
		case <-time.After(time.Duration(attempt+1) * time.Second):
			// Continue to next attempt
		}
	}

	// Build upload result
	uploadResult := &UploadResult{
		Key:            key,
		PublicURL:      p.GetPublicURL(key),
		Size:           size,
		ETag:           aws.ToString(result.ETag),
		Provider:       "backblaze",
		ProcessingTime: time.Since(startTime),
	}

	if result.VersionId != nil {
		uploadResult.VersionID = aws.ToString(result.VersionId)
	}

	// Set expiration if configured
	if opts.ExpirationDays > 0 {
		expiresAt := time.Now().AddDate(0, 0, opts.ExpirationDays)
		uploadResult.ExpiresAt = &expiresAt
	}

	return uploadResult, nil
}

// MultipartUpload handles large file uploads using multipart upload
func (p *BackblazeProvider) MultipartUpload(ctx context.Context, key string, reader io.Reader, opts UploadOptions) (*UploadResult, error) {
	startTime := time.Now()

	// Create multipart upload
	createInput := &s3.CreateMultipartUploadInput{
		Bucket:      aws.String(p.config.Bucket),
		Key:         aws.String(key),
		ContentType: aws.String(opts.ContentType),
	}

	// Add metadata
	if len(opts.Metadata) > 0 {
		createInput.Metadata = opts.Metadata
	}

	// Set storage class
	if opts.StorageClass != "" {
		switch opts.StorageClass {
		case "STANDARD":
			createInput.StorageClass = types.StorageClassStandard
		case "REDUCED_REDUNDANCY":
			createInput.StorageClass = types.StorageClassReducedRedundancy
		default:
			createInput.StorageClass = types.StorageClassStandard
		}
	}

	createResult, err := p.client.CreateMultipartUpload(ctx, createInput)
	if err != nil {
		return nil, NewS3Error("backblaze", "create_multipart", key, 0, err)
	}

	uploadID := aws.ToString(createResult.UploadId)

	// Upload parts
	var completedParts []types.CompletedPart
	partNumber := int32(1)
	chunkSize := opts.ChunkSize
	if chunkSize == 0 {
		chunkSize = p.config.ChunkSize
	}

	buffer := make([]byte, chunkSize)
	totalBytesTransferred := int64(0)

	for {
		n, readErr := reader.Read(buffer)
		if n == 0 {
			break
		}

		// Upload part
		partInput := &s3.UploadPartInput{
			Bucket:     aws.String(p.config.Bucket),
			Key:        aws.String(key),
			PartNumber: partNumber,
			UploadId:   createResult.UploadId,
			Body:       strings.NewReader(string(buffer[:n])),
		}

		partResult, err := p.client.UploadPart(ctx, partInput)
		if err != nil {
			// Abort multipart upload on error
			_, _ = p.client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
				Bucket:   aws.String(p.config.Bucket),
				Key:      aws.String(key),
				UploadId: createResult.UploadId,
			})
			return nil, NewS3Error("backblaze", "upload_part", key, 0, err)
		}

		completedParts = append(completedParts, types.CompletedPart{
			ETag:       partResult.ETag,
			PartNumber: partNumber,
		})

		totalBytesTransferred += int64(n)

		// Report progress
		if opts.ProgressCallback != nil {
			opts.ProgressCallback(totalBytesTransferred, totalBytesTransferred)
		}

		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			// Abort multipart upload on error
			_, _ = p.client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
				Bucket:   aws.String(p.config.Bucket),
				Key:      aws.String(key),
				UploadId: createResult.UploadId,
			})
			return nil, NewS3Error("backblaze", "read_data", key, 0, readErr)
		}

		partNumber++
	}

	// Complete multipart upload
	completeInput := &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(p.config.Bucket),
		Key:      aws.String(key),
		UploadId: createResult.UploadId,
		MultipartUpload: &types.CompletedMultipartUpload{
			Parts: completedParts,
		},
	}

	completeResult, err := p.client.CompleteMultipartUpload(ctx, completeInput)
	if err != nil {
		return nil, NewS3Error("backblaze", "complete_multipart", key, 0, err)
	}

	// Build upload result
	uploadResult := &UploadResult{
		Key:            key,
		PublicURL:      p.GetPublicURL(key),
		Size:           totalBytesTransferred,
		ETag:           aws.ToString(completeResult.ETag),
		UploadID:       uploadID,
		Provider:       "backblaze",
		ProcessingTime: time.Since(startTime),
	}

	if completeResult.VersionId != nil {
		uploadResult.VersionID = aws.ToString(completeResult.VersionId)
	}

	// Set expiration if configured
	if opts.ExpirationDays > 0 {
		expiresAt := time.Now().AddDate(0, 0, opts.ExpirationDays)
		uploadResult.ExpiresAt = &expiresAt
	}

	return uploadResult, nil
}

// UploadBase64 uploads base64-encoded data to the specified key
func (p *BackblazeProvider) UploadBase64(ctx context.Context, key string, data string, opts UploadOptions) (*UploadResult, error) {
	// Parse data URL if present (data:mime/type;base64,xxxxx)
	var base64Data string
	var detectedContentType string

	if strings.HasPrefix(data, "data:") {
		parts := strings.Split(data, ",")
		if len(parts) != 2 {
			return nil, NewS3Error("backblaze", "parse_base64", key, 0, ErrInvalidBase64)
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
		return nil, NewS3Error("backblaze", "decode_base64", key, 0, ErrInvalidBase64)
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
func (p *BackblazeProvider) GetPublicURL(key string) string {
	return p.config.GetPublicURL(key)
}

// SetExpiration sets expiration for an object
func (p *BackblazeProvider) SetExpiration(key string, days int) error {
	// Backblaze B2 supports lifecycle rules for expiration
	// This is a placeholder for future implementation
	return ErrFeatureNotSupported
}

// HealthCheck verifies the provider connection and configuration
func (p *BackblazeProvider) HealthCheck(ctx context.Context) error {
	// Check if bucket exists and is accessible
	_, err := p.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(p.config.Bucket),
	})
	if err != nil {
		return NewS3Error("backblaze", "health_check", "", 0, err)
	}

	return nil
}

// DeleteObject removes an object from storage
func (p *BackblazeProvider) DeleteObject(ctx context.Context, key string) error {
	_, err := p.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(p.config.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return NewS3Error("backblaze", "delete", key, 0, err)
	}

	return nil
}

// GetObjectInfo retrieves metadata about an object
func (p *BackblazeProvider) GetObjectInfo(ctx context.Context, key string) (*ObjectInfo, error) {
	result, err := p.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(p.config.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, NewS3Error("backblaze", "head_object", key, 0, err)
	}

	info := &ObjectInfo{
		Key:          key,
		Size:         result.ContentLength,
		ETag:         aws.ToString(result.ETag),
		ContentType:  aws.ToString(result.ContentType),
		LastModified: aws.ToTime(result.LastModified),
		Metadata:     result.Metadata,
	}

	if result.StorageClass != "" {
		info.StorageClass = string(result.StorageClass)
	}

	if result.VersionId != nil {
		info.VersionID = aws.ToString(result.VersionId)
	}

	return info, nil
}
