package markets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

// MetadataClient fetches market metadata from the Polymarket CLOB API
type MetadataClient struct {
	baseURL           string
	httpClient        *http.Client
	maxRetries        int
	initialBackoff    time.Duration
	maxBackoff        time.Duration
	backoffMultiplier float64
	logger            *zap.Logger
}

// MetadataClientConfig holds configuration for MetadataClient
type MetadataClientConfig struct {
	MaxRetries        int
	InitialBackoff    time.Duration
	MaxBackoff        time.Duration
	BackoffMultiplier float64
	Logger            *zap.Logger
}

// NewMetadataClient creates a new metadata client with default retry configuration
func NewMetadataClient() *MetadataClient {
	return NewMetadataClientWithConfig(MetadataClientConfig{
		MaxRetries:        3,
		InitialBackoff:    500 * time.Millisecond,
		MaxBackoff:        5 * time.Second,
		BackoffMultiplier: 2.0,
		Logger:            zap.NewNop(),
	})
}

// NewMetadataClientWithConfig creates a new metadata client with custom configuration
func NewMetadataClientWithConfig(cfg MetadataClientConfig) *MetadataClient {
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 3
	}
	if cfg.InitialBackoff == 0 {
		cfg.InitialBackoff = 500 * time.Millisecond
	}
	if cfg.MaxBackoff == 0 {
		cfg.MaxBackoff = 5 * time.Second
	}
	if cfg.BackoffMultiplier == 0 {
		cfg.BackoffMultiplier = 2.0
	}
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop()
	}

	return &MetadataClient{
		baseURL: "https://clob.polymarket.com",
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		maxRetries:        cfg.MaxRetries,
		initialBackoff:    cfg.InitialBackoff,
		maxBackoff:        cfg.MaxBackoff,
		backoffMultiplier: cfg.BackoffMultiplier,
		logger:            cfg.Logger,
	}
}

// isRetryable determines if an error should trigger a retry
func isRetryable(err error) bool {
	if err == nil {
		return false
	}

	errMsg := strings.ToLower(err.Error())

	// Network errors
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	// HTTP errors
	if strings.Contains(errMsg, "429") { // Rate limit
		return true
	}
	if strings.Contains(errMsg, "500") { // Server error
		return true
	}
	if strings.Contains(errMsg, "502") { // Bad gateway
		return true
	}
	if strings.Contains(errMsg, "503") { // Service unavailable
		return true
	}
	if strings.Contains(errMsg, "timeout") {
		return true
	}
	if strings.Contains(errMsg, "connection refused") {
		return true
	}
	if strings.Contains(errMsg, "connection reset") {
		return true
	}

	return false
}

// fetchWithRetry wraps an HTTP fetch operation with retry logic
func (c *MetadataClient) fetchWithRetry(ctx context.Context, operation string, fetchFn func() error) error {
	backoff := c.initialBackoff

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		err := fetchFn()

		if err == nil {
			return nil
		}

		// Check if retryable error
		if !isRetryable(err) {
			return err
		}

		// Last attempt - don't wait
		if attempt == c.maxRetries {
			return fmt.Errorf("max retries (%d) exceeded for %s: %w", c.maxRetries, operation, err)
		}

		// Log retry attempt
		c.logger.Warn("metadata-fetch-failed-retrying",
			zap.String("operation", operation),
			zap.Int("attempt", attempt+1),
			zap.Int("max-retries", c.maxRetries),
			zap.Duration("backoff", backoff),
			zap.Error(err))

		// Wait with backoff
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
			// Continue to next attempt
		}

		// Exponential backoff with cap
		backoff = time.Duration(float64(backoff) * c.backoffMultiplier)
		if backoff > c.maxBackoff {
			backoff = c.maxBackoff
		}
	}

	return fmt.Errorf("unreachable")
}

// FetchTickSize fetches tick size for a token from the CLOB API with retry logic
func (c *MetadataClient) FetchTickSize(ctx context.Context, tokenID string) (tickSize float64, err error) {
	url := fmt.Sprintf("%s/tick-size?token_id=%s", c.baseURL, tokenID)

	err = c.fetchWithRetry(ctx, "fetch-tick-size", func() error {
		req, reqErr := http.NewRequestWithContext(ctx, "GET", url, nil)
		if reqErr != nil {
			return reqErr
		}

		resp, respErr := c.httpClient.Do(req)
		if respErr != nil {
			return respErr
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("API error: status %d", resp.StatusCode)
		}

		var data struct {
			MinimumTickSize float64 `json:"minimum_tick_size"`
		}
		if decodeErr := json.NewDecoder(resp.Body).Decode(&data); decodeErr != nil {
			return decodeErr
		}

		tickSize = data.MinimumTickSize
		return nil
	})

	return tickSize, err
}

// FetchMinOrderSize fetches minimum order size for a token with retry logic
// Tries the orderbook endpoint to find this value
func (c *MetadataClient) FetchMinOrderSize(ctx context.Context, tokenID string) (minOrderSize float64, err error) {
	url := fmt.Sprintf("%s/book?token_id=%s", c.baseURL, tokenID)

	// Default value in case of errors
	minOrderSize = 5.0

	err = c.fetchWithRetry(ctx, "fetch-min-order-size", func() error {
		req, reqErr := http.NewRequestWithContext(ctx, "GET", url, nil)
		if reqErr != nil {
			return reqErr
		}

		resp, respErr := c.httpClient.Do(req)
		if respErr != nil {
			return respErr
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			// Use default on non-200 status
			return nil
		}

		var data struct {
			MinSize float64 `json:"min_size"`
			Market  struct {
				MinSize float64 `json:"minimum_order_size"`
			} `json:"market"`
		}
		if decodeErr := json.NewDecoder(resp.Body).Decode(&data); decodeErr != nil {
			// Use default on parse error
			return nil
		}

		if data.MinSize > 0 {
			minOrderSize = data.MinSize
		} else if data.Market.MinSize > 0 {
			minOrderSize = data.Market.MinSize
		}
		// else use default 5.0

		return nil
	})

	// Even if there's an error, return the default value
	return minOrderSize, nil
}

// FetchTokenMetadata fetches both tick size and min order size for a token
func (c *MetadataClient) FetchTokenMetadata(ctx context.Context, tokenID string) (tickSize, minOrderSize float64, err error) {
	start := time.Now()
	defer func() {
		MetadataFetchDuration.Observe(time.Since(start).Seconds())
		if err != nil {
			MetadataFetchErrorsTotal.Inc()
		}
	}()

	tickSize, err = c.FetchTickSize(ctx, tokenID)
	if err != nil {
		tickSize = 0.01 // Default
	}

	minOrderSize, err = c.FetchMinOrderSize(ctx, tokenID)
	if err != nil {
		minOrderSize = 5.0 // Default
	}

	return tickSize, minOrderSize, nil
}
