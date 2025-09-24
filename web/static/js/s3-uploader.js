// S3 Uploader Module
// Handles file uploads to S3 storage with progress tracking

class S3Uploader {
    constructor() {
        this.files = [];
        this.uploads = new Map(); // Track active uploads
        this.polling = new Map(); // Track polling intervals
        this.s3Available = false;
        this.init();
    }

    init() {
        this.setupEventListeners();
        this.checkS3Status();
        this.updateUI();
    }

    // ================================
    // EVENT LISTENERS SETUP
    // ================================

    setupEventListeners() {
        // File input
        const fileInput = document.getElementById('s3-file-input');
        const dropZone = document.getElementById('s3-drop-zone');
        const uploadArea = document.getElementById('s3-upload-area');

        // File selection
        fileInput.addEventListener('change', (e) => {
            this.handleFiles(Array.from(e.target.files));
        });

        // Click to select files
        dropZone.addEventListener('click', () => {
            if (this.s3Available) {
                fileInput.click();
            } else {
                MediaConverter.showToast('error', 'S3 Unavailable', 'S3 upload service is not configured');
            }
        });

        // Drag and drop
        MediaConverter.setupDragAndDrop(uploadArea, {
            onDrop: (files) => {
                if (this.s3Available) {
                    this.handleFiles(files);
                } else {
                    MediaConverter.showToast('error', 'S3 Unavailable', 'S3 upload service is not configured');
                }
            }
        });

        // Action buttons
        document.getElementById('s3-upload-btn').addEventListener('click', () => {
            this.uploadFiles();
        });

        document.getElementById('s3-clear-btn').addEventListener('click', () => {
            this.clearFiles();
        });
    }

    // ================================
    // S3 STATUS CHECK
    // ================================

    async checkS3Status() {
        try {
            const health = await MediaConverter.apiRequest('/upload/s3/health');

            if (health.healthy) {
                this.s3Available = true;
                document.getElementById('s3-indicator').textContent = '🟢';
                document.getElementById('s3-status-text').textContent = 'S3 Connected & Ready';

                // Enable upload area
                document.getElementById('s3-upload-area').style.opacity = '1';
                document.getElementById('s3-options').style.opacity = '1';
            } else {
                throw new Error(health.message || 'S3 service not healthy');
            }

        } catch (error) {
            this.s3Available = false;
            document.getElementById('s3-indicator').textContent = '🔴';
            document.getElementById('s3-status-text').textContent = 'S3 Not Available';

            // Disable upload area
            document.getElementById('s3-upload-area').style.opacity = '0.5';
            document.getElementById('s3-options').style.opacity = '0.5';

            console.warn('S3 not available:', error.message);
        }
    }

    // ================================
    // FILE HANDLING
    // ================================

    handleFiles(fileList) {
        if (!this.s3Available) {
            MediaConverter.showToast('error', 'S3 Unavailable', 'S3 upload service is not configured');
            return;
        }

        const validFiles = [];

        for (const file of fileList) {
            try {
                // Basic validation (no restrictions per requirements)
                validFiles.push({
                    id: MediaConverter.generateId(),
                    file: file,
                    name: file.name,
                    size: file.size,
                    type: MediaConverter.detectContentType(file.name),
                    status: 'ready',
                    progress: 0,
                    uploadId: null,
                    publicUrl: null,
                    error: null
                });

            } catch (error) {
                MediaConverter.showToast('error', 'Invalid File', `${file.name}: ${error.message}`);
            }
        }

        if (validFiles.length > 0) {
            this.files.push(...validFiles);
            this.updateUI();
            MediaConverter.showToast('success', 'Files Added',
                `Added ${validFiles.length} file(s) for S3 upload`);
        }
    }

    // ================================
    // UPLOAD PROCESS
    // ================================

