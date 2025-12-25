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
)
