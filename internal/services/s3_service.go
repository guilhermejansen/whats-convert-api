package services

import (
	"context"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"whats-convert-api/internal/config"
	"whats-convert-api/internal/providers"
)

// S3Service manages S3 operations and provider lifecycle
type S3Service struct {
	provider providers.S3Provider
	config   *config.S3Configuration
	factory  *providers.ProviderFactory
	mu       sync.RWMutex
	stats    *S3Stats
	enabled  bool
}

// S3Stats tracks service statistics
type S3Stats struct {
	TotalUploads     int64     `json:"total_uploads"`
	SuccessfulUploads int64    `json:"successful_uploads"`
	FailedUploads    int64     `json:"failed_uploads"`
	TotalBytes       int64     `json:"total_bytes"`
	AverageUploadTime time.Duration `json:"average_upload_time"`
	LastUpload       time.Time `json:"last_upload"`
	mu               sync.RWMutex
}

// NewS3Service creates a new S3 service
func NewS3Service(cfg *config.S3Configuration) (*S3Service, error) {
	service := &S3Service{
		config:  cfg,
		factory: providers.NewProviderFactory(),
		stats:   &S3Stats{},
		enabled: cfg.Enabled,
	}

	if cfg.Enabled {
		if err := service.initializeProvider(); err != nil {
			return nil, fmt.Errorf("failed to initialize S3 provider: %w", err)
		}

		log.Printf("✅ S3 Service initialized with provider: %s", cfg.Provider)
		cfg.PrintS3Config()
	} else {
		log.Println("📦 S3 Service: Disabled")
	}

	return service, nil
}

// initializeProvider sets up the S3 provider
func (s *S3Service) initializeProvider() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.config.Validate(); err != nil {
		return fmt.Errorf("invalid S3 configuration: %w", err)
	}

	providerConfig := s.config.ToProviderConfig()
	provider, err := s.factory.CreateProvider(providerConfig)
	if err != nil {
		return fmt.Errorf("failed to create S3 provider: %w", err)
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := provider.HealthCheck(ctx); err != nil {
		return fmt.Errorf("S3 provider health check failed: %w", err)
	}

	s.provider = provider
	return nil
}

// IsEnabled returns whether S3 service is enabled
func (s *S3Service) IsEnabled() bool {
	return s.enabled
}

// Upload uploads data to S3
func (s *S3Service) Upload(ctx context.Context, key string, data []byte, opts providers.UploadOptions) (*providers.UploadResult, error) {
	if !s.enabled {
		return nil, fmt.Errorf("S3 service is disabled")
	}

	s.mu.RLock()
	provider := s.provider
	s.mu.RUnlock()

	if provider == nil {
		return nil, fmt.Errorf("S3 provider not initialized")
	}

	startTime := time.Now()

	// Validate content type if restrictions are configured
	if !s.config.IsContentTypeAllowed(opts.ContentType) {
		return nil, fmt.Errorf("content type not allowed: %s", opts.ContentType)
	}

	// Validate file size if restrictions are configured
	if !s.config.IsFileSizeAllowed(int64(len(data))) {
		return nil, fmt.Errorf("file size exceeds maximum allowed: %d bytes", len(data))
	}

	// Set default options from configuration
	if opts.ExpirationDays == 0 {
		opts.ExpirationDays = s.config.DefaultExpirationDays
	}
	if opts.Public != true && s.config.PublicRead {
		opts.Public = true
	}

	// Perform upload
	reader := &dataReader{data: data}
	result, err := provider.Upload(ctx, key, reader, int64(len(data)), opts)

	// Update statistics
	s.updateStats(startTime, int64(len(data)), err == nil)

	if err != nil {
		if s.config.LogUploads {
			log.Printf("❌ S3 Upload failed for key '%s': %v", key, err)
		}
		return nil, err
	}

	if s.config.LogUploads {
		log.Printf("✅ S3 Upload successful: %s (size: %d bytes, time: %v)",
			result.Key, result.Size, result.ProcessingTime)
	}

	return result, nil
}

// UploadBase64 uploads base64-encoded data to S3
func (s *S3Service) UploadBase64(ctx context.Context, key string, base64Data string, opts providers.UploadOptions) (*providers.UploadResult, error) {
	if !s.enabled {
		return nil, fmt.Errorf("S3 service is disabled")
	}

	s.mu.RLock()
	provider := s.provider
	s.mu.RUnlock()

	if provider == nil {
		return nil, fmt.Errorf("S3 provider not initialized")
	}

	startTime := time.Now()

	// Set default options from configuration
	if opts.ExpirationDays == 0 {
		opts.ExpirationDays = s.config.DefaultExpirationDays
	}
	if opts.Public != true && s.config.PublicRead {
		opts.Public = true
	}

	// Perform upload
	result, err := provider.UploadBase64(ctx, key, base64Data, opts)

	// Update statistics
	s.updateStats(startTime, result.Size, err == nil)

	if err != nil {
		if s.config.LogUploads {
			log.Printf("❌ S3 Base64 Upload failed for key '%s': %v", key, err)
		}
		return nil, err
	}

	if s.config.LogUploads {
		log.Printf("✅ S3 Base64 Upload successful: %s (size: %d bytes, time: %v)",
			result.Key, result.Size, result.ProcessingTime)
	}

	return result, nil
}

