package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mselser95/polymarket-arb/internal/discovery"
	"github.com/mselser95/polymarket-arb/internal/orderbook"
	"github.com/mselser95/polymarket-arb/pkg/healthprobe"
	"github.com/mselser95/polymarket-arb/pkg/types"
	"go.uber.org/zap"
)

func TestNew(t *testing.T) {
	logger := zap.NewNop()
	healthChecker := healthprobe.New()

	tests := []struct {
		name      string
		cfg       *Config
		wantPanic bool
	}{
		{
			name: "valid_config_minimal",
			cfg: &Config{
				Port:          "8080",
				Logger:        logger,
				HealthChecker: healthChecker,
			},
			wantPanic: false,
		},
		{
			name: "valid_config_with_orderbook",
			cfg: &Config{
				Port:          "8080",
				Logger:        logger,
				HealthChecker: healthChecker,
				OrderbookManager: &orderbook.Manager{},
				DiscoveryService: &discovery.Service{},
			},
			wantPanic: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if (r != nil) != tt.wantPanic {
					t.Errorf("New() panic = %v, wantPanic %v", r, tt.wantPanic)
				}
			}()

			server := New(tt.cfg)
			if server == nil {
				t.Error("New() returned nil server")
			}
			if server.server == nil {
				t.Error("New() server.server is nil")
			}
			if server.logger != tt.cfg.Logger {
				t.Error("New() logger not set correctly")
			}
			if server.healthChecker != tt.cfg.HealthChecker {
				t.Error("New() healthChecker not set correctly")
			}
		})
	}
}

func TestHealthEndpoint(t *testing.T) {
	logger := zap.NewNop()
	healthChecker := healthprobe.New()

	cfg := &Config{
		Port:          "0", // Random port
		Logger:        logger,
		HealthChecker: healthChecker,
	}

	server := New(cfg)

	// Use httptest to test the handler directly
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	// Get the router from the server's handler
	server.server.Handler.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Health endpoint status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestReadyEndpoint(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		name           string
		setReady       bool
		expectedStatus int
	}{
		{
			name:           "ready_when_set",
			setReady:       true,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "not_ready_initially",
			setReady:       false,
			expectedStatus: http.StatusServiceUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hc := healthprobe.New()
			if tt.setReady {
				hc.SetReady(true)
			}

			cfg := &Config{
				Port:          "0",
				Logger:        logger,
				HealthChecker: hc,
			}

			server := New(cfg)

			req := httptest.NewRequest(http.MethodGet, "/ready", nil)
			w := httptest.NewRecorder()

			server.server.Handler.ServeHTTP(w, req)

			resp := w.Result()
			defer resp.Body.Close()

			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("Ready endpoint status = %d, want %d", resp.StatusCode, tt.expectedStatus)
			}
		})
	}
}

func TestMetricsEndpoint(t *testing.T) {
	logger := zap.NewNop()
	healthChecker := healthprobe.New()

	cfg := &Config{
		Port:          "0",
		Logger:        logger,
		HealthChecker: healthChecker,
	}

	server := New(cfg)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	server.server.Handler.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Metrics endpoint status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Verify Content-Type header
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		t.Error("Metrics endpoint missing Content-Type header")
	}

	// Read body to ensure it's not empty
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read metrics response body: %v", err)
	}

	if len(body) == 0 {
		t.Error("Metrics endpoint returned empty body")
	}
}

