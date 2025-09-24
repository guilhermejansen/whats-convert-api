// Base64 Converter Module
// Handles client-side conversion of any file to Base64 format

class Base64Converter {
    constructor() {
        this.currentFile = null;
        this.converting = false;
        this.worker = null;
        this.init();
    }

    init() {
        this.setupEventListeners();
        this.setupWebWorker();
    }

    // ================================
    // EVENT LISTENERS SETUP
    // ================================

    setupEventListeners() {
        // File input
        const fileInput = document.getElementById('base64-file-input');
        const dropZone = document.getElementById('base64-drop-zone');
        const uploadArea = document.getElementById('base64-upload-area');

        // File selection
        fileInput.addEventListener('change', (e) => {
            if (e.target.files.length > 0) {
                this.handleFile(e.target.files[0]);
            }
        });

        // Click to select file
        dropZone.addEventListener('click', () => {
            fileInput.click();
        });

        // Drag and drop
        MediaConverter.setupDragAndDrop(uploadArea, {
            onDrop: (files) => {
                if (files.length > 0) {
                    this.handleFile(files[0]);
                }
            }
        });

        // Copy button
        document.getElementById('base64-copy-btn').addEventListener('click', () => {
            this.copyResult();
        });

        // Clear button
        document.getElementById('base64-clear-btn').addEventListener('click', () => {
            this.clearResult();
        });
    }

    // ================================
    // WEB WORKER SETUP
    // ================================

    setupWebWorker() {
        // Create inline Web Worker for large file processing
        const workerCode = `
            self.onmessage = function(e) {
                const { file, chunkSize } = e.data;

                try {
                    const reader = new FileReader();

                    reader.onload = function(event) {
                        const result = event.target.result;
                        self.postMessage({
                            type: 'success',
                            data: result,
                            fileName: file.name,
                            fileSize: file.size
                        });
                    };

                    reader.onerror = function(error) {
                        self.postMessage({
                            type: 'error',
                            error: 'Failed to read file: ' + error.message
                        });
                    };

                    reader.onprogress = function(event) {
                        if (event.lengthComputable) {
                            const progress = (event.loaded / event.total) * 100;
                            self.postMessage({
                                type: 'progress',
                                progress: progress
                            });
                        }
                    };

                    // Read file as data URL (base64)
                    reader.readAsDataURL(file);

                } catch (error) {
                    self.postMessage({
                        type: 'error',
                        error: error.message
                    });
                }
            };
        `;

        const blob = new Blob([workerCode], { type: 'application/javascript' });
        this.workerUrl = URL.createObjectURL(blob);
    }

    // ================================
    // FILE HANDLING
    // ================================

    handleFile(file) {
        if (this.converting) {
            MediaConverter.showToast('warning', 'Conversion in Progress', 'Please wait for current conversion to complete');
            return;
        }

        this.currentFile = file;
        this.convertToBase64();
    }

    async convertToBase64() {
        if (!this.currentFile) return;

        this.converting = true;
        const file = this.currentFile;

        // Show progress container
        const progressContainer = document.getElementById('base64-progress');
        const resultContainer = document.getElementById('base64-result');

        progressContainer.classList.remove('hidden');
        resultContainer.classList.add('hidden');

        MediaConverter.updateStatus(`Converting ${file.name} to Base64...`, 'converting');

        try {
            // For small files (<50MB), use direct FileReader
            if (file.size < 50 * 1024 * 1024) {
                await this.convertDirect(file);
            } else {
                // For large files, use Web Worker
                await this.convertWithWorker(file);
            }

        } catch (error) {
            console.error('Base64 conversion failed:', error);
            MediaConverter.showToast('error', 'Conversion Failed', error.message);
            this.hideProgress();
        }

        this.converting = false;
    }

    async convertDirect(file) {
        return new Promise((resolve, reject) => {
            const reader = new FileReader();

            reader.onload = (event) => {
                this.showResult(event.target.result, file);
                resolve();
            };

            reader.onerror = (error) => {
                reject(new Error('Failed to read file: ' + error.message));
            };

            reader.onprogress = (event) => {
                if (event.lengthComputable) {
                    const progress = (event.loaded / event.total) * 100;
                    MediaConverter.updateProgress('base64-progress-fill', progress);
                }
            };

            reader.readAsDataURL(file);
        });
    }