// GenerateKey generates an object key based on configuration
func (s *S3Service) GenerateKey(filename string) string {
	return s.config.GenerateObjectKey(filename)
}

// DeleteObject deletes an object from S3
func (s *S3Service) DeleteObject(ctx context.Context, key string) error {
	if !s.enabled {
		return fmt.Errorf("S3 service is disabled")
	}

	s.mu.RLock()
	provider := s.provider
	s.mu.RUnlock()

	if provider == nil {
		return fmt.Errorf("S3 provider not initialized")
	}

	err := provider.DeleteObject(ctx, key)
	if err != nil {
		if s.config.LogUploads {
			log.Printf("❌ S3 Delete failed for key '%s': %v", key, err)
		}
		return err
	}

	if s.config.LogUploads {
		log.Printf("🗑️ S3 Delete successful: %s", key)
	}

	return nil
}

// GetObjectInfo retrieves metadata about an object
func (s *S3Service) GetObjectInfo(ctx context.Context, key string) (*providers.ObjectInfo, error) {
	if !s.enabled {
		return nil, fmt.Errorf("S3 service is disabled")
	}

	s.mu.RLock()
	provider := s.provider
	s.mu.RUnlock()

	if provider == nil {
		return nil, fmt.Errorf("S3 provider not initialized")
	}

	return provider.GetObjectInfo(ctx, key)
}

// HealthCheck verifies S3 service health
func (s *S3Service) HealthCheck(ctx context.Context) error {
	if !s.enabled {
		return nil // Service is disabled, consider it healthy
	}

	s.mu.RLock()
	provider := s.provider
	s.mu.RUnlock()

	if provider == nil {
		return fmt.Errorf("S3 provider not initialized")
	}

	return provider.HealthCheck(ctx)
}

// GetStats returns service statistics
func (s *S3Service) GetStats() *S3Stats {
	s.stats.mu.RLock()
	defer s.stats.mu.RUnlock()

	// Return a copy to avoid race conditions
	return &S3Stats{
		TotalUploads:      s.stats.TotalUploads,
		SuccessfulUploads: s.stats.SuccessfulUploads,
		FailedUploads:     s.stats.FailedUploads,
		TotalBytes:        s.stats.TotalBytes,
		AverageUploadTime: s.stats.AverageUploadTime,
		LastUpload:        s.stats.LastUpload,
	}
}

// GetConfig returns the service configuration
func (s *S3Service) GetConfig() *config.S3Configuration {
	return s.config
}

// Reload reloads the S3 service configuration
func (s *S3Service) Reload(newConfig *config.S3Configuration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Store old state
	oldEnabled := s.enabled
	oldProvider := s.provider

	s.config = newConfig
	s.enabled = newConfig.Enabled

	if newConfig.Enabled {
		if err := s.initializeProvider(); err != nil {
			// Restore old state on error
			s.enabled = oldEnabled
			s.provider = oldProvider
			return fmt.Errorf("failed to reload S3 service: %w", err)
		}
		log.Printf("🔄 S3 Service reloaded with provider: %s", newConfig.Provider)
	} else {
		s.provider = nil
		log.Println("🔄 S3 Service reloaded: Disabled")
	}

	return nil
}

// updateStats updates service statistics
func (s *S3Service) updateStats(startTime time.Time, bytes int64, success bool) {
	if !s.config.EnableMetrics {
		return
	}

	s.stats.mu.Lock()
	defer s.stats.mu.Unlock()

	s.stats.TotalUploads++
	s.stats.LastUpload = time.Now()

	if success {
		s.stats.SuccessfulUploads++
		s.stats.TotalBytes += bytes

		// Update average upload time
		uploadTime := time.Since(startTime)
		if s.stats.AverageUploadTime == 0 {
			s.stats.AverageUploadTime = uploadTime
		} else {
			// Simple moving average
			s.stats.AverageUploadTime = (s.stats.AverageUploadTime + uploadTime) / 2
		}
	} else {
		s.stats.FailedUploads++
	}
}

// GetSuccessRate returns the upload success rate as a percentage
func (s *S3Stats) GetSuccessRate() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.TotalUploads == 0 {
		return 100.0
	}
	return (float64(s.SuccessfulUploads) / float64(s.TotalUploads)) * 100.0
}

// GetFormattedAverageTime returns the average upload time as a formatted string
func (s *S3Stats) GetFormattedAverageTime() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.AverageUploadTime == 0 {
		return "N/A"
	}
	return s.AverageUploadTime.String()
}

// dataReader implements io.Reader for byte slices
type dataReader struct {
	data []byte
	pos  int
}

func (r *dataReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}

	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}