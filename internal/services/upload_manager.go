package services

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/google/uuid"
	"whats-convert-api/internal/providers"
)

// UploadStatus represents the status of an upload
type UploadStatus string

const (
	UploadStatusPending   UploadStatus = "pending"
	UploadStatusUploading UploadStatus = "uploading"
	UploadStatusCompleted UploadStatus = "completed"
	UploadStatusFailed    UploadStatus = "failed"
	UploadStatusCancelled UploadStatus = "cancelled"
)

// UploadInfo contains information about an ongoing upload
type UploadInfo struct {
	ID               string                  `json:"id"`
	Key              string                  `json:"key"`
	Status           UploadStatus            `json:"status"`
	Progress         float64                 `json:"progress"`
	BytesTransferred int64                   `json:"bytes_transferred"`
	TotalBytes       int64                   `json:"total_bytes"`
	StartTime        time.Time               `json:"start_time"`
	EndTime          *time.Time              `json:"end_time,omitempty"`
	Error            string                  `json:"error,omitempty"`
	Result           *providers.UploadResult `json:"result,omitempty"`
	ContentType      string                  `json:"content_type"`
	OriginalFilename string                  `json:"original_filename,omitempty"`

	// Internal fields
	ctx          context.Context
	cancel       context.CancelFunc
	progressChan chan UploadProgress
	resultChan   chan *UploadResult
	mu           sync.RWMutex
}

// UploadProgress represents upload progress information
type UploadProgress struct {
	UploadID         string    `json:"upload_id"`
	BytesTransferred int64     `json:"bytes_transferred"`
	TotalBytes       int64     `json:"total_bytes"`
	Progress         float64   `json:"progress"`
	Timestamp        time.Time `json:"timestamp"`
}

// UploadResult represents the final upload result
type UploadResult struct {
	UploadID string                  `json:"upload_id"`
	Success  bool                    `json:"success"`
	Result   *providers.UploadResult `json:"result,omitempty"`
	Error    error                   `json:"error,omitempty"`
}

// UploadManager manages concurrent uploads and tracks their progress
type UploadManager struct {
	s3Service      *S3Service
	uploads        map[string]*UploadInfo
	maxConcurrent  int
	currentUploads int
	mu             sync.RWMutex
	cleanupTicker  *time.Ticker
	stopCleanup    chan bool
}

// NewUploadManager creates a new upload manager
func NewUploadManager(s3Service *S3Service, maxConcurrent int) *UploadManager {
	if maxConcurrent <= 0 {
		maxConcurrent = 3 // Default
	}

	manager := &UploadManager{
		s3Service:     s3Service,
		uploads:       make(map[string]*UploadInfo),
		maxConcurrent: maxConcurrent,
		stopCleanup:   make(chan bool),
	}

	// Start cleanup routine for completed uploads
	manager.startCleanupRoutine()

	return manager
}

// StartUpload initiates a new upload
func (um *UploadManager) StartUpload(ctx context.Context, key string, reader io.Reader, size int64, opts providers.UploadOptions) (*UploadInfo, error) {
	// Check if we're at capacity
	um.mu.Lock()
	if um.currentUploads >= um.maxConcurrent {
		um.mu.Unlock()
		return nil, fmt.Errorf("maximum concurrent uploads reached (%d)", um.maxConcurrent)
	}
	um.currentUploads++
	um.mu.Unlock()

	// Create upload info
	uploadID := uuid.New().String()
	uploadCtx, cancel := context.WithCancel(ctx)

	uploadInfo := &UploadInfo{
		ID:               uploadID,
		Key:              key,
		Status:           UploadStatusPending,
		Progress:         0.0,
		BytesTransferred: 0,
		TotalBytes:       size,
		StartTime:        time.Now(),
		ContentType:      opts.ContentType,
		ctx:              uploadCtx,
		cancel:           cancel,
		progressChan:     make(chan UploadProgress, 10),
		resultChan:       make(chan *UploadResult, 1),
	}

	// Store upload info
	um.mu.Lock()
	um.uploads[uploadID] = uploadInfo
	um.mu.Unlock()

	// Start upload in goroutine
	go um.performUpload(uploadInfo, reader, opts)

	return uploadInfo, nil
}

