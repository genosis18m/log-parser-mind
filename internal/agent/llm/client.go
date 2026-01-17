// Package llm provides a client for interacting with Large Language Models.
package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	openai "github.com/sashabaranov/go-openai"
	"go.uber.org/zap"
)

// Config holds LLM client configuration.
type Config struct {
	APIKey      string
	Model       string
	MaxTokens   int
	Temperature float32
	Timeout     time.Duration
	BaseURL     string // Optional: for Azure or local LLMs
}

// DefaultConfig returns default configuration.
func DefaultConfig() Config {
	return Config{
		Model:       "gpt-4",
		MaxTokens:   2000,
		Temperature: 0.3,
		Timeout:     60 * time.Second,
	}
}

// Client wraps the OpenAI client.
type Client struct {
	client *openai.Client
	config Config
	logger *zap.Logger
}

// NewClient creates a new LLM client.
func NewClient(config Config, logger *zap.Logger) *Client {
	clientConfig := openai.DefaultConfig(config.APIKey)
	if config.BaseURL != "" {
		clientConfig.BaseURL = config.BaseURL
	}

	return &Client{
		client: openai.NewClientWithConfig(clientConfig),
		config: config,
		logger: logger,
	}
}

// FixProposal represents a fix proposal from the LLM.
type FixProposal struct {
	RootCause string `json:"root_cause"`
	Fixes     []Fix  `json:"fixes"`
}

// Fix represents a single fix action.
type Fix struct {
	Rank            int      `json:"rank"`
	Description     string   `json:"description"`
	Commands        []string `json:"commands"`
	Risk            string   `json:"risk"` // low, medium, high
	ExpectedOutcome string   `json:"expected_outcome"`
	Confidence      float64  `json:"confidence"`
	Reasoning       string   `json:"reasoning"`
	Prerequisites   []string `json:"prerequisites,omitempty"`
	EstimatedTime   int      `json:"estimated_time_seconds,omitempty"`
}

// GenerateFix generates fix proposals for an issue.
func (c *Client) GenerateFix(ctx context.Context, issueContext, similarExperiences string) (*FixProposal, error) {
	systemPrompt := `You are a DevOps SRE expert analyzing production issues.

Given recent error logs and system context, identify the root cause and propose fixes.

Output valid JSON only:
{
  "root_cause": "Clear description of the root cause",
  "fixes": [
    {
      "rank": 1,
      "description": "Brief description of the fix",
      "commands": ["command1", "command2"],
      "risk": "low|medium|high",
      "expected_outcome": "What should happen after fix",
      "confidence": 0.85,
      "reasoning": "Why this fix should work",
      "prerequisites": ["any prerequisites"],
      "estimated_time_seconds": 120
    }
  ]
}

Rules:
1. Prioritize fixes from past successful experiences if provided
2. Rank fixes by confidence (highest first)
3. Include rollback commands for high-risk fixes
4. Be specific with commands - use actual paths and parameters
5. Maximum 3 fix proposals`

	userPrompt := fmt.Sprintf(`Issue Context:
%s

Similar Past Experiences (if any):
%s

Generate fix proposals in JSON format.`, issueContext, similarExperiences)

	ctx, cancel := context.WithTimeout(ctx, c.config.Timeout)
	defer cancel()

	resp, err := c.client.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Model: c.config.Model,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleSystem,
					Content: systemPrompt,
				},
				{
					Role:    openai.ChatMessageRoleUser,
					Content: userPrompt,
				},
			},
			MaxTokens:   c.config.MaxTokens,
			Temperature: c.config.Temperature,
		},
	)

	if err != nil {
		return nil, fmt.Errorf("OpenAI API error: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response from LLM")
	}

	// Parse JSON response
	content := resp.Choices[0].Message.Content
	content = cleanJSONResponse(content)

	var proposal FixProposal
	if err := json.Unmarshal([]byte(content), &proposal); err != nil {
		c.logger.Error("Failed to parse LLM response",
			zap.String("content", content),
			zap.Error(err),
		)
		return nil, fmt.Errorf("failed to parse LLM response: %w", err)
	}

	return &proposal, nil
}

