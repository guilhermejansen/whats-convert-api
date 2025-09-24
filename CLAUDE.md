# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

WhatsApp Media Converter API is a high-performance Go service optimized for converting audio and image files for WhatsApp. The service is built with Go 1.25.1 and Fiber v3.0, capable of processing 1000+ conversions per second with sub-500ms latency.

## Core Architecture

The project follows Go standard layout with these key components:

- **cmd/api/main.go**: Application entry point
- **internal/config/**: Configuration management with environment variable parsing
- **internal/server/**: Fiber HTTP server setup and middleware
- **internal/pool/**: Worker pool and buffer pool for high-performance processing
- **internal/handlers/**: HTTP request handlers
- **internal/services/**: Core conversion services (audio, image, downloader)

### Performance Architecture
- **Worker Pool**: Configurable workers (default: CPU×4) for concurrent processing
- **Buffer Pool**: Memory buffers (default: 100×10MB) for zero-disk I/O processing
- **Memory-only Processing**: Uses tmpfs and direct pipes, no disk storage

## Common Development Commands

### Local Development
```bash
# Install dependencies and build
make deps
make build

# Run locally (with hot reload if air is installed)
make dev

# Run without hot reload
make run

# Run tests with coverage
make test
make test-coverage

# Run benchmarks
make benchmark

# Lint code
make lint
```

### Docker Development
```bash
# Build and run with Docker (recommended)
make docker-build
make docker-run

# Stop container
make docker-stop

# View logs
make docker-logs

# Access container shell
make docker-shell
```

### Testing and Performance
```bash
# Load testing
make load-test

# Stress testing (1000 req/s)
make stress-test

# System dependencies (choose your OS)
make install-deps-mac    # macOS
make install-deps-ubuntu # Ubuntu/Debian
```

## Configuration

The application uses environment variables with smart defaults. Key configuration files:
- **.env**: Local development environment
- **.env.example**: Template with all available options
- **.env.production**: Production configuration

### Critical Environment Variables
- `MAX_WORKERS`: Worker pool size (default: CPU×4)
- `BUFFER_POOL_SIZE`: Number of memory buffers (default: 100)
- `BUFFER_SIZE`: Size of each buffer (default: 10MB)
- `GOGC`: Go garbage collector percentage (default: 100)
- `GOMEMLIMIT`: Go memory limit (default: 1GiB)

## Performance Optimization

### Memory Management
- Buffer pools prevent GC pressure during high-load processing
- Tmpfs volumes (/tmp, /dev/shm) for RAM-only storage
- Smart worker scaling based on CPU cores

### Key Performance Features
- Zero-disk I/O processing with memory pipes
- FFmpeg for audio conversion (any format → Opus)
- libvips for image conversion (any format → optimized JPEG)
- Concurrent processing with bounded worker pools

## API Endpoints

### Core Conversion Endpoints
- `POST /convert/audio`: Convert audio to Opus format
- `POST /convert/image`: Convert images to optimized JPEG
- `POST /convert/batch/audio`: Batch audio conversion (max 10 files)
- `POST /convert/batch/image`: Batch image conversion (max 10 files)

### Monitoring Endpoints
- `GET /health`: Health check with performance statistics
- `GET /stats`: Detailed performance metrics

## Testing Guidelines

The project includes comprehensive testing infrastructure:
- Unit tests: `make test`
- Coverage reports: `make test-coverage`
- Load testing: `./scripts/load-test.sh`
- Stress testing: `./scripts/stress-test.sh`

### Performance Targets
- Throughput: 1000+ req/s sustained
- Latency P95: <500ms
- Success Rate: >99.9%
- Memory Usage: <2GB with 100×10MB buffer pool

## Deployment

### Docker Production
```bash
# Build production image
make prod-build

# Deploy with production configuration
make prod-deploy
```

### Monitoring
```bash
# Start monitoring stack (Prometheus + Grafana)
make monitoring-up
```

## Code Patterns

### Error Handling
- Use structured logging with performance metrics
- Return detailed error information in API responses
- Implement graceful degradation for non-critical failures

### Concurrency
- All processing uses worker pools to prevent resource exhaustion
- Buffer pools manage memory allocation for high-throughput scenarios
- Request timeouts prevent hung connections

### Configuration
- Environment-first configuration with validation
- Smart defaults based on system resources (CPU, memory)
- Development vs production modes with different optimization profiles

## Dependencies

### Core Dependencies
- **Fiber v3.0**: Ultra-fast HTTP framework
- **godotenv**: Environment variable management

### System Dependencies (for local development)
- **FFmpeg 6.0+**: Audio processing
- **libvips 8.15+**: Image processing (3-8x faster than ImageMagick)

### Development Tools
- **air**: Hot reload for development
- **golangci-lint**: Code linting
- **apache-bench (ab)**: Load testing