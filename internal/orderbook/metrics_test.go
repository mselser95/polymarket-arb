package orderbook

import (
	"testing"
)

// TestMetrics_Registration tests all metrics are initialized
func TestMetrics_Registration(t *testing.T) {
	if UpdatesTotal == nil {
		t.Error("UpdatesTotal not registered")
	}

	if SnapshotsTracked == nil {
		t.Error("SnapshotsTracked not registered")
	}

	if UpdatesDroppedTotal == nil {
		t.Error("UpdatesDroppedTotal not registered")
	}

	if UpdateProcessingDuration == nil {
		t.Error("UpdateProcessingDuration not registered")
	}

	if LockContentionDuration == nil {
		t.Error("LockContentionDuration not registered")
	}
}

// TestMetrics_CounterIncrement tests counter can be incremented
func TestMetrics_CounterIncrement(t *testing.T) {
	UpdatesTotal.WithLabelValues("book").Inc()
	UpdatesDroppedTotal.WithLabelValues("channel_full").Inc()
}

// TestMetrics_GaugeSet tests gauge can be set
func TestMetrics_GaugeSet(t *testing.T) {
	SnapshotsTracked.Set(100)
}

// TestMetrics_HistogramObserve tests histogram can observe values
func TestMetrics_HistogramObserve(t *testing.T) {
	UpdateProcessingDuration.Observe(0.001)
	LockContentionDuration.Observe(0.0005)
}

// TestMetrics_Labels tests label values are accepted
func TestMetrics_Labels(t *testing.T) {
	UpdatesTotal.WithLabelValues("book").Inc()
	UpdatesTotal.WithLabelValues("price_change").Inc()
	UpdatesTotal.WithLabelValues("heartbeat").Inc()

	UpdatesDroppedTotal.WithLabelValues("channel_full").Inc()
	UpdatesDroppedTotal.WithLabelValues("slow_consumer").Inc()
}
