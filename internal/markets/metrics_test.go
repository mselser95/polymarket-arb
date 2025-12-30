package markets

import (
	"testing"
)

// TestMetrics_Registration tests all metrics are initialized
func TestMetrics_Registration(t *testing.T) {
	if MetadataFetchDuration == nil {
		t.Error("MetadataFetchDuration not registered")
	}

	if MetadataFetchErrorsTotal == nil {
		t.Error("MetadataFetchErrorsTotal not registered")
	}

	if MetadataCacheHitsTotal == nil {
		t.Error("MetadataCacheHitsTotal not registered")
	}

	if MetadataCacheMissesTotal == nil {
		t.Error("MetadataCacheMissesTotal not registered")
	}
}

// TestMetrics_CounterIncrement tests counter can be incremented
func TestMetrics_CounterIncrement(t *testing.T) {
	MetadataFetchErrorsTotal.Inc()
	MetadataCacheHitsTotal.Inc()
	MetadataCacheMissesTotal.Inc()
}

// TestMetrics_HistogramObserve tests histogram can observe values
func TestMetrics_HistogramObserve(t *testing.T) {
	MetadataFetchDuration.Observe(0.1)
}
