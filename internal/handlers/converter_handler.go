package handlers

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"whats-convert-api/internal/models"
	"whats-convert-api/internal/services"
)

// ConverterHandler handles HTTP requests for media conversion
type ConverterHandler struct {
	audioConverter *services.AudioConverter
	imageConverter *services.ImageConverter
	requestTimeout time.Duration
}

// NewConverterHandler creates a new converter handler
func NewConverterHandler(
	audioConverter *services.AudioConverter,
	imageConverter *services.ImageConverter,
	requestTimeout time.Duration,
) *ConverterHandler {
	if requestTimeout <= 0 {
		requestTimeout = 5 * time.Minute
	}

	return &ConverterHandler{
		audioConverter: audioConverter,
		imageConverter: imageConverter,
		requestTimeout: requestTimeout,
	}
}

// ConvertAudio godoc
// @Summary Convert audio to WhatsApp-compatible Opus format
// @Description Accepts base64 payloads or multipart uploads and returns an optimized Opus data URI.
// @Tags Conversion
// @Accept json
// @Accept multipart/form-data
// @Produce json
// @Param request body services.AudioRequest true "Audio conversion request"
// @Param file formData file false "Audio file when using multipart"
// @Success 200 {object} services.AudioResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 408 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /convert/audio [post]
func (h *ConverterHandler) ConvertAudio(c fiber.Ctx) error {
	req, err := h.parseAudioRequest(c)
	if err != nil {
		return respondWithError(c, err)
	}

	return h.processAudioConversion(c, req)
}

// ConvertImage godoc
// @Summary Convert image to WhatsApp-optimized JPEG
// @Description Accepts base64 payloads or multipart uploads and returns a compressed JPEG data URI.
// @Tags Conversion
// @Accept json
// @Accept multipart/form-data
// @Produce json
// @Param request body services.ImageRequest true "Image conversion request"
// @Param file formData file false "Image file when using multipart"
// @Success 200 {object} services.ImageResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 408 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /convert/image [post]
func (h *ConverterHandler) ConvertImage(c fiber.Ctx) error {
	req, err := h.parseImageRequest(c)
	if err != nil {
		return respondWithError(c, err)
	}

	return h.processImageConversion(c, req)
}

// ConvertBatchAudio godoc
// @Summary Convert a batch of audio payloads
// @Description Processes up to 10 audio conversion jobs concurrently.
// @Tags Conversion
// @Accept json
// @Produce json
// @Param request body []services.AudioRequest true "Batch audio conversion request"
// @Success 200 {object} models.BatchAudioResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /convert/batch/audio [post]
func (h *ConverterHandler) ConvertBatchAudio(c fiber.Ctx) error {
	// Parse request
	var requests []services.AudioRequest
	if err := c.Bind().Body(&requests); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "Invalid request body",
			Details: err.Error(),
		})
	}

	// Validate batch size
	if len(requests) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error: "Empty batch",
		})
	}

	if len(requests) > 10 { // Limit batch size
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "Batch too large",
			Details: "Maximum 10 items per batch",
		})
	}

	// Create context with extended timeout for batch
	ctx, cancel := context.WithTimeout(context.Background(), h.requestTimeout*time.Duration(len(requests)))
	defer cancel()

	// Convert request slice to pointer slice
	reqPointers := make([]*services.AudioRequest, len(requests))
	for i := range requests {
		reqPointers[i] = &requests[i]
	}

	// Process batch conversion
	start := time.Now()
	responses, err := h.audioConverter.ConvertBatch(ctx, reqPointers)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "Batch conversion failed",
			Details: err.Error(),
		})
	}

	// Set response headers
	c.Set("X-Processing-Time", fmt.Sprintf("%dms", time.Since(start).Milliseconds()))
	c.Set("X-Batch-Size", fmt.Sprintf("%d", len(responses)))

	return c.JSON(models.BatchAudioResponse{
		Results: responses,
		Count:   len(responses),
	})
}

