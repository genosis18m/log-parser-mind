// Package main is the entry point for the experience service.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Config holds the service configuration.
type Config struct {
	HTTPPort string
	GRPCPort string
}

// ExperienceService handles learning from past fixes.
type ExperienceService struct {
	config      Config
	experiences map[string]*Experience // In-memory store for demo
	logger      *zap.Logger
}

// Experience represents a learned experience.
type Experience struct {
	ID                    string                 `json:"id"`
	IssueSignature        string                 `json:"issue_signature"`
	IssueContext          string                 `json:"issue_context"`
	FixApplied            string                 `json:"fix_applied"`
	CommandsExecuted      []string               `json:"commands_executed"`
	Success               bool                   `json:"success"`
	ResolutionTimeSeconds int                    `json:"resolution_time_seconds"`
	FeedbackScore         float64                `json:"feedback_score"`
	TimesReferenced       int                    `json:"times_referenced"`
	Metadata              map[string]interface{} `json:"metadata"`
	CreatedAt             time.Time              `json:"created_at"`
}

// NewExperienceService creates a new experience service.
func NewExperienceService(config Config, logger *zap.Logger) *ExperienceService {
	return &ExperienceService{
		config:      config,
		experiences: make(map[string]*Experience),
		logger:      logger,
	}
}

// StoreRequest represents a request to store an experience.
type StoreRequest struct {
	IssueSignature        string                 `json:"issue_signature"`
	IssueContext          string                 `json:"issue_context"`
	FixApplied            string                 `json:"fix_applied"`
	CommandsExecuted      []string               `json:"commands_executed"`
	Success               bool                   `json:"success"`
	ResolutionTimeSeconds int                    `json:"resolution_time_seconds"`
	Metadata              map[string]interface{} `json:"metadata"`
}

// Store stores a new experience.
func (s *ExperienceService) Store(req *StoreRequest) (*Experience, error) {
	exp := &Experience{
		ID:                    uuid.New().String(),
		IssueSignature:        req.IssueSignature,
		IssueContext:          req.IssueContext,
		FixApplied:            req.FixApplied,
		CommandsExecuted:      req.CommandsExecuted,
		Success:               req.Success,
		ResolutionTimeSeconds: req.ResolutionTimeSeconds,
		FeedbackScore:         0,
		TimesReferenced:       0,
		Metadata:              req.Metadata,
		CreatedAt:             time.Now(),
	}

	s.experiences[exp.ID] = exp
	s.logger.Info("Experience stored",
		zap.String("id", exp.ID),
		zap.Bool("success", exp.Success),
	)

	return exp, nil
}

// SearchSimilar finds similar experiences.
func (s *ExperienceService) SearchSimilar(signature string, topK int, onlySuccessful bool) []*Experience {
	var results []*Experience

	for _, exp := range s.experiences {
		if onlySuccessful && !exp.Success {
			continue
		}

		// Simple string matching for demo
		// In production, use vector similarity search
		if contains(exp.IssueSignature, signature) || contains(signature, exp.IssueSignature) {
			results = append(results, exp)
			if len(results) >= topK {
				break
			}
		}
	}

	return results
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && 
		(s == substr || len(s) > len(substr))
}

// GetStats returns learning statistics.
type LearningStats struct {
	TotalExperiences  int     `json:"total_experiences"`
	SuccessfulFixes   int     `json:"successful_fixes"`
	FailedFixes       int     `json:"failed_fixes"`
	SuccessRate       float64 `json:"success_rate"`
	AvgResolutionTime float64 `json:"avg_resolution_time_seconds"`
}

// GetStats returns learning statistics.
func (s *ExperienceService) GetStats() *LearningStats {
	stats := &LearningStats{}

	var totalTime int
	for _, exp := range s.experiences {
		stats.TotalExperiences++
		if exp.Success {
			stats.SuccessfulFixes++
			totalTime += exp.ResolutionTimeSeconds
		} else {
			stats.FailedFixes++
		}
	}

	if stats.TotalExperiences > 0 {
		stats.SuccessRate = float64(stats.SuccessfulFixes) / float64(stats.TotalExperiences)
	}
	if stats.SuccessfulFixes > 0 {
		stats.AvgResolutionTime = float64(totalTime) / float64(stats.SuccessfulFixes)
	}

	return stats
}

// SubmitFeedback updates feedback for an experience.
func (s *ExperienceService) SubmitFeedback(id string, score float64) error {
	exp, ok := s.experiences[id]
	if !ok {
		return nil
	}
	exp.FeedbackScore = score
	return nil
}

// StartHTTPServer starts the HTTP API server.
func (s *ExperienceService) StartHTTPServer(ctx context.Context) error {
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy"}`))
	})

	// Store experience
	mux.HandleFunc("/store", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req StoreRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		exp, err := s.Store(&req)
		if err != nil {
			http.Error(w, "Failed to store experience", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(exp)
	})

	// Search similar experiences
	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		signature := r.URL.Query().Get("signature")
		if signature == "" {
			http.Error(w, "Missing signature parameter", http.StatusBadRequest)
			return
		}

		results := s.SearchSimilar(signature, 5, true)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"experiences": results,
		})
	})

	// Get stats
	mux.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		stats := s.GetStats()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stats)
	})

	// Submit feedback
	mux.HandleFunc("/feedback", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			ID    string  `json:"id"`
			Score float64 `json:"score"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		s.SubmitFeedback(req.ID, req.Score)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"accepted"}`))
	})

	// List experiences
	mux.HandleFunc("/list", func(w http.ResponseWriter, r *http.Request) {
		experiences := make([]*Experience, 0, len(s.experiences))
		for _, exp := range s.experiences {
			experiences = append(experiences, exp)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"experiences": experiences,
			"total":       len(experiences),
		})
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

func main() {
	// Parse flags
	httpPort := flag.String("http-port", "8120", "HTTP server port")
	grpcPort := flag.String("grpc-port", "8121", "gRPC server port")
	flag.Parse()

	// Initialize logger
	logger, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}
	defer logger.Sync()

	// Create config
	config := Config{
		HTTPPort: *httpPort,
		GRPCPort: *grpcPort,
	}

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create service
	service := NewExperienceService(config, logger)

	// Handle shutdown signals
	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, syscall.SIGINT, syscall.SIGTERM)

	// Start HTTP server
	go func() {
		if err := service.StartHTTPServer(ctx); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server error", zap.Error(err))
		}
	}()

	logger.Info("Experience service started",
		zap.String("http_port", config.HTTPPort),
	)

	// Wait for shutdown signal
	<-sigterm
	logger.Info("Shutting down...")
	cancel()
}
