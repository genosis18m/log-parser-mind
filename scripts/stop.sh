#!/bin/bash

# Stop all Log-Zero services

echo "Stopping Log-Zero services..."

for pidfile in logs/*.pid; do
    if [ -f "$pidfile" ]; then
        pid=$(cat "$pidfile")
        name=$(basename "$pidfile" .pid)
        if kill -0 "$pid" 2>/dev/null; then
            echo "  Stopping $name (PID: $pid)..."
            kill "$pid" 2>/dev/null || true
        fi
        rm -f "$pidfile"
    fi
done

# Kill any remaining go processes for our services
pkill -f "cmd/gateway" 2>/dev/null || true
pkill -f "cmd/compression" 2>/dev/null || true
pkill -f "cmd/ingestion" 2>/dev/null || true
pkill -f "cmd/agent" 2>/dev/null || true
pkill -f "cmd/experience" 2>/dev/null || true
pkill -f "cmd/anomaly" 2>/dev/null || true

echo "All services stopped."
