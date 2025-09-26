package services

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"whats-convert-api/internal/pool"
)

// ImageConverter handles image conversion using libvips or FFmpeg
type ImageConverter struct {
	workerPool *pool.WorkerPool
	bufferPool *pool.BufferPool
	downloader *Downloader
	useVips    bool // Whether vips is available
	mu         sync.RWMutex
	stats      ImageConverterStats
}

// ImageConverterStats tracks conversion metrics
type ImageConverterStats struct {
	TotalConversions  int64
	FailedConversions int64
	AvgConversionTime time.Duration
	VipsConversions   int64
	FFmpegConversions int64
}

// ImageRequest represents an image conversion request
type ImageRequest struct {
	Data      string `json:"data" example:"data:image/jpeg;base64,/9j/4AAQSkZJRgABAQAAAQABAAD"` // base64 or URL
	IsURL     bool   `json:"is_url" example:"false"`                                            // true if data is URL
	MaxWidth  int    `json:"max_width" example:"1920"`                                          // Optional: max width (default 1920)
	MaxHeight int    `json:"max_height" example:"1920"`                                         // Optional: max height (default 1920)
	Quality   int    `json:"quality" example:"90"`                                              // Optional: JPEG quality 1-100 (default 95)
}

// ImageResponse represents the conversion response
type ImageResponse struct {
	Data   string `json:"data" example:"data:image/jpeg;base64,/9j/4AAQSkZJRgABA"` // base64 jpeg image
	Width  int    `json:"width" example:"800"`                                     // Image width
	Height int    `json:"height" example:"600"`                                    // Image height
	Size   int    `json:"size" example:"20480"`                                    // Size in bytes
}

// NewImageConverter creates a new image converter
func NewImageConverter(workerPool *pool.WorkerPool, bufferPool *pool.BufferPool, downloader *Downloader) *ImageConverter {
	// Check if vips is available
	useVips := false
	if _, err := exec.LookPath("vips"); err == nil {
		useVips = true
	}

	return &ImageConverter{
		workerPool: workerPool,
		bufferPool: bufferPool,
		downloader: downloader,
		useVips:    useVips,
	}
}

// Convert processes an image conversion request
func (ic *ImageConverter) Convert(ctx context.Context, req *ImageRequest) (*ImageResponse, error) {
	start := time.Now()

	// Set defaults
	if req.MaxWidth <= 0 {
		req.MaxWidth = 1920
	}
	if req.MaxHeight <= 0 {
		req.MaxHeight = 1920
	}
	if req.Quality <= 0 || req.Quality > 100 {
		req.Quality = 95
	}

	// Get input data
	var inputData []byte
	var err error

	if req.IsURL {
		// Download from URL
		inputData, err = ic.downloader.Download(ctx, req.Data)
		if err != nil {
			ic.recordFailure()
			return nil, fmt.Errorf("download failed: %w", err)
		}
	} else {
		// Decode base64
		inputData, err = base64.StdEncoding.DecodeString(req.Data)
		if err != nil {
			ic.recordFailure()
			return nil, fmt.Errorf("base64 decode failed: %w", err)
		}
	}

	// Validate input size
	if len(inputData) == 0 {
		ic.recordFailure()
		return nil, fmt.Errorf("empty input data")
	}

	if len(inputData) > 200*1024*1024 { // 200MB max for images
		ic.recordFailure()
		return nil, fmt.Errorf("image file too large: %d bytes", len(inputData))
	}

	// Convert to JPEG
	var outputData []byte
	if ic.useVips {
		outputData, err = ic.convertWithVips(ctx, inputData, req.Quality)
		if err == nil {
			ic.recordVipsSuccess(time.Since(start))
		} else {
			// Fallback to FFmpeg if vips fails
			outputData, err = ic.convertWithFFmpeg(ctx, inputData, req.MaxWidth, req.MaxHeight, req.Quality)
			if err != nil {
				ic.recordFailure()
				return nil, fmt.Errorf("conversion failed: %w", err)
			}
			ic.recordFFmpegSuccess(time.Since(start))
		}
	} else {
		outputData, err = ic.convertWithFFmpeg(ctx, inputData, req.MaxWidth, req.MaxHeight, req.Quality)
		if err != nil {
			ic.recordFailure()
			return nil, fmt.Errorf("conversion failed: %w", err)
		}
		ic.recordFFmpegSuccess(time.Since(start))
	}

	// Get image dimensions (optional)
	width, height := ic.getImageDimensions(ctx, outputData)

	// Encode output to Data URI JPEG format
	base64Data := base64.StdEncoding.EncodeToString(outputData)
	dataURI := fmt.Sprintf("data:image/jpeg;base64,%s", base64Data)

	response := &ImageResponse{
		Data:   dataURI,
		Width:  width,
		Height: height,
		Size:   len(outputData),
	}

	return response, nil
}