    async uploadFiles() {
        if (!this.s3Available || this.files.length === 0) return;

        const readyFiles = this.files.filter(f => f.status === 'ready');
        if (readyFiles.length === 0) {
            MediaConverter.showToast('warning', 'No Files Ready', 'No files available for upload');
            return;
        }

        const uploadBtn = document.getElementById('s3-upload-btn');
        const originalText = uploadBtn.textContent;

        uploadBtn.disabled = true;
        uploadBtn.textContent = 'Uploading...';

        MediaConverter.updateStatus(`Uploading ${readyFiles.length} file(s) to S3...`, 'uploading');

        // Get upload options
        const options = this.getUploadOptions();

        let successCount = 0;
        let errorCount = 0;

        // Process uploads (respecting concurrent limits)
        for (const fileInfo of readyFiles) {
            try {
                await this.uploadSingleFile(fileInfo, options);
                successCount++;
            } catch (error) {
                fileInfo.status = 'failed';
                fileInfo.error = error.message;
                errorCount++;
                this.updateFileDisplay(fileInfo);
            }
        }

        // Show final results
        if (successCount > 0) {
            MediaConverter.showToast('success', 'Upload Complete',
                `Successfully uploaded ${successCount} file(s) to S3`);
        }

        if (errorCount > 0) {
            MediaConverter.showToast('error', 'Upload Errors',
                `Failed to upload ${errorCount} file(s)`);
        }

        // Reset UI
        uploadBtn.disabled = false;
        uploadBtn.textContent = originalText;
        MediaConverter.updateStatus('S3 uploads complete', 'success');

        this.updateUI();
    }

    async uploadSingleFile(fileInfo, options) {
        try {
            fileInfo.status = 'uploading';
            fileInfo.progress = 0;
            this.updateFileDisplay(fileInfo);

            // Create FormData
            const formData = new FormData();
            formData.append('file', fileInfo.file);
            formData.append('options', JSON.stringify({
                public: options.public,
                expires_days: options.expirationDays
            }));

            // Start upload
            const uploadResponse = await MediaConverter.apiRequest('/upload/s3', {
                method: 'POST',
                body: formData,
                headers: {} // Remove Content-Type to let browser set it for FormData
            });

            if (uploadResponse.success && uploadResponse.upload_id) {
                fileInfo.uploadId = uploadResponse.upload_id;

                // Start polling for progress
                await this.pollUploadProgress(fileInfo);
            } else {
                throw new Error(uploadResponse.error || 'Upload failed');
            }

        } catch (error) {
            console.error(`S3 upload failed for ${fileInfo.name}:`, error);
            throw error;
        }
    }

    async pollUploadProgress(fileInfo) {
        return new Promise((resolve, reject) => {
            const pollInterval = setInterval(async () => {
                try {
                    const status = await MediaConverter.apiRequest(`/upload/s3/status/${fileInfo.uploadId}`);

                    fileInfo.progress = status.progress || 0;
                    this.updateFileDisplay(fileInfo);

                    switch (status.status) {
                        case 'completed':
                            fileInfo.status = 'completed';
                            fileInfo.progress = 100;
                            fileInfo.publicUrl = status.result?.url;
                            clearInterval(pollInterval);
                            this.polling.delete(fileInfo.id);
                            this.updateFileDisplay(fileInfo);
                            resolve();
                            break;

                        case 'failed':
                            fileInfo.status = 'failed';
                            fileInfo.error = status.error || 'Upload failed';
                            clearInterval(pollInterval);
                            this.polling.delete(fileInfo.id);
                            this.updateFileDisplay(fileInfo);
                            reject(new Error(fileInfo.error));
                            break;

                        case 'uploading':
                            // Continue polling
                            break;

                        default:
                            // Handle unknown status
                            console.warn('Unknown upload status:', status.status);
                    }

                } catch (error) {
                    console.error('Polling failed for upload:', fileInfo.uploadId, error);
                    clearInterval(pollInterval);
                    this.polling.delete(fileInfo.id);
                    reject(error);
                }
            }, 1000); // Poll every second

            // Store polling reference
            this.polling.set(fileInfo.id, pollInterval);

            // Set timeout for very long uploads (1 hour)
            setTimeout(() => {
                if (this.polling.has(fileInfo.id)) {
                    clearInterval(pollInterval);
                    this.polling.delete(fileInfo.id);
                    reject(new Error('Upload timeout'));
                }
            }, 3600000); // 1 hour
        });
    }

    // ================================
    // UI MANAGEMENT
    // ================================

    getUploadOptions() {
        return {
            public: document.getElementById('s3-public').checked,
            expirationDays: parseInt(document.getElementById('s3-expiration').value) || 0
        };
    }

    updateUI() {
        this.updateFileList();
        this.updateButtons();
    }

