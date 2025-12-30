package cache

import (
	"testing"
)

// TestMetrics_Registration tests all metrics are initialized
func TestMetrics_Registration(t *testing.T) {
	if CacheHitsTotal == nil {
		t.Error("CacheHitsTotal not registered")
	}

	if CacheMissesTotal == nil {
		t.Error("CacheMissesTotal not registered")
	}

	if CacheSetsTotal == nil {
		t.Error("CacheSetsTotal not registered")
	}

	if CacheDeletesTotal == nil {
		t.Error("CacheDeletesTotal not registered")
	}

	if CacheHitRate == nil {
		t.Error("CacheHitRate not registered")
	}

	if CacheOperationDuration == nil {
		t.Error("CacheOperationDuration not registered")
	}
}

// TestMetrics_CounterIncrement tests counter can be incremented
func TestMetrics_CounterIncrement(t *testing.T) {
	CacheHitsTotal.Inc()
	CacheMissesTotal.Inc()
	CacheSetsTotal.Inc()
	CacheDeletesTotal.Inc()
}

// TestMetrics_GaugeSet tests gauge can be set
func TestMetrics_GaugeSet(t *testing.T) {
	CacheHitRate.Set(0.95)
}

// TestMetrics_HistogramObserve tests histogram can observe values
func TestMetrics_HistogramObserve(t *testing.T) {
	CacheOperationDuration.WithLabelValues("get").Observe(0.0001)
	CacheOperationDuration.WithLabelValues("set").Observe(0.0002)
	CacheOperationDuration.WithLabelValues("delete").Observe(0.00015)
}

// TestMetrics_Labels tests label values are accepted
func TestMetrics_Labels(t *testing.T) {
	CacheOperationDuration.WithLabelValues("get").Observe(0.0001)
	CacheOperationDuration.WithLabelValues("set").Observe(0.0001)
	CacheOperationDuration.WithLabelValues("delete").Observe(0.0001)
}
