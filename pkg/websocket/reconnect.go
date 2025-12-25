package websocket

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ReconnectConfig holds the configuration for exponential backoff reconnection.
type ReconnectConfig struct {
	InitialDelay      time.Duration
	MaxDelay          time.Duration
	BackoffMultiplier float64
	JitterPercent     float64 // 0.2 = 20%
}

// ReconnectManager handles exponential backoff reconnection with jitter.
type ReconnectManager struct {
	config         ReconnectConfig
	logger         *zap.Logger
	currentBackoff time.Duration
	mu             sync.Mutex
}

// NewReconnectManager creates a new reconnection manager with the specified config.
func NewReconnectManager(cfg ReconnectConfig, logger *zap.Logger) *ReconnectManager {
	return &ReconnectManager{
		config:         cfg,
		logger:         logger,
		currentBackoff: cfg.InitialDelay,
	}
}

// Reconnect attempts to reconnect using the provided connect function with exponential backoff.
func (rm *ReconnectManager) Reconnect(ctx context.Context, connectFunc func(context.Context) error) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Get current backoff duration
		backoff := rm.nextBackoff()

		rm.logger.Info("attempting-reconnection",
			zap.Duration("backoff", backoff))

		ReconnectAttemptsTotal.Inc()

		// Wait for backoff duration or context cancellation
		select {
		case <-time.After(backoff):
			// Continue to connection attempt
		case <-ctx.Done():
			return ctx.Err()
		}

		// Attempt connection
		err := connectFunc(ctx)
		if err == nil {
			// Success - reset backoff
			rm.Reset()
			rm.logger.Info("reconnection-successful")
			return nil
		}

		// Connection failed
		rm.logger.Warn("reconnection-failed", zap.Error(err))
		ReconnectFailuresTotal.Inc()

		// Increment backoff for next attempt
		rm.incrementBackoff()
	}
}

// Reset resets the backoff to the initial delay.
func (rm *ReconnectManager) Reset() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.currentBackoff = rm.config.InitialDelay
}

// nextBackoff returns the current backoff duration with jitter applied.
func (rm *ReconnectManager) nextBackoff() time.Duration {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Apply jitter: backoff * (1.0 + random(0, jitterPercent))
	jitter := rand.Float64() * rm.config.JitterPercent
	backoffFloat := float64(rm.currentBackoff) * (1.0 + jitter)

	return time.Duration(backoffFloat)
}

// incrementBackoff increases the backoff duration by the multiplier.
func (rm *ReconnectManager) incrementBackoff() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Apply backoff multiplier
	newBackoff := time.Duration(float64(rm.currentBackoff) * rm.config.BackoffMultiplier)

	// Cap at max delay
	if newBackoff > rm.config.MaxDelay {
		rm.currentBackoff = rm.config.MaxDelay
	} else {
		rm.currentBackoff = newBackoff
	}
}
