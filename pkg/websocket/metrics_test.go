package websocket

import (
	"testing"
)

// TestMetrics_Registration tests all metrics are initialized
func TestMetrics_Registration(t *testing.T) {
	if ActiveConnections == nil {
		t.Error("ActiveConnections not registered")
	}

	if ReconnectAttemptsTotal == nil {
		t.Error("ReconnectAttemptsTotal not registered")
	}

	if ReconnectFailuresTotal == nil {
		t.Error("ReconnectFailuresTotal not registered")
	}

	if MessagesReceivedTotal == nil {
		t.Error("MessagesReceivedTotal not registered")
	}

	if MessageLatencySeconds == nil {
		t.Error("MessageLatencySeconds not registered")
	}

	if SubscriptionCount == nil {
		t.Error("SubscriptionCount not registered")
	}

	if MessagesDroppedTotal == nil {
		t.Error("MessagesDroppedTotal not registered")
	}

	if ConnectionDuration == nil {
		t.Error("ConnectionDuration not registered")
	}

	if UnsubscriptionsTotal == nil {
		t.Error("UnsubscriptionsTotal not registered")
	}
}

// TestMetrics_CounterIncrement tests counter can be incremented
func TestMetrics_CounterIncrement(t *testing.T) {
	ReconnectAttemptsTotal.Inc()
	ReconnectFailuresTotal.Inc()
	UnsubscriptionsTotal.Inc()
	MessagesReceivedTotal.WithLabelValues("book").Inc()
	MessagesDroppedTotal.WithLabelValues("channel_full").Inc()
}

// TestMetrics_GaugeSet tests gauge can be set
func TestMetrics_GaugeSet(t *testing.T) {
	ActiveConnections.Set(1)
	SubscriptionCount.Set(100)
}

// TestMetrics_HistogramObserve tests histogram can observe values
func TestMetrics_HistogramObserve(t *testing.T) {
	MessageLatencySeconds.Observe(0.001)
	ConnectionDuration.Observe(3600)
}

// TestMetrics_Labels tests label values are accepted
func TestMetrics_Labels(t *testing.T) {
	MessagesReceivedTotal.WithLabelValues("book").Inc()
	MessagesReceivedTotal.WithLabelValues("price_change").Inc()
	MessagesReceivedTotal.WithLabelValues("heartbeat").Inc()

	MessagesDroppedTotal.WithLabelValues("channel_full").Inc()
	MessagesDroppedTotal.WithLabelValues("slow_consumer").Inc()
}
