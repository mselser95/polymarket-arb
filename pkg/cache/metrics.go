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
)
