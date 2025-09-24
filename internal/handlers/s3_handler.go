package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"whats-convert-api/internal/providers"
	"whats-convert-api/internal/services"
)

// S3Handler handles S3 upload operations
type S3Handler struct {
	s3Service     *services.S3Service
	uploadManager *services.UploadManager
}

// NewS3Handler creates a new S3 handler
func NewS3Handler(s3Service *services.S3Service, uploadManager *services.UploadManager) *S3Handler {
	return &S3Handler{
		s3Service:     s3Service,
		uploadManager: uploadManager,
	}
}

// S3UploadRequest represents a file upload request
type S3UploadRequest struct {
	Key            string            `json:"key,omitempty"`
	Public         bool              `json:"public"`
	ExpirationDays int               `json:"expires_days"`
	ContentType    string            `json:"content_type,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	StorageClass   string            `json:"storage_class,omitempty"`
}

// S3Base64UploadRequest represents a base64 upload request
type S3Base64UploadRequest struct {
	Data           string            `json:"data"`
	Filename       string            `json:"filename,omitempty"`
	Key            string            `json:"key,omitempty"`
	Public         bool              `json:"public"`
	ExpirationDays int               `json:"expires_days"`
	ContentType    string            `json:"content_type,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	StorageClass   string            `json:"storage_class,omitempty"`
}

// S3UploadResponse represents an upload response
type S3UploadResponse struct {
	Success  bool                    `json:"success"`
	UploadID string                  `json:"upload_id,omitempty"`
	Result   *providers.UploadResult `json:"result,omitempty"`
	Message  string                  `json:"message,omitempty"`
	Error    string                  `json:"error,omitempty"`
}

// S3UploadStatusResponse represents an upload status response
type S3UploadStatusResponse struct {
	UploadID         string                  `json:"upload_id"`
	Status           string                  `json:"status"`
	Progress         float64                 `json:"progress"`
	BytesTransferred int64                   `json:"bytes_transferred"`
	TotalBytes       int64                   `json:"total_bytes"`
	StartTime        time.Time               `json:"start_time"`
	EndTime          *time.Time              `json:"end_time,omitempty"`
	Error            string                  `json:"error,omitempty"`
	Result           *providers.UploadResult `json:"result,omitempty"`
}

// UploadFile handles file upload via multipart/form-data
func (h *S3Handler) UploadFile(c fiber.Ctx) error {
	if !h.s3Service.IsEnabled() {
		return c.Status(http.StatusServiceUnavailable).JSON(S3UploadResponse{
			Success: false,
			Error:   "S3 upload service is disabled",
		})
	}

	// Parse multipart form
	form, err := c.MultipartForm()
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(S3UploadResponse{
			Success: false,
			Error:   "Failed to parse multipart form: " + err.Error(),
		})
	}

	// Get file from form
	files := form.File["file"]
	if len(files) == 0 {
		return c.Status(http.StatusBadRequest).JSON(S3UploadResponse{
			Success: false,
			Error:   "No file provided",
		})
	}

	file := files[0]

	// Open uploaded file
	src, err := file.Open()
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(S3UploadResponse{
			Success: false,
			Error:   "Failed to open uploaded file: " + err.Error(),
		})
	}

	fileBytes, err := io.ReadAll(src)
	if err != nil {
		src.Close()
		return c.Status(http.StatusInternalServerError).JSON(S3UploadResponse{
			Success: false,
			Error:   "Failed to read uploaded file: " + err.Error(),
		})
	}
	src.Close()

	if len(fileBytes) == 0 {
		return c.Status(http.StatusBadRequest).JSON(S3UploadResponse{
			Success: false,
			Error:   "Uploaded file is empty",
		})
	}

	// Parse options from form
	var options S3UploadRequest
	if optionsData := form.Value["options"]; len(optionsData) > 0 {
		if err := json.Unmarshal([]byte(optionsData[0]), &options); err != nil {
			log.Printf("Warning: Failed to parse upload options: %v", err)
		}
	}

	// Generate key if not provided
	key := options.Key
	if key == "" {
		key = h.s3Service.GenerateKey(file.Filename)
	}

	// Detect content type if not provided
	contentType := options.ContentType
	if contentType == "" {
		contentType = file.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/octet-stream"
		}
	}

	// Prepare upload options
	uploadOpts := providers.UploadOptions{
		ContentType:    contentType,
		Public:         options.Public,
		ExpirationDays: options.ExpirationDays,
		Metadata:       options.Metadata,
		StorageClass:   options.StorageClass,
	}

	// Start upload using upload manager
	reader := bytes.NewReader(fileBytes)
	fileSize := int64(len(fileBytes))

	uploadInfo, err := h.uploadManager.StartUpload(
		context.TODO(),
		key,
		reader,
		fileSize,
		uploadOpts,
	)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(S3UploadResponse{
			Success: false,
			Error:   "Failed to start upload: " + err.Error(),
		})
	}

	// Set original filename
	uploadInfo.OriginalFilename = file.Filename

	return c.Status(http.StatusAccepted).JSON(S3UploadResponse{
		Success:  true,
		UploadID: uploadInfo.ID,
		Message:  "Upload started successfully",
	})
}

