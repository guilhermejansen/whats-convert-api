package providers

import (
	"fmt"
	"strings"
)

// ProviderFactory creates S3Provider instances based on configuration
type ProviderFactory struct{}

// NewProviderFactory creates a new provider factory
func NewProviderFactory() *ProviderFactory {
	return &ProviderFactory{}
}

// CreateProvider creates an S3Provider based on the configuration
func (f *ProviderFactory) CreateProvider(config *S3Config) (S3Provider, error) {
	if config == nil {
		return nil, fmt.Errorf("S3 config cannot be nil")
	}

	// Normalize provider name
	providerType := ProviderType(strings.ToLower(string(config.Provider)))

	switch providerType {
	case ProviderAWS:
		return NewAWSProvider(config)
	case ProviderMinIO:
		return NewMinIOProvider(config)
	case ProviderBackblaze:
		return NewBackblazeProvider(config)
	case ProviderDigitalOcean:
		// DigitalOcean Spaces is S3-compatible, use AWS provider with custom endpoint
		return NewDigitalOceanProvider(config)
	case ProviderCloudflare:
		// Cloudflare R2 is S3-compatible, use AWS provider with custom endpoint
		return NewCloudflareProvider(config)
	case ProviderWasabi:
		// Wasabi is S3-compatible, use AWS provider with custom endpoint
		return NewWasabiProvider(config)
	default:
		return nil, fmt.Errorf("%w: %s", ErrProviderNotSupported, config.Provider)
	}
}

// GetSupportedProviders returns a list of supported provider types
func (f *ProviderFactory) GetSupportedProviders() []ProviderType {
	return []ProviderType{
		ProviderAWS,
		ProviderMinIO,
		ProviderBackblaze,
		ProviderDigitalOcean,
		ProviderCloudflare,
		ProviderWasabi,
	}
}

// IsProviderSupported checks if a provider type is supported
func (f *ProviderFactory) IsProviderSupported(providerType ProviderType) bool {
	supported := f.GetSupportedProviders()
	for _, p := range supported {
		if p == providerType {
			return true
		}
	}
	return false
}

// ValidateProviderConfig validates the configuration for a specific provider
func (f *ProviderFactory) ValidateProviderConfig(config *S3Config) error {
	if config == nil {
		return fmt.Errorf("S3 config cannot be nil")
	}

	providerType := ProviderType(strings.ToLower(string(config.Provider)))

	// Provider-specific validation
	switch providerType {
	case ProviderAWS:
		return f.validateAWSConfig(config)
	case ProviderMinIO:
		return f.validateMinIOConfig(config)
	case ProviderBackblaze:
		return f.validateBackblazeConfig(config)
	case ProviderDigitalOcean:
		return f.validateDigitalOceanConfig(config)
	case ProviderCloudflare:
		return f.validateCloudflareConfig(config)
	case ProviderWasabi:
		return f.validateWasabiConfig(config)
	default:
		return fmt.Errorf("%w: %s", ErrProviderNotSupported, config.Provider)
	}
}

// validateAWSConfig validates AWS S3 specific configuration
func (f *ProviderFactory) validateAWSConfig(config *S3Config) error {
	if config.Region == "" {
		return ErrMissingRegion
	}
	return config.Validate()
}

// validateMinIOConfig validates MinIO specific configuration
func (f *ProviderFactory) validateMinIOConfig(config *S3Config) error {
	// MinIO requires endpoint
	if config.Endpoint == "" {
		return ErrMissingEndpoint
	}
	return config.Validate()
}

// validateBackblazeConfig validates Backblaze B2 specific configuration
func (f *ProviderFactory) validateBackblazeConfig(config *S3Config) error {
	// Backblaze requires specific endpoint format
	if config.Endpoint == "" {
		return ErrMissingEndpoint
	}
	if !strings.Contains(config.Endpoint, "backblazeb2.com") {
		return fmt.Errorf("invalid Backblaze B2 endpoint: %s", config.Endpoint)
	}
	return config.Validate()
}

// validateDigitalOceanConfig validates DigitalOcean Spaces specific configuration
func (f *ProviderFactory) validateDigitalOceanConfig(config *S3Config) error {
	if config.Endpoint == "" {
		return ErrMissingEndpoint
	}
	if config.Region == "" {
		return ErrMissingRegion
	}
	return config.Validate()
}

// validateCloudflareConfig validates Cloudflare R2 specific configuration
func (f *ProviderFactory) validateCloudflareConfig(config *S3Config) error {
	if config.Endpoint == "" {
		return ErrMissingEndpoint
	}
	return config.Validate()
}