// ConvertBatchImage godoc
// @Summary Convert a batch of image payloads
// @Description Processes up to 10 image conversion jobs concurrently.
// @Tags Conversion
// @Accept json
// @Produce json
// @Param request body []services.ImageRequest true "Batch image conversion request"
// @Success 200 {object} models.BatchImageResponse
// @Failure 400 {object} models.ErrorResponse
// @Failure 500 {object} models.ErrorResponse
// @Router /convert/batch/image [post]
func (h *ConverterHandler) ConvertBatchImage(c fiber.Ctx) error {
	// Parse request
	var requests []services.ImageRequest
	if err := c.Bind().Body(&requests); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "Invalid request body",
			Details: err.Error(),
		})
	}

	// Validate batch size
	if len(requests) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error: "Empty batch",
		})
	}

	if len(requests) > 10 { // Limit batch size
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error:   "Batch too large",
			Details: "Maximum 10 items per batch",
		})
	}

	// Create context with extended timeout for batch
	ctx, cancel := context.WithTimeout(context.Background(), h.requestTimeout*time.Duration(len(requests)))
	defer cancel()

	// Convert request slice to pointer slice
	reqPointers := make([]*services.ImageRequest, len(requests))
	for i := range requests {
		reqPointers[i] = &requests[i]
	}

	// Process batch conversion
	start := time.Now()
	responses, err := h.imageConverter.ConvertBatch(ctx, reqPointers)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "Batch conversion failed",
			Details: err.Error(),
		})
	}

	// Set response headers
	c.Set("X-Processing-Time", fmt.Sprintf("%dms", time.Since(start).Milliseconds()))
	c.Set("X-Batch-Size", fmt.Sprintf("%d", len(responses)))

	return c.JSON(models.BatchImageResponse{
		Results: responses,
		Count:   len(responses),
	})
}

// Health godoc
// @Summary Service health snapshot
// @Description Returns aggregated success metrics for audio and image converters.
// @Tags Monitoring
// @Produce json
// @Success 200 {object} models.HealthResponse
// @Router /health [get]
func (h *ConverterHandler) Health(c fiber.Ctx) error {
	// Get converter stats
	audioStats := h.audioConverter.GetStats()
	imageStats := h.imageConverter.GetStats()

	// Calculate success rates
	audioSuccessRate := float64(0)
	if audioStats.TotalConversions > 0 {
		audioSuccessRate = float64(audioStats.TotalConversions-audioStats.FailedConversions) / float64(audioStats.TotalConversions)
	}

	imageSuccessRate := float64(0)
	if imageStats.TotalConversions > 0 {
		imageSuccessRate = float64(imageStats.TotalConversions-imageStats.FailedConversions) / float64(imageStats.TotalConversions)
	}

	return c.JSON(models.HealthResponse{
		Status:    "healthy",
		Timestamp: time.Now().Unix(),
		Audio: models.AudioHealthMetrics{
			TotalConversions:  audioStats.TotalConversions,
			FailedConversions: audioStats.FailedConversions,
			SuccessRate:       fmt.Sprintf("%.2f%%", audioSuccessRate*100),
			AvgConversionMS:   audioStats.AvgConversionTime.Milliseconds(),
		},
		Image: models.ImageHealthMetrics{
			TotalConversions:  imageStats.TotalConversions,
			FailedConversions: imageStats.FailedConversions,
			SuccessRate:       fmt.Sprintf("%.2f%%", imageSuccessRate*100),
			AvgConversionMS:   imageStats.AvgConversionTime.Milliseconds(),
			VipsAvailable:     h.imageConverter.IsVipsAvailable(),
		},
	})
}