// AnalysisResult represents the result of log analysis.
type AnalysisResult struct {
	Issues     []Issue `json:"issues"`
	Summary    string  `json:"summary"`
	Severity   string  `json:"severity"`
	Confidence float64 `json:"confidence"`
}

// Issue represents a detected issue.
type Issue struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Severity    string   `json:"severity"` // low, medium, high, critical
	RootCause   string   `json:"root_cause"`
	AffectedBy  []string `json:"affected_templates"`
	Occurrences int      `json:"occurrences"`
}

// AnalyzeLogs analyzes log patterns and identifies issues.
func (c *Client) AnalyzeLogs(ctx context.Context, logPatterns string) (*AnalysisResult, error) {
	systemPrompt := `You are a log analysis expert. Analyze the provided log patterns and identify issues.

Output valid JSON only:
{
  "issues": [
    {
      "title": "Brief title",
      "description": "Detailed description",
      "severity": "low|medium|high|critical",
      "root_cause": "Likely root cause",
      "affected_templates": ["template_id_1"],
      "occurrences": 100
    }
  ],
  "summary": "Overall summary of findings",
  "severity": "highest severity level",
  "confidence": 0.85
}

Focus on:
1. Error patterns and their frequency
2. Correlations between different log types
3. Anomalies in timing or volume
4. Security-related issues`

	userPrompt := fmt.Sprintf(`Analyze these log patterns:

%s

Identify any issues and provide analysis.`, logPatterns)

	ctx, cancel := context.WithTimeout(ctx, c.config.Timeout)
	defer cancel()

	resp, err := c.client.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Model: c.config.Model,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleSystem,
					Content: systemPrompt,
				},
				{
					Role:    openai.ChatMessageRoleUser,
					Content: userPrompt,
				},
			},
			MaxTokens:   c.config.MaxTokens,
			Temperature: c.config.Temperature,
		},
	)

	if err != nil {
		return nil, fmt.Errorf("OpenAI API error: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response from LLM")
	}

	content := resp.Choices[0].Message.Content
	content = cleanJSONResponse(content)

	var result AnalysisResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("failed to parse LLM response: %w", err)
	}

	return &result, nil
}

// GenerateEmbedding generates an embedding for text (for similarity search).
func (c *Client) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	resp, err := c.client.CreateEmbeddings(ctx, openai.EmbeddingRequest{
		Model: openai.AdaEmbeddingV2,
		Input: []string{text},
	})

	if err != nil {
		return nil, fmt.Errorf("embedding API error: %w", err)
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}

	return resp.Data[0].Embedding, nil
}

// cleanJSONResponse extracts JSON from markdown code blocks if present.
func cleanJSONResponse(content string) string {
	content = strings.TrimSpace(content)

	// Remove markdown code blocks
	if strings.HasPrefix(content, "```json") {
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimSuffix(content, "```")
	} else if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSuffix(content, "```")
	}

	return strings.TrimSpace(content)
}

// StreamResponse streams a response token by token.
type StreamHandler func(token string) error

// GenerateFixStream generates fix proposals with streaming.
func (c *Client) GenerateFixStream(ctx context.Context, issueContext string, handler StreamHandler) error {
	systemPrompt := `You are a DevOps SRE expert. Generate fix proposals for the given issue.`

	stream, err := c.client.CreateChatCompletionStream(
		ctx,
		openai.ChatCompletionRequest{
			Model: c.config.Model,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleSystem,
					Content: systemPrompt,
				},
				{
					Role:    openai.ChatMessageRoleUser,
					Content: issueContext,
				},
			},
			MaxTokens:   c.config.MaxTokens,
			Temperature: c.config.Temperature,
			Stream:      true,
		},
	)

	if err != nil {
		return fmt.Errorf("stream error: %w", err)
	}
	defer stream.Close()

	for {
		response, err := stream.Recv()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return fmt.Errorf("stream recv error: %w", err)
		}

		if len(response.Choices) > 0 {
			token := response.Choices[0].Delta.Content
			if err := handler(token); err != nil {
				return err
			}
		}
	}

	return nil
}
