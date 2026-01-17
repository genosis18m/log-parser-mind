// Package prompts provides prompt templates for LLM interactions.
package prompts

import (
	"fmt"
	"strings"
	"text/template"
)

// Template represents a prompt template.
type Template struct {
	name     string
	template *template.Template
}

// PromptTemplates holds all available prompt templates.
var PromptTemplates = map[string]string{
	"analyze_logs": `You are a log analysis expert. Analyze the following log patterns and identify issues.

Log Patterns:
{{.LogPatterns}}

Time Range: {{.TimeRange}}
Source: {{.Source}}

Focus on:
1. Error patterns and their frequency
2. Correlations between different log types
3. Anomalies in timing or volume
4. Security-related issues

Provide your analysis in JSON format with the following structure:
{
  "issues": [
    {
      "title": "Brief title",
      "description": "Detailed description",
      "severity": "low|medium|high|critical",
      "root_cause": "Likely root cause"
    }
  ],
  "summary": "Overall summary",
  "confidence": 0.0-1.0
}`,

	"generate_fix": `You are a DevOps SRE expert analyzing production issues.

Issue Context:
{{.IssueContext}}

{{if .SimilarExperiences}}
Similar Past Experiences:
{{.SimilarExperiences}}
{{end}}

{{if .SystemContext}}
Current System State:
{{.SystemContext}}
{{end}}

Generate fix proposals in JSON format:
{
  "root_cause": "Clear description of the root cause",
  "fixes": [
    {
      "rank": 1,
      "description": "Brief description",
      "commands": ["command1", "command2"],
      "risk": "low|medium|high",
      "expected_outcome": "Expected result",
      "confidence": 0.0-1.0,
      "reasoning": "Why this should work"
    }
  ]
}

Rules:
1. Prioritize fixes from past successful experiences
2. Rank by confidence (highest first)
3. Include rollback commands for high-risk fixes
4. Maximum 3 proposals`,

	"root_cause_analysis": `You are an expert at root cause analysis for distributed systems.

Symptoms:
{{.Symptoms}}

Log Patterns:
{{.LogPatterns}}

System Metrics:
{{.Metrics}}

Timeline:
{{.Timeline}}

Perform a thorough root cause analysis. Consider:
1. The 5 Whys methodology
2. Correlation between events
3. Common failure modes
4. Recent changes or deployments

Provide your analysis in JSON format:
{
  "root_cause": "Primary root cause",
  "contributing_factors": ["factor1", "factor2"],
  "evidence": ["evidence1", "evidence2"],
  "confidence": 0.0-1.0,
  "recommendations": ["rec1", "rec2"]
}`,

	"anomaly_detection": `You are an anomaly detection expert for log data.

Baseline Patterns:
{{.BaselinePatterns}}

Current Patterns:
{{.CurrentPatterns}}

Identify any anomalies by comparing current patterns to the baseline.
Look for:
1. Unusual spike in error rates
2. New error types not seen before
3. Changes in log volume or frequency
4. Suspicious patterns (potential security issues)

Output JSON:
{
  "anomalies": [
    {
      "type": "spike|new_pattern|security|other",
      "description": "What was detected",
      "severity": "low|medium|high|critical",
      "affected_patterns": ["pattern1"]
    }
  ],
  "is_anomalous": true|false,
  "confidence": 0.0-1.0
}`,

	"summarize_incident": `Summarize the following incident for a post-mortem report.

Incident Timeline:
{{.Timeline}}

Actions Taken:
{{.Actions}}

Resolution:
{{.Resolution}}

Create a concise incident summary suitable for stakeholder communication.
Include:
1. What happened (1-2 sentences)
2. Impact (duration, affected services)
3. Root cause
4. Fix applied
5. Prevention measures

Keep it under 300 words.`,
}

// AnalyzeLogsData holds data for the analyze_logs template.
type AnalyzeLogsData struct {
	LogPatterns string
	TimeRange   string
	Source      string
}

// GenerateFixData holds data for the generate_fix template.
type GenerateFixData struct {
	IssueContext       string
	SimilarExperiences string
	SystemContext      string
}

// RootCauseData holds data for the root_cause_analysis template.
type RootCauseData struct {
	Symptoms    string
	LogPatterns string
	Metrics     string
	Timeline    string
}

// AnomalyData holds data for the anomaly_detection template.
type AnomalyData struct {
	BaselinePatterns string
	CurrentPatterns  string
}

// IncidentSummaryData holds data for the summarize_incident template.
type IncidentSummaryData struct {
	Timeline   string
	Actions    string
	Resolution string
}

// RenderTemplate renders a prompt template with the given data.
func RenderTemplate(name string, data interface{}) (string, error) {
	templateStr, ok := PromptTemplates[name]
	if !ok {
		return "", fmt.Errorf("template not found: %s", name)
	}

	tmpl, err := template.New(name).Parse(templateStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// BuildAnalyzePrompt builds a prompt for log analysis.
func BuildAnalyzePrompt(logPatterns, timeRange, source string) (string, error) {
	data := AnalyzeLogsData{
		LogPatterns: logPatterns,
		TimeRange:   timeRange,
		Source:      source,
	}
	return RenderTemplate("analyze_logs", data)
}

// BuildFixPrompt builds a prompt for fix generation.
func BuildFixPrompt(issueContext, experiences, systemContext string) (string, error) {
	data := GenerateFixData{
		IssueContext:       issueContext,
		SimilarExperiences: experiences,
		SystemContext:      systemContext,
	}
	return RenderTemplate("generate_fix", data)
}

// BuildRootCausePrompt builds a prompt for root cause analysis.
func BuildRootCausePrompt(symptoms, logPatterns, metrics, timeline string) (string, error) {
	data := RootCauseData{
		Symptoms:    symptoms,
		LogPatterns: logPatterns,
		Metrics:     metrics,
		Timeline:    timeline,
	}
	return RenderTemplate("root_cause_analysis", data)
}

// BuildAnomalyPrompt builds a prompt for anomaly detection.
func BuildAnomalyPrompt(baseline, current string) (string, error) {
	data := AnomalyData{
		BaselinePatterns: baseline,
		CurrentPatterns:  current,
	}
	return RenderTemplate("anomaly_detection", data)
}

// BuildIncidentSummaryPrompt builds a prompt for incident summarization.
func BuildIncidentSummaryPrompt(timeline, actions, resolution string) (string, error) {
	data := IncidentSummaryData{
		Timeline:   timeline,
		Actions:    actions,
		Resolution: resolution,
	}
	return RenderTemplate("summarize_incident", data)
}
