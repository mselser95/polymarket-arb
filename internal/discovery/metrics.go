package discovery

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// MarketsDiscoveredTotal tracks total markets discovered.
	MarketsDiscoveredTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "polymarket_discovery_markets_total",
		Help: "Total number of markets discovered from Gamma API",
	})

	// NewMarketsTotal tracks new markets subscribed.
	NewMarketsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "polymarket_discovery_new_markets_total",
		Help: "Total number of new markets subscribed",
	})

	// PollDurationSeconds tracks API poll latency.
	PollDurationSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "polymarket_discovery_poll_duration_seconds",
		Help:    "Duration of Gamma API poll requests",
		Buckets: prometheus.DefBuckets,
	})

	// PollErrorsTotal tracks API poll failures.
	PollErrorsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "polymarket_discovery_poll_errors_total",
		Help: "Total number of Gamma API poll failures",
	})
)
