// Package main is the entry point for the ingestion service.
package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/log-zero/log-zero/internal/compression/drain"
	"github.com/log-zero/log-zero/internal/compression/pii"
	"github.com/log-zero/log-zero/internal/pipeline"
	"go.uber.org/zap"
)

// Config holds the service configuration.
type Config struct {
	HTTPPort    string
	WorkerCount int
	BufferSize  int
	DrainConfig drain.Config
}

// IngestionService handles log ingestion.
type IngestionService struct {
	config     Config
	drainTree  *drain.DrainTree
	redactor   *pii.Redactor
	workerPool *pipeline.WorkerPool
	logger     *zap.Logger
}

// NewIngestionService creates a new ingestion service.
func NewIngestionService(ctx context.Context, config Config, logger *zap.Logger) *IngestionService {
	drainTree := drain.NewDrainTree(config.DrainConfig)
	redactor := pii.NewRedactor(pii.DefaultRedactorConfig())

	poolConfig := pipeline.PoolConfig{
		Workers:    config.WorkerCount,
		BufferSize: config.BufferSize,
		Logger:     logger,
	}
	workerPool := pipeline.NewWorkerPool(ctx, poolConfig)

	svc := &IngestionService{
		config:     config,
		drainTree:  drainTree,
		redactor:   redactor,
		workerPool: workerPool,
		logger:     logger,
	}

	// Start worker pool with handler
	workerPool.Start(svc.processLog)

	return svc
}

// processLog is the worker handler for log processing.
func (s *IngestionService) processLog(ctx context.Context, msg *pipeline.Message) (*pipeline.Result, error) {
	timestamp := msg.Timestamp.UnixNano()

	// Parse log using Drain algorithm
	result, err := s.drainTree.Parse(msg.Content, timestamp)
	if err != nil {
		return nil, err
	}

	// Redact PII
	redactedVars := s.redactor.RedactVariables(result.Variables)

	// Create compressed log
	compressed := &CompressedLog{
		LogID:         uuid.New().String(),
		TemplateID:    result.TemplateID,
		Template:      result.Template,
		Variables:     redactedVars,
		Source:        msg.Source,
		Timestamp:     msg.Timestamp,
		OriginalSize:  len(msg.Content),
	}

	// In production, this would be stored to ClickHouse
	s.logger.Debug("Processed log",
		zap.String("template_id", compressed.TemplateID),
		zap.String("source", compressed.Source),
		zap.Bool("new_template", result.IsNew),
	)

	return &pipeline.Result{
		MessageID: msg.ID,
		Success:   true,
		Data:      compressed,
	}, nil
}

// CompressedLog represents a compressed log entry.
type CompressedLog struct {
	LogID        string
	TemplateID   string
	Template     string
	Variables    map[string]string
	Source       string
	Timestamp    time.Time
	OriginalSize int
}

// StartHTTPServer starts the HTTP API server.
func (s *IngestionService) StartHTTPServer(ctx context.Context) error {
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if s.workerPool.IsHealthy() {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"healthy"}`))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"status":"unhealthy"}`))
		}
	})

	// Metrics
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		metrics := s.workerPool.GetMetrics()
		stats := s.drainTree.GetStats()
		w.Header().Set("Content-Type", "application/json")
		response := `{"processed":` + itoa(metrics.Processed) +
			`,"errors":` + itoa(metrics.Errors) +
			`,"dropped":` + itoa(metrics.Dropped) +
			`,"templates":` + itoa(int64(stats.TotalClusters)) +
			`,"total_logs":` + itoa(stats.TotalLogs) + `}`
		w.Write([]byte(response))
	})

	// Ingest endpoint
	mux.HandleFunc("/ingest", s.handleIngest)

	// Batch ingest
	mux.HandleFunc("/ingest/batch", s.handleBatchIngest)

	// Wrap with CORS middleware
	corsHandler := corsMiddleware(mux)

	server := &http.Server{
		Addr:    ":" + s.config.HTTPPort,
		Handler: corsHandler,
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

func (s *IngestionService) handleIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Simple single log ingest (for testing)
	log := r.FormValue("log")
	source := r.FormValue("source")
	if source == "" {
		source = "http"
	}

	if log == "" {
		http.Error(w, "Missing log parameter", http.StatusBadRequest)
		return
	}

	msg := &pipeline.Message{
		ID:        uuid.New().String(),
		Content:   log,
		Source:    source,
		Timestamp: time.Now(),
	}

	if s.workerPool.Submit(msg) {
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(`{"status":"accepted"}`))
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"status":"rejected","reason":"buffer_full"}`))
	}
}

func (s *IngestionService) handleBatchIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// In production, parse JSON body with multiple logs
	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte(`{"status":"batch_accepted"}`))
}

// corsMiddleware adds CORS headers to responses
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		
		next.ServeHTTP(w, r)
	})
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	
	var result []byte
	negative := n < 0
	if negative {
		n = -n
	}
	
	for n > 0 {
		result = append([]byte{byte('0' + n%10)}, result...)
		n /= 10
	}
	
	if negative {
		result = append([]byte{'-'}, result...)
	}
	
	return string(result)
}

// Stop gracefully shuts down the service.
func (s *IngestionService) Stop() {
	s.workerPool.Stop()
	s.logger.Info("Ingestion service stopped")
}

func main() {
	// Parse flags
	httpPort := flag.String("http-port", "8091", "HTTP server port")
	workerCount := flag.Int("workers", 100, "Number of worker goroutines")
	bufferSize := flag.Int("buffer", 10000, "Worker pool buffer size")
	flag.Parse()

	// Initialize logger
	logger, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}
	defer logger.Sync()

	// Create config
	config := Config{
		HTTPPort:    *httpPort,
		WorkerCount: *workerCount,
		BufferSize:  *bufferSize,
		DrainConfig: drain.DefaultConfig(),
	}

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create service
	service := NewIngestionService(ctx, config, logger)

	// Handle shutdown signals
	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, syscall.SIGINT, syscall.SIGTERM)

	// Start HTTP server
	go func() {
		if err := service.StartHTTPServer(ctx); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server error", zap.Error(err))
		}
	}()

	logger.Info("Ingestion service started",
		zap.String("http_port", config.HTTPPort),
		zap.Int("workers", config.WorkerCount),
	)

	// Wait for shutdown signal
	<-sigterm
	logger.Info("Shutting down...")
	cancel()
	service.Stop()
}
