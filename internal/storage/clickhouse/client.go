// Package clickhouse provides a client for interacting with ClickHouse database.
package clickhouse

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"go.uber.org/zap"
)

// Config holds ClickHouse connection configuration.
type Config struct {
	Host     string
	Port     int
	Database string
	Username string
	Password string
	Debug    bool
}

// DefaultConfig returns default configuration.
func DefaultConfig() Config {
	return Config{
		Host:     "localhost",
		Port:     9000,
		Database: "logzero",
		Username: "default",
		Password: "",
	}
}

// Client wraps ClickHouse connection.
type Client struct {
	conn   driver.Conn
	config Config
	logger *zap.Logger
}

// NewClient creates a new ClickHouse client.
func NewClient(config Config, logger *zap.Logger) (*Client, error) {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{fmt.Sprintf("%s:%d", config.Host, config.Port)},
		Auth: clickhouse.Auth{
			Database: config.Database,
			Username: config.Username,
			Password: config.Password,
		},
		Settings: clickhouse.Settings{
			"max_execution_time": 60,
		},
		DialTimeout:     10 * time.Second,
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: time.Hour,
		Debug:           config.Debug,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to connect to ClickHouse: %w", err)
	}

	// Test connection
	if err := conn.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to ping ClickHouse: %w", err)
	}

	return &Client{
		conn:   conn,
		config: config,
		logger: logger,
	}, nil
}

// InitSchema creates the required tables.
func (c *Client) InitSchema(ctx context.Context) error {
	// Create compressed_logs table
	logsTable := `
		CREATE TABLE IF NOT EXISTS compressed_logs (
			log_id UUID,
			timestamp DateTime64(3),
			template_id String,
			source String,
			variables Map(String, String),
			original_size UInt32,
			compressed_size UInt32,
			created_at DateTime DEFAULT now()
		) ENGINE = MergeTree()
		PARTITION BY toYYYYMM(timestamp)
		ORDER BY (source, template_id, timestamp)
		TTL timestamp + INTERVAL 90 DAY
	`
	if err := c.conn.Exec(ctx, logsTable); err != nil {
		return fmt.Errorf("failed to create compressed_logs table: %w", err)
	}

	// Create templates table
	templatesTable := `
		CREATE TABLE IF NOT EXISTS templates (
			template_id String,
			pattern String,
			log_count UInt64,
			first_seen DateTime64(3),
			last_seen DateTime64(3),
			created_at DateTime DEFAULT now()
		) ENGINE = ReplacingMergeTree(last_seen)
		ORDER BY template_id
	`
	if err := c.conn.Exec(ctx, templatesTable); err != nil {
		return fmt.Errorf("failed to create templates table: %w", err)
	}

	// Create aggregation materialized view
	aggregationsView := `
		CREATE MATERIALIZED VIEW IF NOT EXISTS logs_by_template_mv
		ENGINE = SummingMergeTree()
		ORDER BY (source, template_id, hour)
		AS SELECT
			source,
			template_id,
			toStartOfHour(timestamp) as hour,
			count() as log_count,
			sum(original_size) as total_original_size,
			sum(compressed_size) as total_compressed_size
		FROM compressed_logs
		GROUP BY source, template_id, hour
	`
	if err := c.conn.Exec(ctx, aggregationsView); err != nil {
		c.logger.Warn("Failed to create materialized view (may already exist)", zap.Error(err))
	}

	c.logger.Info("ClickHouse schema initialized")
	return nil
}

// CompressedLog represents a compressed log entry.
type CompressedLog struct {
	LogID          string
	Timestamp      time.Time
	TemplateID     string
	Source         string
	Variables      map[string]string
	OriginalSize   uint32
	CompressedSize uint32
}