    updateFileList() {
        const uploadList = document.getElementById('s3-upload-list');

        if (this.files.length === 0) {
            uploadList.innerHTML = '';
            return;
        }

        uploadList.innerHTML = this.files.map(fileInfo => {
            const statusClass = fileInfo.status;
            const progressBar = ['uploading'].includes(fileInfo.status) ?
                `<div class="progress-bar">
                    <div class="progress-fill" style="width: ${fileInfo.progress}%"></div>
                 </div>
                 <div class="progress-text">${Math.round(fileInfo.progress)}%</div>` : '';

            const urlSection = fileInfo.publicUrl ?
                `<div class="upload-url">
                    <input type="text" value="${fileInfo.publicUrl}" readonly>
                    <button class="btn btn-copy btn-small" onclick="s3Uploader.copyUrl('${fileInfo.id}')">Copy URL</button>
                    <button class="btn btn-secondary btn-small" onclick="window.open('${fileInfo.publicUrl}', '_blank')">Open</button>
                 </div>` : '';

            const errorSection = fileInfo.error ?
                `<div class="error-message" style="color: var(--error-color); font-size: var(--font-size-xs); margin-top: var(--spacing-xs);">
                    ${MediaConverter.escapeHtml(fileInfo.error)}
                 </div>` : '';

            return `
                <div class="upload-item ${statusClass}" data-upload-id="${fileInfo.id}">
                    <div class="upload-header">
                        <div class="upload-name">
                            ${MediaConverter.getFileIcon(fileInfo.name)} ${MediaConverter.escapeHtml(fileInfo.name)}
                            <span style="font-size: var(--font-size-xs); color: var(--text-secondary); margin-left: var(--spacing-xs);">
                                (${MediaConverter.formatFileSize(fileInfo.size)})
                            </span>
                        </div>
                        <span class="upload-status ${statusClass}">${this.getStatusText(fileInfo)}</span>
                    </div>
                    ${progressBar}
                    ${urlSection}
                    ${errorSection}
                    <div class="upload-actions">
                        ${fileInfo.status === 'ready' || fileInfo.status === 'failed' ?
                            `<button class="btn btn-small btn-secondary" onclick="s3Uploader.removeFile('${fileInfo.id}')">Remove</button>` : ''}
                        ${fileInfo.status === 'uploading' ?
                            `<button class="btn btn-small btn-secondary" onclick="s3Uploader.cancelUpload('${fileInfo.id}')">Cancel</button>` : ''}
                    </div>
                </div>
            `;
        }).join('');
    }

    updateFileDisplay(fileInfo) {
        const uploadElement = document.querySelector(`[data-upload-id="${fileInfo.id}"]`);
        if (uploadElement) {
            // Update status
            const statusElement = uploadElement.querySelector('.upload-status');
            if (statusElement) {
                statusElement.textContent = this.getStatusText(fileInfo);
                statusElement.className = `upload-status ${fileInfo.status}`;
            }

            // Update progress
            const progressFill = uploadElement.querySelector('.progress-fill');
            const progressText = uploadElement.querySelector('.progress-text');

            if (progressFill) {
                progressFill.style.width = `${fileInfo.progress}%`;
            }
            if (progressText) {
                progressText.textContent = `${Math.round(fileInfo.progress)}%`;
            }

            // Add URL section when completed
            if (fileInfo.status === 'completed' && fileInfo.publicUrl) {
                const existingUrl = uploadElement.querySelector('.upload-url');
                if (!existingUrl) {
                    const urlSection = document.createElement('div');
                    urlSection.className = 'upload-url';
                    urlSection.innerHTML = `
                        <input type="text" value="${fileInfo.publicUrl}" readonly>
                        <button class="btn btn-copy btn-small" onclick="s3Uploader.copyUrl('${fileInfo.id}')">Copy URL</button>
                        <button class="btn btn-secondary btn-small" onclick="window.open('${fileInfo.publicUrl}', '_blank')">Open</button>
                    `;
                    uploadElement.appendChild(urlSection);
                }
            }

            // Add error section if failed
            if (fileInfo.status === 'failed' && fileInfo.error) {
                const existingError = uploadElement.querySelector('.error-message');
                if (!existingError) {
                    const errorSection = document.createElement('div');
                    errorSection.className = 'error-message';
                    errorSection.style.cssText = 'color: var(--error-color); font-size: var(--font-size-xs); margin-top: var(--spacing-xs);';
                    errorSection.textContent = fileInfo.error;
                    uploadElement.appendChild(errorSection);
                }
            }
        }
    }

