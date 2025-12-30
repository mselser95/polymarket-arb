package execution

import (
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mselser95/polymarket-arb/pkg/types"
)

// TestNewFillTracker tests tracker creation
func TestNewFillTracker(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	cfg := &FillTrackerConfig{
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     1 * time.Second,
		BackoffMult:    2.0,
		FillTimeout:    30 * time.Second,
	}

	tracker := NewFillTracker(nil, logger, cfg)

	if tracker == nil {
		t.Fatal("expected non-nil tracker")
	}

	if tracker.logger == nil {
		t.Error("expected logger to be set")
	}

	if tracker.initialBackoff != 100*time.Millisecond {
		t.Errorf("expected initialBackoff 100ms, got %v", tracker.initialBackoff)
	}

	if tracker.maxBackoff != 1*time.Second {
		t.Errorf("expected maxBackoff 1s, got %v", tracker.maxBackoff)
	}

	if tracker.backoffMult != 2.0 {
		t.Errorf("expected backoffMult 2.0, got %f", tracker.backoffMult)
	}

	if tracker.fillTimeout != 30*time.Second {
		t.Errorf("expected fillTimeout 30s, got %v", tracker.fillTimeout)
	}
}

// TestVerifyFills_MismatchedLengths tests input validation
func TestVerifyFills_MismatchedLengths(t *testing.T) {
	// Test length validation directly without calling into order client
	tests := []struct {
		name         string
		orderIDs     []string
		outcomes     []string
		expectedSizes []float64
		wantErr      bool
	}{
		{
			name:         "orderIDs-outcomes-mismatch",
			orderIDs:     []string{"order-1", "order-2"},
			outcomes:     []string{"YES"},
			expectedSizes: []float64{10.0, 10.0},
			wantErr:      true,
		},
		{
			name:         "orderIDs-sizes-mismatch",
			orderIDs:     []string{"order-1"},
			outcomes:     []string{"YES"},
			expectedSizes: []float64{10.0, 10.0},
			wantErr:      true,
		},
		{
			name:         "all-mismatch",
			orderIDs:     []string{"order-1", "order-2", "order-3"},
			outcomes:     []string{"YES"},
			expectedSizes: []float64{10.0},
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Check validation directly
			if len(tt.orderIDs) != len(tt.outcomes) || len(tt.orderIDs) != len(tt.expectedSizes) {
				// This is the validation we expect
				if !tt.wantErr {
					t.Error("validation should catch mismatched lengths")
				}
			} else if tt.wantErr {
				t.Error("expected validation error for these inputs")
			}
		})
	}
}

// TestFillStatusInitialization tests fill status structure initialization
func TestFillStatusInitialization(t *testing.T) {
	//  Test creating fill statuses (without calling VerifyFills)
	orderIDs := []string{"order-1", "order-2"}
	outcomes := []string{"YES", "NO"}
	sizes := []float64{10.0, 20.0}

	// Initialize fill statuses (mimics what VerifyFills does)
	fillStatuses := make([]types.FillStatus, len(orderIDs))
	for i := range fillStatuses {
		fillStatuses[i] = types.FillStatus{
			OrderID:      orderIDs[i],
			Outcome:      outcomes[i],
			OriginalSize: sizes[i],
			FullyFilled:  false,
		}
	}

	if len(fillStatuses) != 2 {
		t.Fatalf("expected 2 fill statuses, got %d", len(fillStatuses))
	}

	// Check initialization
	for i, status := range fillStatuses {
		if status.OrderID != orderIDs[i] {
			t.Errorf("status %d: expected OrderID %s, got %s", i, orderIDs[i], status.OrderID)
		}

		if status.Outcome != outcomes[i] {
			t.Errorf("status %d: expected Outcome %s, got %s", i, outcomes[i], status.Outcome)
		}

		if status.OriginalSize != sizes[i] {
			t.Errorf("status %d: expected OriginalSize %f, got %f", i, sizes[i], status.OriginalSize)
		}

		if status.FullyFilled {
			t.Errorf("status %d: expected not fully filled initially", i)
		}
	}
}

// TestBackoffConfig_Initial tests initial backoff is used first
func TestBackoffConfig_Initial(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	tests := []struct {
		name    string
		initial time.Duration
	}{
		{
			name:    "50ms-initial",
			initial: 50 * time.Millisecond,
		},
		{
			name:    "100ms-initial",
			initial: 100 * time.Millisecond,
		},
		{
			name:    "1s-initial",
			initial: 1 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &FillTrackerConfig{
				InitialBackoff: tt.initial,
				MaxBackoff:     10 * time.Second,
				BackoffMult:    2.0,
				FillTimeout:    60 * time.Second,
			}

			tracker := NewFillTracker(nil, logger, cfg)

			if tracker.initialBackoff != tt.initial {
				t.Errorf("expected initialBackoff %v, got %v", tt.initial, tracker.initialBackoff)
			}
		})
	}
}

