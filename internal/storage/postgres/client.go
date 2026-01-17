// Package postgres provides a client for interacting with PostgreSQL.
package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// Config holds PostgreSQL connection configuration.
type Config struct {
	Host     string
	Port     int
	Database string
	Username string
	Password string
	MaxConns int
}

// DefaultConfig returns default configuration.
func DefaultConfig() Config {
	return Config{
		Host:     "localhost",
		Port:     5432,
		Database: "logzero",
		Username: "postgres",
		Password: "postgres",
		MaxConns: 10,
	}
}

// Client wraps PostgreSQL connection pool.
type Client struct {
	pool   *pgxpool.Pool
	config Config
	logger *zap.Logger
}

// NewClient creates a new PostgreSQL client.
func NewClient(config Config, logger *zap.Logger) (*Client, error) {
	connStr := fmt.Sprintf(
		"host=%s port=%d dbname=%s user=%s password=%s pool_max_conns=%d",
		config.Host, config.Port, config.Database, config.Username, config.Password, config.MaxConns,
	)

	poolConfig, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	poolConfig.MaxConns = int32(config.MaxConns)
	poolConfig.MinConns = 2
	poolConfig.MaxConnLifetime = time.Hour
	poolConfig.MaxConnIdleTime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create pool: %w", err)
	}

	// Test connection
	if err := pool.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to ping PostgreSQL: %w", err)
	}

	return &Client{
		pool:   pool,
		config: config,
		logger: logger,
	}, nil
}

// InitSchema creates the required tables.
func (c *Client) InitSchema(ctx context.Context) error {
	// Create experiences table
	experiencesTable := `
		CREATE TABLE IF NOT EXISTS experiences (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			issue_signature TEXT NOT NULL,
			issue_context TEXT,
			fix_applied TEXT NOT NULL,
			commands_executed TEXT[],
			success BOOLEAN NOT NULL,
			resolution_time_seconds INTEGER,
			feedback_score REAL DEFAULT 0,
			times_referenced INTEGER DEFAULT 0,
			metadata JSONB,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		);
		
		CREATE INDEX IF NOT EXISTS idx_experiences_issue_signature ON experiences(issue_signature);
		CREATE INDEX IF NOT EXISTS idx_experiences_success ON experiences(success);
		CREATE INDEX IF NOT EXISTS idx_experiences_created_at ON experiences(created_at);
	`
	if _, err := c.pool.Exec(ctx, experiencesTable); err != nil {
		return fmt.Errorf("failed to create experiences table: %w", err)
	}

	// Create alerts table
	alertsTable := `
		CREATE TABLE IF NOT EXISTS alerts (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			issue_id TEXT NOT NULL,
			severity TEXT NOT NULL,
			title TEXT NOT NULL,
			description TEXT,
			source TEXT,
			template_ids TEXT[],
			status TEXT DEFAULT 'open',
			assigned_to TEXT,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			resolved_at TIMESTAMP WITH TIME ZONE,
			metadata JSONB
		);
		
		CREATE INDEX IF NOT EXISTS idx_alerts_status ON alerts(status);
		CREATE INDEX IF NOT EXISTS idx_alerts_severity ON alerts(severity);
		CREATE INDEX IF NOT EXISTS idx_alerts_created_at ON alerts(created_at);
	`
	if _, err := c.pool.Exec(ctx, alertsTable); err != nil {
		return fmt.Errorf("failed to create alerts table: %w", err)
	}

	// Create fix_history table
	fixHistoryTable := `
		CREATE TABLE IF NOT EXISTS fix_history (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			alert_id UUID REFERENCES alerts(id),
			experience_id UUID REFERENCES experiences(id),
			proposal_id TEXT NOT NULL,
			commands_executed TEXT[],
			status TEXT NOT NULL,
			output TEXT,
			error_message TEXT,
			executed_by TEXT,
			executed_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			duration_ms INTEGER
		);
		
		CREATE INDEX IF NOT EXISTS idx_fix_history_alert_id ON fix_history(alert_id);
		CREATE INDEX IF NOT EXISTS idx_fix_history_status ON fix_history(status);
	`
	if _, err := c.pool.Exec(ctx, fixHistoryTable); err != nil {
		return fmt.Errorf("failed to create fix_history table: %w", err)
	}

	c.logger.Info("PostgreSQL schema initialized")
	return nil
}

