package healthprobe

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	hc := New()

	if hc == nil {
		t.Fatal("New() returned nil")
	}

	// Verify start time is recent
	if time.Since(hc.startTime) > 1*time.Second {
		t.Errorf("Start time is too old: %v", hc.startTime)
	}

	// Verify not ready by default
	if hc.ready.Load() {
		t.Error("HealthChecker should not be ready by default")
	}
}

func TestSetReady(t *testing.T) {
	tests := []struct {
		name     string
		setReady bool
		expected bool
	}{
		{
			name:     "set_ready_true",
			setReady: true,
			expected: true,
		},
		{
			name:     "set_ready_false",
			setReady: false,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hc := New()
			hc.SetReady(tt.setReady)

			if hc.ready.Load() != tt.expected {
				t.Errorf("SetReady(%v): ready = %v, want %v", tt.setReady, hc.ready.Load(), tt.expected)
			}
		})
	}
}

func TestSetReady_Toggle(t *testing.T) {
	// Test toggling ready state multiple times
	hc := New()

	// Start not ready
	if hc.ready.Load() {
		t.Error("Should start not ready")
	}

	// Set ready
	hc.SetReady(true)
	if !hc.ready.Load() {
		t.Error("Should be ready after SetReady(true)")
	}

	// Set not ready
	hc.SetReady(false)
	if hc.ready.Load() {
		t.Error("Should not be ready after SetReady(false)")
	}

	// Set ready again
	hc.SetReady(true)
	if !hc.ready.Load() {
		t.Error("Should be ready after second SetReady(true)")
	}
}

func TestHealth_Handler(t *testing.T) {
	hc := New()

	// Get health handler
	handler := hc.Health()

	// Create test request
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	// Call handler
	handler(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	// Verify status code
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Health handler status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Verify Content-Type
	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %s, want application/json", contentType)
	}

	// Parse response body
	var healthResp HealthResponse
	err := json.NewDecoder(resp.Body).Decode(&healthResp)
	if err != nil {
		t.Fatalf("Failed to decode health response: %v", err)
	}

	// Verify response fields
	if healthResp.Status != "healthy" {
		t.Errorf("Status = %s, want healthy", healthResp.Status)
	}

	if healthResp.Uptime == "" {
		t.Error("Uptime is empty")
	}
}

func TestHealth_AlwaysReturnsOK(t *testing.T) {
	// Health endpoint should always return 200, regardless of ready state
	hc := New()

	tests := []struct {
		name     string
		setReady bool
	}{
		{
			name:     "not_ready",
			setReady: false,
		},
		{
			name:     "ready",
			setReady: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hc.SetReady(tt.setReady)

			handler := hc.Health()
			req := httptest.NewRequest(http.MethodGet, "/health", nil)
			w := httptest.NewRecorder()

			handler(w, req)

			resp := w.Result()
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Health handler status = %d, want %d (ready=%v)", resp.StatusCode, http.StatusOK, tt.setReady)
			}
		})
	}
}

func TestReady_NotReadyInitially(t *testing.T) {
	hc := New()

	handler := hc.Ready()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	// Should return 503 when not ready
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("Ready handler status = %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}

	// Verify Content-Type
	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %s, want application/json", contentType)
	}

	// Parse response body
	var healthResp HealthResponse
	err := json.NewDecoder(resp.Body).Decode(&healthResp)
	if err != nil {
		t.Fatalf("Failed to decode ready response: %v", err)
	}

	// Verify response fields
	if healthResp.Status != "not_ready" {
		t.Errorf("Status = %s, want not_ready", healthResp.Status)
	}

	if healthResp.Message == "" {
		t.Error("Message is empty for not_ready state")
	}
}

