package markets

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
)

// TestFetchTickSize_RetryOnTimeout verifies retry behavior on network timeouts
func TestFetchTickSize_RetryOnTimeout(t *testing.T) {
	var attemptCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := attemptCount.Add(1)

		// First two attempts: delay longer than context timeout
		if attempt <= 2 {
			time.Sleep(200 * time.Millisecond)
			return
		}

		// Third attempt: return valid response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"minimum_tick_size": 0.01}`))
	}))
	defer server.Close()

	// Create client with short timeout and fast backoff
	client := &MetadataClient{
		baseURL: server.URL,
		httpClient: &http.Client{
			Timeout: 100 * time.Millisecond, // Shorter than server delay
		},
		maxRetries:        3,
		initialBackoff:    50 * time.Millisecond,
		maxBackoff:        500 * time.Millisecond,
		backoffMultiplier: 2.0,
		logger:            zap.NewNop(),
	}

	ctx := context.Background()
	tickSize, err := client.FetchTickSize(ctx, "test-token")

	// Verify success after retries
	if err != nil {
		t.Fatalf("expected success after retries, got error: %v", err)
	}

	if tickSize != 0.01 {
		t.Errorf("expected tick_size=0.01, got=%.4f", tickSize)
	}

	// Verify 3 attempts were made (2 timeouts + 1 success)
	if attemptCount.Load() != 3 {
		t.Errorf("expected 3 attempts, got %d", attemptCount.Load())
	}
}

// TestFetchTickSize_RetryOn429 verifies retry behavior on rate limit errors
func TestFetchTickSize_RetryOn429(t *testing.T) {
	var attemptCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := attemptCount.Add(1)

		// First two attempts: return 429 rate limit
		if attempt <= 2 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error": "rate limit exceeded"}`))
			return
		}

		// Third attempt: return valid response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"minimum_tick_size": 0.005}`))
	}))
	defer server.Close()

	client := &MetadataClient{
		baseURL: server.URL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		maxRetries:        3,
		initialBackoff:    10 * time.Millisecond,
		maxBackoff:        100 * time.Millisecond,
		backoffMultiplier: 2.0,
		logger:            zap.NewNop(),
	}

	ctx := context.Background()
	tickSize, err := client.FetchTickSize(ctx, "test-token")

	if err != nil {
		t.Fatalf("expected success after retries, got error: %v", err)
	}

	if tickSize != 0.005 {
		t.Errorf("expected tick_size=0.005, got=%.4f", tickSize)
	}

	// Verify 3 attempts were made (2 rate limits + 1 success)
	if attemptCount.Load() != 3 {
		t.Errorf("expected 3 attempts, got %d", attemptCount.Load())
	}
}

