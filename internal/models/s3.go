package models

import "time"

// S3UploadRequest represents a multipart upload initiation payload.
type S3UploadRequest struct {
	Key            string            `json:"key,omitempty" example:"uploads/audio/sample.opus"`
	Public         bool              `json:"public" example:"false"`
	ExpirationDays int               `json:"expires_days" example:"7"`
	ContentType    string            `json:"content_type,omitempty" example:"audio/ogg"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	StorageClass   string            `json:"storage_class,omitempty" example:"STANDARD"`
}

// S3Base64UploadRequest represents a base64 upload initiation payload.
type S3Base64UploadRequest struct {
	Data           string            `json:"data" example:"data:audio/ogg;base64,T2dnUwACAAAAAAAAAAB"`
	Filename       string            `json:"filename,omitempty" example:"voice-note.opus"`
	Key            string            `json:"key,omitempty" example:"uploads/audio/voice-note.opus"`
	Public         bool              `json:"public" example:"false"`
	ExpirationDays int               `json:"expires_days" example:"3"`
	ContentType    string            `json:"content_type,omitempty" example:"audio/ogg"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	StorageClass   string            `json:"storage_class,omitempty" example:"STANDARD"`
}

// S3UploadResponse represents a generic upload acknowledgement payload.
type S3UploadResponse struct {
	Success  bool            `json:"success" example:"true"`
	UploadID string          `json:"upload_id,omitempty" example:"3f99d60f-bd8d-49e6-9ecf-2fbc9e4adffe"`
	Result   *S3UploadResult `json:"result,omitempty"`
	Message  string          `json:"message,omitempty" example:"Upload started successfully"`
	Error    string          `json:"error,omitempty" example:"Failed to start upload"`
}

// S3UploadStatusResponse captures the state of an asynchronous upload job.
type S3UploadStatusResponse struct {
	UploadID         string          `json:"upload_id" example:"3f99d60f-bd8d-49e6-9ecf-2fbc9e4adffe"`
	Status           string          `json:"status" example:"uploading"`
	Progress         float64         `json:"progress" example:"42.5"`
	BytesTransferred int64           `json:"bytes_transferred" example:"1048576"`
	TotalBytes       int64           `json:"total_bytes" example:"5242880"`
	StartTime        time.Time       `json:"start_time" example:"2024-03-31T12:00:00Z"`
	EndTime          *time.Time      `json:"end_time,omitempty" example:"2024-03-31T12:00:10Z"`
	Error            string          `json:"error,omitempty" example:"connection reset by peer"`
	Result           *S3UploadResult `json:"result,omitempty"`
}

// S3UploadListResponse wraps paginated upload summaries.
type S3UploadListResponse struct {
	Uploads []S3UploadStatusResponse `json:"uploads"`
	Count   int                      `json:"count" example:"1"`
}

// S3UploadResult represents a normalized upload result for documentation.
type S3UploadResult struct {
	Key              string     `json:"key" example:"uploads/audio/sample.opus"`
	PublicURL        string     `json:"url" example:"https://cdn.example.com/uploads/audio/sample.opus"`
	Size             int64      `json:"size" example:"7340032"`
	ETag             string     `json:"etag" example:"\"9b2cf535f27731c974343645a3985328\""`
	VersionID        string     `json:"version_id,omitempty" example:"3/L4kqtJlcpXroDTDmJ+rmSpXd3dIbrHY"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty" example:"2024-04-01T12:00:00Z"`
	Provider         string     `json:"provider" example:"minio"`
	UploadID         string     `json:"upload_id,omitempty" example:"44c62b0d-7d55-4c74-9f65-8c7ab1f06642"`
	ProcessingTimeMS int64      `json:"processing_time_ms" example:"1200"`
}
