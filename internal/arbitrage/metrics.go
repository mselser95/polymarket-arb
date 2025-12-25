package arbitrage

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// OpportunitiesDetectedTotal tracks arbitrage opportunities detected.
	OpportunitiesDetectedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "polymarket_arb_opportunities_detected_total",
		Help: "Total number of arbitrage opportunities detected",
	})

	// OpportunityProfitBPS tracks profit margins in basis points.
	OpportunityProfitBPS = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "polymarket_arb_opportunity_profit_bps",
		Help:    "Arbitrage opportunity profit margin in basis points",
		Buckets: []float64{10, 25, 50, 100, 200, 500, 1000, 2000, 5000},
	})

	// OpportunitySizeUSD tracks trade sizes.
	OpportunitySizeUSD = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "polymarket_arb_opportunity_size_usd",
		Help:    "Arbitrage opportunity trade size in USD",
		Buckets: prometheus.ExponentialBuckets(10, 2, 10), // 10, 20, 40, ..., 5120
	})

	// DetectionDurationSeconds tracks detection loop latency.
	DetectionDurationSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "polymarket_arb_detection_duration_seconds",
		Help:    "Duration of arbitrage detection loop",
		Buckets: prometheus.DefBuckets,
	})
)
