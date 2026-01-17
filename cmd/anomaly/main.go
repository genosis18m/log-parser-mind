// Package main is the entry point for the anomaly detection service.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"math"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Config holds the service configuration.
type Config struct {
	HTTPPort        string
	AnomalyWindow   time.Duration
	ErrorThreshold  float64
	VolumeThreshold float64
}

// AnomalyService detects anomalies in log streams.
type AnomalyService struct {
	config     Config
	metrics    *MetricsStore
	alertChan  chan *Alert
	logger     *zap.Logger
}

// MetricsStore holds time-series metrics for anomaly detection.
type MetricsStore struct {
	mu           sync.RWMutex
	errorCounts  map[string][]TimePoint
	volumeCounts map[string][]TimePoint
	baselines    map[string]*Baseline
}

// TimePoint represents a metric at a point in time.
type TimePoint struct {
	Timestamp time.Time
	Value     float64
}

// Baseline represents the expected baseline for a metric.
type Baseline struct {
	Mean   float64
	StdDev float64
	Count  int64
}

// Alert represents a detected anomaly.
type Alert struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"` // "error_spike", "volume_anomaly", "new_pattern"
	Severity    string    `json:"severity"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	TemplateID  string    `json:"template_id,omitempty"`
	Source      string    `json:"source,omitempty"`
	Value       float64   `json:"value"`
	Threshold   float64   `json:"threshold"`
	DetectedAt  time.Time `json:"detected_at"`
}

// NewAnomalyService creates a new anomaly detection service.
func NewAnomalyService(config Config, logger *zap.Logger) *AnomalyService {
	return &AnomalyService{
		config: config,
		metrics: &MetricsStore{
			errorCounts:  make(map[string][]TimePoint),
			volumeCounts: make(map[string][]TimePoint),
			baselines:    make(map[string]*Baseline),
		},
		alertChan: make(chan *Alert, 100),
		logger:    logger,
	}
}

// RecordError records an error occurrence.
func (s *AnomalyService) RecordError(templateID string, timestamp time.Time) {
	s.metrics.mu.Lock()
	defer s.metrics.mu.Unlock()

	s.metrics.errorCounts[templateID] = append(
		s.metrics.errorCounts[templateID],
		TimePoint{Timestamp: timestamp, Value: 1},
	)

	// Check for anomaly
	s.checkErrorAnomaly(templateID)
}

// RecordVolume records log volume.
func (s *AnomalyService) RecordVolume(source string, count float64, timestamp time.Time) {
	s.metrics.mu.Lock()
	defer s.metrics.mu.Unlock()

	s.metrics.volumeCounts[source] = append(
		s.metrics.volumeCounts[source],
		TimePoint{Timestamp: timestamp, Value: count},
	)

	// Check for anomaly
	s.checkVolumeAnomaly(source)
}

func (s *AnomalyService) checkErrorAnomaly(templateID string) {
	points := s.metrics.errorCounts[templateID]
	if len(points) < 10 {
		return
	}

	// Calculate recent rate
	recentCount := 0.0
	cutoff := time.Now().Add(-5 * time.Minute)
	for _, p := range points {
		if p.Timestamp.After(cutoff) {
			recentCount += p.Value
		}
	}

	// Get or create baseline
	baseline, exists := s.metrics.baselines["error:"+templateID]
	if !exists {
		baseline = s.calculateBaseline(points)
		s.metrics.baselines["error:"+templateID] = baseline
	}

	// Check if current rate exceeds threshold
	if baseline.StdDev > 0 {
		zScore := (recentCount - baseline.Mean) / baseline.StdDev
		if zScore > s.config.ErrorThreshold {
			alert := &Alert{
				ID:          uuid.New().String(),
				Type:        "error_spike",
				Severity:    s.getSeverity(zScore),
				Title:       "Error Rate Spike Detected",
				Description: "Error rate for template significantly above baseline",
				TemplateID:  templateID,
				Value:       recentCount,
				Threshold:   baseline.Mean + (baseline.StdDev * s.config.ErrorThreshold),
				DetectedAt:  time.Now(),
			}
			
			select {
			case s.alertChan <- alert:
				s.logger.Warn("Error anomaly detected",
					zap.String("template_id", templateID),
					zap.Float64("z_score", zScore),
				)
			default:
				// Channel full, drop alert
			}
		}
	}
}

func (s *AnomalyService) checkVolumeAnomaly(source string) {
	points := s.metrics.volumeCounts[source]
	if len(points) < 10 {
		return
	}

	// Get recent volume
	recentVolume := 0.0
	cutoff := time.Now().Add(-5 * time.Minute)
	count := 0
	for _, p := range points {
		if p.Timestamp.After(cutoff) {
			recentVolume += p.Value
			count++
		}
	}
	if count > 0 {
		recentVolume /= float64(count)
	}

	// Get or create baseline
	baseline, exists := s.metrics.baselines["volume:"+source]
	if !exists {
		baseline = s.calculateBaseline(points)
		s.metrics.baselines["volume:"+source] = baseline
	}

	// Check if current volume is anomalous (too high or too low)
	if baseline.StdDev > 0 {
		zScore := math.Abs((recentVolume - baseline.Mean) / baseline.StdDev)
		if zScore > s.config.VolumeThreshold {
			anomalyType := "volume_spike"
			if recentVolume < baseline.Mean {
				anomalyType = "volume_drop"
			}

			alert := &Alert{
				ID:          uuid.New().String(),
				Type:        anomalyType,
				Severity:    s.getSeverity(zScore),
				Title:       "Log Volume Anomaly Detected",
				Description: "Log volume significantly different from baseline",
				Source:      source,
				Value:       recentVolume,
				Threshold:   baseline.Mean,
				DetectedAt:  time.Now(),
			}

			select {
			case s.alertChan <- alert:
				s.logger.Warn("Volume anomaly detected",
					zap.String("source", source),
					zap.Float64("z_score", zScore),
				)
			default:
			}
		}
	}
}

func (s *AnomalyService) calculateBaseline(points []TimePoint) *Baseline {
	if len(points) == 0 {
		return &Baseline{Mean: 0, StdDev: 1, Count: 0}
	}

	// Calculate mean
	sum := 0.0
	for _, p := range points {
		sum += p.Value
	}
	mean := sum / float64(len(points))

	// Calculate standard deviation
	sumSquares := 0.0
	for _, p := range points {
		diff := p.Value - mean
		sumSquares += diff * diff
	}
	stdDev := math.Sqrt(sumSquares / float64(len(points)))

	if stdDev == 0 {
		stdDev = 1 // Avoid division by zero
	}

	return &Baseline{
		Mean:   mean,
		StdDev: stdDev,
		Count:  int64(len(points)),
	}
}

func (s *AnomalyService) getSeverity(zScore float64) string {
	if zScore > 5 {
		return "critical"
	} else if zScore > 4 {
		return "high"
	} else if zScore > 3 {
		return "medium"
	}
	return "low"
}

// GetAlerts returns the channel for receiving alerts.
func (s *AnomalyService) GetAlerts() <-chan *Alert {
	return s.alertChan
}

// GetActiveAlerts returns recent alerts.
func (s *AnomalyService) GetActiveAlerts() []*Alert {
	// In production, this would query a database
	return []*Alert{}
}

// StartHTTPServer starts the HTTP API server.
func (s *AnomalyService) StartHTTPServer(ctx context.Context) error {
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy"}`))
	})

	// Record error
	mux.HandleFunc("/record/error", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			TemplateID string `json:"template_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		s.RecordError(req.TemplateID, time.Now())
		w.WriteHeader(http.StatusAccepted)
	})

	// Record volume
	mux.HandleFunc("/record/volume", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Source string  `json:"source"`
			Count  float64 `json:"count"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		s.RecordVolume(req.Source, req.Count, time.Now())
		w.WriteHeader(http.StatusAccepted)
	})

	// Get alerts
	mux.HandleFunc("/alerts", func(w http.ResponseWriter, r *http.Request) {
		alerts := s.GetActiveAlerts()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"alerts": alerts,
		})
	})

	// Get baselines
	mux.HandleFunc("/baselines", func(w http.ResponseWriter, r *http.Request) {
		s.metrics.mu.RLock()
		defer s.metrics.mu.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(s.metrics.baselines)
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

// StartAlertProcessor processes alerts.
func (s *AnomalyService) StartAlertProcessor(ctx context.Context) {
	for {
		select {
		case alert := <-s.alertChan:
			s.logger.Info("Processing alert",
				zap.String("id", alert.ID),
				zap.String("type", alert.Type),
				zap.String("severity", alert.Severity),
			)
			// In production, send to notification system
		case <-ctx.Done():
			return
		}
	}
}

func main() {
	// Parse flags
	httpPort := flag.String("http-port", "8100", "HTTP server port")
	errorThreshold := flag.Float64("error-threshold", 3.0, "Error rate z-score threshold")
	volumeThreshold := flag.Float64("volume-threshold", 3.0, "Volume z-score threshold")
	flag.Parse()

	// Initialize logger
	logger, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}
	defer logger.Sync()

	// Create config
	config := Config{
		HTTPPort:        *httpPort,
		AnomalyWindow:   5 * time.Minute,
		ErrorThreshold:  *errorThreshold,
		VolumeThreshold: *volumeThreshold,
	}

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create service
	service := NewAnomalyService(config, logger)

	// Handle shutdown signals
	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, syscall.SIGINT, syscall.SIGTERM)

	// Start alert processor
	go service.StartAlertProcessor(ctx)

	// Start HTTP server
	go func() {
		if err := service.StartHTTPServer(ctx); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server error", zap.Error(err))
		}
	}()

	logger.Info("Anomaly detection service started",
		zap.String("http_port", config.HTTPPort),
		zap.Float64("error_threshold", config.ErrorThreshold),
	)

	// Wait for shutdown signal
	<-sigterm
	logger.Info("Shutting down...")
	cancel()
}
