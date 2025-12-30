package arbitrage

import (
	"testing"
)

// TestMetrics_Registration tests all metrics are initialized
func TestMetrics_Registration(t *testing.T) {
	if OpportunitiesDetectedTotal == nil {
		t.Error("OpportunitiesDetectedTotal not registered")
	}

	if OpportunityProfitBPS == nil {
		t.Error("OpportunityProfitBPS not registered")
	}

	if OpportunitySizeUSD == nil {
		t.Error("OpportunitySizeUSD not registered")
	}

	if DetectionDurationSeconds == nil {
		t.Error("DetectionDurationSeconds not registered")
	}

	if OpportunitiesRejectedTotal == nil {
		t.Error("OpportunitiesRejectedTotal not registered")
	}

	if NetProfitBPS == nil {
		t.Error("NetProfitBPS not registered")
	}

	if EndToEndLatencySeconds == nil {
		t.Error("EndToEndLatencySeconds not registered")
	}
}

// TestMetrics_CounterIncrement tests counter can be incremented
func TestMetrics_CounterIncrement(t *testing.T) {
	// Test counter increment (no error means it works)
	OpportunitiesDetectedTotal.Inc()

	// Test labeled counter
	OpportunitiesRejectedTotal.WithLabelValues("price_sum_too_high").Inc()
	OpportunitiesRejectedTotal.WithLabelValues("size_too_small").Inc()
}

// TestMetrics_HistogramObserve tests histogram can observe values
func TestMetrics_HistogramObserve(t *testing.T) {
	// Test histograms
	OpportunityProfitBPS.Observe(150.0)
	OpportunitySizeUSD.Observe(50.0)
	DetectionDurationSeconds.Observe(0.001)
	NetProfitBPS.Observe(100.0)
	EndToEndLatencySeconds.Observe(0.0005)
}

// TestMetrics_Labels tests label values are accepted
func TestMetrics_Labels(t *testing.T) {
	// Test different rejection reasons
	reasons := []string{
		"price_sum_too_high",
		"size_too_small",
		"size_too_large",
		"no_token_ids",
		"metadata_error",
	}

	for _, reason := range reasons {
		OpportunitiesRejectedTotal.WithLabelValues(reason).Inc()
	}
}
