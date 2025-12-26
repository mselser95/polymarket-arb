package cache

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

//nolint:gochecknoglobals // Prometheus metrics
var (
	CacheHitsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "polymarket_cache_hits_total",
		Help: "Total number of cache hits",
	})

	CacheMissesTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "polymarket_cache_misses_total",
		Help: "Total number of cache misses",
	})

	CacheSetsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "polymarket_cache_sets_total",
		Help: "Total number of cache sets",
	})

	CacheDeletesTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "polymarket_cache_deletes_total",
		Help: "Total number of cache deletes",
	})

	// CacheHitRate tracks cache hit rate (calculated as hits / (hits + misses)).
	CacheHitRate = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "polymarket_cache_hit_rate",
		Help: "Cache hit rate (hits / (hits + misses))",
	})

	// CacheOperationDuration tracks cache operation latency.
	CacheOperationDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "polymarket_cache_operation_duration_seconds",
		Help:    "Duration of cache operations",
		Buckets: []float64{0.00001, 0.00005, 0.0001, 0.0005, 0.001, 0.005, 0.01},
	}, []string{"operation"})
)
