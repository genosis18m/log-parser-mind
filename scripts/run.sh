#!/bin/bash

# Run all Log-Zero services locally
# Requires: .env file with configuration

set -e

# Load environment
if [ -f .env ]; then
    export $(cat .env | grep -v '^#' | xargs)
fi

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo "🚀 Starting Log-Zero Services"
echo ""

# Create logs directory
mkdir -p logs

# Function to start a service in background
start_service() {
    local name=$1
    local port=$2
    local cmd=$3
    
    echo -e "${GREEN}Starting $name on port $port...${NC}"
    $cmd > logs/$name.log 2>&1 &
    echo $! > logs/$name.pid
    sleep 1
    
    if kill -0 $(cat logs/$name.pid) 2>/dev/null; then
        echo -e "  ${GREEN}✓${NC} $name started (PID: $(cat logs/$name.pid))"
    else
        echo -e "  ${YELLOW}✗${NC} $name failed to start - check logs/$name.log"
    fi
}

# Start services
start_service "compression" "8090" "go run cmd/compression/main.go -http-port=8090"
start_service "ingestion" "8091" "go run cmd/ingestion/main.go -http-port=8091"
start_service "anomaly" "8100" "go run cmd/anomaly/main.go -http-port=8100"
start_service "agent" "8110" "go run cmd/agent/main.go -http-port=8110"
start_service "experience" "8120" "go run cmd/experience/main.go -http-port=8120"

echo ""
echo "Waiting for services to initialize..."
sleep 3

# Start gateway last (depends on other services)
start_service "gateway" "8080" "go run cmd/gateway/main.go -port=8080"

echo ""
echo "=================="
echo -e "${GREEN}All services started!${NC}"
echo ""
echo "Endpoints:"
echo "  Gateway:     http://localhost:8080"
echo "  Health:      http://localhost:8080/health"
echo "  API Docs:    http://localhost:8080/api/v1/"
echo ""
echo "Logs are in ./logs/"
echo "To stop all services: ./scripts/stop.sh"