// convertWithVips uses libvips for fast image conversion
func (ic *ImageConverter) convertWithVips(ctx context.Context, input []byte, quality int) ([]byte, error) {
	// vips is significantly faster than ImageMagick for image processing
	cmd := exec.CommandContext(ctx, "vips",
		"jpegsave_buffer",
		"-",                            // Input from stdin
		"-",                            // Output to stdout
		fmt.Sprintf("--Q=%d", quality), // Quality setting
		"--optimize-coding",            // Optimize Huffman coding tables
		"--strip",                      // Strip all metadata
		"--interlace",                  // Progressive JPEG
		"--trellis-quant",              // Use trellis quantisation
		"--overshoot-deringing",        // Reduce ringing artifacts
		"--optimize-scans",             // Optimize progressive scan layers
		"--quant-table=3",              // Use high quality quantization table
	)

	cmd.Stdin = bytes.NewReader(input)

	var outputBuffer bytes.Buffer
	var errorBuffer bytes.Buffer
	cmd.Stdout = &outputBuffer
	cmd.Stderr = &errorBuffer

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("vips error: %v, stderr: %s", err, errorBuffer.String())
	}

	output := outputBuffer.Bytes()
	if len(output) == 0 {
		return nil, fmt.Errorf("vips produced no output")
	}

	return output, nil
}

// convertWithFFmpeg uses FFmpeg as fallback for image conversion
func (ic *ImageConverter) convertWithFFmpeg(ctx context.Context, input []byte, maxWidth, maxHeight, quality int) ([]byte, error) {
	// Calculate quality value for FFmpeg (2-31, lower is better)
	ffmpegQuality := 31 - (quality * 29 / 100)
	if ffmpegQuality < 2 {
		ffmpegQuality = 2
	}

	// Build scale filter
	scaleFilter := fmt.Sprintf(
		"scale='min(%d,iw)':'min(%d,ih)':force_original_aspect_ratio=decrease:flags=lanczos",
		maxWidth, maxHeight,
	)

	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner",
		"-loglevel", "error",
		"-i", "pipe:0", // Input from stdin
		"-vf", scaleFilter, // Scale filter with Lanczos resampling
		"-q:v", fmt.Sprintf("%d", ffmpegQuality), // Quality setting
		"-vcodec", "mjpeg", // JPEG codec
		"-pix_fmt", "yuvj444p", // High quality pixel format
		"-f", "image2pipe", // Output format
		"-threads", "0", // Use all available threads
		"pipe:1", // Output to stdout
	)

	cmd.Stdin = bytes.NewReader(input)

	var outputBuffer bytes.Buffer
	var errorBuffer bytes.Buffer
	cmd.Stdout = &outputBuffer
	cmd.Stderr = &errorBuffer

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg error: %v, stderr: %s", err, errorBuffer.String())
	}

	output := outputBuffer.Bytes()
	if len(output) == 0 {
		return nil, fmt.Errorf("ffmpeg produced no output")
	}

	return output, nil
}

// convertWithOptimization applies additional optimizations
func (ic *ImageConverter) convertWithOptimization(ctx context.Context, input []byte, req *ImageRequest) ([]byte, error) {
	// First pass: Convert and resize
	resized, err := ic.convertWithFFmpeg(ctx, input, req.MaxWidth, req.MaxHeight, req.Quality)
	if err != nil {
		return nil, err
	}

	// Second pass: Optimize with jpegoptim if available
	if _, err := exec.LookPath("jpegoptim"); err == nil {
		cmd := exec.CommandContext(ctx, "jpegoptim",
			"--stdin",
			"--stdout",
			fmt.Sprintf("--max=%d", req.Quality),
			"--strip-all",
			"--all-progressive",
		)

		cmd.Stdin = bytes.NewReader(resized)

		var outputBuffer bytes.Buffer
		cmd.Stdout = &outputBuffer

		if err := cmd.Run(); err == nil && outputBuffer.Len() > 0 {
			return outputBuffer.Bytes(), nil
		}
	}

	return resized, nil
}

