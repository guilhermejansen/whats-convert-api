package providers

import "errors"

// Provider errors
var (
	// Configuration errors
	ErrInvalidProvider   = errors.New("invalid or unsupported S3 provider")
	ErrMissingEndpoint   = errors.New("S3 endpoint is required")
	ErrMissingBucket     = errors.New("S3 bucket name is required")
	ErrMissingAccessKey  = errors.New("S3 access key is required")
	ErrMissingSecretKey  = errors.New("S3 secret key is required")
	ErrMissingRegion     = errors.New("S3 region is required for AWS provider")

	// Upload errors
	ErrUploadFailed      = errors.New("upload operation failed")
	ErrMultipartFailed   = errors.New("multipart upload failed")
	ErrInvalidBase64     = errors.New("invalid base64 data")
	ErrFileTooLarge      = errors.New("file size exceeds maximum allowed")
	ErrInvalidContentType = errors.New("invalid or unsupported content type")
	ErrEmptyFile         = errors.New("file is empty")

	// Object errors
	ErrObjectNotFound    = errors.New("object not found")
	ErrObjectExists      = errors.New("object already exists")
	ErrDeleteFailed      = errors.New("failed to delete object")

	// Connection errors
	ErrConnectionFailed  = errors.New("failed to connect to S3 provider")
	ErrAuthenticationFailed = errors.New("S3 authentication failed")
	ErrPermissionDenied  = errors.New("insufficient permissions for S3 operation")
	ErrBucketNotFound    = errors.New("S3 bucket not found")

	// Provider-specific errors
	ErrProviderNotSupported = errors.New("S3 provider not supported")
	ErrFeatureNotSupported  = errors.New("feature not supported by this provider")
	ErrQuotaExceeded       = errors.New("storage quota exceeded")

	// Network/timeout errors
	ErrTimeout           = errors.New("operation timed out")
	ErrNetworkError      = errors.New("network error during S3 operation")
	ErrRetryExhausted    = errors.New("maximum retry attempts exceeded")
)

// S3Error wraps provider-specific errors with additional context
type S3Error struct {
	Provider   string
	Operation  string
	Key        string
	StatusCode int
	Err        error
}

func (e *S3Error) Error() string {
	if e.Key != "" {
		return "S3 " + e.Provider + " " + e.Operation + " failed for key '" + e.Key + "': " + e.Err.Error()
	}
	return "S3 " + e.Provider + " " + e.Operation + " failed: " + e.Err.Error()
}

func (e *S3Error) Unwrap() error {
	return e.Err
}

// NewS3Error creates a new S3Error with context
func NewS3Error(provider, operation, key string, statusCode int, err error) *S3Error {
	return &S3Error{
		Provider:   provider,
		Operation:  operation,
		Key:        key,
		StatusCode: statusCode,
		Err:        err,
	}
}

// IsRetryableError checks if an error should trigger a retry
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Network and timeout errors are retryable
	if errors.Is(err, ErrTimeout) || errors.Is(err, ErrNetworkError) {
		return true
	}

	// Check for S3Error with retryable status codes
	var s3Err *S3Error
	if errors.As(err, &s3Err) {
		// HTTP 5xx errors are generally retryable
		if s3Err.StatusCode >= 500 && s3Err.StatusCode < 600 {
			return true
		}
		// HTTP 429 (Too Many Requests) is retryable
		if s3Err.StatusCode == 429 {
			return true
		}
		// HTTP 408 (Request Timeout) is retryable
		if s3Err.StatusCode == 408 {
			return true
		}
	}

	return false
}

// IsTemporaryError checks if an error is temporary/transient
func IsTemporaryError(err error) bool {
	if err == nil {
		return false
	}

	// These errors are typically temporary
	tempErrors := []error{
		ErrTimeout,
		ErrNetworkError,
		ErrConnectionFailed,
	}

	for _, tempErr := range tempErrors {
		if errors.Is(err, tempErr) {
			return true
		}
	}

	return IsRetryableError(err)
}

// IsPermanentError checks if an error is permanent and should not be retried
func IsPermanentError(err error) bool {
	if err == nil {
		return false
	}

	// These errors are permanent and should not be retried
	permErrors := []error{
		ErrInvalidProvider,
		ErrMissingEndpoint,
		ErrMissingBucket,
		ErrMissingAccessKey,
		ErrMissingSecretKey,
		ErrAuthenticationFailed,
		ErrPermissionDenied,
		ErrInvalidBase64,
		ErrInvalidContentType,
		ErrProviderNotSupported,
		ErrFeatureNotSupported,
	}

	for _, permErr := range permErrors {
		if errors.Is(err, permErr) {
			return true
		}
	}

	// Check for S3Error with permanent status codes
	var s3Err *S3Error
	if errors.As(err, &s3Err) {
		// HTTP 4xx errors (except 408, 429) are generally permanent
		if s3Err.StatusCode >= 400 && s3Err.StatusCode < 500 {
			if s3Err.StatusCode != 408 && s3Err.StatusCode != 429 {
				return true
			}
		}
	}

	return false
}