// Stats godoc
// @Summary Converter statistics
// @Description Exposes raw converter counters for observability integrations.
// @Tags Monitoring
// @Produce json
// @Success 200 {object} models.StatsResponse
// @Router /stats [get]
func (h *ConverterHandler) Stats(c fiber.Ctx) error {
	audioStats := h.audioConverter.GetStats()
	imageStats := h.imageConverter.GetStats()

	return c.JSON(models.StatsResponse{
		Audio: models.ConverterStats{
			TotalConversions:    audioStats.TotalConversions,
			FailedConversions:   audioStats.FailedConversions,
			AvgConversionTimeMS: audioStats.AvgConversionTime.Milliseconds(),
		},
		Image: models.ImageConverterStats{
			TotalConversions:    imageStats.TotalConversions,
			FailedConversions:   imageStats.FailedConversions,
			AvgConversionTimeMS: imageStats.AvgConversionTime.Milliseconds(),
			VipsConversions:     imageStats.VipsConversions,
			FFmpegConversions:   imageStats.FFmpegConversions,
		},
		Timestamp: time.Now().Unix(),
	})
}

func (h *ConverterHandler) parseAudioRequest(c fiber.Ctx) (*services.AudioRequest, error) {
	contentType := strings.ToLower(c.Get("Content-Type"))
	if strings.HasPrefix(contentType, "multipart/form-data") {
		return parseMultipartAudio(c)
	}

	var req services.AudioRequest
	if err := c.Bind().Body(&req); err != nil {
		return nil, newRequestError(fiber.StatusBadRequest, "Invalid request body", err.Error())
	}

	return &req, nil
}

func (h *ConverterHandler) parseImageRequest(c fiber.Ctx) (*services.ImageRequest, error) {
	contentType := strings.ToLower(c.Get("Content-Type"))
	if strings.HasPrefix(contentType, "multipart/form-data") {
		return parseMultipartImage(c)
	}

	var req services.ImageRequest
	if err := c.Bind().Body(&req); err != nil {
		return nil, newRequestError(fiber.StatusBadRequest, "Invalid request body", err.Error())
	}

	return &req, nil
}

func (h *ConverterHandler) processAudioConversion(c fiber.Ctx, req *services.AudioRequest) error {
	if req == nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error: "Invalid request",
		})
	}

	req.Data = sanitizeBase64Data(req.Data)
	if strings.TrimSpace(req.Data) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error: "Missing 'data' field",
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), h.requestTimeout)
	defer cancel()

	start := time.Now()
	response, err := h.audioConverter.Convert(ctx, req)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return c.Status(fiber.StatusRequestTimeout).JSON(models.ErrorResponse{
				Error:   "Request timeout",
				Details: "Conversion took too long",
			})
		}

		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "Conversion failed",
			Details: err.Error(),
		})
	}

	c.Set("X-Processing-Time", fmt.Sprintf("%dms", time.Since(start).Milliseconds()))
	c.Set("X-Output-Size", fmt.Sprintf("%d", response.Size))

	return c.JSON(response)
}

func (h *ConverterHandler) processImageConversion(c fiber.Ctx, req *services.ImageRequest) error {
	if req == nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error: "Invalid request",
		})
	}

	req.Data = sanitizeBase64Data(req.Data)
	if strings.TrimSpace(req.Data) == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error: "Missing 'data' field",
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), h.requestTimeout)
	defer cancel()

	start := time.Now()
	response, err := h.imageConverter.Convert(ctx, req)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return c.Status(fiber.StatusRequestTimeout).JSON(models.ErrorResponse{
				Error:   "Request timeout",
				Details: "Conversion took too long",
			})
		}

		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error:   "Conversion failed",
			Details: err.Error(),
		})
	}

	c.Set("X-Processing-Time", fmt.Sprintf("%dms", time.Since(start).Milliseconds()))
	c.Set("X-Output-Size", fmt.Sprintf("%d", response.Size))
	c.Set("X-Output-Dimensions", fmt.Sprintf("%dx%d", response.Width, response.Height))

	return c.JSON(response)
}

