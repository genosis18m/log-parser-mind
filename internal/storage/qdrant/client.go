// Package qdrant provides a client for interacting with Qdrant vector database.
package qdrant

import (
	"context"
	"fmt"

	"go.uber.org/zap"
)

// Config holds Qdrant connection configuration.
type Config struct {
	Host       string
	Port       int
	Collection string
	APIKey     string
}

// DefaultConfig returns default configuration.
func DefaultConfig() Config {
	return Config{
		Host:       "localhost",
		Port:       6333,
		Collection: "experiences",
	}
}

// Client wraps Qdrant connection.
type Client struct {
	config Config
	logger *zap.Logger
}

// NewClient creates a new Qdrant client.
func NewClient(config Config, logger *zap.Logger) (*Client, error) {
	// In production, establish gRPC connection to Qdrant
	return &Client{
		config: config,
		logger: logger,
	}, nil
}

// Experience represents a stored experience.
type Experience struct {
	ID                    string
	IssueSignature        string
	IssueContext          string
	FixApplied            string
	Success               bool
	ResolutionTimeSeconds int
	Vector                []float32
	Metadata              map[string]interface{}
}

// SimilarExperience represents a search result.
type SimilarExperience struct {
	Experience
	Score float32
}

// Store stores an experience with its vector embedding.
func (c *Client) Store(ctx context.Context, exp *Experience) error {
	// In production, upsert point to Qdrant
	c.logger.Debug("Storing experience",
		zap.String("id", exp.ID),
		zap.String("signature", exp.IssueSignature),
	)

	// Placeholder implementation
	// In production:
	// 1. Create point with vector and payload
	// 2. Upsert to collection

	return nil
}

// SearchSimilar finds similar experiences based on vector similarity.
func (c *Client) SearchSimilar(ctx context.Context, queryVector []float32, topK int, onlySuccessful bool) ([]*SimilarExperience, error) {
	// In production, search Qdrant collection
	c.logger.Debug("Searching similar experiences",
		zap.Int("top_k", topK),
		zap.Bool("only_successful", onlySuccessful),
	)

	// Placeholder implementation
	// In production:
	// 1. Build search request with filter
	// 2. Execute search
	// 3. Map results to SimilarExperience

	return []*SimilarExperience{}, nil
}

// Delete removes an experience from the collection.
func (c *Client) Delete(ctx context.Context, id string) error {
	c.logger.Debug("Deleting experience", zap.String("id", id))
	return nil
}

// CreateCollection creates the experiences collection.
func (c *Client) CreateCollection(ctx context.Context, vectorSize int) error {
	c.logger.Info("Creating collection",
		zap.String("name", c.config.Collection),
		zap.Int("vector_size", vectorSize),
	)

	// In production:
	// 1. Check if collection exists
	// 2. Create collection with vector config

	return nil
}

// CollectionInfo returns information about the collection.
type CollectionInfo struct {
	Name        string
	VectorCount int64
	VectorSize  int
}

// GetCollectionInfo returns collection metadata.
func (c *Client) GetCollectionInfo(ctx context.Context) (*CollectionInfo, error) {
	return &CollectionInfo{
		Name:        c.config.Collection,
		VectorCount: 0,
		VectorSize:  1536, // OpenAI ada-002 dimensions
	}, nil
}

// Close closes the connection.
func (c *Client) Close() error {
	return nil
}

// Ping checks the connection.
func (c *Client) Ping(ctx context.Context) error {
	return nil
}

// BatchStore stores multiple experiences at once.
func (c *Client) BatchStore(ctx context.Context, experiences []*Experience) error {
	for _, exp := range experiences {
		if err := c.Store(ctx, exp); err != nil {
			return fmt.Errorf("failed to store experience %s: %w", exp.ID, err)
		}
	}
	return nil
}

// UpdatePayload updates the metadata of an experience.
func (c *Client) UpdatePayload(ctx context.Context, id string, payload map[string]interface{}) error {
	c.logger.Debug("Updating payload", zap.String("id", id))
	return nil
}

// CosineSimilarity calculates cosine similarity between two vectors.
func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}

	var dotProduct, normA, normB float32
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (sqrt(normA) * sqrt(normB))
}

func sqrt(x float32) float32 {
	// Simple Newton-Raphson approximation
	if x <= 0 {
		return 0
	}
	z := x
	for i := 0; i < 10; i++ {
		z = (z + x/z) / 2
	}
	return z
}