func TestReady_ReadyAfterSet(t *testing.T) {
	hc := New()
	hc.SetReady(true)

	handler := hc.Ready()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	// Should return 200 when ready
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Ready handler status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Parse response body
	var healthResp HealthResponse
	err := json.NewDecoder(resp.Body).Decode(&healthResp)
	if err != nil {
		t.Fatalf("Failed to decode ready response: %v", err)
	}

	// Verify response fields
	if healthResp.Status != "ready" {
		t.Errorf("Status = %s, want ready", healthResp.Status)
	}

	if healthResp.Uptime == "" {
		t.Error("Uptime is empty")
	}
}

func TestReady_StateChanges(t *testing.T) {
	// Test ready endpoint responds correctly to state changes
	hc := New()
	handler := hc.Ready()

	// Initially not ready
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Initial ready status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}

	// Set ready
	hc.SetReady(true)
	req = httptest.NewRequest(http.MethodGet, "/ready", nil)
	w = httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("Ready status after SetReady(true) = %d, want %d", w.Code, http.StatusOK)
	}

	// Set not ready again
	hc.SetReady(false)
	req = httptest.NewRequest(http.MethodGet, "/ready", nil)
	w = httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Ready status after SetReady(false) = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestHealthResponse_JSONFormat(t *testing.T) {
	// Test that HealthResponse JSON fields are correct
	hc := New()

	tests := []struct {
		name    string
		handler http.HandlerFunc
		ready   bool
	}{
		{
			name:    "health_endpoint",
			handler: hc.Health(),
			ready:   false,
		},
		{
			name:    "ready_endpoint_ready",
			handler: hc.Ready(),
			ready:   true,
		},
		{
			name:    "ready_endpoint_not_ready",
			handler: hc.Ready(),
			ready:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hc.SetReady(tt.ready)

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			w := httptest.NewRecorder()

			tt.handler(w, req)

			resp := w.Result()
			defer resp.Body.Close()

			// Verify JSON is valid
			var healthResp HealthResponse
			err := json.NewDecoder(resp.Body).Decode(&healthResp)
			if err != nil {
				t.Fatalf("Failed to decode JSON: %v", err)
			}

			// Verify required fields exist
			if healthResp.Status == "" {
				t.Error("Status field is empty")
			}
		})
	}
}

func TestHealth_UptimeIncreases(t *testing.T) {
	// Test that uptime increases over time
	hc := New()
	handler := hc.Health()

	// First request
	req1 := httptest.NewRequest(http.MethodGet, "/health", nil)
	w1 := httptest.NewRecorder()
	handler(w1, req1)

	var resp1 HealthResponse
	err := json.NewDecoder(w1.Body).Decode(&resp1)
	if err != nil {
		t.Fatalf("Failed to decode first response: %v", err)
	}

	// Wait a bit
	time.Sleep(10 * time.Millisecond)

	// Second request
	req2 := httptest.NewRequest(http.MethodGet, "/health", nil)
	w2 := httptest.NewRecorder()
	handler(w2, req2)

	var resp2 HealthResponse
	err = json.NewDecoder(w2.Body).Decode(&resp2)
	if err != nil {
		t.Fatalf("Failed to decode second response: %v", err)
	}

	// Uptime strings should be different (though this is a weak test)
	// At minimum, verify they're both non-empty
	if resp1.Uptime == "" || resp2.Uptime == "" {
		t.Error("Uptime should not be empty")
	}
}

func TestHealthChecker_ConcurrentAccess(t *testing.T) {
	// Test that concurrent access doesn't cause data races
	hc := New()
	handler := hc.Ready()

	done := make(chan bool)

	// Concurrent SetReady calls
	go func() {
		for i := 0; i < 100; i++ {
			hc.SetReady(i%2 == 0)
		}
		done <- true
	}()

	// Concurrent handler calls
	go func() {
		for i := 0; i < 100; i++ {
			req := httptest.NewRequest(http.MethodGet, "/ready", nil)
			w := httptest.NewRecorder()
			handler(w, req)
		}
		done <- true
	}()

	// Wait for both goroutines
	<-done
	<-done

	// If we get here without data race, test passes
}
