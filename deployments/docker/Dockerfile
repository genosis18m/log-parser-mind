# Build stage
FROM golang:1.23-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build all services
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o /bin/gateway ./cmd/gateway
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o /bin/compression ./cmd/compression
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o /bin/ingestion ./cmd/ingestion
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o /bin/agent ./cmd/agent
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o /bin/experience ./cmd/experience
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o /bin/anomaly ./cmd/anomaly

# Final stage - minimal image
FROM alpine:latest

# Install CA certificates for HTTPS
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 logzero && \
    adduser -u 1000 -G logzero -s /bin/sh -D logzero

WORKDIR /app

# Copy binaries from builder
COPY --from=builder /bin/gateway .
COPY --from=builder /bin/compression .
COPY --from=builder /bin/ingestion .
COPY --from=builder /bin/agent .
COPY --from=builder /bin/experience .
COPY --from=builder /bin/anomaly .

# Set ownership
RUN chown -R logzero:logzero /app

USER logzero

# Expose ports
EXPOSE 8080 8090 8091 8100 8110 8120

# Default to gateway
ENTRYPOINT ["./gateway"]