func parseMultipartAudio(c fiber.Ctx) (*services.AudioRequest, error) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		return nil, newRequestError(fiber.StatusBadRequest, "Missing file", "file field is required")
	}

	file, err := fileHeader.Open()
	if err != nil {
		return nil, newRequestError(fiber.StatusInternalServerError, "Failed to open uploaded file", err.Error())
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, newRequestError(fiber.StatusInternalServerError, "Failed to read uploaded file", err.Error())
	}
	if len(data) == 0 {
		return nil, newRequestError(fiber.StatusBadRequest, "Uploaded file is empty", "")
	}

	encoded := base64.StdEncoding.EncodeToString(data)
	inputType := strings.TrimPrefix(strings.ToLower(filepath.Ext(fileHeader.Filename)), ".")
	if inputType == "" {
		inputType = deriveInputTypeFromContentType(fileHeader.Header.Get("Content-Type"))
	}
	if formType := strings.TrimSpace(c.FormValue("input_type")); formType != "" {
		inputType = formType
	}

	return &services.AudioRequest{
		Data:      encoded,
		IsURL:     false,
		InputType: inputType,
	}, nil
}

func parseMultipartImage(c fiber.Ctx) (*services.ImageRequest, error) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		return nil, newRequestError(fiber.StatusBadRequest, "Missing file", "file field is required")
	}

	file, err := fileHeader.Open()
	if err != nil {
		return nil, newRequestError(fiber.StatusInternalServerError, "Failed to open uploaded file", err.Error())
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, newRequestError(fiber.StatusInternalServerError, "Failed to read uploaded file", err.Error())
	}
	if len(data) == 0 {
		return nil, newRequestError(fiber.StatusBadRequest, "Uploaded file is empty", "")
	}

	encoded := base64.StdEncoding.EncodeToString(data)
	req := &services.ImageRequest{
		Data:  encoded,
		IsURL: false,
	}

	if qualityStr := strings.TrimSpace(c.FormValue("quality")); qualityStr != "" {
		quality, convErr := strconv.Atoi(qualityStr)
		if convErr != nil {
			return nil, newRequestError(fiber.StatusBadRequest, "Invalid quality value", "quality must be an integer")
		}
		req.Quality = quality
	}

	if widthStr := strings.TrimSpace(c.FormValue("max_width")); widthStr != "" {
		width, convErr := strconv.Atoi(widthStr)
		if convErr != nil {
			return nil, newRequestError(fiber.StatusBadRequest, "Invalid max_width value", "max_width must be an integer")
		}
		req.MaxWidth = width
	}

	if heightStr := strings.TrimSpace(c.FormValue("max_height")); heightStr != "" {
		height, convErr := strconv.Atoi(heightStr)
		if convErr != nil {
			return nil, newRequestError(fiber.StatusBadRequest, "Invalid max_height value", "max_height must be an integer")
		}
		req.MaxHeight = height
	}

	return req, nil
}

type requestError struct {
	status  int
	message string
	details string
}

func (e *requestError) Error() string {
	if e == nil {
		return ""
	}
	if e.details == "" {
		return e.message
	}
	return fmt.Sprintf("%s: %s", e.message, e.details)
}

func newRequestError(status int, message, details string) error {
	return &requestError{status: status, message: message, details: details}
}

func respondWithError(c fiber.Ctx, err error) error {
	if err == nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error: "Internal Server Error",
		})
	}

	var reqErr *requestError
	if errors.As(err, &reqErr) {
		response := models.ErrorResponse{Error: reqErr.message}
		if reqErr.details != "" {
			response.Details = reqErr.details
		}
		return c.Status(reqErr.status).JSON(response)
	}

	var fiberErr *fiber.Error
	if errors.As(err, &fiberErr) {
		return c.Status(fiberErr.Code).JSON(models.ErrorResponse{
			Error: fiberErr.Message,
		})
	}

	return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
		Error:   "Internal Server Error",
		Details: err.Error(),
	})
}

func sanitizeBase64Data(data string) string {
	trimmed := strings.TrimSpace(data)
	if trimmed == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(trimmed), "data:") {
		if commaIdx := strings.Index(trimmed, ","); commaIdx != -1 && commaIdx < len(trimmed)-1 {
			return trimmed[commaIdx+1:]
		}
	}
	return trimmed
}

func deriveInputTypeFromContentType(contentType string) string {
	if contentType == "" {
		return ""
	}
	parts := strings.Split(contentType, "/")
	if len(parts) != 2 {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