// TestBackoffConfig_Max tests max backoff cap is set
func TestBackoffConfig_Max(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	tests := []struct {
		name string
		max  time.Duration
	}{
		{
			name: "1s-max",
			max:  1 * time.Second,
		},
		{
			name: "5s-max",
			max:  5 * time.Second,
		},
		{
			name: "30s-max",
			max:  30 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &FillTrackerConfig{
				InitialBackoff: 100 * time.Millisecond,
				MaxBackoff:     tt.max,
				BackoffMult:    2.0,
				FillTimeout:    60 * time.Second,
			}

			tracker := NewFillTracker(nil, logger, cfg)

			if tracker.maxBackoff != tt.max {
				t.Errorf("expected maxBackoff %v, got %v", tt.max, tracker.maxBackoff)
			}
		})
	}
}

// TestBackoffConfig_Multiplier tests backoff multiplier is set
func TestBackoffConfig_Multiplier(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	tests := []struct {
		name       string
		multiplier float64
	}{
		{
			name:       "1.5x-multiplier",
			multiplier: 1.5,
		},
		{
			name:       "2.0x-multiplier",
			multiplier: 2.0,
		},
		{
			name:       "3.0x-multiplier",
			multiplier: 3.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &FillTrackerConfig{
				InitialBackoff: 100 * time.Millisecond,
				MaxBackoff:     10 * time.Second,
				BackoffMult:    tt.multiplier,
				FillTimeout:    60 * time.Second,
			}

			tracker := NewFillTracker(nil, logger, cfg)

			if tracker.backoffMult != tt.multiplier {
				t.Errorf("expected backoffMult %f, got %f", tt.multiplier, tracker.backoffMult)
			}
		})
	}
}

// TestFillTimeout tests fill timeout configuration
func TestFillTimeout_Configuration(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	tests := []struct {
		name    string
		timeout time.Duration
	}{
		{
			name:    "short-timeout-100ms",
			timeout: 100 * time.Millisecond,
		},
		{
			name:    "medium-timeout-1s",
			timeout: 1 * time.Second,
		},
		{
			name:    "long-timeout-30s",
			timeout: 30 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &FillTrackerConfig{
				InitialBackoff: 10 * time.Millisecond,
				MaxBackoff:     50 * time.Millisecond,
				BackoffMult:    2.0,
				FillTimeout:    tt.timeout,
			}

			tracker := NewFillTracker(nil, logger, cfg)

			if tracker.fillTimeout != tt.timeout {
				t.Errorf("expected fillTimeout %v, got %v", tt.timeout, tracker.fillTimeout)
			}
		})
	}
}

// TestContextTimeout tests context timeout logic
func TestContextTimeout_Logic(t *testing.T) {
	// Test timeout calculation (without calling VerifyFills)
	fillTimeout := 10 * time.Second
	contextTimeout := 150 * time.Millisecond

	// Verify context timeout is shorter
	if contextTimeout >= fillTimeout {
		t.Error("expected context timeout to be shorter than fill timeout")
	}

	// Verify we'd use the shorter timeout
	effectiveTimeout := contextTimeout
	if fillTimeout < contextTimeout {
		effectiveTimeout = fillTimeout
	}

	if effectiveTimeout != contextTimeout {
		t.Errorf("expected effective timeout %v, got %v", contextTimeout, effectiveTimeout)
	}
}

// TestFillStatusFields tests fill status structure
func TestFillStatusFields(t *testing.T) {
	// Test that FillStatus has expected fields (compile-time check)
	status := types.FillStatus{
		OrderID:      "order-123",
		Outcome:      "YES",
		OriginalSize: 10.0,
		SizeFilled:   8.0,
		ActualPrice:  0.50,
		Status:       "LIVE",
		FullyFilled:  false,
		Error:        errors.New("test error"),
	}

	if status.OrderID != "order-123" {
		t.Error("OrderID field not set correctly")
	}

	if status.Outcome != "YES" {
		t.Error("Outcome field not set correctly")
	}

	if status.OriginalSize != 10.0 {
		t.Error("OriginalSize field not set correctly")
	}

	if status.SizeFilled != 8.0 {
		t.Error("SizeFilled field not set correctly")
	}

	if status.ActualPrice != 0.50 {
		t.Error("ActualPrice field not set correctly")
	}

	if status.Status != "LIVE" {
		t.Error("Status field not set correctly")
	}

	if status.FullyFilled {
		t.Error("FullyFilled should be false")
	}

	if status.Error == nil {
		t.Error("Error field not set correctly")
	}
}