// StartBase64Upload initiates a new base64 upload
func (um *UploadManager) StartBase64Upload(ctx context.Context, key string, base64Data string, opts providers.UploadOptions) (*UploadInfo, error) {
	// Check if we're at capacity
	um.mu.Lock()
	if um.currentUploads >= um.maxConcurrent {
		um.mu.Unlock()
		return nil, fmt.Errorf("maximum concurrent uploads reached (%d)", um.maxConcurrent)
	}
	um.currentUploads++
	um.mu.Unlock()

	// Create upload info
	uploadID := uuid.New().String()
	uploadCtx, cancel := context.WithCancel(ctx)

	uploadInfo := &UploadInfo{
		ID:               uploadID,
		Key:              key,
		Status:           UploadStatusPending,
		Progress:         0.0,
		BytesTransferred: 0,
		TotalBytes:       int64(len(base64Data)), // Approximate
		StartTime:        time.Now(),
		ContentType:      opts.ContentType,
		ctx:              uploadCtx,
		cancel:           cancel,
		progressChan:     make(chan UploadProgress, 10),
		resultChan:       make(chan *UploadResult, 1),
	}

	// Store upload info
	um.mu.Lock()
	um.uploads[uploadID] = uploadInfo
	um.mu.Unlock()

	// Start upload in goroutine
	go um.performBase64Upload(uploadInfo, base64Data, opts)

	return uploadInfo, nil
}

// GetUploadStatus returns the status of an upload
func (um *UploadManager) GetUploadStatus(uploadID string) (*UploadInfo, error) {
	um.mu.RLock()
	defer um.mu.RUnlock()

	uploadInfo, exists := um.uploads[uploadID]
	if !exists {
		return nil, fmt.Errorf("upload not found: %s", uploadID)
	}

	// Return a copy to avoid race conditions
	uploadInfo.mu.RLock()
	defer uploadInfo.mu.RUnlock()

	return &UploadInfo{
		ID:               uploadInfo.ID,
		Key:              uploadInfo.Key,
		Status:           uploadInfo.Status,
		Progress:         uploadInfo.Progress,
		BytesTransferred: uploadInfo.BytesTransferred,
		TotalBytes:       uploadInfo.TotalBytes,
		StartTime:        uploadInfo.StartTime,
		EndTime:          uploadInfo.EndTime,
		Error:            uploadInfo.Error,
		Result:           uploadInfo.Result,
		ContentType:      uploadInfo.ContentType,
		OriginalFilename: uploadInfo.OriginalFilename,
	}, nil
}

// CancelUpload cancels an ongoing upload
func (um *UploadManager) CancelUpload(uploadID string) error {
	um.mu.RLock()
	uploadInfo, exists := um.uploads[uploadID]
	um.mu.RUnlock()

	if !exists {
		return fmt.Errorf("upload not found: %s", uploadID)
	}

	uploadInfo.mu.Lock()
	defer uploadInfo.mu.Unlock()

	if uploadInfo.Status == UploadStatusCompleted || uploadInfo.Status == UploadStatusFailed {
		return fmt.Errorf("cannot cancel upload in status: %s", uploadInfo.Status)
	}

	uploadInfo.cancel()
	uploadInfo.Status = UploadStatusCancelled
	now := time.Now()
	uploadInfo.EndTime = &now

	// Decrement current uploads counter
	um.mu.Lock()
	um.currentUploads--
	um.mu.Unlock()

	return nil
}

// ListUploads returns all uploads (optionally filtered by status)
func (um *UploadManager) ListUploads(status ...UploadStatus) []*UploadInfo {
	um.mu.RLock()
	defer um.mu.RUnlock()

	var result []*UploadInfo

	for _, uploadInfo := range um.uploads {
		uploadInfo.mu.RLock()

		// Filter by status if provided
		if len(status) > 0 {
			matched := false
			for _, s := range status {
				if uploadInfo.Status == s {
					matched = true
					break
				}
			}
			if !matched {
				uploadInfo.mu.RUnlock()
				continue
			}
		}

		// Create a copy
		result = append(result, &UploadInfo{
			ID:               uploadInfo.ID,
			Key:              uploadInfo.Key,
			Status:           uploadInfo.Status,
			Progress:         uploadInfo.Progress,
			BytesTransferred: uploadInfo.BytesTransferred,
			TotalBytes:       uploadInfo.TotalBytes,
			StartTime:        uploadInfo.StartTime,
			EndTime:          uploadInfo.EndTime,
			Error:            uploadInfo.Error,
			Result:           uploadInfo.Result,
			ContentType:      uploadInfo.ContentType,
			OriginalFilename: uploadInfo.OriginalFilename,
		})

		uploadInfo.mu.RUnlock()
	}

	return result
}

