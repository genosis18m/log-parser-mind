// Package logger provides a structured logging wrapper.
package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Config holds logger configuration.
type Config struct {
	Level       string // debug, info, warn, error
	Development bool
	Encoding    string // json, console
}

// DefaultConfig returns production-ready defaults.
func DefaultConfig() Config {
	return Config{
		Level:       "info",
		Development: false,
		Encoding:    "json",
	}
}

// New creates a new logger with the given configuration.
func New(config Config) (*zap.Logger, error) {
	level := zapcore.InfoLevel
	switch config.Level {
	case "debug":
		level = zapcore.DebugLevel
	case "warn":
		level = zapcore.WarnLevel
	case "error":
		level = zapcore.ErrorLevel
	}

	zapConfig := zap.Config{
		Level:       zap.NewAtomicLevelAt(level),
		Development: config.Development,
		Encoding:    config.Encoding,
		EncoderConfig: zapcore.EncoderConfig{
			TimeKey:        "time",
			LevelKey:       "level",
			NameKey:        "logger",
			CallerKey:      "caller",
			FunctionKey:    zapcore.OmitKey,
			MessageKey:     "msg",
			StacktraceKey:  "stacktrace",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    zapcore.LowercaseLevelEncoder,
			EncodeTime:     zapcore.ISO8601TimeEncoder,
			EncodeDuration: zapcore.SecondsDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		},
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
	}

	return zapConfig.Build()
}

// NewProduction creates a production logger.
func NewProduction() (*zap.Logger, error) {
	return New(DefaultConfig())
}

// NewDevelopment creates a development logger with console output.
func NewDevelopment() (*zap.Logger, error) {
	return New(Config{
		Level:       "debug",
		Development: true,
		Encoding:    "console",
	})
}

// FromEnv creates a logger based on environment variables.
func FromEnv() (*zap.Logger, error) {
	config := DefaultConfig()

	if level := os.Getenv("LOG_LEVEL"); level != "" {
		config.Level = level
	}

	if os.Getenv("LOG_DEV") == "true" {
		config.Development = true
		config.Encoding = "console"
	}

	return New(config)
}

// With adds fields to the logger.
func With(logger *zap.Logger, fields ...zap.Field) *zap.Logger {
	return logger.With(fields...)
}

// Named creates a named sub-logger.
func Named(logger *zap.Logger, name string) *zap.Logger {
	return logger.Named(name)
}
