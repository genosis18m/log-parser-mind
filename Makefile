.PHONY: all build clean test proto run-gateway run-compression run-ingestion run-agent run-experience docker

# Variables
BINARY_DIR := bin
CMD_DIR := cmd
PROTO_DIR := api/proto
GO := go
DOCKER := docker

# Service names
SERVICES := gateway compression ingestion agent experience anomaly

all: build

# Build all services
build:
	@echo "Building all services..."
	@mkdir -p $(BINARY_DIR)
	@for service in $(SERVICES); do \
		echo "Building $$service..."; \
		CGO_ENABLED=0 $(GO) build -o $(BINARY_DIR)/$$service ./$(CMD_DIR)/$$service; \
	done
	@echo "Build complete!"

# Build individual services
build-%:
	@echo "Building $*..."
	@mkdir -p $(BINARY_DIR)
	CGO_ENABLED=0 $(GO) build -o $(BINARY_DIR)/$* ./$(CMD_DIR)/$*

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf $(BINARY_DIR)
	@rm -f api/proto/*.pb.go
	@echo "Clean complete!"

# Run tests
test:
	@echo "Running tests..."
	$(GO) test -v -race -cover ./...

# Run tests with coverage report
test-coverage:
	@echo "Running tests with coverage..."
	$(GO) test -v -race -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Generate Protocol Buffers
proto:
	@echo "Generating Protocol Buffers..."
	@if command -v protoc > /dev/null; then \
		protoc --go_out=. --go_opt=paths=source_relative \
			--go-grpc_out=. --go-grpc_opt=paths=source_relative \
			$(PROTO_DIR)/*.proto; \
		echo "Proto generation complete!"; \
	else \
		echo "protoc not installed. Skipping proto generation."; \
	fi

# Run services locally
run-gateway:
	$(GO) run ./$(CMD_DIR)/gateway

run-compression:
	$(GO) run ./$(CMD_DIR)/compression

run-ingestion:
	$(GO) run ./$(CMD_DIR)/ingestion

run-agent:
	$(GO) run ./$(CMD_DIR)/agent

run-experience:
	$(GO) run ./$(CMD_DIR)/experience

run-anomaly:
	$(GO) run ./$(CMD_DIR)/anomaly

# Docker builds
docker:
	@echo "Building Docker images..."
	$(DOCKER) build -f deployments/docker/Dockerfile -t logzero:latest .

docker-compose-up:
	$(DOCKER) compose -f deployments/docker/docker-compose.yml up -d

docker-compose-down:
	$(DOCKER) compose -f deployments/docker/docker-compose.yml down

# Lint
lint:
	@echo "Running linter..."
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed. Run: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

# Format code
fmt:
	$(GO) fmt ./...

# Tidy dependencies
tidy:
	$(GO) mod tidy

# Download dependencies
deps:
	$(GO) mod download

# Help
help:
	@echo "Log-Zero Makefile"
	@echo ""
	@echo "Usage:"
	@echo "  make build          - Build all services"
	@echo "  make build-<svc>    - Build specific service (gateway, compression, etc.)"
	@echo "  make test           - Run all tests"
	@echo "  make test-coverage  - Run tests with coverage report"
	@echo "  make proto          - Generate Protocol Buffer code"
	@echo "  make docker         - Build Docker image"
	@echo "  make run-<svc>      - Run specific service locally"
	@echo "  make clean          - Clean build artifacts"
	@echo "  make lint           - Run linter"
	@echo "  make fmt            - Format code"
	@echo "  make tidy           - Tidy go.mod"
	@echo "  make deps           - Download dependencies"
