package markets

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// MetadataFetchDuration tracks metadata API fetch latency.
	MetadataFetchDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "polymarket_markets_metadata_fetch_duration_seconds",
		Help:    "Duration of metadata fetch from CLOB API",
		Buckets: prometheus.DefBuckets,
	})

	// MetadataFetchErrors tracks metadata fetch failures.
	MetadataFetchErrorsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "polymarket_markets_metadata_fetch_errors_total",
		Help: "Total number of metadata fetch errors",
	})

	// MetadataCacheHits tracks cache hits for metadata.
	MetadataCacheHitsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "polymarket_markets_metadata_cache_hits_total",
		Help: "Total number of metadata cache hits",
	})

	// MetadataCacheMisses tracks cache misses for metadata.
	MetadataCacheMissesTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "polymarket_markets_metadata_cache_misses_total",
		Help: "Total number of metadata cache misses",
	})
)
