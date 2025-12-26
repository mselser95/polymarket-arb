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

	// OpportunitiesRejectedTotal tracks rejected opportunities by reason.
	OpportunitiesRejectedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "polymarket_arb_opportunities_rejected_total",
			Help: "Total number of arbitrage opportunities rejected",
		},
		[]string{"reason"},
	)

	// NetProfitBPS tracks net profit after fees in basis points.
	NetProfitBPS = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "polymarket_arb_net_profit_bps",
		Help:    "Arbitrage opportunity net profit after fees in basis points",
		Buckets: []float64{10, 25, 50, 100, 200, 500, 1000, 2000, 5000},
	})

	// EndToEndLatencySeconds tracks orderbook update to opportunity detection latency.
	EndToEndLatencySeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "polymarket_arb_e2e_latency_seconds",
		Help:    "End-to-end latency from orderbook update to opportunity detection",
		Buckets: []float64{0.0001, 0.0002, 0.0005, 0.001, 0.002, 0.005, 0.01, 0.025, 0.05, 0.1},
	})
)
