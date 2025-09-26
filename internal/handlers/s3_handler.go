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
	"whats-convert-api/internal/models"
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

// UploadFile godoc
// @Summary Start multipart upload to S3-compatible storage
// @Description Accepts large media as multipart form-data and dispatches asynchronous upload jobs.
// @Tags S3
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "Binary file to upload"
// @Param options formData string false "JSON encoded upload options" example:{"public":false}
// @Success 202 {object} models.S3UploadResponse
// @Failure 400 {object} models.S3UploadResponse
// @Failure 500 {object} models.S3UploadResponse
// @Failure 503 {object} models.S3UploadResponse
// @Router /upload/s3 [post]
func (h *S3Handler) UploadFile(c fiber.Ctx) error {
	if !h.s3Service.IsEnabled() {
		return c.Status(http.StatusServiceUnavailable).JSON(models.S3UploadResponse{
			Success: false,
			Error:   "S3 upload service is disabled",
		})
	}

	// Parse multipart form
	form, err := c.MultipartForm()
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(models.S3UploadResponse{
			Success: false,
			Error:   "Failed to parse multipart form: " + err.Error(),
		})
	}

	// Get file from form
	files := form.File["file"]
	if len(files) == 0 {
		return c.Status(http.StatusBadRequest).JSON(models.S3UploadResponse{
			Success: false,
			Error:   "No file provided",
		})
	}

	file := files[0]

	// Open uploaded file
	src, err := file.Open()
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(models.S3UploadResponse{
			Success: false,
			Error:   "Failed to open uploaded file: " + err.Error(),
		})
	}

	fileBytes, err := io.ReadAll(src)
	if err != nil {
		src.Close()
		return c.Status(http.StatusInternalServerError).JSON(models.S3UploadResponse{
			Success: false,
			Error:   "Failed to read uploaded file: " + err.Error(),
		})
	}
	src.Close()

	if len(fileBytes) == 0 {
		return c.Status(http.StatusBadRequest).JSON(models.S3UploadResponse{
			Success: false,
			Error:   "Uploaded file is empty",
		})
	}

	// Parse options from form
	var options models.S3UploadRequest
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
		return c.Status(http.StatusInternalServerError).JSON(models.S3UploadResponse{
			Success: false,
			Error:   "Failed to start upload: " + err.Error(),
		})
	}

	// Set original filename
	uploadInfo.OriginalFilename = file.Filename

	return c.Status(http.StatusAccepted).JSON(models.S3UploadResponse{
		Success:  true,
		UploadID: uploadInfo.ID,
		Message:  "Upload started successfully",
	})
}

