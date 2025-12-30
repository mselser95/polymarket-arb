package circuitbreaker

import (
	"testing"
)

// TestMetrics_Registration tests all metrics are initialized
func TestMetrics_Registration(t *testing.T) {
	if CircuitBreakerEnabled == nil {
		t.Error("CircuitBreakerEnabled not registered")
	}

	if CircuitBreakerBalance == nil {
		t.Error("CircuitBreakerBalance not registered")
	}

	if CircuitBreakerDisableThreshold == nil {
		t.Error("CircuitBreakerDisableThreshold not registered")
	}

	if CircuitBreakerEnableThreshold == nil {
		t.Error("CircuitBreakerEnableThreshold not registered")
	}

	if CircuitBreakerAvgTradeSize == nil {
		t.Error("CircuitBreakerAvgTradeSize not registered")
	}

	if CircuitBreakerStateChanges == nil {
		t.Error("CircuitBreakerStateChanges not registered")
	}

	if CircuitBreakerCheckDuration == nil {
		t.Error("CircuitBreakerCheckDuration not registered")
	}
}

// TestMetrics_GaugeSet tests gauge can be set
func TestMetrics_GaugeSet(t *testing.T) {
	CircuitBreakerEnabled.Set(1.0)
	CircuitBreakerBalance.Set(100.0)
	CircuitBreakerDisableThreshold.Set(30.0)
	CircuitBreakerEnableThreshold.Set(45.0)
	CircuitBreakerAvgTradeSize.Set(10.0)
}

// TestMetrics_CounterIncrement tests counter can be incremented
func TestMetrics_CounterIncrement(t *testing.T) {
	CircuitBreakerStateChanges.Inc()
}

// TestMetrics_HistogramObserve tests histogram can observe values
func TestMetrics_HistogramObserve(t *testing.T) {
	CircuitBreakerCheckDuration.Observe(0.001)
}

// TestMetrics_StateTransitions tests state transitions
func TestMetrics_StateTransitions(t *testing.T) {
	// Enabled state
	CircuitBreakerEnabled.Set(1.0)

	// Disabled state
	CircuitBreakerEnabled.Set(0.0)

	// Track state change
	CircuitBreakerStateChanges.Inc()
}
