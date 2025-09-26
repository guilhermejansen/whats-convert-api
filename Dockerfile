# Build stage
FROM golang:1.25.1-alpine AS builder

ARG VERSION=dev

# Install build dependencies
RUN apk add --no-cache \
    gcc \
    musl-dev \
    git

WORKDIR /build

# Copy go mod files first for better caching
COPY go.mod go.sum ./
COPY docs/ ./docs/
COPY web/ ./web/

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application with optimizations
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
    -ldflags="-w -s -extldflags '-static'" \
    -a -installsuffix cgo \
    -o media-converter \
    cmd/api/main.go

# Runtime stage
FROM alpine:3.20

ARG VERSION=dev

# Install runtime dependencies
RUN apk add --no-cache \
    ffmpeg \
    vips \
    vips-tools \
    ca-certificates \
    tini \
    && rm -rf /var/cache/apk/*

# Create non-root user
RUN addgroup -g 1000 -S appuser && \
    adduser -u 1000 -S appuser -G appuser

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/media-converter .

# Create directories for temporary files
RUN mkdir -p /tmp /dev/shm && \
    chown -R appuser:appuser /app /tmp

# Use tmpfs for temporary files (mounted at runtime)
VOLUME ["/tmp", "/dev/shm"]

# Set environment variables for performance
ENV PORT=8080 \
    GOGC=100 \
    GOMEMLIMIT=1GiB \
    GOMAXPROCS=0 \
    FFMPEG_THREADS=0 \
    MAX_WORKERS=0 \
    BUFFER_POOL_SIZE=100 \
    BUFFER_SIZE=10485760 \
    REQUEST_TIMEOUT=5m \
    BODY_LIMIT=524288000

# Change to non-root user
USER appuser

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Use tini for proper signal handling
LABEL org.opencontainers.image.title="WhatsApp Media Converter API" \
      org.opencontainers.image.description="High-performance WhatsApp media conversion service" \
      org.opencontainers.image.url="https://github.com/guilhermejansen/whats-convert-api" \
      org.opencontainers.image.source="https://github.com/guilhermejansen/whats-convert-api" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.licenses="MIT"

ENTRYPOINT ["/sbin/tini", "--"]

# Run the application
CMD ["./media-converter"]