    updateButtons() {
        const uploadBtn = document.getElementById('s3-upload-btn');
        const clearBtn = document.getElementById('s3-clear-btn');

        const readyFiles = this.files.filter(f => f.status === 'ready').length;
        const uploadingFiles = this.files.filter(f => f.status === 'uploading').length;

        uploadBtn.disabled = !this.s3Available || readyFiles === 0 || uploadingFiles > 0;
        clearBtn.disabled = uploadingFiles > 0;

        // Update button text with count
        if (readyFiles > 0) {
            uploadBtn.textContent = `Upload ${readyFiles} file(s) to S3`;
        } else if (uploadingFiles > 0) {
            uploadBtn.textContent = `Uploading ${uploadingFiles} file(s)...`;
        } else {
            uploadBtn.textContent = 'Upload to S3';
        }
    }

    // ================================
    // UPLOAD MANAGEMENT
    // ================================

    async uploadSingleFile(fileInfo, options) {
        try {
            fileInfo.status = 'uploading';
            fileInfo.progress = 0;
            this.updateFileDisplay(fileInfo);

            // Create FormData
            const formData = new FormData();
            formData.append('file', fileInfo.file);
            formData.append('options', JSON.stringify({
                public: options.public,
                expires_days: options.expirationDays
            }));

            // Start upload
            const uploadResponse = await MediaConverter.apiRequest('/upload/s3', {
                method: 'POST',
                body: formData,
                headers: {} // Remove Content-Type for FormData
            });

            if (uploadResponse.success && uploadResponse.upload_id) {
                fileInfo.uploadId = uploadResponse.upload_id;
                this.uploads.set(fileInfo.id, fileInfo);

                // Start polling for progress
                await this.pollUploadProgress(fileInfo);
            } else {
                throw new Error(uploadResponse.error || 'Upload failed');
            }

        } catch (error) {
            console.error(`S3 upload failed for ${fileInfo.name}:`, error);
            throw error;
        }
    }

    async pollUploadProgress(fileInfo) {
        return new Promise((resolve, reject) => {
            let attempts = 0;
            const maxAttempts = 3600; // 1 hour at 1 second intervals

            const pollInterval = setInterval(async () => {
                attempts++;

                try {
                    const status = await MediaConverter.apiRequest(`/upload/s3/status/${fileInfo.uploadId}`);

                    fileInfo.progress = status.progress || 0;
                    this.updateFileDisplay(fileInfo);

                    switch (status.status) {
                        case 'completed':
                            fileInfo.status = 'completed';
                            fileInfo.progress = 100;
                            fileInfo.publicUrl = status.result?.url;
                            clearInterval(pollInterval);
                            this.polling.delete(fileInfo.id);
                            this.uploads.delete(fileInfo.id);
                            this.updateFileDisplay(fileInfo);
                            resolve();
                            break;

                        case 'failed':
                            fileInfo.status = 'failed';
                            fileInfo.error = status.error || 'Upload failed';
                            clearInterval(pollInterval);
                            this.polling.delete(fileInfo.id);
                            this.uploads.delete(fileInfo.id);
                            this.updateFileDisplay(fileInfo);
                            reject(new Error(fileInfo.error));
                            break;

                        case 'uploading':
                        case 'pending':
                            // Continue polling
                            break;

                        default:
                            console.warn('Unknown upload status:', status.status);
                    }

                } catch (error) {
                    console.error('Polling failed for upload:', fileInfo.uploadId, error);

                    // Retry a few times before giving up
                    if (attempts < 5) {
                        return; // Continue polling
                    }

                    clearInterval(pollInterval);
                    this.polling.delete(fileInfo.id);
                    this.uploads.delete(fileInfo.id);
                    reject(error);
                }

                // Safety timeout
                if (attempts >= maxAttempts) {
                    clearInterval(pollInterval);
                    this.polling.delete(fileInfo.id);
                    this.uploads.delete(fileInfo.id);
                    reject(new Error('Upload timeout - operation took too long'));
                }

            }, 1000); // Poll every second

            // Store polling reference
            this.polling.set(fileInfo.id, pollInterval);
        });
    }

    // ================================
    // FILE MANAGEMENT
    // ================================

