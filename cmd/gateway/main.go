// Package main is the entry point for the API gateway.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/websocket/v2"
	"go.uber.org/zap"
)

// Config holds the gateway configuration.
type Config struct {
	Port               string
	CompressionService string
	IngestionService   string
	AgentService       string
	ExperienceService  string
}

// Gateway is the API gateway server.
type Gateway struct {
	app    *fiber.App
	config Config
	logger *zap.Logger
}

// NewGateway creates a new API gateway.
func NewGateway(config Config, log *zap.Logger) *Gateway {
	app := fiber.New(fiber.Config{
		ServerHeader: "Log-Zero",
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	})

	// Middleware
	app.Use(recover.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,PUT,DELETE,OPTIONS",
		AllowHeaders: "Origin,Content-Type,Accept,Authorization",
	}))
	app.Use(logger.New(logger.Config{
		Format:     "${time} | ${status} | ${latency} | ${method} ${path}\n",
		TimeFormat: "2006-01-02 15:04:05",
	}))

	return &Gateway{
		app:    app,
		config: config,
		logger: log,
	}
}

// SetupRoutes configures all API routes.
func (g *Gateway) SetupRoutes() {
	// Health check
	g.app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":  "healthy",
			"service": "gateway",
			"time":    time.Now().Format(time.RFC3339),
		})
	})

	// API v1 group
	api := g.app.Group("/api/v1")

	// Logs endpoints
	api.Post("/logs/upload", g.handleLogUpload)
	api.Get("/logs/query", g.handleLogQuery)
	api.Get("/logs/templates", g.handleGetTemplates)
	api.Get("/logs/stats", g.handleGetStats)

	// Agent endpoints
	api.Post("/agent/analyze", g.handleAnalyze)
	api.Post("/agent/fix", g.handleGenerateFix)

	// Experience endpoints
	api.Post("/experiences", g.handleStoreExperience)
	api.Get("/experiences", g.handleListExperiences)
	api.Get("/experiences/search", g.handleSearchExperiences)
	api.Post("/experiences/feedback", g.handleSubmitFeedback)
	api.Get("/experiences/stats", g.handleGetLearningStats)

	// Metrics endpoints
	api.Get("/metrics/sustainability", g.handleSustainabilityMetrics)
	api.Get("/metrics/mttr", g.handleMTTRMetrics)

	// WebSocket for live updates
	g.app.Get("/ws", websocket.New(g.handleWebSocket))

	// Static files for frontend (if any)
	g.app.Static("/", "./web/build")
}

// Logs handlers

func (g *Gateway) handleLogUpload(c *fiber.Ctx) error {
	// Forward to ingestion service
	resp, err := g.proxyRequest("POST", g.config.IngestionService+"/ingest", c.Body())
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error": "Failed to upload logs",
		})
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	return c.Status(resp.StatusCode).Send(body)
}

func (g *Gateway) handleLogQuery(c *fiber.Ctx) error {
	templateID := c.Query("template_id")
	source := c.Query("source")
	startTime := c.Query("start_time")
	endTime := c.Query("end_time")
	limit := c.Query("limit", "100")

	// In production, forward to compression service
	return c.JSON(fiber.Map{
		"logs": []fiber.Map{},
		"query": fiber.Map{
			"template_id": templateID,
			"source":      source,
			"start_time":  startTime,
			"end_time":    endTime,
			"limit":       limit,
		},
	})
}

func (g *Gateway) handleGetTemplates(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"templates": []fiber.Map{
			{
				"id":        "tmpl_abc123",
				"pattern":   "Error connecting to database at <*>:<*>",
				"log_count": 1500,
				"last_seen": time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
			},
			{
				"id":        "tmpl_def456",
				"pattern":   "Request processed in <*>ms for user <*>",
				"log_count": 50000,
				"last_seen": time.Now().Format(time.RFC3339),
			},
		},
		"total": 2,
	})
}

func (g *Gateway) handleGetStats(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"total_logs":          1000000,
		"unique_templates":    150,
		"compression_ratio":   0.15,
		"bytes_saved":         850000000,
		"logs_per_second":     50000,
		"last_24h_logs":       4320000,
		"error_rate_percent":  0.5,
	})
}

// Agent handlers

func (g *Gateway) handleAnalyze(c *fiber.Ctx) error {
	resp, err := g.proxyRequest("POST", g.config.AgentService+"/analyze", c.Body())
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error": "Analysis service unavailable",
		})
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	return c.Status(resp.StatusCode).Send(body)
}

func (g *Gateway) handleGenerateFix(c *fiber.Ctx) error {
	resp, err := g.proxyRequest("POST", g.config.AgentService+"/fix", c.Body())
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error": "Agent service unavailable",
		})
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	return c.Status(resp.StatusCode).Send(body)
}

// Experience handlers

func (g *Gateway) handleStoreExperience(c *fiber.Ctx) error {
	resp, err := g.proxyRequest("POST", g.config.ExperienceService+"/store", c.Body())
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error": "Experience service unavailable",
		})
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	return c.Status(resp.StatusCode).Send(body)
}

