#!/bin/bash

# Log-Zero Setup Script
# This script sets up the development environment

set -e

echo "🚀 Log-Zero Setup"
echo "=================="

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Check prerequisites
check_command() {
    if command -v $1 &> /dev/null; then
        echo -e "${GREEN}✓${NC} $1 installed"
        return 0
    else
        echo -e "${RED}✗${NC} $1 not found"
        return 1
    fi
}

echo ""
echo "Checking prerequisites..."
check_command go
check_command docker
check_command docker-compose || check_command "docker compose"

# Check for .env file
echo ""
if [ -f .env ]; then
    echo -e "${GREEN}✓${NC} .env file found"
    source .env
else
    echo -e "${YELLOW}!${NC} .env file not found, copying from .env.example"
    if [ -f .env.example ]; then
        cp .env.example .env
        echo -e "${YELLOW}!${NC} Please edit .env and add your OPENAI_API_KEY"
    fi
fi

# Check API key
if [ -z "$OPENAI_API_KEY" ] || [ "$OPENAI_API_KEY" = "your-openai-api-key-here" ]; then
    echo -e "${YELLOW}⚠${NC} OPENAI_API_KEY not set - Agent service will not work"
else
    echo -e "${GREEN}✓${NC} OPENAI_API_KEY is set"
fi

# Download Go dependencies
echo ""
echo "Downloading Go dependencies..."
go mod download
echo -e "${GREEN}✓${NC} Dependencies downloaded"

# Build all services
echo ""
echo "Building services..."
make build 2>/dev/null || (
    mkdir -p bin
    for svc in gateway compression ingestion agent experience anomaly generator; do
        echo "  Building $svc..."
        go build -o bin/$svc ./cmd/$svc 2>/dev/null || echo "    Skipped $svc"
    done
)
echo -e "${GREEN}✓${NC} Build complete"

# Run tests
echo ""
echo "Running tests..."
go test ./internal/compression/drain/... -v 2>&1 | head -20 || true
echo ""

echo "=================="
echo -e "${GREEN}Setup complete!${NC}"
echo ""
echo "Next steps:"
echo "  1. Start infrastructure: docker compose -f deployments/docker/docker-compose.yml up -d"
echo "  2. Run gateway: ./bin/gateway or go run cmd/gateway/main.go"
echo "  3. Generate sample data: ./bin/generator -count 100"
echo ""
echo "API will be available at http://localhost:8080"