// TestFetchTickSize_RetryOn5xx verifies retry behavior on server errors
func TestFetchTickSize_RetryOn5xx(t *testing.T) {
	testCases := []struct {
		name       string
		statusCode int
	}{
		{"500 Internal Server Error", http.StatusInternalServerError},
		{"502 Bad Gateway", http.StatusBadGateway},
		{"503 Service Unavailable", http.StatusServiceUnavailable},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var attemptCount atomic.Int32

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempt := attemptCount.Add(1)

				// First attempt: return server error
				if attempt == 1 {
					w.WriteHeader(tc.statusCode)
					_, _ = w.Write([]byte("server error"))
					return
				}

				// Second attempt: return valid response
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"minimum_tick_size": 0.02}`))
			}))
			defer server.Close()

			client := &MetadataClient{
				baseURL: server.URL,
				httpClient: &http.Client{
					Timeout: 5 * time.Second,
				},
				maxRetries:        3,
				initialBackoff:    10 * time.Millisecond,
				maxBackoff:        100 * time.Millisecond,
				backoffMultiplier: 2.0,
				logger:            zap.NewNop(),
			}

			ctx := context.Background()
			tickSize, err := client.FetchTickSize(ctx, "test-token")

			if err != nil {
				t.Fatalf("expected success after retry, got error: %v", err)
			}

			if tickSize != 0.02 {
				t.Errorf("expected tick_size=0.02, got=%.4f", tickSize)
			}

			// Verify 2 attempts were made (1 error + 1 success)
			if attemptCount.Load() != 2 {
				t.Errorf("expected 2 attempts, got %d", attemptCount.Load())
			}
		})
	}
}

// TestFetchMinOrderSize_NoRetryOn404 verifies no retry on 4xx client errors
func TestFetchMinOrderSize_NoRetryOn404(t *testing.T) {
	testCases := []struct {
		name       string
		statusCode int
	}{
		{"400 Bad Request", http.StatusBadRequest},
		{"403 Forbidden", http.StatusForbidden},
		{"404 Not Found", http.StatusNotFound},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var attemptCount atomic.Int32

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attemptCount.Add(1)
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte("client error"))
			}))
			defer server.Close()

			client := &MetadataClient{
				baseURL: server.URL,
				httpClient: &http.Client{
					Timeout: 5 * time.Second,
				},
				maxRetries:        3,
				initialBackoff:    10 * time.Millisecond,
				maxBackoff:        100 * time.Millisecond,
				backoffMultiplier: 2.0,
				logger:            zap.NewNop(),
			}

			ctx := context.Background()
			minSize, err := client.FetchMinOrderSize(ctx, "test-token")

			// FetchMinOrderSize returns default value (5.0) on non-200 status
			// This is intentional behavior - it doesn't propagate the error
			if err != nil {
				t.Errorf("expected nil error (default fallback), got: %v", err)
			}

			if minSize != 5.0 {
				t.Errorf("expected default min_size=5.0, got=%.1f", minSize)
			}

			// Verify only 1 attempt was made (no retry on 4xx)
			if attemptCount.Load() != 1 {
				t.Errorf("expected 1 attempt (no retry on %d), got %d", tc.statusCode, attemptCount.Load())
			}
		})
	}
}

// TestFetchTickSize_MaxRetriesExceeded verifies error after max attempts
func TestFetchTickSize_MaxRetriesExceeded(t *testing.T) {
	var attemptCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount.Add(1)
		// Always return 500 error
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("persistent server error"))
	}))
	defer server.Close()

	client := &MetadataClient{
		baseURL: server.URL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		maxRetries:        3,
		initialBackoff:    10 * time.Millisecond,
		maxBackoff:        100 * time.Millisecond,
		backoffMultiplier: 2.0,
		logger:            zap.NewNop(),
	}

	ctx := context.Background()
	_, err := client.FetchTickSize(ctx, "test-token")

	// Verify error is returned
	if err == nil {
		t.Fatal("expected error after max retries, got nil")
	}

	// Verify error message contains "max retries"
	if !strings.Contains(err.Error(), "max retries") {
		t.Errorf("expected error to mention 'max retries', got: %v", err)
	}

	// Verify error message contains retry count
	if !strings.Contains(err.Error(), "3") {
		t.Errorf("expected error to mention retry count (3), got: %v", err)
	}

	// Verify 4 total attempts were made (initial + 3 retries)
	if attemptCount.Load() != 4 {
		t.Errorf("expected 4 attempts (initial + 3 retries), got %d", attemptCount.Load())
	}
}

// TestFetchTickSize_ContextCancelledDuringBackoff verifies context cancellation handling
func TestFetchTickSize_ContextCancelledDuringBackoff(t *testing.T) {
	var attemptCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount.Add(1)
		// Always return 500 to trigger retry
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("server error"))
	}))
	defer server.Close()

	client := &MetadataClient{
		baseURL: server.URL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		maxRetries:        5,
		initialBackoff:    200 * time.Millisecond, // Long backoff
		maxBackoff:        5 * time.Second,
		backoffMultiplier: 2.0,
		logger:            zap.NewNop(),
	}

	// Create context that will be cancelled during backoff
	ctx, cancel := context.WithCancel(context.Background())

	// Start fetch in goroutine
	errChan := make(chan error, 1)
	go func() {
		_, err := client.FetchTickSize(ctx, "test-token")
		errChan <- err
	}()

	// Wait for first attempt to complete, then cancel during backoff
	time.Sleep(100 * time.Millisecond)
	cancel()

	// Wait for result with timeout
	select {
	case err := <-errChan:
		// Verify context cancellation error is returned
		if err == nil {
			t.Fatal("expected error after context cancellation, got nil")
		}

		if !strings.Contains(err.Error(), "context canceled") {
			t.Errorf("expected context cancellation error, got: %v", err)
		}

	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for fetch to return after context cancellation")
	}

	// Verify only 1-2 attempts were made (cancelled during backoff)
	attempts := attemptCount.Load()
	if attempts > 2 {
		t.Errorf("expected 1-2 attempts (cancelled during backoff), got %d", attempts)
	}
}

// TestFetchTickSize_BackoffCapping verifies backoff sequence caps at MaxBackoff
func TestFetchTickSize_BackoffCapping(t *testing.T) {
	var attemptCount atomic.Int32
	var attemptTimes []time.Time

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptTimes = append(attemptTimes, time.Now())
		attempt := attemptCount.Add(1)

		// Last attempt succeeds
		if attempt == 6 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"minimum_tick_size": 0.01}`))
			return
		}

		// All other attempts fail
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("server error"))
	}))
	defer server.Close()

	client := &MetadataClient{
		baseURL: server.URL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		maxRetries:        5,
		initialBackoff:    50 * time.Millisecond,
		maxBackoff:        150 * time.Millisecond, // Cap backoff
		backoffMultiplier: 2.0,
		logger:            zap.NewNop(),
	}

	ctx := context.Background()
	tickSize, err := client.FetchTickSize(ctx, "test-token")

	if err != nil {
		t.Fatalf("expected success after retries, got error: %v", err)
	}

	if tickSize != 0.01 {
		t.Errorf("expected tick_size=0.01, got=%.4f", tickSize)
	}

	// Verify 6 attempts were made
	if attemptCount.Load() != 6 {
		t.Fatalf("expected 6 attempts, got %d", attemptCount.Load())
	}

	// Verify backoff sequence (with tolerance for scheduling variance)
	// Expected: 50ms, 100ms, 150ms (capped), 150ms (capped), 150ms (capped)
	for i := 1; i < len(attemptTimes); i++ {
		delay := attemptTimes[i].Sub(attemptTimes[i-1])

		var expectedMin, expectedMax time.Duration
		switch i {
		case 1:
			// First backoff: 50ms
			expectedMin = 40 * time.Millisecond
			expectedMax = 80 * time.Millisecond
		case 2:
			// Second backoff: 100ms
			expectedMin = 80 * time.Millisecond
			expectedMax = 150 * time.Millisecond
		case 3, 4, 5:
			// Third+ backoff: 150ms (capped)
			expectedMin = 120 * time.Millisecond
			expectedMax = 200 * time.Millisecond
		}

		if delay < expectedMin || delay > expectedMax {
			t.Errorf("attempt %d: backoff delay out of range: got %v, expected %v-%v",
				i, delay, expectedMin, expectedMax)
		}
	}
}

