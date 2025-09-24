package providers

import (
	"context"
	"io"
	"time"
)

// S3Provider defines the interface for all S3-compatible storage providers
type S3Provider interface {
	// Upload uploads data from a reader to the specified key
	Upload(ctx context.Context, key string, reader io.Reader, size int64, opts UploadOptions) (*UploadResult, error)

	// MultipartUpload handles large file uploads using multipart upload
	MultipartUpload(ctx context.Context, key string, reader io.Reader, opts UploadOptions) (*UploadResult, error)

	// UploadBase64 uploads base64-encoded data to the specified key
	UploadBase64(ctx context.Context, key string, data string, opts UploadOptions) (*UploadResult, error)

	// GetPublicURL returns the public URL for accessing the uploaded object
	GetPublicURL(key string) string

	// SetExpiration sets expiration for an object (if supported by provider)
	SetExpiration(key string, days int) error

	// HealthCheck verifies the provider connection and configuration
	HealthCheck(ctx context.Context) error

	// DeleteObject removes an object from storage
	DeleteObject(ctx context.Context, key string) error

	// GetObjectInfo retrieves metadata about an object
	GetObjectInfo(ctx context.Context, key string) (*ObjectInfo, error)
}

// UploadOptions contains options for upload operations
type UploadOptions struct {
	// ContentType specifies the MIME type of the object
	ContentType string

	// Metadata contains user-defined metadata key-value pairs
	Metadata map[string]string

	// Public determines if the object should be publicly readable
	Public bool

	// ExpirationDays sets object expiration (0 = no expiration)
	ExpirationDays int

	// StorageClass specifies the storage class (STANDARD, REDUCED_REDUNDANCY, etc.)
	StorageClass string

	// ProgressCallback is called during upload to report progress
	ProgressCallback func(bytesTransferred, totalBytes int64)

	// ChunkSize for multipart uploads (in bytes)
	ChunkSize int64

	// MaxConcurrentParts for parallel multipart uploads
	MaxConcurrentParts int
}

// UploadResult contains information about a successful upload
type UploadResult struct {
	// Key is the object key/path in the bucket
	Key string `json:"key"`

	// PublicURL is the public URL to access the object
	PublicURL string `json:"url"`

	// Size is the uploaded object size in bytes
	Size int64 `json:"size"`

	// ETag is the entity tag of the uploaded object
	ETag string `json:"etag"`

	// VersionID is the version identifier (if versioning is enabled)
	VersionID string `json:"version_id,omitempty"`

	// ExpiresAt indicates when the object expires (if applicable)
	ExpiresAt *time.Time `json:"expires_at,omitempty"`

	// Provider identifies which S3 provider was used
	Provider string `json:"provider"`

	// UploadID for tracking multipart uploads
	UploadID string `json:"upload_id,omitempty"`

	// ProcessingTime tracks how long the upload took
	ProcessingTime time.Duration `json:"processing_time"`
}

// ObjectInfo contains metadata about an object
type ObjectInfo struct {
	Key          string            `json:"key"`
	Size         int64             `json:"size"`
	ETag         string            `json:"etag"`
	ContentType  string            `json:"content_type"`
	LastModified time.Time         `json:"last_modified"`
	Metadata     map[string]string `json:"metadata"`
	StorageClass string            `json:"storage_class"`
	VersionID    string            `json:"version_id,omitempty"`
}

// ProviderType represents the supported S3 provider types
type ProviderType string

const (
	ProviderAWS          ProviderType = "aws"
	ProviderMinIO        ProviderType = "minio"
	ProviderBackblaze    ProviderType = "backblaze"
	ProviderDigitalOcean ProviderType = "digitalocean"
	ProviderCloudflare   ProviderType = "cloudflare"
	ProviderWasabi       ProviderType = "wasabi"
)

// S3Config contains configuration for S3 providers
type S3Config struct {
	// Provider type (aws, minio, backblaze, etc.)
	Provider ProviderType `json:"provider"`

	// Endpoint URL (e.g., https://s3.amazonaws.com)
	Endpoint string `json:"endpoint"`

	// PublicEndpoint for generating public URLs
	PublicEndpoint string `json:"public_endpoint"`

	// Region for AWS and compatible services
	Region string `json:"region"`

	// Bucket name
	Bucket string `json:"bucket"`

	// AccessKey for authentication
	AccessKey string `json:"access_key"`

	// SecretKey for authentication
	SecretKey string `json:"secret_key"`

	// UseSSL determines if HTTPS should be used
	UseSSL bool `json:"use_ssl"`

	// PathStyle forces path-style URLs (for MinIO compatibility)
	PathStyle bool `json:"path_style"`

	// PublicRead makes all uploaded objects publicly readable
	PublicRead bool `json:"public_read"`

	// DefaultExpirationDays for uploaded objects (0 = no expiration)
	DefaultExpirationDays int `json:"default_expiration_days"`

	// MultipartThreshold in bytes (default: 5MB)
	MultipartThreshold int64 `json:"multipart_threshold"`

	// ChunkSize for multipart uploads in bytes (default: 10MB)
	ChunkSize int64 `json:"chunk_size"`

	// MaxConcurrentUploads limits parallel operations
	MaxConcurrentUploads int `json:"max_concurrent_uploads"`

	// UploadTimeout for individual upload operations
	UploadTimeout time.Duration `json:"upload_timeout"`

	// RetryCount for failed operations
	RetryCount int `json:"retry_count"`
}

// DefaultUploadOptions returns default upload options
func DefaultUploadOptions() UploadOptions {
	return UploadOptions{
		ContentType:        "application/octet-stream",
		Metadata:          make(map[string]string),
		Public:            true,
		ExpirationDays:    0,
		StorageClass:      "STANDARD",
		ChunkSize:         10 * 1024 * 1024, // 10MB
		MaxConcurrentParts: 3,
	}
}

// Validate checks if the S3Config is valid
func (c *S3Config) Validate() error {
	if c.Provider == "" {
		return ErrInvalidProvider
	}
	if c.Endpoint == "" {
		return ErrMissingEndpoint
	}
	if c.Bucket == "" {
		return ErrMissingBucket
	}
	if c.AccessKey == "" {
		return ErrMissingAccessKey
	}
	if c.SecretKey == "" {
		return ErrMissingSecretKey
	}

	// Set defaults
	if c.MultipartThreshold == 0 {
		c.MultipartThreshold = 5 * 1024 * 1024 // 5MB
	}
	if c.ChunkSize == 0 {
		c.ChunkSize = 10 * 1024 * 1024 // 10MB
	}
	if c.MaxConcurrentUploads == 0 {
		c.MaxConcurrentUploads = 3
	}
	if c.UploadTimeout == 0 {
		c.UploadTimeout = time.Hour // 1 hour default
	}
	if c.RetryCount == 0 {
		c.RetryCount = 3
	}

	return nil
}

// GetPublicURL generates a public URL for the given key
func (c *S3Config) GetPublicURL(key string) string {
	if c.PublicEndpoint != "" {
		if c.PathStyle {
			return c.PublicEndpoint + "/" + c.Bucket + "/" + key
		}
		return c.PublicEndpoint + "/" + key
	}

	// Fallback to endpoint
	if c.PathStyle {
		return c.Endpoint + "/" + c.Bucket + "/" + key
	}
	return "https://" + c.Bucket + "." + c.Endpoint + "/" + key
}