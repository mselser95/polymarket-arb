package discovery

import (
	"testing"
)

// TestMetrics_Registration tests all metrics are initialized
func TestMetrics_Registration(t *testing.T) {
	if MarketsDiscoveredTotal == nil {
		t.Error("MarketsDiscoveredTotal not registered")
	}

	if NewMarketsTotal == nil {
		t.Error("NewMarketsTotal not registered")
	}

	if PollDurationSeconds == nil {
		t.Error("PollDurationSeconds not registered")
	}

	if PollErrorsTotal == nil {
		t.Error("PollErrorsTotal not registered")
	}

	if MarketsFilteredByEndDateTotal == nil {
		t.Error("MarketsFilteredByEndDateTotal not registered")
	}
}

// TestMetrics_CounterIncrement tests counter can be incremented
func TestMetrics_CounterIncrement(t *testing.T) {
	MarketsDiscoveredTotal.Inc()
	NewMarketsTotal.Inc()
	PollErrorsTotal.Inc()
	MarketsFilteredByEndDateTotal.Inc()
}

// TestMetrics_HistogramObserve tests histogram can observe values
func TestMetrics_HistogramObserve(t *testing.T) {
	PollDurationSeconds.Observe(0.5)
}