    async uploadFiles() {
        if (!this.s3Available) return;

        const readyFiles = this.files.filter(f => f.status === 'ready');
        if (readyFiles.length === 0) return;

        // Get options
        const options = this.getUploadOptions();

        // Upload files sequentially to avoid overwhelming the server
        for (const fileInfo of readyFiles) {
            try {
                await this.uploadSingleFile(fileInfo, options);
            } catch (error) {
                // Continue with other files even if one fails
                console.error('Upload failed:', error);
            }
        }

        this.updateUI();
    }

    removeFile(fileId) {
        const fileInfo = this.files.find(f => f.id === fileId);

        if (fileInfo && fileInfo.status === 'uploading') {
            MediaConverter.showToast('warning', 'Cannot Remove', 'Cannot remove file during upload. Cancel upload first.');
            return;
        }

        this.files = this.files.filter(f => f.id !== fileId);
        this.updateUI();
        MediaConverter.showToast('info', 'File Removed', 'File removed from upload queue');
    }

    async cancelUpload(fileId) {
        const fileInfo = this.files.find(f => f.id === fileId);

        if (!fileInfo || !fileInfo.uploadId) return;

        try {
            // Cancel upload via API
            await MediaConverter.apiRequest(`/upload/s3/status/${fileInfo.uploadId}`, {
                method: 'DELETE'
            });

            // Stop polling
            if (this.polling.has(fileId)) {
                clearInterval(this.polling.get(fileId));
                this.polling.delete(fileId);
            }

            // Update file status
            fileInfo.status = 'cancelled';
            fileInfo.error = 'Upload cancelled by user';
            this.uploads.delete(fileId);
            this.updateFileDisplay(fileInfo);

            MediaConverter.showToast('info', 'Upload Cancelled', `Cancelled upload of ${fileInfo.name}`);

        } catch (error) {
            console.error('Failed to cancel upload:', error);
            MediaConverter.showToast('error', 'Cancel Failed', 'Could not cancel upload');
        }

        this.updateUI();
    }

    clearFiles() {
        const uploadingFiles = this.files.filter(f => f.status === 'uploading');

        if (uploadingFiles.length > 0) {
            MediaConverter.showToast('warning', 'Cannot Clear', 'Cancel active uploads first');
            return;
        }

        // Clear polling intervals
        this.polling.forEach((interval) => clearInterval(interval));
        this.polling.clear();
        this.uploads.clear();

        this.files = [];
        this.updateUI();
        MediaConverter.showToast('info', 'Files Cleared', 'All files removed from queue');
    }

    async copyUrl(fileId) {
        const fileInfo = this.files.find(f => f.id === fileId);

        if (!fileInfo || !fileInfo.publicUrl) {
            MediaConverter.showToast('error', 'No URL', 'No public URL available for this file');
            return;
        }

        const success = await MediaConverter.copyToClipboard(fileInfo.publicUrl);

        if (success) {
            MediaConverter.showToast('success', 'URL Copied', 'Public URL copied to clipboard');
        } else {
            MediaConverter.showToast('error', 'Copy Failed', 'Could not copy URL to clipboard');
        }
    }

    // ================================
    // UTILITY FUNCTIONS
    // ================================

    getStatusText(fileInfo) {
        switch (fileInfo.status) {
            case 'ready':
                return 'Ready';
            case 'uploading':
                return `${Math.round(fileInfo.progress)}%`;
            case 'completed':
                return '✅ Uploaded';
            case 'failed':
                return '❌ Failed';
            case 'cancelled':
                return '🚫 Cancelled';
            default:
                return 'Unknown';
        }
    }

    // ================================
    // PUBLIC API
    // ================================

    getStats() {
        const total = this.files.length;
        const ready = this.files.filter(f => f.status === 'ready').length;
        const uploading = this.files.filter(f => f.status === 'uploading').length;
        const completed = this.files.filter(f => f.status === 'completed').length;
        const failed = this.files.filter(f => f.status === 'failed').length;

        return {
            total,
            ready,
            uploading,
            completed,
            failed,
            s3Available: this.s3Available,
            activeUploads: this.uploads.size
        };
    }
}

// ================================
// INITIALIZATION
// ================================

let s3Uploader;

// Initialize when DOM is ready
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => {
        s3Uploader = new S3Uploader();
    });
} else {
    s3Uploader = new S3Uploader();
}
