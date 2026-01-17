// Package pii provides utilities for redacting personally identifiable information from log data.
package pii

import (
	"regexp"
	"strings"
)

// Redactor handles PII redaction in log content.
type Redactor struct {
	patterns map[string]*regexp.Regexp
	enabled  bool
}

// RedactorConfig configures which PII types to redact.
type RedactorConfig struct {
	RedactEmails      bool
	RedactPhones      bool
	RedactSSN         bool
	RedactCreditCards bool
	RedactIPv4        bool
	RedactIPv6        bool
	CustomPatterns    map[string]string
}

// DefaultRedactorConfig returns a configuration that redacts common PII.
func DefaultRedactorConfig() RedactorConfig {
	return RedactorConfig{
		RedactEmails:      true,
		RedactPhones:      true,
		RedactSSN:         true,
		RedactCreditCards: true,
		RedactIPv4:        false, // Often needed for debugging
		RedactIPv6:        false,
	}
}

// NewRedactor creates a new PII redactor with the given configuration.
func NewRedactor(config RedactorConfig) *Redactor {
	patterns := make(map[string]*regexp.Regexp)

	if config.RedactEmails {
		patterns["email"] = regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
	}

	if config.RedactPhones {
		// Matches various phone formats
		patterns["phone"] = regexp.MustCompile(`\b(?:\+?1[-.\s]?)?\(?\d{3}\)?[-.\s]?\d{3}[-.\s]?\d{4}\b`)
	}

	if config.RedactSSN {
		patterns["ssn"] = regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`)
	}

	if config.RedactCreditCards {
		// Matches common credit card formats
		patterns["credit_card"] = regexp.MustCompile(`\b(?:\d{4}[-\s]?){3}\d{4}\b`)
	}

	if config.RedactIPv4 {
		patterns["ipv4"] = regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`)
	}

	if config.RedactIPv6 {
		patterns["ipv6"] = regexp.MustCompile(`\b(?:[0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}\b`)
	}

	// Add custom patterns
	for name, pattern := range config.CustomPatterns {
		if re, err := regexp.Compile(pattern); err == nil {
			patterns[name] = re
		}
	}

	return &Redactor{
		patterns: patterns,
		enabled:  true,
	}
}

// Placeholders for redacted content
var placeholders = map[string]string{
	"email":       "[EMAIL_REDACTED]",
	"phone":       "[PHONE_REDACTED]",
	"ssn":         "[SSN_REDACTED]",
	"credit_card": "[CC_REDACTED]",
	"ipv4":        "[IPV4_REDACTED]",
	"ipv6":        "[IPV6_REDACTED]",
}

// Redact replaces PII in the given text with placeholders.
func (r *Redactor) Redact(text string) string {
	if !r.enabled {
		return text
	}

	result := text
	for piiType, pattern := range r.patterns {
		placeholder := placeholders[piiType]
		if placeholder == "" {
			placeholder = "[REDACTED]"
		}
		result = pattern.ReplaceAllString(result, placeholder)
	}

	return result
}

// RedactVariables redacts PII from a map of variables.
func (r *Redactor) RedactVariables(variables map[string]string) map[string]string {
	if !r.enabled {
		return variables
	}

	result := make(map[string]string, len(variables))
	for key, value := range variables {
		result[key] = r.Redact(value)
	}

	return result
}

// Enable enables PII redaction.
func (r *Redactor) Enable() {
	r.enabled = true
}

// Disable disables PII redaction.
func (r *Redactor) Disable() {
	r.enabled = false
}

// IsEnabled returns whether redaction is enabled.
func (r *Redactor) IsEnabled() bool {
	return r.enabled
}

// DetectPII checks if text contains any PII and returns the types found.
func (r *Redactor) DetectPII(text string) []string {
	var found []string

	for piiType, pattern := range r.patterns {
		if pattern.MatchString(text) {
			found = append(found, piiType)
		}
	}

	return found
}

// Mask partially masks sensitive data instead of fully redacting.
// For example, "john@example.com" becomes "j***@example.com"
func Mask(text string, visibleChars int) string {
	if len(text) <= visibleChars {
		return strings.Repeat("*", len(text))
	}

	visible := text[:visibleChars]
	masked := strings.Repeat("*", len(text)-visibleChars)
	return visible + masked
}

// MaskEmail masks an email address, keeping first char and domain.
func MaskEmail(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return "[INVALID_EMAIL]"
	}

	localPart := parts[0]
	domain := parts[1]

	if len(localPart) <= 1 {
		return localPart + "@" + domain
	}

	masked := string(localPart[0]) + strings.Repeat("*", len(localPart)-1)
	return masked + "@" + domain
}
