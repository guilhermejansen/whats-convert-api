package models

import (
	"time"

	"whats-convert-api/internal/services"
)

// APIInfoResponse describes the metadata returned by GET /api.
type APIInfoResponse struct {
	Name      string            `json:"name" example:"WhatsApp Media Converter API"`
	Version   string            `json:"version" example:"1.0.0"`
	Endpoints map[string]string `json:"endpoints"`
}

// ErrorResponse represents a generic error payload used across endpoints.
type ErrorResponse struct {
	Error   string `json:"error" example:"Invalid request"`
	Details string `json:"details,omitempty" example:"Missing 'data' field"`
}

// BatchAudioResponse models the batch conversion response for audio payloads.
type BatchAudioResponse struct {
	Results []*services.AudioResponse `json:"results"`
	Count   int                       `json:"count" example:"2"`
}

// BatchImageResponse models the batch conversion response for image payloads.
type BatchImageResponse struct {
	Results []*services.ImageResponse `json:"results"`
	Count   int                       `json:"count" example:"2"`
}

// ConverterStats provides aggregated counters for conversion services.
type ConverterStats struct {
	TotalConversions    int64 `json:"total_conversions" example:"1280"`
	FailedConversions   int64 `json:"failed_conversions" example:"12"`
	AvgConversionTimeMS int64 `json:"avg_conversion_time_ms" example:"135"`
}

// ImageConverterStats extends ConverterStats with engine breakdown metrics.
type ImageConverterStats struct {
	TotalConversions    int64 `json:"total_conversions" example:"980"`
	FailedConversions   int64 `json:"failed_conversions" example:"8"`
	AvgConversionTimeMS int64 `json:"avg_conversion_time_ms" example:"110"`
	VipsConversions     int64 `json:"vips_conversions" example:"620"`
	FFmpegConversions   int64 `json:"ffmpeg_conversions" example:"360"`
}

// AudioHealthMetrics aggregates health metrics for the audio converter.
type AudioHealthMetrics struct {
	TotalConversions  int64  `json:"total_conversions" example:"1280"`
	FailedConversions int64  `json:"failed_conversions" example:"12"`
	SuccessRate       string `json:"success_rate" example:"99.06%"`
	AvgConversionMS   int64  `json:"avg_conversion_time" example:"135"`
}

// ImageHealthMetrics aggregates health metrics for the image converter.
type ImageHealthMetrics struct {
	TotalConversions  int64  `json:"total_conversions" example:"980"`
	FailedConversions int64  `json:"failed_conversions" example:"8"`
	SuccessRate       string `json:"success_rate" example:"99.18%"`
	AvgConversionMS   int64  `json:"avg_conversion_time" example:"110"`
	VipsAvailable     bool   `json:"vips_available" example:"true"`
}

// HealthResponse captures the payload returned by GET /health.
type HealthResponse struct {
	Status    string             `json:"status" example:"healthy"`
	Timestamp int64              `json:"timestamp" example:"1700000000"`
	Audio     AudioHealthMetrics `json:"audio"`
	Image     ImageHealthMetrics `json:"image"`
}

// StatsResponse captures aggregated converter statistics returned by GET /stats.
type StatsResponse struct {
	Audio     ConverterStats      `json:"audio"`
	Image     ImageConverterStats `json:"image"`
	Timestamp int64               `json:"timestamp" example:"1700000000"`
}

// MessageResponse represents a simple success payload with contextual message.
type MessageResponse struct {
	Success bool   `json:"success" example:"true"`
	Message string `json:"message" example:"Operation completed successfully"`
}

// S3ServiceStats summarises the state of the underlying S3 provider.
type S3ServiceStats struct {
	Enabled           bool      `json:"enabled" example:"true"`
	TotalUploads      int64     `json:"total_uploads" example:"240"`
	SuccessfulUploads int64     `json:"successful_uploads" example:"236"`
	FailedUploads     int64     `json:"failed_uploads" example:"4"`
	TotalBytes        int64     `json:"total_bytes" example:"73400320"`
	SuccessRate       float64   `json:"success_rate" example:"98.33"`
	AvgUploadTime     string    `json:"avg_upload_time" example:"1.2s"`
	LastUpload        time.Time `json:"last_upload" example:"2024-03-31T12:00:00Z"`
}

// S3UploadManagerStats represents queue health for the concurrent upload manager.
type S3UploadManagerStats struct {
	TotalUploads   int            `json:"total_uploads" example:"5"`
	CurrentUploads int            `json:"current_uploads" example:"1"`
	MaxConcurrent  int            `json:"max_concurrent" example:"3"`
	StatusCounts   map[string]int `json:"status_counts"`
	CapacityUsed   float64        `json:"capacity_used" example:"33.33"`
}

// S3StatsResponse merges provider and upload manager metrics.
type S3StatsResponse struct {
	S3Service     S3ServiceStats       `json:"s3_service"`
	UploadManager S3UploadManagerStats `json:"upload_manager"`
}

// S3HealthResponse models the health payload for the S3 subsystem.
type S3HealthResponse struct {
	Status  string `json:"status" example:"healthy"`
	Healthy bool   `json:"healthy" example:"true"`
	Message string `json:"message,omitempty" example:"S3 service is operational"`
	Error   string `json:"error,omitempty" example:"failed to connect to bucket"`
}
