package markets

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMetadataClient_FetchTickSize(t *testing.T) {
	tests := []struct {
		name         string
		tokenID      string
		responseCode int
		responseBody map[string]interface{}
		expectError  bool
		expectedSize float64
	}{
		{
			name:         "Valid tick size response",
			tokenID:      "test-token-123",
			responseCode: http.StatusOK,
			responseBody: map[string]interface{}{
				"minimum_tick_size": 0.01,
			},
			expectError:  false,
			expectedSize: 0.01,
		},
		{
			name:         "High precision tick size",
			tokenID:      "test-token-456",
			responseCode: http.StatusOK,
			responseBody: map[string]interface{}{
				"minimum_tick_size": 0.001,
			},
			expectError:  false,
			expectedSize: 0.001,
		},
		{
			name:         "API error response",
			tokenID:      "invalid-token",
			responseCode: http.StatusNotFound,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("token_id") != tt.tokenID {
					t.Errorf("Expected token_id=%s, got %s", tt.tokenID, r.URL.Query().Get("token_id"))
				}

				w.WriteHeader(tt.responseCode)
				if tt.responseBody != nil {
					json.NewEncoder(w).Encode(tt.responseBody)
				}
			}))
			defer server.Close()

			// Create client with custom base URL
			client := &MetadataClient{
				baseURL:    server.URL,
				httpClient: &http.Client{Timeout: 10 * time.Second},
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			tickSize, err := client.FetchTickSize(ctx, tt.tokenID)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if tickSize != tt.expectedSize {
				t.Errorf("Expected tick size %.4f, got %.4f", tt.expectedSize, tickSize)
			}
		})
	}
}

func TestMetadataClient_FetchMinOrderSize(t *testing.T) {
	tests := []struct {
		name         string
		tokenID      string
		responseCode int
		responseBody map[string]interface{}
		expectedSize float64
	}{
		{
			name:         "Min size from top level",
			tokenID:      "test-token-123",
			responseCode: http.StatusOK,
			responseBody: map[string]interface{}{
				"min_size": 5.0,
			},
			expectedSize: 5.0,
		},
		{
			name:         "Min size from market object",
			tokenID:      "test-token-456",
			responseCode: http.StatusOK,
			responseBody: map[string]interface{}{
				"market": map[string]interface{}{
					"minimum_order_size": 10.0,
				},
			},
			expectedSize: 10.0,
		},
		{
			name:         "Default on API error",
			tokenID:      "invalid-token",
			responseCode: http.StatusNotFound,
			expectedSize: 5.0, // Default
		},
		{
			name:         "Default on missing field",
			tokenID:      "test-token-789",
			responseCode: http.StatusOK,
			responseBody: map[string]interface{}{
				"other_field": "value",
			},
			expectedSize: 5.0, // Default
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("token_id") != tt.tokenID {
					t.Errorf("Expected token_id=%s, got %s", tt.tokenID, r.URL.Query().Get("token_id"))
				}

				w.WriteHeader(tt.responseCode)
				if tt.responseBody != nil {
					json.NewEncoder(w).Encode(tt.responseBody)
				}
			}))
			defer server.Close()

			// Create client with custom base URL
			client := &MetadataClient{
				baseURL:    server.URL,
				httpClient: &http.Client{Timeout: 10 * time.Second},
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			minSize, err := client.FetchMinOrderSize(ctx, tt.tokenID)

			// Should never error, returns default on failure
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if minSize != tt.expectedSize {
				t.Errorf("Expected min order size %.2f, got %.2f", tt.expectedSize, minSize)
			}
		})
	}
}

func TestMetadataClient_FetchTokenMetadata(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/tick-size" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"minimum_tick_size": 0.001,
			})
			return
		}
		if r.URL.Path == "/book" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"min_size": 10.0,
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	// Create client
	client := &MetadataClient{
		baseURL:    server.URL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tickSize, minSize, err := client.FetchTokenMetadata(ctx, "test-token")

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if tickSize != 0.001 {
		t.Errorf("Expected tick size 0.001, got %.4f", tickSize)
	}

	if minSize != 10.0 {
		t.Errorf("Expected min size 10.0, got %.2f", minSize)
	}
}

func TestMetadataClient_FetchTokenMetadata_WithDefaults(t *testing.T) {
	// Create mock server that returns errors
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	// Create client
	client := &MetadataClient{
		baseURL:    server.URL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tickSize, minSize, err := client.FetchTokenMetadata(ctx, "test-token")

	// Should not error, returns defaults
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Check defaults
	if tickSize != 0.01 {
		t.Errorf("Expected default tick size 0.01, got %.4f", tickSize)
	}

	if minSize != 5.0 {
		t.Errorf("Expected default min size 5.0, got %.2f", minSize)
	}
}

func TestNewMetadataClient(t *testing.T) {
	client := NewMetadataClient()

	if client == nil {
		t.Fatal("Expected non-nil client")
	}

	if client.baseURL != "https://clob.polymarket.com" {
		t.Errorf("Expected baseURL to be https://clob.polymarket.com, got %s", client.baseURL)
	}

	if client.httpClient == nil {
		t.Error("Expected non-nil httpClient")
	}
}
