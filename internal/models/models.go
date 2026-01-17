// Package models provides shared data models with validation.
package models

import (
	"fmt"
	"regexp"
	"time"
)

// Severity levels for issues
type Severity string

const (
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

// Status for alerts and fixes
type Status string

const (
	StatusOpen         Status = "open"
	StatusAcknowledged Status = "acknowledged"
	StatusInProgress   Status = "in_progress"
	StatusResolved     Status = "resolved"
	StatusClosed       Status = "closed"
)

// RiskLevel for fix proposals
type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

// CompressedLog represents a compressed log entry.
type CompressedLog struct {
	LogID          string            `json:"log_id"`
	TemplateID     string            `json:"template_id"`
	Template       string            `json:"template"`
	Timestamp      time.Time         `json:"timestamp"`
	Source         string            `json:"source"`
	Variables      map[string]string `json:"variables"`
	OriginalSize   int               `json:"original_size"`
	CompressedSize int               `json:"compressed_size"`
}

// Validate validates the compressed log.
func (c *CompressedLog) Validate() error {
	if c.TemplateID == "" {
		return fmt.Errorf("template_id is required")
	}
	if c.Source == "" {
		return fmt.Errorf("source is required")
	}
	return nil
}

// Template represents a log template.
type Template struct {
	ID        string    `json:"id"`
	Pattern   string    `json:"pattern"`
	LogCount  int64     `json:"log_count"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
}

// Alert represents a detected issue.
type Alert struct {
	ID            string                 `json:"id"`
	IssueID       string                 `json:"issue_id"`
	Title         string                 `json:"title"`
	Description   string                 `json:"description"`
	Severity      Severity               `json:"severity"`
	Status        Status                 `json:"status"`
	Source        string                 `json:"source"`
	TemplateIDs   []string               `json:"template_ids"`
	AssignedTo    string                 `json:"assigned_to,omitempty"`
	CreatedAt     time.Time              `json:"created_at"`
	AcknowledgedAt *time.Time            `json:"acknowledged_at,omitempty"`
	ResolvedAt    *time.Time             `json:"resolved_at,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// Validate validates the alert.
func (a *Alert) Validate() error {
	if a.Title == "" {
		return fmt.Errorf("title is required")
	}
	if !isValidSeverity(a.Severity) {
		return fmt.Errorf("invalid severity: %s", a.Severity)
	}
	return nil
}

// Experience represents a learning experience from past fixes.
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
	Metadata              map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt             time.Time              `json:"created_at"`
	UpdatedAt             time.Time              `json:"updated_at"`
}

// Validate validates the experience.
func (e *Experience) Validate() error {
	if e.IssueSignature == "" {
		return fmt.Errorf("issue_signature is required")
	}
	if e.FixApplied == "" {
		return fmt.Errorf("fix_applied is required")
	}
	if e.FeedbackScore < 0 || e.FeedbackScore > 5 {
		return fmt.Errorf("feedback_score must be between 0 and 5")
	}
	return nil
}

// FixProposal represents a proposed fix for an issue.
type FixProposal struct {
	ID              string    `json:"id"`
	Rank            int       `json:"rank"`
	Description     string    `json:"description"`
	Commands        []string  `json:"commands"`
	Risk            RiskLevel `json:"risk"`
	ExpectedOutcome string    `json:"expected_outcome"`
	Confidence      float64   `json:"confidence"`
	Reasoning       string    `json:"reasoning,omitempty"`
	Prerequisites   []string  `json:"prerequisites,omitempty"`
	EstimatedTime   int       `json:"estimated_time_seconds,omitempty"`
}

// Validate validates the fix proposal.
func (f *FixProposal) Validate() error {
	if f.Description == "" {
		return fmt.Errorf("description is required")
	}
	if len(f.Commands) == 0 {
		return fmt.Errorf("at least one command is required")
	}
	if f.Confidence < 0 || f.Confidence > 1 {
		return fmt.Errorf("confidence must be between 0 and 1")
	}
	return nil
}

// AnalysisResult represents the result of log analysis.
type AnalysisResult struct {
	RequestID  string   `json:"request_id"`
	Issues     []Issue  `json:"issues"`
	Summary    string   `json:"summary"`
	Severity   Severity `json:"severity"`
	Confidence float64  `json:"confidence"`
}

// Issue represents a detected issue from analysis.
type Issue struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Severity    Severity `json:"severity"`
	RootCause   string   `json:"root_cause"`
	Templates   []string `json:"affected_templates"`
	Occurrences int      `json:"occurrences"`
}

// Metrics represents sustainability metrics.
type Metrics struct {
	StorageSavedGB       float64 `json:"storage_saved_gb"`
	CO2SavedKG           float64 `json:"co2_saved_kg"`
	EnergySavedKWH       float64 `json:"energy_saved_kwh"`
	CostSavedUSD         float64 `json:"cost_saved_usd"`
	CompressionRatio     float64 `json:"compression_ratio"`
	LogsProcessed        int64   `json:"logs_processed"`
	Period               string  `json:"period"`
}

// LearningStats represents learning statistics.
type LearningStats struct {
	TotalExperiences   int     `json:"total_experiences"`
	SuccessfulFixes    int     `json:"successful_fixes"`
	FailedFixes        int     `json:"failed_fixes"`
	SuccessRate        float64 `json:"success_rate"`
	AvgResolutionTime  float64 `json:"avg_resolution_time_seconds"`
	MTTRImprovement    float64 `json:"mttr_improvement_percent"`
}

// Helper functions

func isValidSeverity(s Severity) bool {
	switch s {
	case SeverityLow, SeverityMedium, SeverityHigh, SeverityCritical:
		return true
	}
	return false
}

// IsValidEmail checks if an email is valid.
func IsValidEmail(email string) bool {
	pattern := `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`
	matched, _ := regexp.MatchString(pattern, email)
	return matched
}

// IsValidUUID checks if a string is a valid UUID.
func IsValidUUID(u string) bool {
	pattern := `^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`
	matched, _ := regexp.MatchString(pattern, u)
	return matched
}
