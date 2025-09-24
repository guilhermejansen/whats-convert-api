// WhatsApp Converter Module
// Handles conversion of audio and image files using the existing API

class WhatsAppConverter {
    constructor() {
        this.files = [];
        this.converting = false;
        this.init();
    }

    init() {
        this.setupEventListeners();
        this.updateUI();
    }

    // ================================
    // EVENT LISTENERS SETUP
    // ================================

    setupEventListeners() {
        // File input
        const fileInput = document.getElementById('whatsapp-file-input');
        const dropZone = document.getElementById('whatsapp-drop-zone');
        const uploadArea = document.getElementById('whatsapp-upload-area');

        // File selection
        fileInput.addEventListener('change', (e) => {
            this.handleFiles(Array.from(e.target.files));
        });

        // Click to select files
        dropZone.addEventListener('click', () => {
            fileInput.click();
        });

        // Drag and drop
        MediaConverter.setupDragAndDrop(uploadArea, {
            onDrop: (files) => this.handleFiles(files)
        });

        // Action buttons
        document.getElementById('whatsapp-convert-btn').addEventListener('click', () => {
            this.convertFiles();
        });

        document.getElementById('whatsapp-clear-btn').addEventListener('click', () => {
            this.clearFiles();
        });
    }

    // ================================
    // FILE HANDLING
    // ================================

    handleFiles(fileList) {
        const validFiles = [];

        for (const file of fileList) {
            try {
                // Validate file type (audio or image only)
                if (!MediaConverter.isAudioFile(file.name) && !MediaConverter.isImageFile(file.name)) {
                    throw new Error(`Only audio and image files are supported for WhatsApp conversion`);
                }

                // Basic validation (no size limit for WhatsApp conversion)
                MediaConverter.validateFile(file, {
                    allowedTypes: [
                        // Audio types
                        'audio/mpeg', 'audio/wav', 'audio/ogg', 'audio/opus', 'audio/webm', 'audio/weba', 'audio/mp4', 'audio/aac', 'audio/flac',
                        // Image types
                        'image/jpeg', 'image/jpg', 'image/png', 'image/gif', 'image/webp', 'image/bmp'
                    ]
                });

                validFiles.push({
                    id: MediaConverter.generateId(),
                    file: file,
                    name: file.name,
                    size: file.size,
                    type: MediaConverter.isAudioFile(file.name) ? 'audio' : 'image',
                    status: 'ready',
                    progress: 0
                });

            } catch (error) {
                MediaConverter.showToast('error', 'Invalid File', `${file.name}: ${error.message}`);
            }
        }

        if (validFiles.length > 0) {
            this.files.push(...validFiles);
            this.updateUI();
            MediaConverter.showToast('success', 'Files Added', `Added ${validFiles.length} file(s) for conversion`);
        }
    }

    // ================================
    // CONVERSION PROCESS
    // ================================

    async convertFiles() {
        if (this.converting || this.files.length === 0) return;

        this.converting = true;
        const convertBtn = document.getElementById('whatsapp-convert-btn');
        const originalText = convertBtn.textContent;

        convertBtn.disabled = true;
        convertBtn.textContent = 'Converting...';

        MediaConverter.updateStatus('Converting files for WhatsApp...', 'converting');

        let successCount = 0;
        let errorCount = 0;

        for (let i = 0; i < this.files.length; i++) {
            const fileInfo = this.files[i];

            if (fileInfo.status !== 'ready') continue;

            try {
                // Update file status
                fileInfo.status = 'converting';
                fileInfo.progress = 0;
                this.updateFileDisplay(fileInfo);

                // Determine API endpoint
                const endpoint = fileInfo.type === 'audio' ? '/convert/audio' : '/convert/image';

                // Create FormData
                const formData = new FormData();
                formData.append('file', fileInfo.file);

                // Add image-specific options
                if (fileInfo.type === 'image') {
                    formData.append('quality', '95');
                    formData.append('max_width', '1920');
                    formData.append('max_height', '1920');
                }

                // Make request with progress tracking
                const response = await this.makeConversionRequest(endpoint, formData, (progress) => {
                    fileInfo.progress = progress;
                    this.updateFileDisplay(fileInfo);
                });

                if (response.data) {
                    fileInfo.status = 'completed';
                    fileInfo.progress = 100;
                    fileInfo.result = response.data;
                    fileInfo.convertedSize = response.size;
                    fileInfo.duration = response.duration;

                    // Auto-download converted file
                    const extension = fileInfo.type === 'audio' ? '.ogg' : '.jpg';
                    const filename = fileInfo.name.replace(/\.[^/.]+$/, '') + '_whatsapp' + extension;

                    MediaConverter.downloadFromBase64(response.data, filename);
                    successCount++;

                } else {
                    throw new Error('No conversion data received');
                }

            } catch (error) {
                fileInfo.status = 'error';
                fileInfo.error = error.message;
                errorCount++;
                console.error(`Conversion failed for ${fileInfo.name}:`, error);
            }

            this.updateFileDisplay(fileInfo);
        }

        // Show final results
        if (successCount > 0) {
            MediaConverter.showToast('success', 'Conversion Complete',
                `Successfully converted ${successCount} file(s) for WhatsApp`);
        }

        if (errorCount > 0) {
            MediaConverter.showToast('error', 'Conversion Errors',
                `Failed to convert ${errorCount} file(s)`);
        }

        // Reset UI
        this.converting = false;
        convertBtn.disabled = false;
        convertBtn.textContent = originalText;
        MediaConverter.updateStatus('Conversion complete', 'success');

        this.updateUI();
    }