func (g *Gateway) handleListExperiences(c *fiber.Ctx) error {
	resp, err := g.proxyRequest("GET", g.config.ExperienceService+"/list", nil)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error": "Experience service unavailable",
		})
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	return c.Status(resp.StatusCode).Send(body)
}

func (g *Gateway) handleSearchExperiences(c *fiber.Ctx) error {
	signature := c.Query("signature")
	url := g.config.ExperienceService + "/search?signature=" + signature

	resp, err := g.proxyRequest("GET", url, nil)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error": "Experience service unavailable",
		})
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	return c.Status(resp.StatusCode).Send(body)
}

func (g *Gateway) handleSubmitFeedback(c *fiber.Ctx) error {
	resp, err := g.proxyRequest("POST", g.config.ExperienceService+"/feedback", c.Body())
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error": "Experience service unavailable",
		})
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	return c.Status(resp.StatusCode).Send(body)
}

func (g *Gateway) handleGetLearningStats(c *fiber.Ctx) error {
	resp, err := g.proxyRequest("GET", g.config.ExperienceService+"/stats", nil)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error": "Experience service unavailable",
		})
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	return c.Status(resp.StatusCode).Send(body)
}

// Metrics handlers

func (g *Gateway) handleSustainabilityMetrics(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"storage_saved_gb":         85.5,
		"co2_saved_kg":             12.3,
		"energy_saved_kwh":         45.6,
		"cost_saved_usd":           450.00,
		"compression_ratio":        0.15,
		"logs_processed_millions":  150.5,
		"period":                   "last_30_days",
	})
}

func (g *Gateway) handleMTTRMetrics(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"current_mttr_minutes":     15.5,
		"previous_mttr_minutes":    45.2,
		"improvement_percent":      65.7,
		"auto_resolved_count":      45,
		"manual_resolved_count":    12,
		"pending_issues":           3,
		"success_rate":             0.89,
		"period":                   "last_7_days",
	})
}

// WebSocket handler

func (g *Gateway) handleWebSocket(c *websocket.Conn) {
	g.logger.Info("WebSocket connection established")
	defer c.Close()

	// Send initial connection message
	c.WriteJSON(fiber.Map{
		"type":    "connected",
		"message": "Connected to Log-Zero real-time stream",
		"time":    time.Now().Format(time.RFC3339),
	})

	// Simulate real-time log updates
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Send sample update
			update := fiber.Map{
				"type": "log_update",
				"data": fiber.Map{
					"logs_processed": 1000 + time.Now().Second()*100,
					"templates":      150,
					"errors":         5,
				},
				"time": time.Now().Format(time.RFC3339),
			}

			if err := c.WriteJSON(update); err != nil {
				g.logger.Debug("WebSocket write error", zap.Error(err))
				return
			}
		}
	}
}

// Helper functions

func (g *Gateway) proxyRequest(method, url string, body []byte) (*http.Response, error) {
	var req *http.Request
	var err error

	if body != nil {
		req, err = http.NewRequest(method, url, jsonReader(body))
	} else {
		req, err = http.NewRequest(method, url, nil)
	}

	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	return client.Do(req)
}

type jsonBodyReader struct {
	data []byte
	pos  int
}

func jsonReader(data []byte) io.Reader {
	return &jsonBodyReader{data: data}
}

func (r *jsonBodyReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

// Start starts the gateway server.
func (g *Gateway) Start() error {
	return g.app.Listen(":" + g.config.Port)
}

// Shutdown gracefully shuts down the gateway.
func (g *Gateway) Shutdown() error {
	return g.app.Shutdown()
}

func main() {
	// Parse flags
	port := flag.String("port", "8080", "Gateway port")
	compressionSvc := flag.String("compression-svc", "http://localhost:8090", "Compression service URL")
	ingestionSvc := flag.String("ingestion-svc", "http://localhost:8091", "Ingestion service URL")
	agentSvc := flag.String("agent-svc", "http://localhost:8110", "Agent service URL")
	experienceSvc := flag.String("experience-svc", "http://localhost:8120", "Experience service URL")
	flag.Parse()

	// Initialize logger
	zapLogger, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}
	defer zapLogger.Sync()

	// Create config
	config := Config{
		Port:               *port,
		CompressionService: *compressionSvc,
		IngestionService:   *ingestionSvc,
		AgentService:       *agentSvc,
		ExperienceService:  *experienceSvc,
	}

	// Create gateway
	gateway := NewGateway(config, zapLogger)
	gateway.SetupRoutes()

	// Handle shutdown signals
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigterm
		zapLogger.Info("Shutting down gateway...")
		gateway.Shutdown()
	}()

	zapLogger.Info("Starting Log-Zero API Gateway",
		zap.String("port", config.Port),
	)

	// Block on context
	go func() {
		if err := gateway.Start(); err != nil {
			zapLogger.Error("Gateway error", zap.Error(err))
		}
	}()

	<-ctx.Done()
}

// Ensure json import is used
var _ = json.Marshal
