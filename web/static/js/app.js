// WhatsApp Media Converter - Core Application JavaScript
// Global utilities and shared functionality

class MediaConverterApp {
    constructor() {
        this.toasts = [];
        this.globalStatus = '';
        this.init();
    }

    init() {
        this.setupGlobalListeners();
        this.checkAPIHealth();
        this.updateGlobalStatus('Ready');
    }

    // ================================
    // TOAST NOTIFICATION SYSTEM
    // ================================

    showToast(type, title, message, duration = 5000) {
        const toast = this.createToastElement(type, title, message);
        const container = document.getElementById('toast-container');

        container.appendChild(toast);
        this.toasts.push(toast);

        // Auto-remove after duration
        setTimeout(() => {
            this.removeToast(toast);
        }, duration);

        return toast;
    }

    createToastElement(type, title, message) {
        const toast = document.createElement('div');
        toast.className = `toast ${type}`;

        const icons = {
            success: '✅',
            error: '❌',
            warning: '⚠️',
            info: 'ℹ️'
        };

        toast.innerHTML = `
            <div class="toast-icon">${icons[type] || icons.info}</div>
            <div class="toast-content">
                <div class="toast-title">${this.escapeHtml(title)}</div>
                <div class="toast-message">${this.escapeHtml(message)}</div>
            </div>
            <button class="toast-close" onclick="app.removeToast(this.parentElement)">×</button>
        `;

        return toast;
    }

    removeToast(toast) {
        if (toast && toast.parentElement) {
            toast.style.transform = 'translateX(100%)';
            toast.style.opacity = '0';

            setTimeout(() => {
                if (toast.parentElement) {
                    toast.parentElement.removeChild(toast);
                }
                const index = this.toasts.indexOf(toast);
                if (index > -1) {
                    this.toasts.splice(index, 1);
                }
            }, 300);
        }
    }

    // ================================
    // LOADING OVERLAY MANAGEMENT
    // ================================

    showLoading(message = 'Processing...') {
        const overlay = document.getElementById('loading-overlay');
        const text = overlay.querySelector('.loading-text');
        text.textContent = message;
        overlay.classList.remove('hidden');
    }

    hideLoading() {
        const overlay = document.getElementById('loading-overlay');
        overlay.classList.add('hidden');
    }

    // ================================
    // GLOBAL STATUS MANAGEMENT
    // ================================

    updateGlobalStatus(message, type = 'info') {
        this.globalStatus = message;
        const statusBar = document.getElementById('global-status');
        const statusMessage = statusBar.querySelector('.status-message');
        const statusIcon = statusBar.querySelector('.status-icon');

        const icons = {
            info: 'ℹ️',
            success: '✅',
            error: '❌',
            warning: '⚠️',
            uploading: '📤',
            converting: '🔄'
        };

        statusIcon.textContent = icons[type] || icons.info;
        statusMessage.textContent = message;

        // Show status bar
        statusBar.classList.add('visible');

        // Auto-hide after 3 seconds for success messages
        if (type === 'success') {
            setTimeout(() => {
                statusBar.classList.remove('visible');
            }, 3000);
        }
    }

    // ================================
    // FILE UTILITIES
    // ================================

    formatFileSize(bytes) {
        if (bytes === 0) return '0 Bytes';

        const k = 1024;
        const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB'];
        const i = Math.floor(Math.log(bytes) / Math.log(k));

        return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
    }

    getFileIcon(fileName) {
        const ext = fileName.split('.').pop().toLowerCase();

        const icons = {
            // Audio
            mp3: '🎵', wav: '🎵', ogg: '🎵', m4a: '🎵', aac: '🎵', flac: '🎵', opus: '🎵', oga: '🎵', webm: '🎵', weba: '🎵',
            // Images
            jpg: '🖼️', jpeg: '🖼️', png: '🖼️', gif: '🖼️', webp: '🖼️', svg: '🖼️',
            // Video
            mp4: '🎬', avi: '🎬', mov: '🎬', wmv: '🎬', mkv: '🎬',
            // Documents
            pdf: '📄', doc: '📄', docx: '📄', txt: '📄', rtf: '📄',
            // Archives
            zip: '📦', rar: '📦', '7z': '📦', tar: '📦', gz: '📦',
            // Default
            default: '📁'
        };

        return icons[ext] || icons.default;
    }

    detectContentType(fileName) {
        const ext = fileName.split('.').pop().toLowerCase();

        const types = {
            // Audio
            mp3: 'audio/mpeg', wav: 'audio/wav', ogg: 'audio/ogg', opus: 'audio/opus', oga: 'audio/ogg', webm: 'audio/webm',
            m4a: 'audio/mp4', aac: 'audio/aac', flac: 'audio/flac',
            // Images
            jpg: 'image/jpeg', jpeg: 'image/jpeg', png: 'image/png',
            gif: 'image/gif', webp: 'image/webp', svg: 'image/svg+xml',
            // Video
            mp4: 'video/mp4', avi: 'video/x-msvideo', mov: 'video/quicktime',
            // Documents
            pdf: 'application/pdf', txt: 'text/plain',
            // Default
            default: 'application/octet-stream'
        };

        return types[ext] || types.default;
    }

