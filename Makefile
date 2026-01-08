# CloudUnify Makefile

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Binary name
BINARY_NAME=cloudunify
BINARY_DIR=bin

# Build parameters
VERSION?=0.1.0
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS=-ldflags "-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)"

# Platforms for cross-compilation
PLATFORMS=darwin/amd64 darwin/arm64 linux/amd64 windows/amd64

.PHONY: all build build-dev build-all clean test deps run fmt lint help

# Default target
all: deps build

# Download dependencies
deps:
	$(GOMOD) download
	$(GOMOD) tidy

# Build for development (current platform)
build-dev:
	$(GOBUILD) $(LDFLAGS) -o $(BINARY_DIR)/$(BINARY_NAME) ./cmd/cloudunify

# Build optimized release binary
build:
	CGO_ENABLED=1 $(GOBUILD) $(LDFLAGS) -o $(BINARY_DIR)/$(BINARY_NAME) ./cmd/cloudunify

# Build for all platforms
build-all: clean
	@for platform in $(PLATFORMS); do \
		GOOS=$${platform%/*} GOARCH=$${platform#*/} \
		$(GOBUILD) $(LDFLAGS) -o $(BINARY_DIR)/$(BINARY_NAME)-$${platform%/*}-$${platform#*/}$$(if [ "$${platform%/*}" = "windows" ]; then echo ".exe"; fi) ./cmd/cloudunify; \
		echo "Built: $(BINARY_NAME)-$${platform%/*}-$${platform#*/}"; \
	done

# Run the application
run: build-dev
	./$(BINARY_DIR)/$(BINARY_NAME)

# Run without FUSE mount (for development)
run-no-mount: build-dev
	./$(BINARY_DIR)/$(BINARY_NAME) --no-mount

# Run tests
test:
	$(GOTEST) -v ./...

# Run tests with coverage
test-cover:
	$(GOTEST) -v -cover -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

# Format code
fmt:
	$(GOCMD) fmt ./...

# Lint code (requires golangci-lint)
lint:
	golangci-lint run

# Clean build artifacts
clean:
	$(GOCLEAN)
	rm -rf $(BINARY_DIR)

# Install dependencies and tools
setup:
	$(GOGET) -u github.com/golangci/golangci-lint/cmd/golangci-lint

# Generate (placeholder for code generation)
generate:
	$(GOCMD) generate ./...

# Database operations
db-reset:
	rm -f ~/.local/share/cloudunify/cloudunify.db

# macOS specific
.PHONY: install-macos
install-macos:
	brew install macfuse || true
	@echo "Please restart your computer after installing macFUSE"

# Build web UI
.PHONY: web-deps web-build web-dev
web-deps:
	cd web && npm install

web-build: web-deps
	cd web && npm run build

web-dev:
	cd web && npm run dev

# Help
help:
	@echo "CloudUnify Build System"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  all          - Download deps and build"
	@echo "  build        - Build optimized binary"
	@echo "  build-dev    - Build development binary"
	@echo "  build-all    - Build for all platforms"
	@echo "  run          - Build and run"
	@echo "  run-no-mount - Run without FUSE mount"
	@echo "  test         - Run tests"
	@echo "  test-cover   - Run tests with coverage"
	@echo "  fmt          - Format code"
	@echo "  lint         - Lint code"
	@echo "  clean        - Clean build artifacts"
	@echo "  deps         - Download dependencies"
	@echo "  setup        - Install development tools"
	@echo "  web-deps     - Install web dependencies"
	@echo "  web-build    - Build web UI"
	@echo "  web-dev      - Run web dev server"
	@echo "  help         - Show this help"