    async makeConversionRequest(endpoint, formData, progressCallback) {
        return new Promise((resolve, reject) => {
            const xhr = new XMLHttpRequest();

            // Setup progress tracking
            xhr.upload.addEventListener('progress', (e) => {
                if (e.lengthComputable) {
                    const progress = (e.loaded / e.total) * 50; // Upload is 50% of total
                    progressCallback(progress);
                }
            });

            // Setup download progress
            xhr.addEventListener('progress', (e) => {
                if (e.lengthComputable) {
                    const progress = 50 + (e.loaded / e.total) * 50; // Download is remaining 50%
                    progressCallback(progress);
                }
            });

            xhr.addEventListener('load', () => {
                if (xhr.status >= 200 && xhr.status < 300) {
                    try {
                        const response = JSON.parse(xhr.responseText);
                        progressCallback(100);
                        resolve(response);
                    } catch (error) {
                        reject(new Error('Invalid response format'));
                    }
                } else {
                    reject(new Error(`HTTP ${xhr.status}: ${xhr.statusText}`));
                }
            });

            xhr.addEventListener('error', () => {
                reject(new Error('Network error during conversion'));
            });

            xhr.addEventListener('timeout', () => {
                reject(new Error('Conversion timed out'));
            });

            // Set timeout (5 minutes)
            xhr.timeout = 300000;

            // Send request
            xhr.open('POST', endpoint);
            xhr.send(formData);
        });
    }

    // ================================
    // UI MANAGEMENT
    // ================================

    updateUI() {
        this.updateFileList();
        this.updateButtons();
    }

    updateFileList() {
        const fileList = document.getElementById('whatsapp-file-list');

        if (this.files.length === 0) {
            fileList.innerHTML = '';
            return;
        }

        fileList.innerHTML = this.files.map(fileInfo => {
            const statusClass = fileInfo.status;
            const statusText = this.getStatusText(fileInfo);
            const progressBar = fileInfo.status === 'converting' ?
                `<div class="progress-bar">
                    <div class="progress-fill" style="width: ${fileInfo.progress}%"></div>
                 </div>` : '';

            const resultSnippet = (fileInfo.status === 'completed' && fileInfo.result)
                ? `<div class="file-result-snippet">${MediaConverter.escapeHtml(this.getResultSnippet(fileInfo.result))}</div>`
                : '';

            const actionButtons = [];

            if ((fileInfo.status === 'ready' || fileInfo.status === 'error')) {
                actionButtons.push(`<button class="btn btn-small btn-secondary" onclick="whatsappConverter.removeFile('${fileInfo.id}')">Remove</button>`);
            }

            if (fileInfo.status === 'completed' && fileInfo.result) {
                actionButtons.push(`<button class="btn btn-small btn-primary" onclick="whatsappConverter.copyResult('${fileInfo.id}')">Copy Data URI</button>`);
            }

            const actionsHtml = actionButtons.join('');

            return `
                <div class="file-item ${statusClass}" data-file-id="${fileInfo.id}">
                    <div class="file-info">
                        <div class="file-icon">${MediaConverter.getFileIcon(fileInfo.name)}</div>
                        <div class="file-details">
                            <div class="file-name">${MediaConverter.escapeHtml(fileInfo.name)}</div>
                            <div class="file-size">
                                ${MediaConverter.formatFileSize(fileInfo.size)} • ${fileInfo.type}
                                ${fileInfo.convertedSize ? ` → ${MediaConverter.formatFileSize(fileInfo.convertedSize)}` : ''}
                            </div>
                            ${resultSnippet}
                            ${progressBar}
                        </div>
                    </div>
                    <div class="file-actions">
                        <span class="upload-status ${statusClass}">${statusText}</span>
                        ${actionsHtml}
                    </div>
                </div>
            `;
        }).join('');
    }