// validateWasabiConfig validates Wasabi specific configuration
func (f *ProviderFactory) validateWasabiConfig(config *S3Config) error {
	if config.Endpoint == "" {
		return ErrMissingEndpoint
	}
	if config.Region == "" {
		return ErrMissingRegion
	}
	return config.Validate()
}

// GetProviderDefaults returns default configuration for each provider
func (f *ProviderFactory) GetProviderDefaults(providerType ProviderType) *S3Config {
	switch providerType {
	case ProviderAWS:
		return &S3Config{
			Provider:               ProviderAWS,
			Endpoint:               "https://s3.amazonaws.com",
			UseSSL:                 true,
			PathStyle:              false,
			PublicRead:             false,
			MultipartThreshold:     5 * 1024 * 1024,  // 5MB
			ChunkSize:              10 * 1024 * 1024, // 10MB
			MaxConcurrentUploads:   3,
			RetryCount:             3,
		}
	case ProviderMinIO:
		return &S3Config{
			Provider:               ProviderMinIO,
			UseSSL:                 true,
			PathStyle:              true,
			PublicRead:             true,
			MultipartThreshold:     5 * 1024 * 1024,  // 5MB
			ChunkSize:              10 * 1024 * 1024, // 10MB
			MaxConcurrentUploads:   3,
			RetryCount:             3,
		}
	case ProviderBackblaze:
		return &S3Config{
			Provider:               ProviderBackblaze,
			UseSSL:                 true,
			PathStyle:              true,
			PublicRead:             false,
			MultipartThreshold:     5 * 1024 * 1024,  // 5MB
			ChunkSize:              10 * 1024 * 1024, // 10MB
			MaxConcurrentUploads:   3,
			RetryCount:             3,
		}
	case ProviderDigitalOcean:
		return &S3Config{
			Provider:               ProviderDigitalOcean,
			UseSSL:                 true,
			PathStyle:              false,
			PublicRead:             false,
			MultipartThreshold:     5 * 1024 * 1024,  // 5MB
			ChunkSize:              10 * 1024 * 1024, // 10MB
			MaxConcurrentUploads:   3,
			RetryCount:             3,
		}
	case ProviderCloudflare:
		return &S3Config{
			Provider:               ProviderCloudflare,
			UseSSL:                 true,
			PathStyle:              false,
			PublicRead:             false,
			MultipartThreshold:     5 * 1024 * 1024,  // 5MB
			ChunkSize:              10 * 1024 * 1024, // 10MB
			MaxConcurrentUploads:   3,
			RetryCount:             3,
		}
	case ProviderWasabi:
		return &S3Config{
			Provider:               ProviderWasabi,
			UseSSL:                 true,
			PathStyle:              false,
			PublicRead:             false,
			MultipartThreshold:     5 * 1024 * 1024,  // 5MB
			ChunkSize:              10 * 1024 * 1024, // 10MB
			MaxConcurrentUploads:   3,
			RetryCount:             3,
		}
	default:
		return nil
	}
}

// NewDigitalOceanProvider creates a new DigitalOcean Spaces provider
// DigitalOcean Spaces is S3-compatible, so we use the AWS provider
func NewDigitalOceanProvider(cfg *S3Config) (*AWSS3Provider, error) {
	// Set defaults for DigitalOcean Spaces
	if cfg.Region == "" {
		cfg.Region = "nyc3" // Default region
	}

	// DigitalOcean Spaces uses virtual-hosted style URLs
	cfg.PathStyle = false

	return NewAWSProvider(cfg)
}

// NewCloudflareProvider creates a new Cloudflare R2 provider
// Cloudflare R2 is S3-compatible, so we use the AWS provider
func NewCloudflareProvider(cfg *S3Config) (*AWSS3Provider, error) {
	// Cloudflare R2 uses auto region
	if cfg.Region == "" {
		cfg.Region = "auto"
	}

	// Cloudflare R2 uses virtual-hosted style URLs
	cfg.PathStyle = false

	return NewAWSProvider(cfg)
}

// NewWasabiProvider creates a new Wasabi provider
// Wasabi is S3-compatible, so we use the AWS provider
func NewWasabiProvider(cfg *S3Config) (*AWSS3Provider, error) {
	// Set defaults for Wasabi
	if cfg.Region == "" {
		cfg.Region = "us-east-1" // Default region
	}

	// Wasabi uses virtual-hosted style URLs
	cfg.PathStyle = false

	return NewAWSProvider(cfg)
}