package config

import (
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all configuration for the application
type Config struct {
	// Server configuration
	Port         string
	AppEnv       string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
	BodyLimit    int

	// Worker pool configuration
	MaxWorkers          int
	QueueSizeMultiplier int
	RequestTimeout      time.Duration

	// Buffer pool configuration
	BufferPoolSize int
	BufferSize     int

	// HTTP client configuration
	DownloadTimeout     time.Duration
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	IdleConnTimeout     time.Duration

	// Performance tuning
	GOGC       int
	GoMemLimit string

	// Audio conversion settings
	AudioBitrate          string
	AudioSampleRate       int
	AudioChannels         int
	AudioCompressionLevel int
	MaxAudioSize          int64

	// Image conversion settings
	DefaultImageQuality int
	DefaultMaxWidth     int
	DefaultMaxHeight    int
	MaxImageSize        int64
	ImageEngine         string

	// Logging configuration
	LogLevel               string
	LogFormat              string
	EnablePerformanceLogs  bool

	// Development settings
	Debug           bool
	HotReload       bool
	EnableProfiling bool

	// Production settings
	ProductionMode     bool
	EnableRequestID    bool
	EnableCORS         bool
	TrustedProxies     []string

	// Monitoring settings
	EnableHealthCheck    bool
	EnableStatsEndpoint  bool
	HealthCheckInterval  time.Duration

	// Security settings
	EnableAPIAuth     bool
	APIKey            string
	EnableRateLimit   bool
	RateLimit         int

	// Docker settings
	ContainerName string
	RestartPolicy string
	UseTmpfs      bool
	TmpfsSize     string

	// S3 upload configuration
	S3 *S3Configuration
}

// Load loads configuration from environment variables and .env file
func Load() *Config {
	// Try to load .env file (optional)
	if err := godotenv.Load(); err != nil {
		// .env file not found or couldn't be loaded - that's ok
		log.Printf("Note: .env file not found or couldn't be loaded: %v", err)
	} else {
		log.Println("✅ Loaded configuration from .env file")
	}

	return &Config{
		// Server configuration
		Port:         getEnv("PORT", "8080"),
		AppEnv:       getEnv("APP_ENV", "development"),
		ReadTimeout:  getDuration("READ_TIMEOUT", 5*time.Minute),
		WriteTimeout: getDuration("WRITE_TIMEOUT", 5*time.Minute),
		IdleTimeout:  getDuration("IDLE_TIMEOUT", 5*time.Minute),
		BodyLimit:    getInt("BODY_LIMIT", 500*1024*1024), // 500MB

		// Worker pool - smart defaults based on CPU
		MaxWorkers:          getWorkerCount(),
		QueueSizeMultiplier: getInt("QUEUE_SIZE_MULTIPLIER", 10),
		RequestTimeout:      getDuration("REQUEST_TIMEOUT", 5*time.Minute),

		// Buffer pool - optimized for high throughput
		BufferPoolSize: getInt("BUFFER_POOL_SIZE", 100),
		BufferSize:     getInt("BUFFER_SIZE", 10*1024*1024), // 10MB

		// HTTP client settings
		DownloadTimeout:     getDuration("DOWNLOAD_TIMEOUT", 30*time.Second),
		MaxIdleConns:        getInt("MAX_IDLE_CONNS", 100),
		MaxIdleConnsPerHost: getInt("MAX_IDLE_CONNS_PER_HOST", 100),
		IdleConnTimeout:     getDuration("IDLE_CONN_TIMEOUT", 90*time.Second),

		// GC and memory tuning
		GOGC:       getInt("GOGC", 100),
		GoMemLimit: getEnv("GOMEMLIMIT", "1GiB"),

		// Audio conversion settings
		AudioBitrate:          getEnv("AUDIO_BITRATE", "128k"),
		AudioSampleRate:       getInt("AUDIO_SAMPLE_RATE", 48000),
		AudioChannels:         getInt("AUDIO_CHANNELS", 1),
		AudioCompressionLevel: getInt("AUDIO_COMPRESSION_LEVEL", 10),
		MaxAudioSize:          getInt64("MAX_AUDIO_SIZE", 100*1024*1024), // 100MB

		// Image conversion settings
		DefaultImageQuality: getInt("DEFAULT_IMAGE_QUALITY", 95),
		DefaultMaxWidth:     getInt("DEFAULT_MAX_WIDTH", 1920),
		DefaultMaxHeight:    getInt("DEFAULT_MAX_HEIGHT", 1920),
		MaxImageSize:        getInt64("MAX_IMAGE_SIZE", 200*1024*1024), // 200MB
		ImageEngine:         getEnv("IMAGE_ENGINE", "auto"),

		// Logging configuration
		LogLevel:              getEnv("LOG_LEVEL", "info"),
		LogFormat:             getEnv("LOG_FORMAT", "text"),
		EnablePerformanceLogs: getBool("ENABLE_PERFORMANCE_LOGS", true),

		// Development settings
		Debug:           getBool("DEBUG", false),
		HotReload:       getBool("HOT_RELOAD", false),
		EnableProfiling: getBool("ENABLE_PROFILING", false),

		// Production settings
		ProductionMode:  getBool("PRODUCTION_MODE", false),
		EnableRequestID: getBool("ENABLE_REQUEST_ID", true),
		EnableCORS:      getBool("ENABLE_CORS", true),
		TrustedProxies:  getStringSlice("TRUSTED_PROXIES", []string{"127.0.0.1", "::1"}),

		// Monitoring settings
		EnableHealthCheck:   getBool("ENABLE_HEALTH_CHECK", true),
		EnableStatsEndpoint: getBool("ENABLE_STATS_ENDPOINT", true),
		HealthCheckInterval: getDuration("HEALTH_CHECK_INTERVAL", 10*time.Second),

		// Security settings
		EnableAPIAuth:   getBool("ENABLE_API_AUTH", false),
		APIKey:          getEnv("API_KEY", ""),
		EnableRateLimit: getBool("ENABLE_RATE_LIMITING", false),
		RateLimit:       getInt("RATE_LIMIT", 1000),

		// Docker settings
		ContainerName: getEnv("CONTAINER_NAME", "whats-media-converter"),
		RestartPolicy: getEnv("RESTART_POLICY", "unless-stopped"),
		UseTmpfs:      getBool("USE_TMPFS", true),
		TmpfsSize:     getEnv("TMPFS_SIZE", "2G"),

		// S3 upload configuration
		S3: LoadS3Config(),
	}
}

// Helper functions for environment variable parsing

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
		log.Printf("Warning: Invalid integer value for %s: %s, using default: %d", key, value, defaultValue)
	}
	return defaultValue
}