    isAudioFile(fileName) {
        const ext = fileName.split('.').pop().toLowerCase();
        const audioExts = ['mp3', 'wav', 'ogg', 'm4a', 'aac', 'flac', 'wma', 'opus', 'oga', 'webm', 'weba'];
        return audioExts.includes(ext);
    }

    isImageFile(fileName) {
        const ext = fileName.split('.').pop().toLowerCase();
        const imageExts = ['jpg', 'jpeg', 'png', 'gif', 'webp', 'bmp', 'svg'];
        return imageExts.includes(ext);
    }

    // ================================
    // API COMMUNICATION
    // ================================

    async apiRequest(endpoint, options = {}) {
        const defaultOptions = {
            method: 'GET',
            headers: {
                'Content-Type': 'application/json',
            },
        };

        const config = { ...defaultOptions, ...options };

        try {
            const response = await fetch(endpoint, config);

            if (!response.ok) {
                const errorData = await response.json().catch(() => ({
                    error: `HTTP ${response.status}: ${response.statusText}`
                }));
                throw new Error(errorData.error || `Request failed with status ${response.status}`);
            }

            return await response.json();
        } catch (error) {
            console.error('API Request failed:', error);
            throw error;
        }
    }

    async checkAPIHealth() {
        try {
            const health = await this.apiRequest('/health');
            this.updateGlobalStatus('API Connected', 'success');

            // Check S3 status if enabled
            try {
                const s3Health = await this.apiRequest('/upload/s3/health');
                if (s3Health.healthy) {
                    document.getElementById('s3-indicator').textContent = '🟢';
                    document.getElementById('s3-status-text').textContent = 'S3 Connected';
                } else {
                    document.getElementById('s3-indicator').textContent = '🟡';
                    document.getElementById('s3-status-text').textContent = 'S3 Available but not configured';
                }
            } catch (error) {
                document.getElementById('s3-indicator').textContent = '🔴';
                document.getElementById('s3-status-text').textContent = 'S3 Disabled';
            }
        } catch (error) {
            this.updateGlobalStatus('API Connection Failed', 'error');
            this.showToast('error', 'Connection Error', 'Failed to connect to API');
        }
    }

    // ================================
    // DRAG & DROP UTILITIES
    // ================================

    setupDragAndDrop(element, callbacks) {
        const { onDragEnter, onDragLeave, onDrop } = callbacks;

        element.addEventListener('dragenter', (e) => {
            e.preventDefault();
            e.stopPropagation();
            element.classList.add('drag-over');
            if (onDragEnter) onDragEnter(e);
        });

        element.addEventListener('dragover', (e) => {
            e.preventDefault();
            e.stopPropagation();
        });

        element.addEventListener('dragleave', (e) => {
            e.preventDefault();
            e.stopPropagation();
            if (e.target === element) {
                element.classList.remove('drag-over');
                if (onDragLeave) onDragLeave(e);
            }
        });

        element.addEventListener('drop', (e) => {
            e.preventDefault();
            e.stopPropagation();
            element.classList.remove('drag-over');

            const files = Array.from(e.dataTransfer.files);
            if (onDrop) onDrop(files);
        });
    }

    // ================================
    // CLIPBOARD UTILITIES
    // ================================

    async copyToClipboard(text) {
        try {
            if (navigator.clipboard && window.isSecureContext) {
                await navigator.clipboard.writeText(text);
                return true;
            } else {
                // Fallback for older browsers
                const textArea = document.createElement('textarea');
                textArea.value = text;
                textArea.style.position = 'fixed';
                textArea.style.left = '-999999px';
                textArea.style.top = '-999999px';
                document.body.appendChild(textArea);
                textArea.focus();
                textArea.select();

                const success = document.execCommand('copy');
                document.body.removeChild(textArea);
                return success;
            }
        } catch (error) {
            console.error('Failed to copy to clipboard:', error);
            return false;
        }
    }

    // ================================
    // PROGRESS TRACKING
    // ================================

    updateProgress(elementId, progress, text = null) {
        const progressFill = document.getElementById(elementId);
        const progressText = document.getElementById(elementId.replace('-fill', '-text'));

        if (progressFill) {
            progressFill.style.width = `${progress}%`;
        }

        if (progressText) {
            progressText.textContent = text || `${Math.round(progress)}%`;
        }
    }

    // ================================
    // UTILITY FUNCTIONS
    // ================================

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    generateId() {
        return Date.now().toString(36) + Math.random().toString(36).substr(2);
    }

    debounce(func, wait) {
        let timeout;
        return function executedFunction(...args) {
            const later = () => {
                clearTimeout(timeout);
                func(...args);
            };
            clearTimeout(timeout);
            timeout = setTimeout(later, wait);
        };
    }