// UploadBase64 godoc
// @Summary Start base64 upload to S3-compatible storage
// @Description Accepts pre-encoded data URIs and dispatches asynchronous upload jobs.
// @Tags S3
// @Accept json
// @Produce json
// @Param request body models.S3Base64UploadRequest true "Base64 upload request"
// @Success 202 {object} models.S3UploadResponse
// @Failure 400 {object} models.S3UploadResponse
// @Failure 500 {object} models.S3UploadResponse
// @Failure 503 {object} models.S3UploadResponse
// @Router /upload/s3/base64 [post]
func (h *S3Handler) UploadBase64(c fiber.Ctx) error {
	if !h.s3Service.IsEnabled() {
		return c.Status(http.StatusServiceUnavailable).JSON(models.S3UploadResponse{
			Success: false,
			Error:   "S3 upload service is disabled",
		})
	}

	var req models.S3Base64UploadRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(http.StatusBadRequest).JSON(models.S3UploadResponse{
			Success: false,
			Error:   "Invalid JSON payload: " + err.Error(),
		})
	}

	// Validate required fields
	if req.Data == "" {
		return c.Status(http.StatusBadRequest).JSON(models.S3UploadResponse{
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
		return c.Status(http.StatusInternalServerError).JSON(models.S3UploadResponse{
			Success: false,
			Error:   "Failed to start upload: " + err.Error(),
		})
	}

	// Set original filename
	uploadInfo.OriginalFilename = req.Filename

	return c.Status(http.StatusAccepted).JSON(models.S3UploadResponse{
		Success:  true,
		UploadID: uploadInfo.ID,
		Message:  "Upload started successfully",
	})
}

// GetUploadStatus godoc
// @Summary Retrieve asynchronous upload status
// @Tags S3
// @Produce json
// @Param id path string true "Upload identifier"
// @Success 200 {object} models.S3UploadStatusResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Router /upload/s3/status/{id} [get]
func (h *S3Handler) GetUploadStatus(c fiber.Ctx) error {
	uploadID := c.Params("id")
	if uploadID == "" {
		return c.Status(http.StatusBadRequest).JSON(models.ErrorResponse{
			Error: "Upload ID is required",
		})
	}

	uploadInfo, err := h.uploadManager.GetUploadStatus(uploadID)
	if err != nil {
		return c.Status(http.StatusNotFound).JSON(models.ErrorResponse{
			Error: "Upload not found",
		})
	}

	response := models.S3UploadStatusResponse{
		UploadID:         uploadInfo.ID,
		Status:           string(uploadInfo.Status),
		Progress:         uploadInfo.Progress,
		BytesTransferred: uploadInfo.BytesTransferred,
		TotalBytes:       uploadInfo.TotalBytes,
		StartTime:        uploadInfo.StartTime,
		EndTime:          uploadInfo.EndTime,
		Error:            uploadInfo.Error,
		Result:           toS3UploadResult(uploadInfo.Result),
	}

	return c.JSON(response)
}

// CancelUpload godoc
// @Summary Cancel an in-flight upload job
// @Tags S3
// @Produce json
// @Param id path string true "Upload identifier"
// @Success 200 {object} models.MessageResponse
// @Failure 400 {object} models.ErrorResponse
// @Router /upload/s3/status/{id} [delete]
func (h *S3Handler) CancelUpload(c fiber.Ctx) error {
	uploadID := c.Params("id")
	if uploadID == "" {
		return c.Status(http.StatusBadRequest).JSON(models.ErrorResponse{
			Error: "Upload ID is required",
		})
	}

	err := h.uploadManager.CancelUpload(uploadID)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(models.ErrorResponse{
			Error: err.Error(),
		})
	}

	return c.JSON(models.MessageResponse{
		Success: true,
		Message: "Upload cancelled successfully",
	})
}

// ListUploads godoc
// @Summary List recent upload jobs
// @Tags S3
// @Produce json
// @Param status query string false "Filter by upload status (pending|uploading|completed|failed|cancelled)"
// @Param limit query int false "Maximum number of results" default(50)
// @Success 200 {object} models.S3UploadListResponse
// @Router /upload/s3/list [get]
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
	var response []models.S3UploadStatusResponse
	for _, upload := range uploads {
		response = append(response, models.S3UploadStatusResponse{
			UploadID:         upload.ID,
			Status:           string(upload.Status),
			Progress:         upload.Progress,
			BytesTransferred: upload.BytesTransferred,
			TotalBytes:       upload.TotalBytes,
			StartTime:        upload.StartTime,
			EndTime:          upload.EndTime,
			Error:            upload.Error,
			Result:           toS3UploadResult(upload.Result),
		})
	}

	return c.JSON(models.S3UploadListResponse{
		Uploads: response,
		Count:   len(response),
	})
}

// GetS3Stats godoc
// @Summary S3 provider and upload manager metrics
// @Tags S3
// @Produce json
// @Success 200 {object} models.S3StatsResponse
// @Router /upload/s3/stats [get]
func (h *S3Handler) GetS3Stats(c fiber.Ctx) error {
	s3Stats := h.s3Service.GetStats()
	uploadStats := h.uploadManager.GetStats()

	managerStats := models.S3UploadManagerStats{
		StatusCounts: make(map[string]int),
	}

	if total, ok := uploadStats["total_uploads"].(int); ok {
		managerStats.TotalUploads = total
	}
	if current, ok := uploadStats["current_uploads"].(int); ok {
		managerStats.CurrentUploads = current
	}
	if max, ok := uploadStats["max_concurrent"].(int); ok {
		managerStats.MaxConcurrent = max
	}
	if capacity, ok := uploadStats["capacity_used"].(float64); ok {
		managerStats.CapacityUsed = capacity
	}

	if counts, ok := uploadStats["status_counts"].(map[services.UploadStatus]int); ok {
		for status, count := range counts {
			managerStats.StatusCounts[string(status)] = count
		}
	} else if counts, ok := uploadStats["status_counts"].(map[string]int); ok {
		for status, count := range counts {
			managerStats.StatusCounts[status] = count
		}
	}

	response := models.S3StatsResponse{
		S3Service: models.S3ServiceStats{
			Enabled:           h.s3Service.IsEnabled(),
			TotalUploads:      s3Stats.TotalUploads,
			SuccessfulUploads: s3Stats.SuccessfulUploads,
			FailedUploads:     s3Stats.FailedUploads,
			TotalBytes:        s3Stats.TotalBytes,
			SuccessRate:       s3Stats.GetSuccessRate(),
			AvgUploadTime:     s3Stats.GetFormattedAverageTime(),
			LastUpload:        s3Stats.LastUpload,
		},
		UploadManager: managerStats,
	}

	return c.JSON(response)
}

