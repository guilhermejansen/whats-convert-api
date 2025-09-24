package handlers

import (
	"html/template"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v3"
)

// WebHandler handles web interface requests
type WebHandler struct {
	templates *template.Template
}

// NewWebHandler creates a new web handler
func NewWebHandler() (*WebHandler, error) {
	// Parse templates
	templates, err := template.ParseGlob("web/templates/*.html")
	if err != nil {
		return nil, err
	}

	return &WebHandler{
		templates: templates,
	}, nil
}

// PageData represents data passed to templates
type PageData struct {
	Title   string
	Content template.HTML
	Scripts []string
}

// ServeHome serves the main interface page
func (h *WebHandler) ServeHome(c fiber.Ctx) error {
	// Define scripts to load for the home page
	scripts := []string{
		"whatsapp-converter.js",
		"base64-converter.js",
		"s3-uploader.js",
	}

	// Parse the index template to get content
	indexTemplate := h.templates.Lookup("index.html")
	if indexTemplate == nil {
		return c.Status(http.StatusInternalServerError).SendString("Template not found")
	}

	// Execute index template to get content
	var contentBuffer []byte
	contentWriter := &bufferWriter{buffer: &contentBuffer}

	err := indexTemplate.Execute(contentWriter, nil)
	if err != nil {
		return c.Status(http.StatusInternalServerError).SendString("Template execution failed")
	}

	// Prepare page data
	pageData := PageData{
		Title:   "Media Converter",
		Content: template.HTML(contentBuffer),
		Scripts: scripts,
	}

	// Execute layout template
	var layoutBuffer []byte
	layoutWriter := &bufferWriter{buffer: &layoutBuffer}

	layoutTemplate := h.templates.Lookup("layout.html")
	if layoutTemplate == nil {
		return c.Status(http.StatusInternalServerError).SendString("Layout template not found")
	}

	err = layoutTemplate.Execute(layoutWriter, pageData)
	if err != nil {
		return c.Status(http.StatusInternalServerError).SendString("Layout execution failed")
	}

	// Set content type and return HTML
	c.Set("Content-Type", "text/html; charset=utf-8")
	return c.Send(layoutBuffer)
}

// ServeStatic serves static files (CSS, JS, images)
func (h *WebHandler) ServeStatic(c fiber.Ctx) error {
	// Get the requested file path
	requestedPath := c.Params("*")

	if requestedPath == "" {
		return c.Status(http.StatusBadRequest).SendString("Invalid file path")
	}

	// Normalize and validate the path to prevent traversal outside static root.
	cleanedPath := filepath.Clean(requestedPath)
	if cleanedPath == "." {
		return c.Status(http.StatusBadRequest).SendString("Invalid file path")
	}

	normalizedPath := filepath.ToSlash(cleanedPath)
	if strings.HasPrefix(normalizedPath, "/") {
		return c.Status(http.StatusBadRequest).SendString("Invalid file path")
	}

	for _, segment := range strings.Split(normalizedPath, "/") {
		if segment == ".." {
			return c.Status(http.StatusBadRequest).SendString("Invalid file path")
		}
	}

	absStaticRoot, err := filepath.Abs(filepath.Join("web", "static"))
	if err != nil {
		return c.Status(http.StatusInternalServerError).SendString("Failed to resolve static directory")
	}

	safeRelative := strings.ReplaceAll(normalizedPath, "/", string(filepath.Separator))
	filePath := filepath.Join(absStaticRoot, safeRelative)

	rel, err := filepath.Rel(absStaticRoot, filePath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return c.Status(http.StatusBadRequest).SendString("Invalid file path")
	}

	// Set appropriate content type based on extension
	ext := filepath.Ext(filePath)
	switch ext {
	case ".css":
		c.Set("Content-Type", "text/css")
		c.Set("Cache-Control", "public, max-age=86400") // 24 hours
	case ".js":
		c.Set("Content-Type", "application/javascript")
		c.Set("Cache-Control", "public, max-age=86400") // 24 hours
	case ".ico":
		c.Set("Content-Type", "image/x-icon")
		c.Set("Cache-Control", "public, max-age=604800") // 7 days
	case ".png":
		c.Set("Content-Type", "image/png")
		c.Set("Cache-Control", "public, max-age=604800") // 7 days
	case ".jpg", ".jpeg":
		c.Set("Content-Type", "image/jpeg")
		c.Set("Cache-Control", "public, max-age=604800") // 7 days
	case ".svg":
		c.Set("Content-Type", "image/svg+xml")
		c.Set("Cache-Control", "public, max-age=604800") // 7 days
	default:
		c.Set("Content-Type", "application/octet-stream")
	}

	// Serve file
	return c.SendFile(filePath)
}

// RegisterWebRoutes registers all web interface routes
func (h *WebHandler) RegisterWebRoutes(app *fiber.App) {
	// Home page
	app.Get("/", h.ServeHome)

	// Static files
	app.Get("/static/*", h.ServeStatic)

	// Additional web routes can be added here
	// For example: app.Get("/docs", h.ServeDocs)
}

// bufferWriter implements io.Writer for capturing template output
type bufferWriter struct {
	buffer *[]byte
}

func (w *bufferWriter) Write(p []byte) (n int, err error) {
	*w.buffer = append(*w.buffer, p...)
	return len(p), nil
}
