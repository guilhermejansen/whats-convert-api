package services

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"whats-convert-api/internal/pool"
)

// Downloader handles HTTP downloads with optimized connection pooling
type Downloader struct {
	httpClient *http.Client
	bufferPool *pool.BufferPool
	maxSize    int64
	mu         sync.RWMutex
	stats      DownloaderStats
}

// DownloaderStats tracks download performance metrics
type DownloaderStats struct {
	TotalDownloads   int64
	FailedDownloads  int64
	TotalBytes       int64
	AvgDownloadTime  time.Duration
}

// NewDownloader creates an optimized HTTP downloader
func NewDownloader(bufferPool *pool.BufferPool, maxSize int64) *Downloader {
	if maxSize <= 0 {
		maxSize = 500 * 1024 * 1024 // 500MB default
	}

	return &Downloader{
		httpClient: &http.Client{
			Timeout: 30 * time.Second, // Aggressive timeout for downloads
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 100,
				MaxConnsPerHost:     100,
				IdleConnTimeout:     90 * time.Second,
				DisableCompression:  true, // We're downloading media files
				DisableKeepAlives:   false, // Keep connections alive for reuse
				ForceAttemptHTTP2:   true,
				TLSHandshakeTimeout: 10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
				ResponseHeaderTimeout: 10 * time.Second,
				ReadBufferSize:       32 * 1024, // 32KB read buffer
				WriteBufferSize:      32 * 1024, // 32KB write buffer
			},
		},
		bufferPool: bufferPool,
		maxSize:    maxSize,
	}
}

// Download fetches content from URL with context support
func (d *Downloader) Download(ctx context.Context, url string) ([]byte, error) {
	start := time.Now()

	// Create request with context
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		d.recordFailure()
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Set headers for better compatibility
	req.Header.Set("User-Agent", "WhatsApp-Media-Converter/1.0")
	req.Header.Set("Accept", "*/*")

	// Execute request
	resp, err := d.httpClient.Do(req)
	if err != nil {
		d.recordFailure()
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		d.recordFailure()
		return nil, fmt.Errorf("http status %d", resp.StatusCode)
	}

	// Check content length if provided
	if resp.ContentLength > 0 && resp.ContentLength > d.maxSize {
		d.recordFailure()
		return nil, fmt.Errorf("content too large: %d bytes (max: %d)", resp.ContentLength, d.maxSize)
	}

	// Get buffer from pool for efficient copying
	buffer := d.bufferPool.Get()
	defer d.bufferPool.Put(buffer)

	// Read response body with size limit
	var result bytes.Buffer
	limitReader := io.LimitReader(resp.Body, d.maxSize)

	// Use buffer for efficient copying
	written, err := io.CopyBuffer(&result, limitReader, buffer)
	if err != nil {
		d.recordFailure()
		return nil, fmt.Errorf("read response: %w", err)
	}

	// Check if we hit the size limit
	if written >= d.maxSize {
		// Try to read one more byte to see if content was truncated
		var testByte [1]byte
		if n, _ := resp.Body.Read(testByte[:]); n > 0 {
			d.recordFailure()
			return nil, fmt.Errorf("content exceeds maximum size of %d bytes", d.maxSize)
		}
	}

	// Record success
	d.recordSuccess(written, time.Since(start))

	return result.Bytes(), nil
}

// DownloadWithTimeout downloads with a custom timeout
func (d *Downloader) DownloadWithTimeout(ctx context.Context, url string, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return d.Download(ctx, url)
}

// StreamDownload downloads and processes data in chunks
func (d *Downloader) StreamDownload(ctx context.Context, url string, processor func(chunk []byte) error) error {
	start := time.Now()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		d.recordFailure()
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("User-Agent", "WhatsApp-Media-Converter/1.0")
	req.Header.Set("Accept", "*/*")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		d.recordFailure()
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		d.recordFailure()
		return fmt.Errorf("http status %d", resp.StatusCode)
	}

	// Get buffer from pool
	buffer := d.bufferPool.GetSized(32 * 1024) // 32KB chunks
	defer d.bufferPool.PutSized(buffer)

	totalBytes := int64(0)
	limitReader := io.LimitReader(resp.Body, d.maxSize)

	for {
		n, err := limitReader.Read(buffer)
		if n > 0 {
			totalBytes += int64(n)
			if procErr := processor(buffer[:n]); procErr != nil {
				d.recordFailure()
				return fmt.Errorf("process chunk: %w", procErr)
			}
		}

		if err == io.EOF {
			break
		}

		if err != nil {
			d.recordFailure()
			return fmt.Errorf("read stream: %w", err)
		}

		// Check context cancellation
		select {
		case <-ctx.Done():
			d.recordFailure()
			return ctx.Err()
		default:
		}
	}

	d.recordSuccess(totalBytes, time.Since(start))
	return nil
}

// Validate checks if a URL is accessible without downloading the content
func (d *Downloader) Validate(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, "HEAD", url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("User-Agent", "WhatsApp-Media-Converter/1.0")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http status %d", resp.StatusCode)
	}

	// Check content length if available
	if resp.ContentLength > 0 && resp.ContentLength > d.maxSize {
		return fmt.Errorf("content too large: %d bytes (max: %d)", resp.ContentLength, d.maxSize)
	}

	return nil
}

// GetContentType fetches just the content type of a URL
func (d *Downloader) GetContentType(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "HEAD", url, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("http status %d", resp.StatusCode)
	}

	return resp.Header.Get("Content-Type"), nil
}

// Stats recording methods
func (d *Downloader) recordSuccess(bytes int64, duration time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.stats.TotalDownloads++
	d.stats.TotalBytes += bytes

	// Update average download time (simple moving average)
	if d.stats.AvgDownloadTime == 0 {
		d.stats.AvgDownloadTime = duration
	} else {
		d.stats.AvgDownloadTime = (d.stats.AvgDownloadTime*9 + duration) / 10
	}
}

func (d *Downloader) recordFailure() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.stats.TotalDownloads++
	d.stats.FailedDownloads++
}

// GetStats returns current downloader statistics
func (d *Downloader) GetStats() DownloaderStats {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.stats
}

// Close cleans up the HTTP client connections
func (d *Downloader) Close() {
	if d.httpClient != nil {
		d.httpClient.CloseIdleConnections()
	}
}