// InsertLog inserts a compressed log.
func (c *Client) InsertLog(ctx context.Context, log *CompressedLog) error {
	query := `
		INSERT INTO compressed_logs (log_id, timestamp, template_id, source, variables, original_size, compressed_size)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	return c.conn.Exec(ctx, query,
		log.LogID,
		log.Timestamp,
		log.TemplateID,
		log.Source,
		log.Variables,
		log.OriginalSize,
		log.CompressedSize,
	)
}

// InsertLogsBatch inserts multiple logs in a batch.
func (c *Client) InsertLogsBatch(ctx context.Context, logs []*CompressedLog) error {
	batch, err := c.conn.PrepareBatch(ctx, `
		INSERT INTO compressed_logs (log_id, timestamp, template_id, source, variables, original_size, compressed_size)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare batch: %w", err)
	}

	for _, log := range logs {
		if err := batch.Append(
			log.LogID,
			log.Timestamp,
			log.TemplateID,
			log.Source,
			log.Variables,
			log.OriginalSize,
			log.CompressedSize,
		); err != nil {
			return fmt.Errorf("failed to append log: %w", err)
		}
	}

	return batch.Send()
}

// QueryRequest holds query parameters.
type QueryRequest struct {
	TemplateID string
	Source     string
	StartTime  time.Time
	EndTime    time.Time
	Limit      int
	Offset     int
}

// QueryLogs queries compressed logs.
func (c *Client) QueryLogs(ctx context.Context, req *QueryRequest) ([]*CompressedLog, error) {
	query := `
		SELECT log_id, timestamp, template_id, source, variables, original_size, compressed_size
		FROM compressed_logs
		WHERE 1=1
	`
	args := make([]interface{}, 0)

	if req.TemplateID != "" {
		query += " AND template_id = ?"
		args = append(args, req.TemplateID)
	}
	if req.Source != "" {
		query += " AND source = ?"
		args = append(args, req.Source)
	}
	if !req.StartTime.IsZero() {
		query += " AND timestamp >= ?"
		args = append(args, req.StartTime)
	}
	if !req.EndTime.IsZero() {
		query += " AND timestamp <= ?"
		args = append(args, req.EndTime)
	}

	query += " ORDER BY timestamp DESC"

	if req.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", req.Limit)
	}
	if req.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", req.Offset)
	}

	rows, err := c.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var logs []*CompressedLog
	for rows.Next() {
		var log CompressedLog
		if err := rows.Scan(
			&log.LogID,
			&log.Timestamp,
			&log.TemplateID,
			&log.Source,
			&log.Variables,
			&log.OriginalSize,
			&log.CompressedSize,
		); err != nil {
			return nil, fmt.Errorf("scan failed: %w", err)
		}
		logs = append(logs, &log)
	}

	return logs, nil
}

// GetCompressionStats returns compression statistics.
type CompressionStats struct {
	TotalLogs           int64
	UniqueTemplates     int64
	TotalOriginalSize   int64
	TotalCompressedSize int64
	CompressionRatio    float64
}

// GetStats retrieves compression statistics.
func (c *Client) GetStats(ctx context.Context) (*CompressionStats, error) {
	query := `
		SELECT
			count() as total_logs,
			uniq(template_id) as unique_templates,
			sum(original_size) as total_original,
			sum(compressed_size) as total_compressed
		FROM compressed_logs
	`

	row := c.conn.QueryRow(ctx, query)

	var stats CompressionStats
	if err := row.Scan(
		&stats.TotalLogs,
		&stats.UniqueTemplates,
		&stats.TotalOriginalSize,
		&stats.TotalCompressedSize,
	); err != nil {
		return nil, fmt.Errorf("failed to get stats: %w", err)
	}

	if stats.TotalOriginalSize > 0 {
		stats.CompressionRatio = float64(stats.TotalCompressedSize) / float64(stats.TotalOriginalSize)
	}

	return &stats, nil
}

// Close closes the connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

// Ping checks the connection.
func (c *Client) Ping(ctx context.Context) error {
	return c.conn.Ping(ctx)
}