// GetStats returns upload manager statistics
func (um *UploadManager) GetStats() map[string]interface{} {
	um.mu.RLock()
	defer um.mu.RUnlock()

	statusCounts := make(map[UploadStatus]int)
	totalUploads := len(um.uploads)

	for _, uploadInfo := range um.uploads {
		uploadInfo.mu.RLock()
		statusCounts[uploadInfo.Status]++
		uploadInfo.mu.RUnlock()
	}

	return map[string]interface{}{
		"total_uploads":   totalUploads,
		"current_uploads": um.currentUploads,
		"max_concurrent":  um.maxConcurrent,
		"status_counts":   statusCounts,
		"capacity_used":   float64(um.currentUploads) / float64(um.maxConcurrent) * 100,
	}
}

// performUpload performs the actual upload
func (um *UploadManager) performUpload(uploadInfo *UploadInfo, reader io.Reader, opts providers.UploadOptions) {
	defer func() {
		um.mu.Lock()
		um.currentUploads--
		um.mu.Unlock()
	}()

	// Update status to uploading
	uploadInfo.mu.Lock()
	uploadInfo.Status = UploadStatusUploading
	uploadInfo.mu.Unlock()

	// Wrap reader to track progress locally
	readerWithProgress := um.wrapWithProgress(reader, uploadInfo, opts)

	// Ensure providers don't attempt to use external callbacks
	opts.ProgressCallback = nil

	// Perform upload
	result, err := um.s3Service.provider.Upload(uploadInfo.ctx, uploadInfo.Key, readerWithProgress, uploadInfo.TotalBytes, opts)

	// Update final status
	uploadInfo.mu.Lock()
	now := time.Now()
	uploadInfo.EndTime = &now

	if err != nil {
		uploadInfo.Status = UploadStatusFailed
		uploadInfo.Error = err.Error()
	} else {
		uploadInfo.Status = UploadStatusCompleted
		uploadInfo.Result = result
		uploadInfo.Progress = 100.0
		uploadInfo.BytesTransferred = result.Size
	}
	uploadInfo.mu.Unlock()

	// Send final result
	select {
	case uploadInfo.resultChan <- &UploadResult{
		UploadID: uploadInfo.ID,
		Success:  err == nil,
		Result:   result,
		Error:    err,
	}:
	default:
		// Channel is full
	}
}

func (um *UploadManager) wrapWithProgress(reader io.Reader, uploadInfo *UploadInfo, opts providers.UploadOptions) io.Reader {
	progressFn := func(bytesTransferred, totalBytes int64) {
		total := totalBytes
		if total <= 0 {
			total = uploadInfo.TotalBytes
		}
		var progress float64
		if total > 0 {
			progress = float64(bytesTransferred) / float64(total) * 100
		}

		uploadInfo.mu.Lock()
		uploadInfo.BytesTransferred = bytesTransferred
		if total > 0 {
			uploadInfo.TotalBytes = total
		}
		uploadInfo.Progress = progress
		uploadInfo.mu.Unlock()

		select {
		case uploadInfo.progressChan <- UploadProgress{
			UploadID:         uploadInfo.ID,
			BytesTransferred: bytesTransferred,
			TotalBytes:       uploadInfo.TotalBytes,
			Progress:         progress,
			Timestamp:        time.Now(),
		}:
		default:
		}
	}

	if uploadInfo.TotalBytes > 0 {
		progressFn(0, uploadInfo.TotalBytes)
	}

	if seeker, ok := reader.(io.ReadSeeker); ok {
		return &progressReadSeeker{
			reader:   seeker,
			total:    uploadInfo.TotalBytes,
			callback: progressFn,
		}
	}

	return &progressReader{
		reader:   reader,
		total:    uploadInfo.TotalBytes,
		callback: progressFn,
	}
}