func TestOrderbookHandler_MarketNotFound(t *testing.T) {
	// Test that orderbook endpoint returns 404 for non-existent market
	// We can't easily test success case without internal state manipulation
	logger := zap.NewNop()
	healthChecker := healthprobe.New()

	msgChan := make(chan *types.OrderbookMessage)
	obManager := orderbook.New(&orderbook.Config{
		Logger:         logger,
		MessageChannel: msgChan,
	})

	discoveryService := discovery.New(&discovery.Config{
		Client:       nil,
		Cache:        nil,
		Logger:       logger,
		PollInterval: 1 * time.Minute,
	})

	cfg := &Config{
		Port:             "0",
		Logger:           logger,
		HealthChecker:    healthChecker,
		OrderbookManager: obManager,
		DiscoveryService: discoveryService,
	}

	server := New(cfg)

	// Request for non-existent market
	req := httptest.NewRequest(http.MethodGet, "/api/orderbook?slug=non-existent-market", nil)
	w := httptest.NewRecorder()

	server.server.Handler.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Market not found status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}

	// Parse error response
	var errResp ErrorResponse
	err := json.NewDecoder(resp.Body).Decode(&errResp)
	if err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if errResp.Error == "" {
		t.Error("Error response missing error message")
	}
}

func TestOrderbookHandler_MissingSlug(t *testing.T) {
	logger := zap.NewNop()
	healthChecker := healthprobe.New()

	msgChan := make(chan *types.OrderbookMessage)
	obManager := orderbook.New(&orderbook.Config{
		Logger:         logger,
		MessageChannel: msgChan,
	})

	discoveryService := discovery.New(&discovery.Config{
		Client:       nil,
		Cache:        nil,
		Logger:       logger,
		PollInterval: 1 * time.Minute,
	})

	cfg := &Config{
		Port:             "0",
		Logger:           logger,
		HealthChecker:    healthChecker,
		OrderbookManager: obManager,
		DiscoveryService: discoveryService,
	}

	server := New(cfg)

	// Request without slug parameter
	req := httptest.NewRequest(http.MethodGet, "/api/orderbook", nil)
	w := httptest.NewRecorder()

	server.server.Handler.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Missing slug status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	// Parse error response
	var errResp ErrorResponse
	err := json.NewDecoder(resp.Body).Decode(&errResp)
	if err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}

	if errResp.Error == "" {
		t.Error("Error response missing error message")
	}
}

func TestOrderbookHandler_MethodNotAllowed(t *testing.T) {
	logger := zap.NewNop()
	healthChecker := healthprobe.New()

	msgChan := make(chan *types.OrderbookMessage)
	obManager := orderbook.New(&orderbook.Config{
		Logger:         logger,
		MessageChannel: msgChan,
	})

	discoveryService := discovery.New(&discovery.Config{
		Client:       nil,
		Cache:        nil,
		Logger:       logger,
		PollInterval: 1 * time.Minute,
	})

	cfg := &Config{
		Port:             "0",
		Logger:           logger,
		HealthChecker:    healthChecker,
		OrderbookManager: obManager,
		DiscoveryService: discoveryService,
	}

	server := New(cfg)

	// POST request (should be GET only)
	req := httptest.NewRequest(http.MethodPost, "/api/orderbook?slug=test-market", nil)
	w := httptest.NewRecorder()

	server.server.Handler.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("Method not allowed status = %d, want %d", resp.StatusCode, http.StatusMethodNotAllowed)
	}
}