// GetS3Health godoc
// @Summary S3 subsystem health
// @Tags S3
// @Produce json
// @Success 200 {object} models.S3HealthResponse
// @Failure 503 {object} models.S3HealthResponse
// @Router /upload/s3/health [get]
func (h *S3Handler) GetS3Health(c fiber.Ctx) error {
	if !h.s3Service.IsEnabled() {
		return c.JSON(models.S3HealthResponse{
			Status:  "disabled",
			Healthy: true,
			Message: "S3 service is disabled",
		})
	}

	err := h.s3Service.HealthCheck(context.TODO())
	if err != nil {
		return c.Status(http.StatusServiceUnavailable).JSON(models.S3HealthResponse{
			Status:  "unhealthy",
			Healthy: false,
			Error:   err.Error(),
		})
	}

	return c.JSON(models.S3HealthResponse{
		Status:  "healthy",
		Healthy: true,
		Message: "S3 service is operational",
	})
}

// DeleteObject godoc
// @Summary Delete object from storage
// @Tags S3
// @Produce json
// @Param key path string true "Object key"
// @Success 200 {object} models.MessageResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Failure 503 {object} models.ErrorResponse
// @Router /upload/s3/object/{key} [delete]
func (h *S3Handler) DeleteObject(c fiber.Ctx) error {
	if !h.s3Service.IsEnabled() {
		return c.Status(http.StatusServiceUnavailable).JSON(models.ErrorResponse{
			Error: "S3 upload service is disabled",
		})
	}

	key := c.Params("key")
	if key == "" {
		return c.Status(http.StatusBadRequest).JSON(models.ErrorResponse{
			Error: "Object key is required",
		})
	}

	err := h.s3Service.DeleteObject(context.TODO(), key)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(models.ErrorResponse{
			Error: "Failed to delete object: " + err.Error(),
		})
	}

	return c.JSON(models.MessageResponse{
		Success: true,
		Message: "Object deleted successfully",
	})
}

// GetObjectInfo godoc
// @Summary Retrieve object metadata
// @Tags S3
// @Produce json
// @Param key path string true "Object key"
// @Success 200 {object} providers.ObjectInfo
// @Failure 400 {object} models.ErrorResponse
// @Failure 404 {object} models.ErrorResponse
// @Failure 503 {object} models.ErrorResponse
// @Router /upload/s3/object/{key} [get]
func (h *S3Handler) GetObjectInfo(c fiber.Ctx) error {
	if !h.s3Service.IsEnabled() {
		return c.Status(http.StatusServiceUnavailable).JSON(models.ErrorResponse{
			Error: "S3 upload service is disabled",
		})
	}

	key := c.Params("key")
	if key == "" {
		return c.Status(http.StatusBadRequest).JSON(models.ErrorResponse{
			Error: "Object key is required",
		})
	}

	info, err := h.s3Service.GetObjectInfo(context.TODO(), key)
	if err != nil {
		return c.Status(http.StatusNotFound).JSON(models.ErrorResponse{
			Error: "Object not found: " + err.Error(),
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

func toS3UploadResult(res *providers.UploadResult) *models.S3UploadResult {
	if res == nil {
		return nil
	}

	var expiresAt *time.Time
	if res.ExpiresAt != nil {
		copyTime := *res.ExpiresAt
		expiresAt = &copyTime
	}

	return &models.S3UploadResult{
		Key:              res.Key,
		PublicURL:        res.PublicURL,
		Size:             res.Size,
		ETag:             res.ETag,
		VersionID:        res.VersionID,
		ExpiresAt:        expiresAt,
		Provider:         res.Provider,
		UploadID:         res.UploadID,
		ProcessingTimeMS: res.ProcessingTime.Milliseconds(),
	}
}