// Experience represents a learning experience.
type Experience struct {
	ID                    string
	IssueSignature        string
	IssueContext          string
	FixApplied            string
	CommandsExecuted      []string
	Success               bool
	ResolutionTimeSeconds int
	FeedbackScore         float64
	TimesReferenced       int
	Metadata              map[string]interface{}
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// CreateExperience stores a new experience.
func (c *Client) CreateExperience(ctx context.Context, exp *Experience) error {
	query := `
		INSERT INTO experiences (issue_signature, issue_context, fix_applied, commands_executed, success, resolution_time_seconds, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`
	return c.pool.QueryRow(ctx, query,
		exp.IssueSignature,
		exp.IssueContext,
		exp.FixApplied,
		exp.CommandsExecuted,
		exp.Success,
		exp.ResolutionTimeSeconds,
		exp.Metadata,
	).Scan(&exp.ID)
}

// GetExperience retrieves an experience by ID.
func (c *Client) GetExperience(ctx context.Context, id string) (*Experience, error) {
	query := `
		SELECT id, issue_signature, issue_context, fix_applied, commands_executed, 
			   success, resolution_time_seconds, feedback_score, times_referenced, 
			   metadata, created_at, updated_at
		FROM experiences
		WHERE id = $1
	`

	var exp Experience
	err := c.pool.QueryRow(ctx, query, id).Scan(
		&exp.ID,
		&exp.IssueSignature,
		&exp.IssueContext,
		&exp.FixApplied,
		&exp.CommandsExecuted,
		&exp.Success,
		&exp.ResolutionTimeSeconds,
		&exp.FeedbackScore,
		&exp.TimesReferenced,
		&exp.Metadata,
		&exp.CreatedAt,
		&exp.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get experience: %w", err)
	}

	return &exp, nil
}

// ListExperiences retrieves a list of experiences.
func (c *Client) ListExperiences(ctx context.Context, limit, offset int, onlySuccessful bool) ([]*Experience, error) {
	query := `
		SELECT id, issue_signature, issue_context, fix_applied, commands_executed, 
			   success, resolution_time_seconds, feedback_score, times_referenced, 
			   metadata, created_at, updated_at
		FROM experiences
		WHERE ($1 = false OR success = true)
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := c.pool.Query(ctx, query, onlySuccessful, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list experiences: %w", err)
	}
	defer rows.Close()

	var experiences []*Experience
	for rows.Next() {
		var exp Experience
		if err := rows.Scan(
			&exp.ID,
			&exp.IssueSignature,
			&exp.IssueContext,
			&exp.FixApplied,
			&exp.CommandsExecuted,
			&exp.Success,
			&exp.ResolutionTimeSeconds,
			&exp.FeedbackScore,
			&exp.TimesReferenced,
			&exp.Metadata,
			&exp.CreatedAt,
			&exp.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan experience: %w", err)
		}
		experiences = append(experiences, &exp)
	}

	return experiences, nil
}

// UpdateFeedback updates the feedback score for an experience.
func (c *Client) UpdateFeedback(ctx context.Context, id string, score float64) error {
	query := `
		UPDATE experiences 
		SET feedback_score = $2, updated_at = NOW()
		WHERE id = $1
	`
	_, err := c.pool.Exec(ctx, query, id, score)
	return err
}

// IncrementReferences increments the times_referenced counter.
func (c *Client) IncrementReferences(ctx context.Context, id string) error {
	query := `
		UPDATE experiences 
		SET times_referenced = times_referenced + 1, updated_at = NOW()
		WHERE id = $1
	`
	_, err := c.pool.Exec(ctx, query, id)
	return err
}

// Alert represents an alert/issue.
type Alert struct {
	ID          string
	IssueID     string
	Severity    string
	Title       string
	Description string
	Source      string
	TemplateIDs []string
	Status      string
	AssignedTo  string
	CreatedAt   time.Time
	ResolvedAt  *time.Time
	Metadata    map[string]interface{}
}

// CreateAlert creates a new alert.
func (c *Client) CreateAlert(ctx context.Context, alert *Alert) error {
	query := `
		INSERT INTO alerts (issue_id, severity, title, description, source, template_ids, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`
	return c.pool.QueryRow(ctx, query,
		alert.IssueID,
		alert.Severity,
		alert.Title,
		alert.Description,
		alert.Source,
		alert.TemplateIDs,
		alert.Metadata,
	).Scan(&alert.ID)
}

// ResolveAlert marks an alert as resolved.
func (c *Client) ResolveAlert(ctx context.Context, id string) error {
	query := `UPDATE alerts SET status = 'resolved', resolved_at = NOW() WHERE id = $1`
	_, err := c.pool.Exec(ctx, query, id)
	return err
}

// GetLearningStats retrieves learning statistics.
type LearningStats struct {
	TotalExperiences   int
	SuccessfulFixes    int
	FailedFixes        int
	SuccessRate        float64
	AvgResolutionTime  float64
	TopPatterns        []string
}

// GetLearningStats retrieves learning statistics.
func (c *Client) GetLearningStats(ctx context.Context) (*LearningStats, error) {
	query := `
		SELECT 
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE success = true) as successful,
			COUNT(*) FILTER (WHERE success = false) as failed,
			AVG(resolution_time_seconds) FILTER (WHERE success = true) as avg_resolution
		FROM experiences
	`

	var stats LearningStats
	var avgResolution *float64
	err := c.pool.QueryRow(ctx, query).Scan(
		&stats.TotalExperiences,
		&stats.SuccessfulFixes,
		&stats.FailedFixes,
		&avgResolution,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get learning stats: %w", err)
	}

	if avgResolution != nil {
		stats.AvgResolutionTime = *avgResolution
	}
	if stats.TotalExperiences > 0 {
		stats.SuccessRate = float64(stats.SuccessfulFixes) / float64(stats.TotalExperiences)
	}

	return &stats, nil
}

// Close closes the connection pool.
func (c *Client) Close() {
	c.pool.Close()
}

// Ping checks the connection.
func (c *Client) Ping(ctx context.Context) error {
	return c.pool.Ping(ctx)
}
