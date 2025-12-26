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

	// ExecutionErrorsByType tracks execution failures by error type.
	ExecutionErrorsByType = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "polymarket_execution_errors_by_type_total",
			Help: "Total number of execution errors classified by type",
		},
		[]string{"error_type"},
	)

	// OpportunitiesReceived tracks opportunities received for execution.
	OpportunitiesReceived = promauto.NewCounter(prometheus.CounterOpts{
		Name: "polymarket_execution_opportunities_received_total",
		Help: "Total number of arbitrage opportunities received for execution",
	})

	// OpportunitiesExecuted tracks successfully executed opportunities.
	OpportunitiesExecuted = promauto.NewCounter(prometheus.CounterOpts{
		Name: "polymarket_execution_opportunities_executed_total",
		Help: "Total number of opportunities successfully executed",
	})

	// OpportunitiesSkippedTotal tracks opportunities skipped for various reasons.
	OpportunitiesSkippedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "polymarket_execution_opportunities_skipped_total",
			Help: "Total number of opportunities skipped (by reason)",
		},
		[]string{"reason"},
	)

	// FillVerificationTotal tracks fill verification attempts by result.
	FillVerificationTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "polymarket_execution_fill_verification_total",
			Help: "Total fill verification attempts by result (success, partial, timeout)",
		},
		[]string{"result"},
	)

	// FillVerificationDurationSeconds tracks fill verification duration.
	FillVerificationDurationSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "polymarket_execution_fill_verification_duration_seconds",
		Help:    "Duration of fill verification process",
		Buckets: []float64{1, 2, 5, 10, 20, 30, 60},
	})

	// ActualFillPriceDeviation tracks difference between expected and actual fill prices.
	ActualFillPriceDeviation = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "polymarket_execution_actual_fill_price_deviation",
		Help:    "Difference between expected and actual fill price",
		Buckets: prometheus.LinearBuckets(-0.01, 0.001, 20),
	})
)