    // ================================
    // GLOBAL EVENT LISTENERS
    // ================================

    setupGlobalListeners() {
        // Prevent default drag behaviors on document
        document.addEventListener('dragenter', (e) => e.preventDefault());
        document.addEventListener('dragover', (e) => e.preventDefault());
        document.addEventListener('drop', (e) => e.preventDefault());

        // Handle global errors
        window.addEventListener('error', (e) => {
            console.error('Global error:', e.error);
            this.showToast('error', 'JavaScript Error', e.error.message);
        });

        // Handle unhandled promise rejections
        window.addEventListener('unhandledrejection', (e) => {
            console.error('Unhandled promise rejection:', e.reason);
            this.showToast('error', 'Request Failed', e.reason.message || 'An unexpected error occurred');
        });

        // Close loading overlay on click (emergency exit)
        document.getElementById('loading-overlay').addEventListener('click', () => {
            this.hideLoading();
        });
    }

    // ================================
    // FILE VALIDATION
    // ================================

    validateFile(file, options = {}) {
        const {
            maxSize = Infinity,
            allowedTypes = [],
            allowedExtensions = []
        } = options;

        // Check file size
        if (file.size > maxSize) {
            throw new Error(`File too large. Maximum size: ${this.formatFileSize(maxSize)}`);
        }

        // Check MIME type
        if (allowedTypes.length > 0) {
            const normalizedType = file.type ? file.type.split(';')[0].toLowerCase() : '';
            if (normalizedType && !allowedTypes.includes(normalizedType)) {
                throw new Error(`File type not allowed: ${file.type || 'unknown'}`);
            }
        }

        // Check file extension
        if (allowedExtensions.length > 0) {
            const ext = file.name.split('.').pop().toLowerCase();
            if (!allowedExtensions.includes(ext)) {
                throw new Error(`File extension not allowed: .${ext}`);
            }
        }

        return true;
    }

    // ================================
    // DOWNLOAD UTILITIES
    // ================================

    downloadBlob(blob, filename) {
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = filename;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        URL.revokeObjectURL(url);
    }

    downloadFromBase64(base64Data, filename) {
        try {
            // Extract MIME type and data from data URL
            let mimeType = 'application/octet-stream';
            let data = base64Data;

            if (base64Data.startsWith('data:')) {
                const [header, b64Data] = base64Data.split(',');
                data = b64Data;
                const mimeMatch = header.match(/data:([^;]+)/);
                if (mimeMatch) {
                    mimeType = mimeMatch[1];
                }
            }

            // Convert base64 to blob
            const byteCharacters = atob(data);
            const byteNumbers = new Array(byteCharacters.length);

            for (let i = 0; i < byteCharacters.length; i++) {
                byteNumbers[i] = byteCharacters.charCodeAt(i);
            }

            const byteArray = new Uint8Array(byteNumbers);
            const blob = new Blob([byteArray], { type: mimeType });

            this.downloadBlob(blob, filename);
            return true;
        } catch (error) {
            console.error('Download failed:', error);
            this.showToast('error', 'Download Failed', error.message);
            return false;
        }
    }
}

// ================================
// INITIALIZATION
// ================================

// Initialize app when DOM is ready
let app;

if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => {
        app = new MediaConverterApp();
    });
} else {
    app = new MediaConverterApp();
}

// ================================
// GLOBAL UTILITIES (for modules)
// ================================

// Export utilities for use by individual modules
window.MediaConverter = {
    // Toast system
    showToast: (type, title, message, duration) => app.showToast(type, title, message, duration),

    // Loading system
    showLoading: (message) => app.showLoading(message),
    hideLoading: () => app.hideLoading(),

    // Status system
    updateStatus: (message, type) => app.updateGlobalStatus(message, type),

    // File utilities
    formatFileSize: (bytes) => app.formatFileSize(bytes),
    getFileIcon: (fileName) => app.getFileIcon(fileName),
    detectContentType: (fileName) => app.detectContentType(fileName),
    isAudioFile: (fileName) => app.isAudioFile(fileName),
    isImageFile: (fileName) => app.isImageFile(fileName),
    validateFile: (file, options) => app.validateFile(file, options),

    // Clipboard utilities
    copyToClipboard: (text) => app.copyToClipboard(text),

    // Download utilities
    downloadBlob: (blob, filename) => app.downloadBlob(blob, filename),
    downloadFromBase64: (base64Data, filename) => app.downloadFromBase64(base64Data, filename),

    // API utilities
    apiRequest: (endpoint, options) => app.apiRequest(endpoint, options),

    // Progress utilities
    updateProgress: (elementId, progress, text) => app.updateProgress(elementId, progress, text),

    // Drag & drop utilities
    setupDragAndDrop: (element, callbacks) => app.setupDragAndDrop(element, callbacks),

    // Utility functions
    escapeHtml: (text) => app.escapeHtml(text),
    generateId: () => app.generateId(),
    debounce: (func, wait) => app.debounce(func, wait)
};
