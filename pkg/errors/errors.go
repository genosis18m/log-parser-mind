// Package errors provides custom error types for Log-Zero.
package errors

import (
	"fmt"
)

// Code represents an error code.
type Code string

const (
	CodeNotFound       Code = "NOT_FOUND"
	CodeInvalidInput   Code = "INVALID_INPUT"
	CodeInternal       Code = "INTERNAL_ERROR"
	CodeUnavailable    Code = "UNAVAILABLE"
	CodeRateLimit      Code = "RATE_LIMITED"
	CodeUnauthorized   Code = "UNAUTHORIZED"
	CodeTimeout        Code = "TIMEOUT"
	CodeConflict       Code = "CONFLICT"
)

// Error represents a structured error.
type Error struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
	Cause   error  `json:"-"`
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("[%s] %s: %s", e.Code, e.Message, e.Details)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap returns the underlying error.
func (e *Error) Unwrap() error {
	return e.Cause
}

// New creates a new error.
func New(code Code, message string) *Error {
	return &Error{
		Code:    code,
		Message: message,
	}
}

// WithDetails adds details to the error.
func (e *Error) WithDetails(details string) *Error {
	e.Details = details
	return e
}

// WithCause adds an underlying cause to the error.
func (e *Error) WithCause(err error) *Error {
	e.Cause = err
	return e
}

// Wrap wraps an existing error.
func Wrap(err error, code Code, message string) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Cause:   err,
	}
}

// Common error constructors

// NotFound creates a not found error.
func NotFound(resource string) *Error {
	return New(CodeNotFound, fmt.Sprintf("%s not found", resource))
}

// InvalidInput creates an invalid input error.
func InvalidInput(message string) *Error {
	return New(CodeInvalidInput, message)
}

// Internal creates an internal error.
func Internal(message string) *Error {
	return New(CodeInternal, message)
}

// Unavailable creates an unavailable error.
func Unavailable(service string) *Error {
	return New(CodeUnavailable, fmt.Sprintf("%s is unavailable", service))
}

// RateLimited creates a rate limit error.
func RateLimited() *Error {
	return New(CodeRateLimit, "rate limit exceeded")
}

// Unauthorized creates an unauthorized error.
func Unauthorized() *Error {
	return New(CodeUnauthorized, "unauthorized")
}

// Timeout creates a timeout error.
func Timeout(operation string) *Error {
	return New(CodeTimeout, fmt.Sprintf("%s timed out", operation))
}

// IsCode checks if an error has a specific code.
func IsCode(err error, code Code) bool {
	if e, ok := err.(*Error); ok {
		return e.Code == code
	}
	return false
}

// IsNotFound checks if an error is a not found error.
func IsNotFound(err error) bool {
	return IsCode(err, CodeNotFound)
}

// IsInternal checks if an error is an internal error.
func IsInternal(err error) bool {
	return IsCode(err, CodeInternal)
}