// UploadBase64 handles base64 data upload
func (h *S3Handler) UploadBase64(c fiber.Ctx) error {
	if !h.s3Service.IsEnabled() {
		return c.Status(http.StatusServiceUnavailable).JSON(S3UploadResponse{
			Success: false,
			Error:   "S3 upload service is disabled",
		})
	}

	var req S3Base64UploadRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(http.StatusBadRequest).JSON(S3UploadResponse{
			Success: false,
			Error:   "Invalid JSON payload: " + err.Error(),
		})
	}

	// Validate required fields
	if req.Data == "" {
		return c.Status(http.StatusBadRequest).JSON(S3UploadResponse{
			Success: false,
			Error:   "Missing required field: data",
		})
	}

	// Generate key if not provided
	key := req.Key
	if key == "" {
		filename := req.Filename
		if filename == "" {
			filename = "file"
		}
		key = h.s3Service.GenerateKey(filename)
	}

	// Detect content type from data URL if not provided
	contentType := req.ContentType
	if contentType == "" && strings.HasPrefix(req.Data, "data:") {
		if parts := strings.Split(req.Data, ";"); len(parts) > 0 {
			contentType = strings.TrimPrefix(parts[0], "data:")
		}
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Prepare upload options
	uploadOpts := providers.UploadOptions{
		ContentType:    contentType,
		Public:         req.Public,
		ExpirationDays: req.ExpirationDays,
		Metadata:       req.Metadata,
		StorageClass:   req.StorageClass,
	}

	// Start base64 upload using upload manager
	uploadInfo, err := h.uploadManager.StartBase64Upload(
		context.TODO(),
		key,
		req.Data,
		uploadOpts,
	)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(S3UploadResponse{
			Success: false,
			Error:   "Failed to start upload: " + err.Error(),
		})
	}

	// Set original filename
	uploadInfo.OriginalFilename = req.Filename

	return c.Status(http.StatusAccepted).JSON(S3UploadResponse{
		Success:  true,
		UploadID: uploadInfo.ID,
		Message:  "Upload started successfully",
	})
}

// GetUploadStatus returns the status of an upload
func (h *S3Handler) GetUploadStatus(c fiber.Ctx) error {
	uploadID := c.Params("id")
	if uploadID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Upload ID is required",
		})
	}

	uploadInfo, err := h.uploadManager.GetUploadStatus(uploadID)
	if err != nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{
			"error": "Upload not found",
		})
	}

	response := S3UploadStatusResponse{
		UploadID:         uploadInfo.ID,
		Status:           string(uploadInfo.Status),
		Progress:         uploadInfo.Progress,
		BytesTransferred: uploadInfo.BytesTransferred,
		TotalBytes:       uploadInfo.TotalBytes,
		StartTime:        uploadInfo.StartTime,
		EndTime:          uploadInfo.EndTime,
		Error:            uploadInfo.Error,
		Result:           uploadInfo.Result,
	}

	return c.JSON(response)
}

// CancelUpload cancels an ongoing upload
func (h *S3Handler) CancelUpload(c fiber.Ctx) error {
	uploadID := c.Params("id")
	if uploadID == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Upload ID is required",
		})
	}

	err := h.uploadManager.CancelUpload(uploadID)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Upload cancelled successfully",
	})
}

