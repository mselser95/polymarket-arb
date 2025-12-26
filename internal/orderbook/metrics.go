package orderbook

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// UpdatesTotal tracks orderbook updates by event type.
	UpdatesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "polymarket_orderbook_updates_total",
			Help: "Total number of orderbook updates",
		},
		[]string{"event_type"},
	)

	// SnapshotsTracked tracks the number of orderbook snapshots in memory.
	SnapshotsTracked = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "polymarket_orderbook_snapshots_tracked",
		Help: "Number of orderbook snapshots tracked in memory",
	})

	// UpdatesDroppedTotal tracks orderbook updates dropped due to full channel.
	UpdatesDroppedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "polymarket_orderbook_updates_dropped_total",
			Help: "Total number of orderbook updates dropped due to channel full",
		},
		[]string{"reason"},
	)

	// UpdateProcessingDuration tracks orderbook update processing time.
	UpdateProcessingDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "polymarket_orderbook_update_processing_duration_seconds",
		Help:    "Time to process orderbook update (parse + update + notify)",
		Buckets: []float64{0.0001, 0.0002, 0.0005, 0.001, 0.002, 0.005, 0.01, 0.025, 0.05, 0.1},
	})

	// LockContentionDuration tracks time waiting for mutex acquisition.
	LockContentionDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "polymarket_orderbook_lock_contention_seconds",
		Help:    "Time waiting to acquire orderbook mutex lock",
		Buckets: []float64{0.0001, 0.0002, 0.0005, 0.001, 0.002, 0.005, 0.01, 0.025, 0.05, 0.1},
	})
)
