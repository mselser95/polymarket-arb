package websocket

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// ActiveConnections tracks active WebSocket connections.
	ActiveConnections = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "polymarket_ws_active_connections",
		Help: "Number of active WebSocket connections",
	})

	// ReconnectAttemptsTotal tracks reconnection attempts.
	ReconnectAttemptsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "polymarket_ws_reconnect_attempts_total",
		Help: "Total number of WebSocket reconnection attempts",
	})

	// ReconnectFailuresTotal tracks reconnection failures.
	ReconnectFailuresTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "polymarket_ws_reconnect_failures_total",
		Help: "Total number of WebSocket reconnection failures",
	})

	// MessagesReceivedTotal tracks messages received by type.
	MessagesReceivedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "polymarket_ws_messages_received_total",
			Help: "Total number of WebSocket messages received",
		},
		[]string{"event_type"},
	)

	// MessageLatencySeconds tracks message processing latency.
	MessageLatencySeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "polymarket_ws_message_latency_seconds",
		Help:    "WebSocket message processing latency",
		Buckets: prometheus.DefBuckets,
	})

	// SubscriptionCount tracks active market subscriptions.
	SubscriptionCount = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "polymarket_ws_subscription_count",
		Help: "Number of active market subscriptions",
	})
)
