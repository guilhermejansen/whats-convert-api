package handlers

import (
	"github.com/gofiber/fiber/v3"
	"whats-convert-api/internal/models"
)

// MetaHandler exposes informational endpoints about the API surface.
type MetaHandler struct {
	version   string
	s3Enabled bool
}

// NewMetaHandler constructs a metadata handler.
func NewMetaHandler(version string, s3Enabled bool) *MetaHandler {
	if version == "" {
		version = "1.0.0"
	}

	return &MetaHandler{
		version:   version,
		s3Enabled: s3Enabled,
	}
}

// APIInfo godoc
// @Summary API metadata
// @Description Provides API version and available endpoint catalogue.
// @Tags General
// @Produce json
// @Success 200 {object} models.APIInfoResponse
// @Router /api [get]
func (h *MetaHandler) APIInfo(c fiber.Ctx) error {
	endpoints := map[string]string{
		"audio":       "/convert/audio",
		"image":       "/convert/image",
		"batch_audio": "/convert/batch/audio",
		"batch_image": "/convert/batch/image",
		"health":      "/health",
		"stats":       "/stats",
	}

	if h.s3Enabled {
		endpoints["s3_upload_form"] = "/upload/s3"
		endpoints["s3_upload_base64"] = "/upload/s3/base64"
		endpoints["s3_status"] = "/upload/s3/status/{id}"
		endpoints["s3_list"] = "/upload/s3/list"
		endpoints["s3_object"] = "/upload/s3/object/{key}"
		endpoints["s3_health"] = "/upload/s3/health"
		endpoints["s3_stats"] = "/upload/s3/stats"
	}

	return c.JSON(models.APIInfoResponse{
		Name:      "WhatsApp Media Converter API",
		Version:   h.version,
		Endpoints: endpoints,
	})
}