// getImageDimensions gets the dimensions of an image
func (ic *ImageConverter) getImageDimensions(ctx context.Context, imageData []byte) (int, int) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// Try with ffprobe first
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-hide_banner",
		"-loglevel", "error",
		"-i", "pipe:0",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height",
		"-of", "csv=p=0",
	)

	cmd.Stdin = bytes.NewReader(imageData)

	output, err := cmd.Output()
	if err != nil {
		return 0, 0
	}

	// Parse dimensions from output (format: width,height)
	parts := strings.Split(strings.TrimSpace(string(output)), ",")
	if len(parts) != 2 {
		return 0, 0
	}

	var width, height int
	fmt.Sscanf(parts[0], "%d", &width)
	fmt.Sscanf(parts[1], "%d", &height)

	return width, height
}

// ConvertBatch processes multiple image conversions in parallel
func (ic *ImageConverter) ConvertBatch(ctx context.Context, requests []*ImageRequest) ([]*ImageResponse, error) {
	responses := make([]*ImageResponse, len(requests))
	errors := make([]error, len(requests))
	var wg sync.WaitGroup

	for i, req := range requests {
		wg.Add(1)
		go func(index int, request *ImageRequest) {
			defer wg.Done()

			// Create individual context with timeout
			convertCtx, cancel := context.WithTimeout(ctx, 1*time.Minute)
			defer cancel()

			resp, err := ic.Convert(convertCtx, request)
			if err != nil {
				errors[index] = err
			} else {
				responses[index] = resp
			}
		}(i, req)
	}

	wg.Wait()

	// Check for any errors
	for i, err := range errors {
		if err != nil {
			return responses, fmt.Errorf("conversion %d failed: %w", i, err)
		}
	}

	return responses, nil
}

// ValidateInput checks if the input data is a valid image
func (ic *ImageConverter) ValidateInput(ctx context.Context, data []byte) error {
	// Use ffprobe to validate
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-hide_banner",
		"-loglevel", "error",
		"-i", "pipe:0",
		"-select_streams", "v:0",
		"-show_entries", "stream=codec_type",
	)

	cmd.Stdin = bytes.NewReader(data)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("invalid image data: %w", err)
	}

	if !strings.Contains(string(output), "video") && !strings.Contains(string(output), "image") {
		return fmt.Errorf("input is not an image")
	}

	return nil
}

// Stats recording
func (ic *ImageConverter) recordVipsSuccess(duration time.Duration) {
	ic.mu.Lock()
	defer ic.mu.Unlock()

	ic.stats.TotalConversions++
	ic.stats.VipsConversions++
	ic.updateAvgTime(duration)
}

func (ic *ImageConverter) recordFFmpegSuccess(duration time.Duration) {
	ic.mu.Lock()
	defer ic.mu.Unlock()

	ic.stats.TotalConversions++
	ic.stats.FFmpegConversions++
	ic.updateAvgTime(duration)
}

func (ic *ImageConverter) updateAvgTime(duration time.Duration) {
	if ic.stats.AvgConversionTime == 0 {
		ic.stats.AvgConversionTime = duration
	} else {
		ic.stats.AvgConversionTime = (ic.stats.AvgConversionTime*9 + duration) / 10
	}
}

func (ic *ImageConverter) recordFailure() {
	ic.mu.Lock()
	defer ic.mu.Unlock()

	ic.stats.TotalConversions++
	ic.stats.FailedConversions++
}

// GetStats returns conversion statistics
func (ic *ImageConverter) GetStats() ImageConverterStats {
	ic.mu.RLock()
	defer ic.mu.RUnlock()

	return ic.stats
}

// IsVipsAvailable returns whether vips is available
func (ic *ImageConverter) IsVipsAvailable() bool {
	return ic.useVips
}
