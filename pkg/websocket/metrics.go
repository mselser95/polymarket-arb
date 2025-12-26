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

	// MessagesDroppedTotal tracks messages dropped due to full channel.
	MessagesDroppedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "polymarket_ws_messages_dropped_total",
			Help: "Total number of WebSocket messages dropped due to channel full",
		},
		[]string{"reason"},
	)

	// ConnectionDuration tracks WebSocket connection lifetime.
	ConnectionDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "polymarket_ws_connection_duration_seconds",
		Help:    "Duration of WebSocket connections before disconnect",
		Buckets: []float64{60, 300, 600, 1800, 3600, 7200, 14400, 28800, 43200, 86400},
	})

	// UnsubscriptionsTotal tracks market unsubscriptions.
	UnsubscriptionsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "polymarket_ws_unsubscriptions_total",
		Help: "Total number of market unsubscriptions",
	})

	// ==============================
	// Pool-specific metrics
	// ==============================

	// PoolActiveConnections tracks active connections in pool.
	PoolActiveConnections = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "polymarket_ws_pool_active_connections",
		Help: "Number of active connections in WebSocket pool",
	})

	// PoolSubscriptionDistribution tracks distribution of subscriptions across pool connections.
	PoolSubscriptionDistribution = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "polymarket_ws_pool_subscription_distribution",
		Help:    "Distribution of subscriptions across pool connections",
		Buckets: prometheus.LinearBuckets(0, 100, 10),
	})

	// PoolMessageMultiplexLatency tracks latency added by message multiplexing.
	PoolMessageMultiplexLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "polymarket_ws_pool_multiplex_latency_seconds",
		Help:    "Latency added by message multiplexing in pool",
		Buckets: prometheus.ExponentialBuckets(0.000001, 2, 20),
	})
)
