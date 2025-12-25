package config

import (
	"fmt"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// NewLogger creates a new zap logger based on the LOG_LEVEL environment variable.
// Valid levels: debug, info, warn, error.
// Default: info.
func NewLogger() (*zap.Logger, error) {
	levelStr := os.Getenv("LOG_LEVEL")
	if levelStr == "" {
		levelStr = "info"
	}

	var level zapcore.Level
	err := level.UnmarshalText([]byte(levelStr))
	if err != nil {
		return nil, fmt.Errorf("invalid log level %q: %w", levelStr, err)
	}

	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(level)
	config.Encoding = "json"
	config.EncoderConfig.TimeKey = "timestamp"
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	logger, err := config.Build()
	if err != nil {
		return nil, fmt.Errorf("build logger: %w", err)
	}

	return logger, nil
}