func getInt64(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
			return parsed
		}
		log.Printf("Warning: Invalid int64 value for %s: %s, using default: %d", key, value, defaultValue)
	}
	return defaultValue
}

func getBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			return parsed
		}
		log.Printf("Warning: Invalid boolean value for %s: %s, using default: %t", key, value, defaultValue)
	}
	return defaultValue
}

func getDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil {
			return parsed
		}
		log.Printf("Warning: Invalid duration value for %s: %s, using default: %s", key, value, defaultValue)
	}
	return defaultValue
}

func getStringSlice(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		// Split by comma and trim spaces
		parts := strings.Split(value, ",")
		result := make([]string, len(parts))
		for i, part := range parts {
			result[i] = strings.TrimSpace(part)
		}
		return result
	}
	return defaultValue
}

func getWorkerCount() int {
	// Check if explicitly set
	if value := os.Getenv("MAX_WORKERS"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			return parsed
		}
	}

	// Auto-detect based on CPU cores
	cpuCount := runtime.NumCPU()

	// Smart scaling based on CPU count
	switch {
	case cpuCount <= 2:
		return cpuCount * 2 // Low-end systems
	case cpuCount <= 8:
		return cpuCount * 4 // Standard systems
	case cpuCount <= 16:
		return cpuCount * 3 // High-end systems
	default:
		return cpuCount * 2 // Very high-end systems (avoid over-scheduling)
	}
}

// IsDevelopment returns true if running in development mode
func (c *Config) IsDevelopment() bool {
	return c.AppEnv == "development" || c.Debug
}

// IsProduction returns true if running in production mode
func (c *Config) IsProduction() bool {
	return c.AppEnv == "production" || c.ProductionMode
}

// GetQueueSize returns the calculated queue size
func (c *Config) GetQueueSize() int {
	return c.MaxWorkers * c.QueueSizeMultiplier
}

// PrintConfig logs the current configuration (without sensitive data)
func (c *Config) PrintConfig() {
	log.Println("===========================================")
	log.Println("📋 WhatsApp Media Converter Configuration")
	log.Println("===========================================")
	log.Printf("🌍 Environment:      %s", c.AppEnv)
	log.Printf("🚪 Port:             %s", c.Port)
	log.Printf("⚡ Workers:          %d (CPU: %d)", c.MaxWorkers, runtime.NumCPU())
	log.Printf("📦 Buffer Pool:      %d × %dMB", c.BufferPoolSize, c.BufferSize/1024/1024)
	log.Printf("🕒 Request Timeout:  %s", c.RequestTimeout)
	log.Printf("📊 Body Limit:       %dMB", c.BodyLimit/1024/1024)
	log.Printf("🧠 Memory Limit:     %s", c.GoMemLimit)
	log.Printf("🔄 GOGC:            %d", c.GOGC)
	log.Printf("🎵 Audio Max Size:   %dMB", c.MaxAudioSize/1024/1024)
	log.Printf("🖼️ Image Max Size:   %dMB", c.MaxImageSize/1024/1024)
	log.Printf("📈 Performance Logs: %t", c.EnablePerformanceLogs)
	log.Printf("🏥 Health Check:     %t", c.EnableHealthCheck)
	log.Printf("📊 Stats Endpoint:   %t", c.EnableStatsEndpoint)
	log.Printf("🔐 API Auth:         %t", c.EnableAPIAuth)
	log.Printf("🚦 Rate Limiting:    %t", c.EnableRateLimit)
	if c.EnableRateLimit {
		log.Printf("📏 Rate Limit:       %d req/s", c.RateLimit)
	}
	log.Println("===========================================")
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.MaxWorkers <= 0 {
		log.Printf("Warning: MAX_WORKERS is 0 or negative, auto-setting to %d", runtime.NumCPU()*4)
		c.MaxWorkers = runtime.NumCPU() * 4
	}

	if c.BufferPoolSize <= 0 {
		log.Printf("Warning: BUFFER_POOL_SIZE is 0 or negative, setting to default: 100")
		c.BufferPoolSize = 100
	}

	if c.BufferSize <= 0 {
		log.Printf("Warning: BUFFER_SIZE is 0 or negative, setting to default: 10MB")
		c.BufferSize = 10 * 1024 * 1024
	}

	if c.RequestTimeout <= 0 {
		log.Printf("Warning: REQUEST_TIMEOUT is 0 or negative, setting to default: 5m")
		c.RequestTimeout = 5 * time.Minute
	}

	return nil
}