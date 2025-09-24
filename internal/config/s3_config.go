package config

import (
	"fmt"
	"log"
	"strings"
	"time"

	"whats-convert-api/internal/providers"
)

// S3Configuration holds all S3-related settings
type S3Configuration struct {
	// Enable S3 upload functionality
	Enabled bool `json:"enabled"`

	// Provider configuration
	Provider     providers.ProviderType `json:"provider"`
	Endpoint     string                 `json:"endpoint"`
	PublicEndpoint string               `json:"public_endpoint"`
	Region       string                 `json:"region"`
	Bucket       string                 `json:"bucket"`
	AccessKey    string                 `json:"access_key"`
	SecretKey    string                 `json:"secret_key"`

	// Connection settings
	UseSSL    bool `json:"use_ssl"`
	PathStyle bool `json:"path_style"`

	// Upload behavior
	PublicRead            bool `json:"public_read"`
	DefaultExpirationDays int  `json:"default_expiration_days"`

	// Performance settings
	MultipartThreshold   int64         `json:"multipart_threshold"`
	ChunkSize           int64         `json:"chunk_size"`
	MaxConcurrentUploads int           `json:"max_concurrent_uploads"`
	UploadTimeout       time.Duration `json:"upload_timeout"`
	RetryCount          int           `json:"retry_count"`

	// Key generation settings
	KeyPrefix          string `json:"key_prefix"`
	UseTimestampInKey  bool   `json:"use_timestamp_in_key"`
	UseUUIDInKey       bool   `json:"use_uuid_in_key"`
	PreserveFilename   bool   `json:"preserve_filename"`

	// Security settings
	AllowedContentTypes []string `json:"allowed_content_types"`
	MaxFileSize         int64    `json:"max_file_size"`
	ScanUploads         bool     `json:"scan_uploads"`

	// Monitoring
	EnableMetrics bool `json:"enable_metrics"`
	LogUploads    bool `json:"log_uploads"`
}

// LoadS3Config loads S3 configuration from environment variables
func LoadS3Config() *S3Configuration {
	config := &S3Configuration{
		// Defaults
		Enabled:               getBool("S3_ENABLED", false),
		Provider:              providers.ProviderType(getEnv("S3_PROVIDER", "aws")),
		Endpoint:              getEnv("S3_ENDPOINT", "https://s3.amazonaws.com"),
		PublicEndpoint:        getEnv("S3_PUBLIC_ENDPOINT", ""),
		Region:                getEnv("S3_REGION", "us-east-1"),
		Bucket:                getEnv("S3_BUCKET", ""),
		AccessKey:             getEnv("S3_ACCESS_KEY", ""),
		SecretKey:             getEnv("S3_SECRET_KEY", ""),
		UseSSL:                getBool("S3_USE_SSL", true),
		PathStyle:             getBool("S3_PATH_STYLE", false),
		PublicRead:            getBool("S3_PUBLIC_READ", true),
		DefaultExpirationDays: getInt("S3_EXPIRATION_DAYS", 0),
		MultipartThreshold:    getInt64("S3_MULTIPART_THRESHOLD", 5*1024*1024),   // 5MB
		ChunkSize:             getInt64("S3_CHUNK_SIZE", 10*1024*1024),           // 10MB
		MaxConcurrentUploads:  getInt("S3_MAX_CONCURRENT_UPLOADS", 3),
		UploadTimeout:         getDuration("S3_UPLOAD_TIMEOUT", time.Hour),
		RetryCount:            getInt("S3_RETRY_COUNT", 3),
		KeyPrefix:             getEnv("S3_KEY_PREFIX", "uploads/"),
		UseTimestampInKey:     getBool("S3_USE_TIMESTAMP_IN_KEY", true),
		UseUUIDInKey:          getBool("S3_USE_UUID_IN_KEY", true),
		PreserveFilename:      getBool("S3_PRESERVE_FILENAME", true),
		AllowedContentTypes:   getStringSlice("S3_ALLOWED_CONTENT_TYPES", []string{}),
		MaxFileSize:           getInt64("S3_MAX_FILE_SIZE", 0), // 0 = no limit
		ScanUploads:           getBool("S3_SCAN_UPLOADS", false),
		EnableMetrics:         getBool("S3_ENABLE_METRICS", true),
		LogUploads:            getBool("S3_LOG_UPLOADS", true),
	}

	// Set provider-specific defaults
	config.applyProviderDefaults()

	return config
}

