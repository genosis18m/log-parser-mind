// Package drain implements the Drain log parsing algorithm.
// Drain is a tree-based algorithm for online log parsing that extracts
// log templates from raw log messages with high accuracy and speed.
package drain

import (
	"fmt"
	"hash/fnv"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// DrainTree is the main data structure for the Drain algorithm.
// It maintains a tree of clusters for efficient log template matching.
type DrainTree struct {
	root         *ClusterNode
	clusters     map[string]*LogCluster
	mu           sync.RWMutex
	maxDepth     int
	simThreshold float64
	maxChildren  int
	maxClusters  int
	patterns     []*regexp.Regexp
}

// ClusterNode represents a node in the Drain tree.
type ClusterNode struct {
	KeyToChildNode map[string]*ClusterNode
	Clusters       []*LogCluster
	Depth          int
}

// LogCluster represents a group of logs that share the same template.
type LogCluster struct {
	ID         string
	Template   string
	Tokens     []string
	Size       int64
	FirstSeen  int64
	LastSeen   int64
	SampleLogs []string
	mu         sync.Mutex
}

// ParseResult contains the result of parsing a log message.
type ParseResult struct {
	TemplateID string
	Template   string
	Variables  map[string]string
	IsNew      bool
}

// Config holds configuration for the Drain algorithm.
type Config struct {
	MaxDepth       int     // Maximum depth of the parse tree (default: 4)
	SimThreshold   float64 // Similarity threshold for template matching (default: 0.5)
	MaxChildren    int     // Maximum children per node (default: 100)
	MaxClusters    int     // Maximum clusters per leaf node (default: 20)
	MaxSampleLogs  int     // Maximum sample logs to keep per template
	ExtraDelimiter string  // Additional delimiter for tokenization
}

// DefaultConfig returns the default configuration.
func DefaultConfig() Config {
	return Config{
		MaxDepth:      4,
		SimThreshold:  0.5,
		MaxChildren:   100,
		MaxClusters:   20,
		MaxSampleLogs: 5,
	}
}

// NewDrainTree creates a new Drain tree for log parsing.
func NewDrainTree(config Config) *DrainTree {
	if config.MaxDepth == 0 {
		config.MaxDepth = 4
	}
	if config.SimThreshold == 0 {
		config.SimThreshold = 0.5
	}
	if config.MaxChildren == 0 {
		config.MaxChildren = 100
	}
	if config.MaxClusters == 0 {
		config.MaxClusters = 20
	}

	return &DrainTree{
		root: &ClusterNode{
			KeyToChildNode: make(map[string]*ClusterNode),
			Depth:          0,
		},
		clusters:     make(map[string]*LogCluster),
		maxDepth:     config.MaxDepth,
		simThreshold: config.SimThreshold,
		maxChildren:  config.MaxChildren,
		maxClusters:  config.MaxClusters,
		patterns:     compilePatterns(),
	}
}

// compilePatterns compiles regex patterns for variable detection.
func compilePatterns() []*regexp.Regexp {
	patternStrings := []string{
		// IP addresses
		`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`,
		// UUIDs
		`\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b`,
		// Hex strings (8+ chars)
		`\b[0-9a-fA-F]{8,}\b`,
		// Numbers
		`\b\d+\b`,
		// File paths
		`/[^\s]+`,
		// URLs
		`https?://[^\s]+`,
		// Email addresses
		`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`,
	}

	patterns := make([]*regexp.Regexp, 0, len(patternStrings))
	for _, p := range patternStrings {
		if re, err := regexp.Compile(p); err == nil {
			patterns = append(patterns, re)
		}
	}
	return patterns
}

// Parse processes a log message and returns the template ID and extracted variables.
func (dt *DrainTree) Parse(logContent string, timestamp int64) (*ParseResult, error) {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	// Tokenize the log content
	tokens := dt.tokenize(logContent)
	if len(tokens) == 0 {
		return nil, fmt.Errorf("empty log content")
	}

	// Create preprocessed tokens (replace obvious variables)
	processedTokens := dt.preprocessTokens(tokens)

	// Search for matching cluster in tree
	cluster := dt.treeSearch(dt.root, processedTokens, 1)

	isNew := false
	if cluster == nil {
		// Create new cluster
		cluster = dt.createCluster(processedTokens, timestamp)
		isNew = true
	} else {
		// Update existing cluster
		dt.updateCluster(cluster, processedTokens, timestamp)
	}

	// Extract variables
	variables := dt.extractVariables(cluster.Template, logContent)

	return &ParseResult{
		TemplateID: cluster.ID,
		Template:   cluster.Template,
		Variables:  variables,
		IsNew:      isNew,
	}, nil
}

// tokenize splits a log message into tokens.
func (dt *DrainTree) tokenize(content string) []string {
	// Split by whitespace
	tokens := strings.Fields(content)
	return tokens
}

// preprocessTokens replaces obvious variables with wildcards.
func (dt *DrainTree) preprocessTokens(tokens []string) []string {
	result := make([]string, len(tokens))
	for i, token := range tokens {
		if dt.isVariable(token) {
			result[i] = "<*>"
		} else {
			result[i] = token
		}
	}
	return result
}

// isVariable checks if a token is likely a variable.
func (dt *DrainTree) isVariable(token string) bool {
	// Check if it's a pure number
	if _, err := strconv.ParseFloat(token, 64); err == nil {
		return true
	}

	// Check against compiled patterns
	for _, pattern := range dt.patterns {
		if pattern.MatchString(token) {
			return true
		}
	}

	return false
}

// treeSearch traverses the tree to find a matching cluster.
func (dt *DrainTree) treeSearch(node *ClusterNode, tokens []string, depth int) *LogCluster {
	if depth >= dt.maxDepth || depth > len(tokens) {
		return dt.findBestMatch(node.Clusters, tokens)
	}

	// Use length as first level key
	if depth == 1 {
		lengthKey := fmt.Sprintf("len_%d", len(tokens))
		if childNode, exists := node.KeyToChildNode[lengthKey]; exists {
			return dt.treeSearch(childNode, tokens, depth+1)
		}
		return nil
	}

	// Use token as key
	tokenIdx := depth - 2 // Adjust for length-based first level
	if tokenIdx < len(tokens) {
		key := tokens[tokenIdx]

		// Try exact match first
		if childNode, exists := node.KeyToChildNode[key]; exists {
			return dt.treeSearch(childNode, tokens, depth+1)
		}

		// Try wildcard path
		if wildcardNode, exists := node.KeyToChildNode["<*>"]; exists {
			return dt.treeSearch(wildcardNode, tokens, depth+1)
		}
	}

	return dt.findBestMatch(node.Clusters, tokens)
}

// findBestMatch finds the best matching cluster from a list.
func (dt *DrainTree) findBestMatch(clusters []*LogCluster, tokens []string) *LogCluster {
	var bestMatch *LogCluster
	maxSim := 0.0

	for _, cluster := range clusters {
		if len(cluster.Tokens) != len(tokens) {
			continue
		}

		sim := dt.calculateSimilarity(cluster.Tokens, tokens)
		if sim > maxSim && sim >= dt.simThreshold {
			maxSim = sim
			bestMatch = cluster
		}
	}

	return bestMatch
}

// calculateSimilarity computes the similarity between template and log tokens.
func (dt *DrainTree) calculateSimilarity(template, log []string) float64 {
	if len(template) != len(log) {
		return 0.0
	}

	matches := 0
	for i := range template {
		if template[i] == log[i] || template[i] == "<*>" {
			matches++
		}
	}

	return float64(matches) / float64(len(template))
}

// createCluster creates a new log cluster.
func (dt *DrainTree) createCluster(tokens []string, timestamp int64) *LogCluster {
	id := dt.generateClusterID(tokens)
	template := dt.createTemplate(tokens)

	cluster := &LogCluster{
		ID:         id,
		Template:   template,
		Tokens:     make([]string, len(tokens)),
		Size:       1,
		FirstSeen:  timestamp,
		LastSeen:   timestamp,
		SampleLogs: make([]string, 0, 5),
	}
	copy(cluster.Tokens, tokens)

	dt.clusters[id] = cluster
	dt.addToTree(dt.root, cluster, tokens, 1)

	return cluster
}

// generateClusterID creates a unique ID for a cluster.
func (dt *DrainTree) generateClusterID(tokens []string) string {
	h := fnv.New64a()
	h.Write([]byte(strings.Join(tokens, " ")))
	return fmt.Sprintf("tmpl_%x", h.Sum64())
}

// createTemplate creates a template string from tokens.
func (dt *DrainTree) createTemplate(tokens []string) string {
	return strings.Join(tokens, " ")
}

// addToTree adds a cluster to the tree.
func (dt *DrainTree) addToTree(node *ClusterNode, cluster *LogCluster, tokens []string, depth int) {
	if depth >= dt.maxDepth || depth > len(tokens) {
		node.Clusters = append(node.Clusters, cluster)
		return
	}

	var key string
	if depth == 1 {
		key = fmt.Sprintf("len_%d", len(tokens))
	} else {
		tokenIdx := depth - 2
		if tokenIdx < len(tokens) {
			key = tokens[tokenIdx]
		} else {
			node.Clusters = append(node.Clusters, cluster)
			return
		}
	}

	childNode, exists := node.KeyToChildNode[key]
	if !exists {
		childNode = &ClusterNode{
			KeyToChildNode: make(map[string]*ClusterNode),
			Depth:          depth,
		}
		node.KeyToChildNode[key] = childNode
	}

	dt.addToTree(childNode, cluster, tokens, depth+1)
}

// updateCluster updates an existing cluster with a new log.
func (dt *DrainTree) updateCluster(cluster *LogCluster, tokens []string, timestamp int64) {
	cluster.mu.Lock()
	defer cluster.mu.Unlock()

	cluster.Size++
	cluster.LastSeen = timestamp

	// Update template by generalizing differing positions
	newTokens := make([]string, len(cluster.Tokens))
	for i := range cluster.Tokens {
		if i < len(tokens) && cluster.Tokens[i] != tokens[i] {
			newTokens[i] = "<*>"
		} else {
			newTokens[i] = cluster.Tokens[i]
		}
	}
	cluster.Tokens = newTokens
	cluster.Template = strings.Join(newTokens, " ")
}

// extractVariables extracts variable values from a log using the template.
func (dt *DrainTree) extractVariables(template, logContent string) map[string]string {
	templateTokens := strings.Fields(template)
	logTokens := strings.Fields(logContent)
	variables := make(map[string]string)

	varCounter := 0
	for i, token := range templateTokens {
		if token == "<*>" && i < len(logTokens) {
			key := fmt.Sprintf("var_%d", varCounter)
			variables[key] = logTokens[i]
			varCounter++
		}
	}

	return variables
}

// GetCluster returns a cluster by ID.
func (dt *DrainTree) GetCluster(id string) (*LogCluster, bool) {
	dt.mu.RLock()
	defer dt.mu.RUnlock()

	cluster, exists := dt.clusters[id]
	return cluster, exists
}

// GetAllClusters returns all clusters.
func (dt *DrainTree) GetAllClusters() []*LogCluster {
	dt.mu.RLock()
	defer dt.mu.RUnlock()

	clusters := make([]*LogCluster, 0, len(dt.clusters))
	for _, cluster := range dt.clusters {
		clusters = append(clusters, cluster)
	}
	return clusters
}

// ClusterCount returns the number of clusters.
func (dt *DrainTree) ClusterCount() int {
	dt.mu.RLock()
	defer dt.mu.RUnlock()
	return len(dt.clusters)
}

// Stats returns statistics about the drain tree.
type Stats struct {
	TotalClusters int
	TotalLogs     int64
	AverageSize   float64
}

// GetStats returns statistics about the drain tree.
func (dt *DrainTree) GetStats() Stats {
	dt.mu.RLock()
	defer dt.mu.RUnlock()

	var totalLogs int64
	for _, cluster := range dt.clusters {
		totalLogs += cluster.Size
	}

	avgSize := 0.0
	if len(dt.clusters) > 0 {
		avgSize = float64(totalLogs) / float64(len(dt.clusters))
	}

	return Stats{
		TotalClusters: len(dt.clusters),
		TotalLogs:     totalLogs,
		AverageSize:   avgSize,
	}
}