type progressReader struct {
	reader   io.Reader
	total    int64
	read     int64
	callback func(bytesTransferred, totalBytes int64)
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	if n > 0 && pr.callback != nil {
		pr.read += int64(n)
		pr.callback(pr.read, pr.total)
	}
	return n, err
}

type progressReadSeeker struct {
	reader   io.ReadSeeker
	total    int64
	read     int64
	callback func(bytesTransferred, totalBytes int64)
}

func (prs *progressReadSeeker) Read(p []byte) (int, error) {
	n, err := prs.reader.Read(p)
	if n > 0 && prs.callback != nil {
		prs.read += int64(n)
		prs.callback(prs.read, prs.total)
	}
	return n, err
}

func (prs *progressReadSeeker) Seek(offset int64, whence int) (int64, error) {
	pos, err := prs.reader.Seek(offset, whence)
	if err != nil {
		return pos, err
	}

	switch whence {
	case io.SeekStart:
		prs.read = pos
	case io.SeekCurrent:
		prs.read += offset
	case io.SeekEnd:
		prs.read = pos
	}

	if prs.callback != nil {
		prs.callback(prs.read, prs.total)
	}

	return pos, nil
}

// performBase64Upload performs the actual base64 upload
func (um *UploadManager) performBase64Upload(uploadInfo *UploadInfo, base64Data string, opts providers.UploadOptions) {
	defer func() {
		um.mu.Lock()
		um.currentUploads--
		um.mu.Unlock()
	}()

	// Update status to uploading
	uploadInfo.mu.Lock()
	uploadInfo.Status = UploadStatusUploading
	uploadInfo.mu.Unlock()

	// Perform upload
	result, err := um.s3Service.provider.UploadBase64(uploadInfo.ctx, uploadInfo.Key, base64Data, opts)

	// Update final status
	uploadInfo.mu.Lock()
	now := time.Now()
	uploadInfo.EndTime = &now

	if err != nil {
		uploadInfo.Status = UploadStatusFailed
		uploadInfo.Error = err.Error()
	} else {
		uploadInfo.Status = UploadStatusCompleted
		uploadInfo.Result = result
		uploadInfo.Progress = 100.0
		uploadInfo.BytesTransferred = result.Size
		uploadInfo.TotalBytes = result.Size
	}
	uploadInfo.mu.Unlock()

	// Send final result
	select {
	case uploadInfo.resultChan <- &UploadResult{
		UploadID: uploadInfo.ID,
		Success:  err == nil,
		Result:   result,
		Error:    err,
	}:
	default:
		// Channel is full
	}
}

// startCleanupRoutine starts a routine to clean up old completed uploads
func (um *UploadManager) startCleanupRoutine() {
	um.cleanupTicker = time.NewTicker(time.Hour) // Clean up every hour

	go func() {
		for {
			select {
			case <-um.cleanupTicker.C:
				um.cleanupOldUploads()
			case <-um.stopCleanup:
				um.cleanupTicker.Stop()
				return
			}
		}
	}()
}

// cleanupOldUploads removes upload records older than 24 hours
func (um *UploadManager) cleanupOldUploads() {
	um.mu.Lock()
	defer um.mu.Unlock()

	cutoff := time.Now().Add(-24 * time.Hour)
	var toDelete []string

	for id, uploadInfo := range um.uploads {
		uploadInfo.mu.RLock()
		if uploadInfo.EndTime != nil && uploadInfo.EndTime.Before(cutoff) {
			toDelete = append(toDelete, id)
		}
		uploadInfo.mu.RUnlock()
	}

	for _, id := range toDelete {
		delete(um.uploads, id)
	}

	if len(toDelete) > 0 {
		fmt.Printf("🧹 Cleaned up %d old upload records\n", len(toDelete))
	}
}

// Stop stops the upload manager and cleanup routines
func (um *UploadManager) Stop() {
	close(um.stopCleanup)

	// Cancel all ongoing uploads
	um.mu.RLock()
	for _, uploadInfo := range um.uploads {
		uploadInfo.mu.RLock()
		if uploadInfo.Status == UploadStatusUploading || uploadInfo.Status == UploadStatusPending {
			uploadInfo.cancel()
		}
		uploadInfo.mu.RUnlock()
	}
	um.mu.RUnlock()
}
