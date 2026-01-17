// Package main is the entry point for the compression service.
package main

import (
	"context"
	"flag"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/log-zero/log-zero/internal/compression/drain"
	"github.com/log-zero/log-zero/internal/compression/pii"
	"go.uber.org/zap"
)

// Config holds the service configuration.
type Config struct {
	GRPCPort    string
	HTTPPort    string
	MetricsPort string
	WorkerCount int
	DrainConfig drain.Config
}

// CompressionService handles log compression.
type CompressionService struct {
	config    Config
	drainTree *drain.DrainTree
	redactor  *pii.Redactor
	logger    *zap.Logger
}

// NewCompressionService creates a new compression service.
func NewCompressionService(config Config, logger *zap.Logger) *CompressionService {
	drainTree := drain.NewDrainTree(config.DrainConfig)
	redactor := pii.NewRedactor(pii.DefaultRedactorConfig())

	return &CompressionService{
		config:    config,
		drainTree: drainTree,
		redactor:  redactor,
		logger:    logger,
	}
}

// CompressLog compresses a single log entry.
func (s *CompressionService) CompressLog(content string, source string, timestamp int64) (*CompressedLog, error) {
	// Parse log using Drain algorithm
	result, err := s.drainTree.Parse(content, timestamp)
	if err != nil {
		return nil, err
	}

	// Redact PII from variables
	redactedVars := s.redactor.RedactVariables(result.Variables)

	return &CompressedLog{
		TemplateID:     result.TemplateID,
		Template:       result.Template,
		Variables:      redactedVars,
		Source:         source,
		Timestamp:      timestamp,
		IsNewTemplate:  result.IsNew,
		OriginalSize:   len(content),
		CompressedSize: len(result.TemplateID) + estimateVariablesSize(redactedVars),
	}, nil
}

// CompressedLog represents a compressed log entry.
type CompressedLog struct {
	TemplateID     string
	Template       string
	Variables      map[string]string
	Source         string
	Timestamp      int64
	IsNewTemplate  bool
	OriginalSize   int
	CompressedSize int
}

// estimateVariablesSize estimates the storage size of variables.
func estimateVariablesSize(vars map[string]string) int {
	size := 0
	for k, v := range vars {
		size += len(k) + len(v)
	}
	return size
}

// GetStats returns compression statistics.
func (s *CompressionService) GetStats() drain.Stats {
	return s.drainTree.GetStats()
}

// GetTemplates returns all templates.
func (s *CompressionService) GetTemplates() []*drain.LogCluster {
	return s.drainTree.GetAllClusters()
}

// GetTemplate returns a template by ID.
func (s *CompressionService) GetTemplate(id string) (*drain.LogCluster, bool) {
	return s.drainTree.GetCluster(id)
}

// StartHTTPServer starts the HTTP API server.
func (s *CompressionService) StartHTTPServer(ctx context.Context) error {
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy"}`))
	})

	// Stats endpoint
	mux.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		stats := s.GetStats()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"total_clusters":` + string(rune(stats.TotalClusters)) + `,"total_logs":` + string(rune(stats.TotalLogs)) + `}`))
	})

	server := &http.Server{
		Addr:    ":" + s.config.HTTPPort,
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdownCtx)
	}()

	s.logger.Info("Starting HTTP server", zap.String("port", s.config.HTTPPort))
	return server.ListenAndServe()
}

// StartGRPCServer starts the gRPC server.
func (s *CompressionService) StartGRPCServer(ctx context.Context) error {
	listener, err := net.Listen("tcp", ":"+s.config.GRPCPort)
	if err != nil {
		return err
	}

	s.logger.Info("Starting gRPC server", zap.String("port", s.config.GRPCPort))

	// In production, register gRPC service here
	// For now, just listen
	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	// Placeholder - actual gRPC server would be configured here
	<-ctx.Done()
	return nil
}

func main() {
	// Parse flags
	grpcPort := flag.String("grpc-port", "8090", "gRPC server port")
	httpPort := flag.String("http-port", "8091", "HTTP server port")
	metricsPort := flag.String("metrics-port", "8092", "Metrics server port")
	workerCount := flag.Int("workers", 100, "Number of worker goroutines")
	flag.Parse()

	// Initialize logger
	logger, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}
	defer logger.Sync()

	// Create config
	config := Config{
		GRPCPort:    *grpcPort,
		HTTPPort:    *httpPort,
		MetricsPort: *metricsPort,
		WorkerCount: *workerCount,
		DrainConfig: drain.DefaultConfig(),
	}

	// Create service
	service := NewCompressionService(config, logger)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, syscall.SIGINT, syscall.SIGTERM)

	// Start servers
	go func() {
		if err := service.StartHTTPServer(ctx); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server error", zap.Error(err))
		}
	}()

	go func() {
		if err := service.StartGRPCServer(ctx); err != nil {
			logger.Error("gRPC server error", zap.Error(err))
		}
	}()

	logger.Info("Compression service started",
		zap.String("grpc_port", config.GRPCPort),
		zap.String("http_port", config.HTTPPort),
	)

	// Wait for shutdown signal
	<-sigterm
	logger.Info("Shutting down...")
	cancel()
}