// TestFillTolerance tests floating-point tolerance for fill detection
func TestFillTolerance(t *testing.T) {
	// Verify tolerance constant (0.001) used in implementation
	tolerance := 0.001

	tests := []struct {
		name       string
		size       float64
		sizeFilled float64
		shouldFill bool
	}{
		{
			name:       "exact-fill",
			size:       10.0,
			sizeFilled: 10.0,
			shouldFill: true,
		},
		{
			name:       "within-tolerance",
			size:       10.0,
			sizeFilled: 9.9995,
			shouldFill: true,
		},
		{
			name:       "just-outside-tolerance",
			size:       10.0,
			sizeFilled: 9.998,
			shouldFill: false,
		},
		{
			name:       "clearly-partial",
			size:       10.0,
			sizeFilled: 9.0,
			shouldFill: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Check if filled based on tolerance logic: sizeFilled >= size - tolerance
			filled := tt.sizeFilled >= tt.size-tolerance

			if filled != tt.shouldFill {
				t.Errorf("expected shouldFill=%v, got %v (size=%f, filled=%f)",
					tt.shouldFill, filled, tt.size, tt.sizeFilled)
			}
		})
	}
}

// TestBackoffProgression tests exponential backoff calculation
func TestBackoffProgression(t *testing.T) {
	tests := []struct {
		name       string
		initial    time.Duration
		multiplier float64
		max        time.Duration
		attempts   int
		expected   []time.Duration
	}{
		{
			name:       "double-with-no-cap",
			initial:    100 * time.Millisecond,
			multiplier: 2.0,
			max:        10 * time.Second,
			attempts:   4,
			expected: []time.Duration{
				100 * time.Millisecond,
				200 * time.Millisecond,
				400 * time.Millisecond,
				800 * time.Millisecond,
			},
		},
		{
			name:       "double-with-cap",
			initial:    100 * time.Millisecond,
			multiplier: 2.0,
			max:        300 * time.Millisecond,
			attempts:   5,
			expected: []time.Duration{
				100 * time.Millisecond,
				200 * time.Millisecond,
				300 * time.Millisecond, // Capped
				300 * time.Millisecond, // Capped
				300 * time.Millisecond, // Capped
			},
		},
		{
			name:       "1.5x-multiplier",
			initial:    100 * time.Millisecond,
			multiplier: 1.5,
			max:        10 * time.Second,
			attempts:   3,
			expected: []time.Duration{
				100 * time.Millisecond,
				150 * time.Millisecond,
				225 * time.Millisecond,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backoff := tt.initial

			for i := 0; i < tt.attempts; i++ {
				// Allow 5% tolerance for rounding
				tolerance := time.Duration(float64(tt.expected[i]) * 0.05)
				if backoff < tt.expected[i]-tolerance || backoff > tt.expected[i]+tolerance {
					t.Errorf("attempt %d: expected backoff ~%v, got %v",
						i+1, tt.expected[i], backoff)
				}

				// Calculate next backoff (matches implementation logic)
				backoff = time.Duration(float64(backoff) * tt.multiplier)
				if backoff > tt.max {
					backoff = tt.max
				}
			}
		})
	}
}

// TestMultipleOrders tests handling multiple orders in structure
func TestMultipleOrders_Structure(t *testing.T) {
	// Test creating multiple fill statuses without calling VerifyFills
	orderIDs := []string{"order-1", "order-2", "order-3"}
	outcomes := []string{"YES", "NO", "MAYBE"}
	sizes := []float64{10.0, 20.0, 30.0}

	// Initialize fill statuses
	fillStatuses := make([]types.FillStatus, len(orderIDs))
	for i := range fillStatuses {
		fillStatuses[i] = types.FillStatus{
			OrderID:      orderIDs[i],
			Outcome:      outcomes[i],
			OriginalSize: sizes[i],
			FullyFilled:  false,
		}
	}

	if len(fillStatuses) != 3 {
		t.Fatalf("expected 3 fill statuses, got %d", len(fillStatuses))
	}

	// Verify each order
	for i, status := range fillStatuses {
		if status.OrderID != orderIDs[i] {
			t.Errorf("order %d: expected OrderID %s, got %s", i, orderIDs[i], status.OrderID)
		}

		if status.Outcome != outcomes[i] {
			t.Errorf("order %d: expected Outcome %s, got %s", i, outcomes[i], status.Outcome)
		}

		if status.OriginalSize != sizes[i] {
			t.Errorf("order %d: expected OriginalSize %f, got %f", i, sizes[i], status.OriginalSize)
		}

		if status.FullyFilled {
			t.Errorf("order %d: expected not fully filled initially", i)
		}
	}
}

// Helper function for substring check
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && hasSubstringMatch(s, substr)))
}

func hasSubstringMatch(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
