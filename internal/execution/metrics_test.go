package execution

import (
	"testing"
)

// TestMetrics_Registration tests all metrics are initialized
func TestMetrics_Registration(t *testing.T) {
	if TradesTotal == nil {
		t.Error("TradesTotal not registered")
	}

	if ProfitRealizedUSD == nil {
		t.Error("ProfitRealizedUSD not registered")
	}

	if ExecutionDurationSeconds == nil {
		t.Error("ExecutionDurationSeconds not registered")
	}

	if ExecutionErrorsTotal == nil {
		t.Error("ExecutionErrorsTotal not registered")
	}

	if ExecutionErrorsByType == nil {
		t.Error("ExecutionErrorsByType not registered")
	}

	if OpportunitiesReceived == nil {
		t.Error("OpportunitiesReceived not registered")
	}

	if OpportunitiesExecuted == nil {
		t.Error("OpportunitiesExecuted not registered")
	}

	if OpportunitiesSkippedTotal == nil {
		t.Error("OpportunitiesSkippedTotal not registered")
	}
}

// TestMetrics_CounterIncrement tests counter can be incremented
func TestMetrics_CounterIncrement(t *testing.T) {
	TradesTotal.WithLabelValues("paper", "yes").Inc()
	TradesTotal.WithLabelValues("paper", "no").Inc()
	ProfitRealizedUSD.WithLabelValues("paper").Add(10.5)
	ExecutionErrorsTotal.Inc()
	ExecutionErrorsByType.WithLabelValues("order_failed").Inc()
	OpportunitiesReceived.Inc()
	OpportunitiesExecuted.Inc()
	OpportunitiesSkippedTotal.WithLabelValues("circuit_breaker").Inc()
}

// TestMetrics_HistogramObserve tests histogram can observe values
func TestMetrics_HistogramObserve(t *testing.T) {
	ExecutionDurationSeconds.Observe(0.1)
}

// TestMetrics_Labels tests label values are accepted
func TestMetrics_Labels(t *testing.T) {
	TradesTotal.WithLabelValues("live", "yes").Inc()
	TradesTotal.WithLabelValues("paper", "no").Inc()

	ProfitRealizedUSD.WithLabelValues("live").Add(5.0)
	ProfitRealizedUSD.WithLabelValues("paper").Add(10.0)

	ExecutionErrorsByType.WithLabelValues("order_failed").Inc()
	ExecutionErrorsByType.WithLabelValues("insufficient_balance").Inc()

	OpportunitiesSkippedTotal.WithLabelValues("circuit_breaker").Inc()
	OpportunitiesSkippedTotal.WithLabelValues("validation_failed").Inc()
}
