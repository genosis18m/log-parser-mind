// Package main is the entry point for the agent service.
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
	"github.com/log-zero/log-zero/internal/agent/llm"
	"go.uber.org/zap"
)

// Config holds the service configuration.
type Config struct {
	HTTPPort  string
	GRPCPort  string
	OpenAIKey string
	Model     string
}

// AgentService handles log analysis and fix proposals.
type AgentService struct {
	config    Config
	llmClient *llm.Client
	logger    *zap.Logger
}

// NewAgentService creates a new agent service.
func NewAgentService(config Config, logger *zap.Logger) *AgentService {
	llmConfig := llm.Config{
		APIKey:      config.OpenAIKey,
		Model:       config.Model,
		MaxTokens:   2000,
		Temperature: 0.3,
		Timeout:     60 * time.Second,
	}

	return &AgentService{
		config:    config,
		llmClient: llm.NewClient(llmConfig, logger),
		logger:    logger,
	}
}

// AnalyzeRequest represents an analysis request.
type AnalyzeRequest struct {
	TemplateIDs []string `json:"template_ids"`
	TimeRange   string   `json:"time_range"`
	Source      string   `json:"source"`
	LogPatterns string   `json:"log_patterns"`
}

// AnalyzeResponse represents an analysis response.
type AnalyzeResponse struct {
	RequestID  string        `json:"request_id"`
	Issues     []Issue       `json:"issues"`
	Summary    string        `json:"summary"`
	Severity   string        `json:"severity"`
	Confidence float64       `json:"confidence"`
	Proposals  []FixProposal `json:"proposals,omitempty"`
}

// Issue represents a detected issue.
type Issue struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Severity    string   `json:"severity"`
	RootCause   string   `json:"root_cause"`
	Templates   []string `json:"affected_templates"`
	Occurrences int      `json:"occurrences"`
}

// FixProposal represents a fix proposal.
type FixProposal struct {
	ID              string   `json:"id"`
	Rank            int      `json:"rank"`
	Description     string   `json:"description"`
	Commands        []string `json:"commands"`
	Risk            string   `json:"risk"`
	ExpectedOutcome string   `json:"expected_outcome"`
	Confidence      float64  `json:"confidence"`
}

// GenerateFixRequest represents a fix generation request.
type GenerateFixRequest struct {
	IssueID       string `json:"issue_id"`
	IssueContext  string `json:"issue_context"`
	SystemContext string `json:"system_context"`
}

// Analyze analyzes log patterns and identifies issues.
func (s *AgentService) Analyze(ctx context.Context, req *AnalyzeRequest) (*AnalyzeResponse, error) {
	result, err := s.llmClient.AnalyzeLogs(ctx, req.LogPatterns)
	if err != nil {
		return nil, err
	}

	response := &AnalyzeResponse{
		RequestID:  uuid.New().String(),
		Summary:    result.Summary,
		Severity:   result.Severity,
		Confidence: result.Confidence,
	}

	for _, issue := range result.Issues {
		response.Issues = append(response.Issues, Issue{
			ID:          uuid.New().String(),
			Title:       issue.Title,
			Description: issue.Description,
			Severity:    issue.Severity,
			RootCause:   issue.RootCause,
			Templates:   issue.AffectedBy,
			Occurrences: issue.Occurrences,
		})
	}

	return response, nil
}

// GenerateFix generates fix proposals for an issue.
func (s *AgentService) GenerateFix(ctx context.Context, req *GenerateFixRequest) ([]FixProposal, error) {
	result, err := s.llmClient.GenerateFix(ctx, req.IssueContext, "")
	if err != nil {
		return nil, err
	}

	var proposals []FixProposal
	for _, fix := range result.Fixes {
		proposals = append(proposals, FixProposal{
			ID:              uuid.New().String(),
			Rank:            fix.Rank,
			Description:     fix.Description,
			Commands:        fix.Commands,
			Risk:            fix.Risk,
			ExpectedOutcome: fix.ExpectedOutcome,
			Confidence:      fix.Confidence,
		})
	}

	return proposals, nil
}

// StartHTTPServer starts the HTTP API server.
func (s *AgentService) StartHTTPServer(ctx context.Context) error {
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy"}`))
	})

	// Analyze endpoint
	mux.HandleFunc("/analyze", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req AnalyzeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		response, err := s.Analyze(r.Context(), &req)
		if err != nil {
			s.logger.Error("Analysis failed", zap.Error(err))
			http.Error(w, "Analysis failed", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	// Generate fix endpoint
	mux.HandleFunc("/fix", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req GenerateFixRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		proposals, err := s.GenerateFix(r.Context(), &req)
		if err != nil {
			s.logger.Error("Fix generation failed", zap.Error(err))
			http.Error(w, "Fix generation failed", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"proposals": proposals,
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
	httpPort := flag.String("http-port", "8110", "HTTP server port")
	grpcPort := flag.String("grpc-port", "8111", "gRPC server port")
	model := flag.String("model", "gpt-4", "LLM model to use")
	flag.Parse()

	// Get API key from environment
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		apiKey = "demo-key" // For testing without API key
	}

	// Initialize logger
	logger, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}
	defer logger.Sync()

	// Create config
	config := Config{
		HTTPPort:  *httpPort,
		GRPCPort:  *grpcPort,
		OpenAIKey: apiKey,
		Model:     *model,
	}

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create service
	service := NewAgentService(config, logger)

	// Handle shutdown signals
	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, syscall.SIGINT, syscall.SIGTERM)

	// Start HTTP server
	go func() {
		if err := service.StartHTTPServer(ctx); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server error", zap.Error(err))
		}
	}()

	logger.Info("Agent service started",
		zap.String("http_port", config.HTTPPort),
		zap.String("model", config.Model),
	)

	// Wait for shutdown signal
	<-sigterm
	logger.Info("Shutting down...")
	cancel()
}