    async convertWithWorker(file) {
        return new Promise((resolve, reject) => {
            // Create new worker for this conversion
            const worker = new Worker(this.workerUrl);

            worker.onmessage = (e) => {
                const { type, data, progress, error } = e.data;

                switch (type) {
                    case 'progress':
                        MediaConverter.updateProgress('base64-progress-fill', progress);
                        break;

                    case 'success':
                        this.showResult(data, file);
                        worker.terminate();
                        resolve();
                        break;

                    case 'error':
                        worker.terminate();
                        reject(new Error(error));
                        break;
                }
            };

            worker.onerror = (error) => {
                worker.terminate();
                reject(new Error('Worker error: ' + error.message));
            };

            // Start conversion
            worker.postMessage({ file });
        });
    }

    // ================================
    // RESULT HANDLING
    // ================================

    showResult(base64Data, file) {
        const resultContainer = document.getElementById('base64-result');
        const outputTextarea = document.getElementById('base64-output');
        const progressContainer = document.getElementById('base64-progress');

        // Hide progress, show result
        progressContainer.classList.add('hidden');
        resultContainer.classList.remove('hidden');

        // Set result data
        outputTextarea.value = base64Data;

        // Show success message
        const originalSize = file.size;
        const base64Size = base64Data.length;
        const overhead = ((base64Size - originalSize) / originalSize * 100).toFixed(1);

        MediaConverter.showToast('success', 'Base64 Conversion Complete',
            `File: ${file.name}\nOriginal: ${MediaConverter.formatFileSize(originalSize)}\nBase64: ${MediaConverter.formatFileSize(base64Size)} (+${overhead}%)`);

        MediaConverter.updateStatus('Base64 conversion complete', 'success');
    }

    hideProgress() {
        const progressContainer = document.getElementById('base64-progress');
        progressContainer.classList.add('hidden');
    }

    async copyResult() {
        const outputTextarea = document.getElementById('base64-output');
        const copyBtn = document.getElementById('base64-copy-btn');

        if (!outputTextarea.value) {
            MediaConverter.showToast('warning', 'Nothing to Copy', 'No Base64 data available');
            return;
        }

        const originalText = copyBtn.textContent;
        copyBtn.textContent = '⏳ Copying...';
        copyBtn.disabled = true;

        try {
            const success = await MediaConverter.copyToClipboard(outputTextarea.value);

            if (success) {
                copyBtn.textContent = '✅ Copied!';
                MediaConverter.showToast('success', 'Copied to Clipboard',
                    `${MediaConverter.formatFileSize(outputTextarea.value.length)} of Base64 data copied`);
            } else {
                throw new Error('Clipboard operation failed');
            }

        } catch (error) {
            copyBtn.textContent = '❌ Failed';
            MediaConverter.showToast('error', 'Copy Failed', 'Could not copy to clipboard. Please select and copy manually.');
        }

        // Reset button after 2 seconds
        setTimeout(() => {
            copyBtn.textContent = originalText;
            copyBtn.disabled = false;
        }, 2000);
    }

    clearResult() {
        if (this.converting) {
            MediaConverter.showToast('warning', 'Cannot Clear', 'Cannot clear during conversion');
            return;
        }

        // Reset everything
        this.currentFile = null;
        document.getElementById('base64-output').value = '';
        document.getElementById('base64-result').classList.add('hidden');
        document.getElementById('base64-progress').classList.add('hidden');

        // Reset progress
        MediaConverter.updateProgress('base64-progress-fill', 0);

        MediaConverter.showToast('info', 'Cleared', 'Base64 converter reset');
        MediaConverter.updateStatus('Ready', 'info');
    }

    // ================================
    // UTILITY FUNCTIONS
    // ================================

    getStatusText(fileInfo) {
        switch (fileInfo.status) {
            case 'ready':
                return 'Ready';
            case 'converting':
                return `${Math.round(fileInfo.progress)}%`;
            case 'completed':
                return 'Converted';
            case 'error':
                return 'Failed';
            default:
                return 'Unknown';
        }
    }

    // ================================
    // PUBLIC API
    // ================================

    getStats() {
        return {
            hasFile: !!this.currentFile,
            fileName: this.currentFile?.name,
            fileSize: this.currentFile?.size,
            isConverting: this.converting,
            hasResult: !!document.getElementById('base64-output').value
        };
    }
}

// ================================
// INITIALIZATION
// ================================

let base64Converter;

// Initialize when DOM is ready
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => {
        base64Converter = new Base64Converter();
    });
} else {
    base64Converter = new Base64Converter();
}