    updateFileDisplay(fileInfo) {
        const fileElement = document.querySelector(`[data-file-id="${fileInfo.id}"]`);
        if (fileElement) {
            const statusElement = fileElement.querySelector('.upload-status');
            const progressBar = fileElement.querySelector('.progress-fill');

            if (statusElement) {
                statusElement.textContent = this.getStatusText(fileInfo);
                statusElement.className = `upload-status ${fileInfo.status}`;
            }

            if (progressBar && fileInfo.status === 'converting') {
                progressBar.style.width = `${fileInfo.progress}%`;
            }
        }
    }

    updateButtons() {
        const convertBtn = document.getElementById('whatsapp-convert-btn');
        const clearBtn = document.getElementById('whatsapp-clear-btn');

        const readyFiles = this.files.filter(f => f.status === 'ready').length;

        convertBtn.disabled = readyFiles === 0 || this.converting;
        clearBtn.disabled = this.converting;

        // Update button text with count
        if (readyFiles > 0) {
            convertBtn.textContent = `Convert ${readyFiles} file(s) for WhatsApp`;
        } else {
            convertBtn.textContent = 'Convert for WhatsApp';
        }
    }

    getStatusText(fileInfo) {
        switch (fileInfo.status) {
            case 'ready':
                return 'Ready';
            case 'converting':
                return `${Math.round(fileInfo.progress)}%`;
            case 'completed':
                return '✅ Converted';
            case 'error':
                return '❌ Failed';
            default:
                return 'Unknown';
        }
    }

    // ================================
    // FILE MANAGEMENT
    // ================================

    removeFile(fileId) {
        this.files = this.files.filter(f => f.id !== fileId);
        this.updateUI();
        MediaConverter.showToast('info', 'File Removed', 'File removed from conversion queue');
    }

    clearFiles() {
        if (this.converting) {
            MediaConverter.showToast('warning', 'Cannot Clear', 'Cannot clear files during conversion');
            return;
        }

        this.files = [];
        this.updateUI();
        MediaConverter.showToast('info', 'Files Cleared', 'All files removed from queue');
    }

    // ================================
    // PUBLIC API
    // ================================

    getStats() {
        const total = this.files.length;
        const ready = this.files.filter(f => f.status === 'ready').length;
        const converting = this.files.filter(f => f.status === 'converting').length;
        const completed = this.files.filter(f => f.status === 'completed').length;
        const failed = this.files.filter(f => f.status === 'error').length;

        return {
            total,
            ready,
            converting,
            completed,
            failed,
            isConverting: this.converting
        };
    }

    getResultSnippet(data) {
        if (!data) return '';

        const maxLength = 80;
        const trimmed = data.trim();

        if (trimmed.length <= maxLength) {
            return trimmed;
        }

        return `${trimmed.slice(0, maxLength)}...`;
    }

    async copyResult(fileId) {
        const fileInfo = this.files.find(f => f.id === fileId);

        if (!fileInfo || !fileInfo.result) {
            MediaConverter.showToast('error', 'Copy Failed', 'Converted data is not available for this file');
            return;
        }

        const success = await MediaConverter.copyToClipboard(fileInfo.result);

        if (success) {
            MediaConverter.showToast('success', 'Copied', `${fileInfo.name} data URI copied to clipboard`);
        } else {
            MediaConverter.showToast('error', 'Copy Failed', 'Unable to copy data URI to clipboard');
        }
    }
}

// ================================
// INITIALIZATION
// ================================

let whatsappConverter;

// Initialize when DOM is ready
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => {
        whatsappConverter = new WhatsAppConverter();
    });
} else {
    whatsappConverter = new WhatsAppConverter();
}