// applyProviderDefaults sets provider-specific default values
func (c *S3Configuration) applyProviderDefaults() {
	switch c.Provider {
	case providers.ProviderAWS:
		if c.Endpoint == "" {
			c.Endpoint = "https://s3.amazonaws.com"
		}
		c.PathStyle = false // AWS S3 prefers virtual-hosted style

	case providers.ProviderMinIO:
		c.PathStyle = true // MinIO typically uses path-style
		if c.PublicEndpoint == "" && c.Endpoint != "" {
			// Try to derive public endpoint for MinIO
			c.PublicEndpoint = c.Endpoint
		}

	case providers.ProviderBackblaze:
		c.PathStyle = true // Backblaze B2 uses path-style
		if c.Region == "" {
			c.Region = "us-west-000" // Default Backblaze region
		}

	case providers.ProviderDigitalOcean:
		c.PathStyle = false // DigitalOcean Spaces uses virtual-hosted style
		if c.Region == "" {
			c.Region = "nyc3" // Default DO region
		}

	case providers.ProviderCloudflare:
		c.PathStyle = false // Cloudflare R2 uses virtual-hosted style
		if c.Region == "" {
			c.Region = "auto" // Cloudflare R2 region
		}

	case providers.ProviderWasabi:
		c.PathStyle = false // Wasabi uses virtual-hosted style
		if c.Region == "" {
			c.Region = "us-east-1" // Default Wasabi region
		}
	}
}

// ToProviderConfig converts S3Configuration to providers.S3Config
func (c *S3Configuration) ToProviderConfig() *providers.S3Config {
	return &providers.S3Config{
		Provider:              c.Provider,
		Endpoint:              c.Endpoint,
		PublicEndpoint:        c.PublicEndpoint,
		Region:                c.Region,
		Bucket:                c.Bucket,
		AccessKey:             c.AccessKey,
		SecretKey:             c.SecretKey,
		UseSSL:                c.UseSSL,
		PathStyle:             c.PathStyle,
		PublicRead:            c.PublicRead,
		DefaultExpirationDays: c.DefaultExpirationDays,
		MultipartThreshold:    c.MultipartThreshold,
		ChunkSize:             c.ChunkSize,
		MaxConcurrentUploads:  c.MaxConcurrentUploads,
		UploadTimeout:         c.UploadTimeout,
		RetryCount:            c.RetryCount,
	}
}

// Validate checks if the S3 configuration is valid
func (c *S3Configuration) Validate() error {
	if !c.Enabled {
		return nil // S3 is disabled, no validation needed
	}

	if c.Provider == "" {
		return fmt.Errorf("S3_PROVIDER is required when S3 is enabled")
	}

	if c.Bucket == "" {
		return fmt.Errorf("S3_BUCKET is required when S3 is enabled")
	}

	if c.AccessKey == "" {
		return fmt.Errorf("S3_ACCESS_KEY is required when S3 is enabled")
	}

	if c.SecretKey == "" {
		return fmt.Errorf("S3_SECRET_KEY is required when S3 is enabled")
	}

	if c.Endpoint == "" {
		return fmt.Errorf("S3_ENDPOINT is required when S3 is enabled")
	}

	// Validate provider-specific requirements
	switch c.Provider {
	case providers.ProviderAWS, providers.ProviderDigitalOcean, providers.ProviderWasabi:
		if c.Region == "" {
			return fmt.Errorf("S3_REGION is required for %s provider", c.Provider)
		}

	case providers.ProviderBackblaze:
		if !strings.Contains(c.Endpoint, "backblazeb2.com") {
			return fmt.Errorf("invalid Backblaze B2 endpoint: %s", c.Endpoint)
		}
	}

	// Validate numeric values
	if c.MultipartThreshold <= 0 {
		c.MultipartThreshold = 5 * 1024 * 1024 // 5MB default
	}

	if c.ChunkSize <= 0 {
		c.ChunkSize = 10 * 1024 * 1024 // 10MB default
	}

	if c.MaxConcurrentUploads <= 0 {
		c.MaxConcurrentUploads = 3
	}

	if c.RetryCount < 0 {
		c.RetryCount = 3
	}

	return nil
}

