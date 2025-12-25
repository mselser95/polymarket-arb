package healthprobe

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
	"time"
)

// HealthChecker provides health and readiness checks.
type HealthChecker struct {
	startTime time.Time
	ready     atomic.Bool
}

// New creates a new HealthChecker.
func New() *HealthChecker {
	return &HealthChecker{
		startTime: time.Now(),
	}
}

// SetReady marks the application as ready to serve traffic.
func (h *HealthChecker) SetReady(ready bool) {
	h.ready.Store(ready)
}

// HealthResponse represents the health check response.
type HealthResponse struct {
	Status  string `json:"status"`
	Uptime  string `json:"uptime"`
	Message string `json:"message,omitempty"`
}

// Health returns an HTTP handler for liveness checks.
// Always returns 200 OK if the application is running.
func (h *HealthChecker) Health() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uptime := time.Since(h.startTime)
		resp := HealthResponse{
			Status: "healthy",
			Uptime: uptime.String(),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// Ready returns an HTTP handler for readiness checks.
// Returns 200 OK if ready, 503 Service Unavailable if not.
func (h *HealthChecker) Ready() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.ready.Load() {
			resp := HealthResponse{
				Status:  "not_ready",
				Message: "application is starting",
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		uptime := time.Since(h.startTime)
		resp := HealthResponse{
			Status: "ready",
			Uptime: uptime.String(),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}
}
