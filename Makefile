# Overwatch API - Makefile

.PHONY: all build build-api build-scraper clean test run docker-build docker-up docker-down help

# Default target
all: build

# Build both binaries
build: build-api build-scraper
	@echo "✓ Build complete: api.exe and scraper.exe"

# Build API binary
build-api:
	@echo "Building API server..."
	go build -o api.exe .

# Build Scraper binary
build-scraper:
	@echo "Building Scraper service..."
	go build -o scraper.exe ./cmd/scraper

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -f api.exe scraper.exe api scraper

# Run tests
test:
	@echo "Running tests..."
	go test ./...

# Run API server locally
run:
	@echo "Starting API server..."
	go run .

# Run scraper locally
run-scraper:
	@echo "Starting Scraper service..."
	go run ./cmd/scraper

# Docker: Build images
docker-build:
	@echo "Building Docker images..."
	docker-compose build

# Docker: Start all services
docker-up:
	@echo "Starting Docker services..."
	docker-compose up -d

# Docker: Stop all services
docker-down:
	@echo "Stopping Docker services..."
	docker-compose down

# Docker: View logs
docker-logs:
	docker-compose logs -f

# Help
help:
	@echo "Overwatch API - Available commands:"
	@echo ""
	@echo "  make build          - Build both API and Scraper binaries"
	@echo "  make build-api      - Build only API binary (api.exe)"
	@echo "  make build-scraper  - Build only Scraper binary (scraper.exe)"
	@echo "  make clean          - Remove build artifacts"
	@echo "  make test           - Run tests"
	@echo "  make run            - Run API server locally"
	@echo "  make run-scraper    - Run Scraper service locally"
	@echo "  make docker-build   - Build Docker images"
	@echo "  make docker-up      - Start all Docker services"
	@echo "  make docker-down    - Stop all Docker services"
	@echo "  make docker-logs    - View Docker logs"
	@echo "  make help           - Show this help message"