// ListUploads returns a list of uploads
func (h *S3Handler) ListUploads(c fiber.Ctx) error {
	statusFilter := c.Query("status")
	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	if limit <= 0 || limit > 1000 {
		limit = 50
	}

	var uploads []*services.UploadInfo
	if statusFilter != "" {
		uploads = h.uploadManager.ListUploads(services.UploadStatus(statusFilter))
	} else {
		uploads = h.uploadManager.ListUploads()
	}

	// Apply limit
	if len(uploads) > limit {
		uploads = uploads[:limit]
	}

	// Convert to response format
	var response []S3UploadStatusResponse
	for _, upload := range uploads {
		response = append(response, S3UploadStatusResponse{
			UploadID:         upload.ID,
			Status:           string(upload.Status),
			Progress:         upload.Progress,
			BytesTransferred: upload.BytesTransferred,
			TotalBytes:       upload.TotalBytes,
			StartTime:        upload.StartTime,
			EndTime:          upload.EndTime,
			Error:            upload.Error,
			Result:           upload.Result,
		})
	}

	return c.JSON(fiber.Map{
		"uploads": response,
		"count":   len(response),
	})
}

// GetS3Stats returns S3 service statistics
func (h *S3Handler) GetS3Stats(c fiber.Ctx) error {
	s3Stats := h.s3Service.GetStats()
	uploadStats := h.uploadManager.GetStats()

	return c.JSON(fiber.Map{
		"s3_service": fiber.Map{
			"enabled":            h.s3Service.IsEnabled(),
			"total_uploads":      s3Stats.TotalUploads,
			"successful_uploads": s3Stats.SuccessfulUploads,
			"failed_uploads":     s3Stats.FailedUploads,
			"total_bytes":        s3Stats.TotalBytes,
			"success_rate":       s3Stats.GetSuccessRate(),
			"avg_upload_time":    s3Stats.GetFormattedAverageTime(),
			"last_upload":        s3Stats.LastUpload,
		},
		"upload_manager": uploadStats,
	})
}

// GetS3Health checks S3 service health
func (h *S3Handler) GetS3Health(c fiber.Ctx) error {
	if !h.s3Service.IsEnabled() {
		return c.JSON(fiber.Map{
			"status":  "disabled",
			"healthy": true,
			"message": "S3 service is disabled",
		})
	}

	err := h.s3Service.HealthCheck(context.TODO())
	if err != nil {
		return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{
			"status":  "unhealthy",
			"healthy": false,
			"error":   err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"status":  "healthy",
		"healthy": true,
		"message": "S3 service is operational",
	})
}

// DeleteObject deletes an object from S3
func (h *S3Handler) DeleteObject(c fiber.Ctx) error {
	if !h.s3Service.IsEnabled() {
		return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "S3 upload service is disabled",
		})
	}

	key := c.Params("key")
	if key == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Object key is required",
		})
	}

	err := h.s3Service.DeleteObject(context.TODO(), key)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to delete object: " + err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Object deleted successfully",
	})
}

// GetObjectInfo retrieves metadata about an object
func (h *S3Handler) GetObjectInfo(c fiber.Ctx) error {
	if !h.s3Service.IsEnabled() {
		return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "S3 upload service is disabled",
		})
	}

	key := c.Params("key")
	if key == "" {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{
			"error": "Object key is required",
		})
	}

	info, err := h.s3Service.GetObjectInfo(context.TODO(), key)
	if err != nil {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{
			"error": "Object not found: " + err.Error(),
		})
	}

	return c.JSON(info)
}

// RegisterS3Routes registers all S3-related routes
func (h *S3Handler) RegisterS3Routes(app *fiber.App) {
	s3 := app.Group("/upload/s3")

	// Upload endpoints - register both variants to support strict routing
	s3.Post("/", h.UploadFile)
	s3.Post("", h.UploadFile)
	s3.Post("/base64", h.UploadBase64)

	// Status and management endpoints
	s3.Get("/status/:id", h.GetUploadStatus)
	s3.Delete("/status/:id", h.CancelUpload)
	s3.Get("/list", h.ListUploads)

	// Object management endpoints
	s3.Delete("/object/:key", h.DeleteObject)
	s3.Get("/object/:key", h.GetObjectInfo)

	// Service endpoints
	s3.Get("/stats", h.GetS3Stats)
	s3.Get("/health", h.GetS3Health)
}
