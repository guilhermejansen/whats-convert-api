package services

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"whats-convert-api/internal/pool"
)

// AudioConverter handles audio conversion using FFmpeg
type AudioConverter struct {
	workerPool *pool.WorkerPool
	bufferPool *pool.BufferPool
	downloader *Downloader
	mu         sync.RWMutex
	stats      AudioConverterStats
}

// AudioConverterStats tracks conversion metrics
type AudioConverterStats struct {
	TotalConversions  int64
	FailedConversions int64
	AvgConversionTime time.Duration
}

// AudioRequest represents an audio conversion request
type AudioRequest struct {
	Data      string `json:"data" example:"data:audio/aac;base64,T2dnUwACAAAAAAAAAAB"` // base64 or URL
	IsURL     bool   `json:"is_url" example:"false"`                                   // true if data is URL
	InputType string `json:"input_type" example:"mp3"`                                 // Optional: mp3, wav, m4a, etc.
}

// AudioResponse represents the conversion response
type AudioResponse struct {
	Data     string `json:"data" example:"data:audio/ogg;codecs=opus;base64,T2dnUwACAAAA"` // base64 opus audio
	Duration int    `json:"duration" example:"8"`                                          // Duration in seconds
	Size     int    `json:"size" example:"42144"`                                          // Size in bytes
}

// NewAudioConverter creates a new audio converter
func NewAudioConverter(workerPool *pool.WorkerPool, bufferPool *pool.BufferPool, downloader *Downloader) *AudioConverter {
	return &AudioConverter{
		workerPool: workerPool,
		bufferPool: bufferPool,
		downloader: downloader,
	}
}

// Convert processes an audio conversion request
func (ac *AudioConverter) Convert(ctx context.Context, req *AudioRequest) (*AudioResponse, error) {
	start := time.Now()

	// Get input data
	var inputData []byte
	var err error

	if req.IsURL {
		// Download from URL
		inputData, err = ac.downloader.Download(ctx, req.Data)
		if err != nil {
			ac.recordFailure()
			return nil, fmt.Errorf("download failed: %w", err)
		}
	} else {
		// Decode base64
		inputData, err = base64.StdEncoding.DecodeString(req.Data)
		if err != nil {
			ac.recordFailure()
			return nil, fmt.Errorf("base64 decode failed: %w", err)
		}
	}

	// Validate input size
	if len(inputData) == 0 {
		ac.recordFailure()
		return nil, fmt.Errorf("empty input data")
	}

	if len(inputData) > 100*1024*1024 { // 100MB max for audio
		ac.recordFailure()
		return nil, fmt.Errorf("audio file too large: %d bytes", len(inputData))
	}

	// Convert to Opus
	outputData, err := ac.convertToOpus(ctx, inputData)
	if err != nil {
		ac.recordFailure()
		return nil, fmt.Errorf("conversion failed: %w", err)
	}

	// Get audio duration (optional, adds slight overhead)
	duration := ac.getAudioDuration(ctx, outputData)

	// Record success
	ac.recordSuccess(time.Since(start))

	// Encode output to Data URI with Opus codec
	base64Data := base64.StdEncoding.EncodeToString(outputData)
	dataURI := fmt.Sprintf("data:audio/ogg;codecs=opus;base64,%s", base64Data)

	response := &AudioResponse{
		Data:     dataURI,
		Duration: duration,
		Size:     len(outputData),
	}

	return response, nil
}

// convertToOpus converts audio to Opus format optimized for WhatsApp
func (ac *AudioConverter) convertToOpus(ctx context.Context, input []byte) ([]byte, error) {
	// FFmpeg command optimized for WhatsApp Opus
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner",       // Hide FFmpeg banner
		"-loglevel", "error", // Only show errors
		"-i", "pipe:0", // Input from stdin
		"-vn",           // Ignore video streams (important for WebM)
		"-map", "0:a:0", // Select only first audio stream
		"-c:a", "libopus", // Opus codec
		"-b:a", "128k", // Bitrate 128kbps (WhatsApp standard)
		"-vbr", "on", // Variable bitrate for better quality
		"-compression_level", "10", // Maximum compression quality
		"-application", "voip", // Optimized for voice (WhatsApp voice notes)
		"-frame_duration", "20", // Frame duration in ms
		"-packet_loss", "10", // Expected packet loss percentage
		"-cutoff", "20000", // Frequency cutoff (20kHz)
		"-ar", "48000", // Sample rate 48kHz (Opus standard)
		"-ac", "1", // Mono (WhatsApp uses mono for voice)
		"-f", "ogg", // OGG container (WhatsApp compatible)
		"-threads", "0", // Use all available CPU threads
		"pipe:1", // Output to stdout
	)

	// Set up pipes
	cmd.Stdin = bytes.NewReader(input)

	// Get buffers from pool for output
	var outputBuffer bytes.Buffer
	var errorBuffer bytes.Buffer

	cmd.Stdout = &outputBuffer
	cmd.Stderr = &errorBuffer

	// Run conversion
	err := cmd.Run()
	if err != nil {
		// Include FFmpeg error output for debugging
		return nil, fmt.Errorf("ffmpeg error: %v, stderr: %s", err, errorBuffer.String())
	}

	output := outputBuffer.Bytes()
	if len(output) == 0 {
		return nil, fmt.Errorf("ffmpeg produced no output")
	}

	return output, nil
}

