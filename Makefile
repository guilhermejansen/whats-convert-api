.PHONY: help build run test clean docker-build docker-run docker-stop benchmark deps lint

# Variables
APP_NAME = media-converter
VERSION := $(shell cat VERSION 2>/dev/null || echo 0.0.0)
DOCKER_IMAGE ?= whats-convert-api
DOCKER_REGISTRY ?=
DOCKER_REPOSITORY ?= $(DOCKER_IMAGE)
DOCKER_TAG ?= $(VERSION)
DOCKER_IMAGE_REF := $(if $(DOCKER_REGISTRY),$(DOCKER_REGISTRY)/)$(DOCKER_REPOSITORY)
GOLANGCI_VERSION ?= v1.61.0
GO_FILES = $(shell find . -name '*.go' -type f)
BINARY = $(APP_NAME)

# Colors for output
RED = \033[0;31m
GREEN = \033[0;32m
YELLOW = \033[1;33m
NC = \033[0m # No Color

help: ## Show this help message
	@echo "${GREEN}WhatsApp Media Converter API${NC}"
	@echo ""
	@echo "Available commands:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  ${YELLOW}%-20s${NC} %s\n", $$1, $$2}'

deps: ## Install dependencies
	@echo "${GREEN}Installing dependencies...${NC}"
	go mod download
	go mod tidy

build: deps ## Build the application
	@echo "${GREEN}Building application...${NC}"
	CGO_ENABLED=0 go build -ldflags="-w -s" -o $(BINARY) cmd/api/main.go
	@echo "${GREEN}Build complete: $(BINARY)${NC}"

run: ## Run the application locally
	@echo "${GREEN}Starting application...${NC}"
	go run cmd/api/main.go

dev: ## Run in development mode with hot reload
	@echo "${GREEN}Starting development server...${NC}"
	@which air > /dev/null || (echo "${RED}Installing air...${NC}" && go install github.com/cosmtrek/air@latest)
	air

test: ## Run tests
	@echo "${GREEN}Running tests...${NC}"
	go test -v -race -cover ./...

test-coverage: ## Run tests with coverage report
	@echo "${GREEN}Running tests with coverage...${NC}"
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "${GREEN}Coverage report generated: coverage.html${NC}"

benchmark: ## Run benchmarks
	@echo "${GREEN}Running benchmarks...${NC}"
	go test -bench=. -benchmem ./...

lint: ## Run linter
	@echo "${GREEN}Running linter...${NC}"
	@echo "${GREEN}Ensuring golangci-lint $(GOLANGCI_VERSION) is installed...${NC}"
	GOBIN=$(GOPATH)/bin go install -gcflags=all=-lang=go1.24 github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_VERSION)
	golangci-lint run

clean: ## Clean build artifacts
	@echo "${GREEN}Cleaning build artifacts...${NC}"
	rm -f $(BINARY)
	rm -f coverage.out coverage.html
	rm -rf tmp/
	@echo "${GREEN}Clean complete${NC}"

# Docker commands
docker-build: ## Build Docker image tagged with VERSION
	@echo "${GREEN}Building Docker image...${NC}"
	docker build --build-arg VERSION=$(VERSION) -t $(DOCKER_IMAGE_REF):$(DOCKER_TAG) -t $(DOCKER_IMAGE_REF):latest .
	@echo "${GREEN}Docker image built:${NC} $(DOCKER_IMAGE_REF):$(DOCKER_TAG)"

docker-run: ## Run Docker container
	@echo "${GREEN}Starting Docker container...${NC}"
	docker-compose up -d
	@echo "${GREEN}Container started. API available at http://localhost:8080${NC}"

docker-stop: ## Stop Docker container
	@echo "${YELLOW}Stopping Docker container...${NC}"
	docker-compose down
	@echo "${GREEN}Container stopped${NC}"

docker-logs: ## Show Docker container logs
	docker-compose logs -f media-converter

docker-stats: ## Show Docker container stats
	docker stats $(docker-compose ps -q)

docker-shell: ## Open shell in Docker container
	docker exec -it whats-media-converter /bin/sh

docker-clean: ## Clean Docker resources
	@echo "${YELLOW}Cleaning Docker resources...${NC}"
	docker-compose down -v --remove-orphans
	docker image prune -f
	@echo "${GREEN}Docker resources cleaned${NC}"

docker-push: docker-build ## Build and push Docker image (VERSION + latest)
	@echo "${GREEN}Pushing Docker image tags...${NC}"
	docker push $(DOCKER_IMAGE_REF):$(DOCKER_TAG)
	docker push $(DOCKER_IMAGE_REF):latest
	@echo "${GREEN}Docker image pushed${NC}"

# Production commands
prod-build: ## Build for production
	@echo "${GREEN}Building for production...${NC}"
	docker build --target runtime --build-arg VERSION=$(VERSION) -t $(DOCKER_IMAGE_REF):$(DOCKER_TAG) -t $(DOCKER_IMAGE_REF):latest .
	@echo "${GREEN}Production build complete${NC}"

prod-deploy: prod-build ## Deploy to production
	@echo "${GREEN}Deploying to production...${NC}"
	docker-compose --profile production up -d
	@echo "${GREEN}Production deployment complete${NC}"

# Load testing
load-test: ## Run load test (requires Apache Bench)
	@echo "${GREEN}Running load test...${NC}"
	@which ab > /dev/null || (echo "${RED}Apache Bench (ab) not found. Please install it.${NC}" && exit 1)
	@echo "Testing audio conversion endpoint..."
	./scripts/load-test.sh audio
	@echo "Testing image conversion endpoint..."
	./scripts/load-test.sh image

stress-test: ## Run stress test (1000 req/s)
	@echo "${GREEN}Running stress test (1000 req/s)...${NC}"
	./scripts/stress-test.sh

# Monitoring
monitoring-up: ## Start monitoring stack
	@echo "${GREEN}Starting monitoring stack...${NC}"
	docker-compose --profile monitoring up -d
	@echo "${GREEN}Monitoring available at:${NC}"
	@echo "  Prometheus: http://localhost:9090"
	@echo "  Grafana: http://localhost:3000"

monitoring-down: ## Stop monitoring stack
	@echo "${YELLOW}Stopping monitoring stack...${NC}"
	docker-compose --profile monitoring down

# Installation helpers
install-deps-ubuntu: ## Install system dependencies (Ubuntu/Debian)
	@echo "${GREEN}Installing system dependencies for Ubuntu/Debian...${NC}"
	sudo apt-get update
	sudo apt-get install -y ffmpeg libvips-tools

install-deps-mac: ## Install system dependencies (macOS)
	@echo "${GREEN}Installing system dependencies for macOS...${NC}"
	brew install ffmpeg vips

# Quick commands
up: docker-run ## Alias for docker-run
down: docker-stop ## Alias for docker-stop
logs: docker-logs ## Alias for docker-logs

# Default target
all: deps build test ## Build and test everything