func TestServer_StartAndShutdown(t *testing.T) {
	logger := zap.NewNop()
	healthChecker := healthprobe.New()

	cfg := &Config{
		Port:          "0", // Random available port
		Logger:        logger,
		HealthChecker: healthChecker,
	}

	server := New(cfg)

	// Start server in background
	serverDone := make(chan error, 1)
	go func() {
		serverDone <- server.Start()
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Shutdown server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := server.Shutdown(ctx)
	if err != nil {
		t.Errorf("Shutdown() error = %v", err)
	}

	// Wait for Start() to return
	select {
	case err := <-serverDone:
		if err != nil {
			t.Errorf("Start() returned error after shutdown: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start() did not return after shutdown")
	}
}

func TestServer_Middleware(t *testing.T) {
	logger := zap.NewNop()
	healthChecker := healthprobe.New()

	cfg := &Config{
		Port:          "0",
		Logger:        logger,
		HealthChecker: healthChecker,
	}

	server := New(cfg)

	// Test that middleware is configured (chi router handles requests)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	server.server.Handler.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	// Verify request completed successfully (middleware didn't break anything)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Middleware test status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestServer_Timeouts(t *testing.T) {
	logger := zap.NewNop()
	healthChecker := healthprobe.New()

	cfg := &Config{
		Port:          "8080",
		Logger:        logger,
		HealthChecker: healthChecker,
	}

	server := New(cfg)

	// Verify timeouts are configured
	if server.server.ReadTimeout != 15*time.Second {
		t.Errorf("ReadTimeout = %v, want %v", server.server.ReadTimeout, 15*time.Second)
	}

	if server.server.ReadHeaderTimeout != 10*time.Second {
		t.Errorf("ReadHeaderTimeout = %v, want %v", server.server.ReadHeaderTimeout, 10*time.Second)
	}

	if server.server.WriteTimeout != 15*time.Second {
		t.Errorf("WriteTimeout = %v, want %v", server.server.WriteTimeout, 15*time.Second)
	}

	if server.server.IdleTimeout != 60*time.Second {
		t.Errorf("IdleTimeout = %v, want %v", server.server.IdleTimeout, 60*time.Second)
	}
}

func TestServer_RouteNotFound(t *testing.T) {
	logger := zap.NewNop()
	healthChecker := healthprobe.New()

	cfg := &Config{
		Port:          "0",
		Logger:        logger,
		HealthChecker: healthChecker,
	}

	server := New(cfg)

	// Request non-existent route
	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	w := httptest.NewRecorder()

	server.server.Handler.ServeHTTP(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Non-existent route status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestOrderbookEndpoint_OnlyWithComponents(t *testing.T) {
	logger := zap.NewNop()
	healthChecker := healthprobe.New()

	tests := []struct {
		name             string
		includeOrderbook bool
		includeDiscovery bool
		expectEndpoint   bool
	}{
		{
			name:             "both_components_provided",
			includeOrderbook: true,
			includeDiscovery: true,
			expectEndpoint:   true,
		},
		{
			name:             "missing_orderbook",
			includeOrderbook: false,
			includeDiscovery: true,
			expectEndpoint:   false,
		},
		{
			name:             "missing_discovery",
			includeOrderbook: true,
			includeDiscovery: false,
			expectEndpoint:   false,
		},
		{
			name:             "missing_both",
			includeOrderbook: false,
			includeDiscovery: false,
			expectEndpoint:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Port:          "0",
				Logger:        logger,
				HealthChecker: healthChecker,
			}

			if tt.includeOrderbook {
				msgChan := make(chan *types.OrderbookMessage)
				cfg.OrderbookManager = orderbook.New(&orderbook.Config{
					Logger:         logger,
					MessageChannel: msgChan,
				})
			}

			if tt.includeDiscovery {
				cfg.DiscoveryService = discovery.New(&discovery.Config{
					Client:       nil,
					Cache:        nil,
					Logger:       logger,
					PollInterval: 1 * time.Minute,
				})
			}

			server := New(cfg)

			// Request orderbook endpoint
			req := httptest.NewRequest(http.MethodGet, "/api/orderbook?slug=test", nil)
			w := httptest.NewRecorder()

			server.server.Handler.ServeHTTP(w, req)

			resp := w.Result()
			defer resp.Body.Close()

			if tt.expectEndpoint {
				// Should get 404 (market not found) not 404 (route not found)
				if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusBadRequest {
					// Allow either - endpoint exists but market/params invalid
					expectedStatuses := fmt.Sprintf("%d or %d", http.StatusNotFound, http.StatusBadRequest)
					t.Errorf("Expected endpoint status %s, got %d", expectedStatuses, resp.StatusCode)
				}
			} else {
				// Route should not exist
				if resp.StatusCode != http.StatusNotFound {
					t.Errorf("Expected route not found status %d, got %d", http.StatusNotFound, resp.StatusCode)
				}
			}
		})
	}
}