// PrintS3Config logs the current S3 configuration (without sensitive data)
func (c *S3Configuration) PrintS3Config() {
	if !c.Enabled {
		log.Println("📦 S3 Upload: Disabled")
		return
	}

	log.Println("===========================================")
	log.Println("📦 S3 Upload Configuration")
	log.Println("===========================================")
	log.Printf("🔧 Provider:         %s", c.Provider)
	log.Printf("🌐 Endpoint:         %s", c.Endpoint)
	log.Printf("🌍 Region:           %s", c.Region)
	log.Printf("🪣 Bucket:           %s", c.Bucket)
	log.Printf("🔗 Public Endpoint:  %s", c.PublicEndpoint)
	log.Printf("🔐 Path Style:       %t", c.PathStyle)
	log.Printf("👁️  Public Read:      %t", c.PublicRead)
	log.Printf("⏰ Expiration:       %d days", c.DefaultExpirationDays)
	log.Printf("📊 Multipart:        %dMB threshold", c.MultipartThreshold/1024/1024)
	log.Printf("🧩 Chunk Size:       %dMB", c.ChunkSize/1024/1024)
	log.Printf("🔄 Concurrent:       %d uploads", c.MaxConcurrentUploads)
	log.Printf("⏱️  Timeout:          %s", c.UploadTimeout)
	log.Printf("🔁 Retry Count:      %d", c.RetryCount)
	log.Printf("📈 Metrics:          %t", c.EnableMetrics)
	log.Printf("📝 Logging:          %t", c.LogUploads)
	log.Println("===========================================")
}

// IsContentTypeAllowed checks if a content type is allowed for upload
func (c *S3Configuration) IsContentTypeAllowed(contentType string) bool {
	if len(c.AllowedContentTypes) == 0 {
		return true // No restrictions
	}

	for _, allowedType := range c.AllowedContentTypes {
		if strings.EqualFold(allowedType, contentType) {
			return true
		}
		// Support wildcard matching (e.g., "image/*")
		if strings.HasSuffix(allowedType, "/*") {
			prefix := strings.TrimSuffix(allowedType, "/*")
			if strings.HasPrefix(contentType, prefix+"/") {
				return true
			}
		}
	}

	return false
}

// IsFileSizeAllowed checks if a file size is within allowed limits
func (c *S3Configuration) IsFileSizeAllowed(size int64) bool {
	if c.MaxFileSize == 0 {
		return true // No size limit
	}
	return size <= c.MaxFileSize
}

// GenerateObjectKey generates an object key based on configuration
func (c *S3Configuration) GenerateObjectKey(filename string) string {
	var keyParts []string

	// Add prefix if configured
	if c.KeyPrefix != "" {
		keyParts = append(keyParts, strings.TrimSuffix(c.KeyPrefix, "/"))
	}

	// Add timestamp if configured
	if c.UseTimestampInKey {
		timestamp := time.Now().Format("2006/01/02")
		keyParts = append(keyParts, timestamp)
	}

	// Add UUID if configured
	if c.UseUUIDInKey {
		// Simple UUID-like string (should use proper UUID library in production)
		uuid := fmt.Sprintf("%d", time.Now().UnixNano())
		keyParts = append(keyParts, uuid)
	}

	// Add filename
	if c.PreserveFilename && filename != "" {
		keyParts = append(keyParts, filename)
	} else if filename != "" {
		// Generate a simple filename with extension
		ext := ""
		if dotIndex := strings.LastIndex(filename, "."); dotIndex != -1 {
			ext = filename[dotIndex:]
		}
		generatedName := fmt.Sprintf("file_%d%s", time.Now().UnixNano(), ext)
		keyParts = append(keyParts, generatedName)
	} else {
		// No filename provided, generate one
		generatedName := fmt.Sprintf("file_%d", time.Now().UnixNano())
		keyParts = append(keyParts, generatedName)
	}

	return strings.Join(keyParts, "/")
}

