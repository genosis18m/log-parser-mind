# Log-Zero

High-Performance AI-Powered Log Compression + Self-Healing System

## Overview

Log-Zero is a Go-first microservices architecture that:
- **Compresses logs** using the Drain algorithm (100k logs/sec)
- **Detects anomalies** in real-time log streams
- **Self-heals** by proposing and executing fixes via LLM agents
- **Learns** from past fixes to improve over time

## Architecture

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│  Ingestion  │────▶│ Compression │────▶│  ClickHouse │
│   Service   │     │   Service   │     │   Storage   │
└─────────────┘     └──────┬──────┘     └─────────────┘
                           │
                           ▼
                   ┌───────────────┐
                   │    Anomaly    │
                   │   Detection   │
                   └───────┬───────┘
                           │
                           ▼
                   ┌───────────────┐     ┌─────────────┐
                   │  Agent (LLM)  │────▶│  Experience │
                   │    Service    │     │   Service   │
                   └───────────────┘     └─────────────┘
```

## Quick Start

### Prerequisites
- Go 1.23+
- Docker & Docker Compose
- Make

### Build

```bash
# Build all services
make build

# Build specific service
make build-gateway
```

### Run Locally

```bash
# Start infrastructure (ClickHouse, Redis, PostgreSQL)
make docker-compose-up

# Run API Gateway
make run-gateway
```

### Docker

```bash
# Build Docker image
make docker

# Run with Docker Compose
docker compose -f deployments/docker/docker-compose.yml up
```

## Services

| Service | Port | Description |
|---------|------|-------------|
| Gateway | 8080 | API Gateway (REST + WebSocket) |
| Compression | 8090 | Log compression with Drain algorithm |
| Ingestion | 8091 | Kafka consumer for log ingestion |
| Anomaly | 8100 | Real-time anomaly detection |
| Agent | 8110 | LLM-powered fix proposals |
| Experience | 8120 | Learning from past fixes |

## API Endpoints

### Logs
- `POST /api/v1/logs/upload` - Upload logs for compression
- `GET /api/v1/logs/query` - Query compressed logs
- `GET /api/v1/logs/templates` - Get log templates

### Agent
- `POST /api/v1/agent/analyze` - Analyze logs for issues
- `POST /api/v1/agent/fix` - Execute a fix proposal

### Metrics
- `GET /api/v1/metrics/sustainability` - Compression savings
- `GET /health` - Health check

## Configuration

Set environment variables or use `config.yaml`:

```yaml
server:
  port: 8080

clickhouse:
  host: localhost
  port: 9000
  database: logzero

redis:
  host: localhost
  port: 6379

openai:
  api_key: ${OPENAI_API_KEY}
  model: gpt-4
```

## Performance

| Metric | Value |
|--------|-------|
| Compression Throughput | 100k logs/sec |
| Memory per Instance | 200-500 MB |
| Container Size | ~20 MB |
| Cold Start | <1s |

## License

MIT
# log-parser-mind