// TestFetchTokenMetadata_IntegrationRetry tests the wrapper function
func TestFetchTokenMetadata_IntegrationRetry(t *testing.T) {
	var tickSizeAttempts atomic.Int32
	var minSizeAttempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Route based on endpoint
		if strings.Contains(r.URL.Path, "/tick-size") {
			attempt := tickSizeAttempts.Add(1)

			// First attempt fails, second succeeds
			if attempt == 1 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"minimum_tick_size": 0.01}`))
			return
		}

		if strings.Contains(r.URL.Path, "/book") {
			minSizeAttempts.Add(1)

			// Min size endpoint always returns valid data
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"min_size": 10.0}`))
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := &MetadataClient{
		baseURL: server.URL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		maxRetries:        3,
		initialBackoff:    10 * time.Millisecond,
		maxBackoff:        100 * time.Millisecond,
		backoffMultiplier: 2.0,
		logger:            zap.NewNop(),
	}

	ctx := context.Background()
	tickSize, minSize, err := client.FetchTokenMetadata(ctx, "test-token")

	// FetchTokenMetadata never returns errors (uses defaults)
	if err != nil {
		t.Errorf("expected nil error, got: %v", err)
	}

	// Tick size should be successfully fetched after retry
	if tickSize != 0.01 {
		t.Errorf("expected tick_size=0.01, got=%.4f", tickSize)
	}

	// Min size should be successfully fetched
	if minSize != 10.0 {
		t.Errorf("expected min_size=10.0, got=%.1f", minSize)
	}

	// Verify retry occurred for tick size endpoint (2 attempts)
	if tickSizeAttempts.Load() != 2 {
		t.Errorf("expected 2 tick_size attempts, got %d", tickSizeAttempts.Load())
	}

	// Verify min size endpoint was called once (no retry needed)
	if minSizeAttempts.Load() != 1 {
		t.Errorf("expected 1 min_size attempt, got %d", minSizeAttempts.Load())
	}
}

// TestIsRetryable verifies the retry classification logic
func TestIsRetryable(t *testing.T) {
	testCases := []struct {
		name      string
		err       error
		retryable bool
	}{
		{"nil error", nil, false},
		{"context deadline", context.DeadlineExceeded, true},
		{"429 rate limit", fmt.Errorf("API error: status 429"), true},
		{"500 server error", fmt.Errorf("API error: status 500"), true},
		{"502 bad gateway", fmt.Errorf("API error: status 502"), true},
		{"503 unavailable", fmt.Errorf("API error: status 503"), true},
		{"timeout in message", fmt.Errorf("request timeout"), true},
		{"connection refused", fmt.Errorf("connection refused"), true},
		{"connection reset", fmt.Errorf("connection reset by peer"), true},
		{"400 bad request", fmt.Errorf("API error: status 400"), false},
		{"404 not found", fmt.Errorf("API error: status 404"), false},
		{"invalid json", fmt.Errorf("invalid JSON"), false},
		{"generic error", fmt.Errorf("something went wrong"), false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isRetryable(tc.err)
			if result != tc.retryable {
				t.Errorf("expected retryable=%v, got=%v for error: %v",
					tc.retryable, result, tc.err)
			}
		})
	}
}