// convertToOpusAdvanced provides more control over conversion parameters
func (ac *AudioConverter) convertToOpusAdvanced(ctx context.Context, input []byte, bitrate string, mono bool) ([]byte, error) {
	channels := "2"
	if mono {
		channels = "1"
	}

	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner",
		"-loglevel", "error",
		"-i", "pipe:0",
		"-vn",           // Ignore video streams (important for WebM)
		"-map", "0:a:0", // Select only first audio stream
		"-c:a", "libopus",
		"-b:a", bitrate, // Custom bitrate
		"-vbr", "on",
		"-compression_level", "10",
		"-application", "voip",
		"-ar", "48000",
		"-ac", channels, // Custom channel config
		"-filter:a", "loudnorm=I=-16:LRA=11:TP=-1.5", // Normalize audio levels
		"-f", "ogg", // OGG container (WhatsApp compatible)
		"-threads", "0",
		"pipe:1",
	)

	cmd.Stdin = bytes.NewReader(input)
	var outputBuffer bytes.Buffer
	var errorBuffer bytes.Buffer
	cmd.Stdout = &outputBuffer
	cmd.Stderr = &errorBuffer

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg error: %v, stderr: %s", err, errorBuffer.String())
	}

	return outputBuffer.Bytes(), nil
}

// getAudioDuration gets the duration of audio in seconds
func (ac *AudioConverter) getAudioDuration(ctx context.Context, audioData []byte) int {
	// Use ffprobe to get duration without re-encoding
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ffprobe",
		"-hide_banner",
		"-loglevel", "error",
		"-i", "pipe:0",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
	)

	cmd.Stdin = bytes.NewReader(audioData)

	output, err := cmd.Output()
	if err != nil {
		// Duration is optional, don't fail the conversion
		return 0
	}

	// Parse duration from output
	var duration float64
	fmt.Sscanf(string(output), "%f", &duration)

	return int(duration)
}

// ConvertBatch processes multiple audio conversions in parallel
func (ac *AudioConverter) ConvertBatch(ctx context.Context, requests []*AudioRequest) ([]*AudioResponse, error) {
	responses := make([]*AudioResponse, len(requests))
	errors := make([]error, len(requests))
	var wg sync.WaitGroup

	for i, req := range requests {
		wg.Add(1)
		go func(index int, request *AudioRequest) {
			defer wg.Done()

			// Create individual context with timeout
			convertCtx, cancel := context.WithTimeout(ctx, 1*time.Minute)
			defer cancel()

			resp, err := ac.Convert(convertCtx, request)
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

// ValidateInput checks if the input data is valid audio
func (ac *AudioConverter) ValidateInput(ctx context.Context, data []byte) error {
	// Use ffprobe to validate
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-hide_banner",
		"-loglevel", "error",
		"-i", "pipe:0",
	)

	cmd.Stdin = bytes.NewReader(data)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("invalid audio data: %w", err)
	}

	return nil
}

// Stats recording
func (ac *AudioConverter) recordSuccess(duration time.Duration) {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	ac.stats.TotalConversions++

	// Update average conversion time
	if ac.stats.AvgConversionTime == 0 {
		ac.stats.AvgConversionTime = duration
	} else {
		ac.stats.AvgConversionTime = (ac.stats.AvgConversionTime*9 + duration) / 10
	}
}

func (ac *AudioConverter) recordFailure() {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	ac.stats.TotalConversions++
	ac.stats.FailedConversions++
}

// GetStats returns conversion statistics
func (ac *AudioConverter) GetStats() AudioConverterStats {
	ac.mu.RLock()
	defer ac.mu.RUnlock()

	return ac.stats
}
