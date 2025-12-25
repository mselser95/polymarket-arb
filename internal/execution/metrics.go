package execution

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// TradesTotal tracks trade executions.
	TradesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "polymarket_execution_trades_total",
			Help: "Total number of trades executed",
		},
		[]string{"mode", "outcome"},
	)

	// ProfitRealizedUSD tracks cumulative profit.
	ProfitRealizedUSD = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "polymarket_execution_profit_realized_usd",
			Help: "Cumulative profit realized (hypothetical for paper trading)",
		},
		[]string{"mode"},
	)

	// ExecutionDurationSeconds tracks execution latency.
	ExecutionDurationSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "polymarket_execution_duration_seconds",
		Help:    "Duration of trade execution",
		Buckets: prometheus.DefBuckets,
	})

	// ExecutionErrorsTotal tracks execution failures.
	ExecutionErrorsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "polymarket_execution_errors_total",
		Help: "Total number of execution errors",
	})
)
