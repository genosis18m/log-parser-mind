// Package redis provides a client for interacting with Redis.
package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Config holds Redis connection configuration.
type Config struct {
	Host     string
	Port     int
	Password string
	DB       int
}

// DefaultConfig returns default configuration.
func DefaultConfig() Config {
	return Config{
		Host: "localhost",
		Port: 6379,
		DB:   0,
	}
}

// Client wraps Redis connection.
type Client struct {
	client *redis.Client
	config Config
	logger *zap.Logger
}

// NewClient creates a new Redis client.
func NewClient(config Config, logger *zap.Logger) (*Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%d", config.Host, config.Port),
		Password:     config.Password,
		DB:           config.DB,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     10,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &Client{
		client: rdb,
		config: config,
		logger: logger,
	}, nil
}

// Template represents a cached log template.
type Template struct {
	ID        string    `json:"id"`
	Pattern   string    `json:"pattern"`
	LogCount  int64     `json:"log_count"`
	LastSeen  time.Time `json:"last_seen"`
	FirstSeen time.Time `json:"first_seen"`
}

const (
	templateKeyPrefix = "template:"
	templateTTL       = 24 * time.Hour
)

// CacheTemplate stores a template in Redis.
func (c *Client) CacheTemplate(ctx context.Context, template *Template) error {
	key := templateKeyPrefix + template.ID

	data, err := json.Marshal(template)
	if err != nil {
		return fmt.Errorf("failed to marshal template: %w", err)
	}

	return c.client.Set(ctx, key, data, templateTTL).Err()
}

// GetTemplate retrieves a template from Redis.
func (c *Client) GetTemplate(ctx context.Context, templateID string) (*Template, error) {
	key := templateKeyPrefix + templateID

	data, err := c.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, nil // Not found
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get template: %w", err)
	}

	var template Template
	if err := json.Unmarshal(data, &template); err != nil {
		return nil, fmt.Errorf("failed to unmarshal template: %w", err)
	}

	return &template, nil
}

// UpdateTemplateCount increments the log count for a template.
func (c *Client) UpdateTemplateCount(ctx context.Context, templateID string) error {
	key := templateKeyPrefix + templateID + ":count"
	return c.client.Incr(ctx, key).Err()
}

// GetTemplateCount gets the log count for a template.
func (c *Client) GetTemplateCount(ctx context.Context, templateID string) (int64, error) {
	key := templateKeyPrefix + templateID + ":count"
	return c.client.Get(ctx, key).Int64()
}

// Rate limiting
const rateLimitKeyPrefix = "ratelimit:"

// CheckRateLimit checks if a request is within rate limits.
func (c *Client) CheckRateLimit(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	fullKey := rateLimitKeyPrefix + key

	pipe := c.client.Pipeline()
	incr := pipe.Incr(ctx, fullKey)
	pipe.Expire(ctx, fullKey, window)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return false, fmt.Errorf("rate limit check failed: %w", err)
	}

	return incr.Val() <= int64(limit), nil
}

// Pub/Sub for real-time notifications
const alertChannel = "logzero:alerts"

// PublishAlert publishes an alert to subscribers.
func (c *Client) PublishAlert(ctx context.Context, alert interface{}) error {
	data, err := json.Marshal(alert)
	if err != nil {
		return fmt.Errorf("failed to marshal alert: %w", err)
	}

	return c.client.Publish(ctx, alertChannel, data).Err()
}

// SubscribeAlerts subscribes to alert notifications.
func (c *Client) SubscribeAlerts(ctx context.Context) (<-chan *redis.Message, error) {
	pubsub := c.client.Subscribe(ctx, alertChannel)

	// Wait for subscription confirmation
	_, err := pubsub.Receive(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe: %w", err)
	}

	return pubsub.Channel(), nil
}

// Queue operations for background jobs
const jobQueueKey = "logzero:jobs"

// EnqueueJob adds a job to the queue.
func (c *Client) EnqueueJob(ctx context.Context, job interface{}) error {
	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	return c.client.LPush(ctx, jobQueueKey, data).Err()
}

// DequeueJob retrieves a job from the queue (blocking).
func (c *Client) DequeueJob(ctx context.Context, timeout time.Duration) ([]byte, error) {
	result, err := c.client.BRPop(ctx, timeout, jobQueueKey).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to dequeue job: %w", err)
	}

	if len(result) < 2 {
		return nil, nil
	}

	return []byte(result[1]), nil
}

// GetQueueLength returns the number of pending jobs.
func (c *Client) GetQueueLength(ctx context.Context) (int64, error) {
	return c.client.LLen(ctx, jobQueueKey).Result()
}

// Close closes the connection.
func (c *Client) Close() error {
	return c.client.Close()
}

// Ping checks the connection.
func (c *Client) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

// FlushDB flushes the current database (use with caution).
func (c *Client) FlushDB(ctx context.Context) error {
	return c.client.FlushDB(ctx).Err()
}
