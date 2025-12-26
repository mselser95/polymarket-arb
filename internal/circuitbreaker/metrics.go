package circuitbreaker

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// CircuitBreakerEnabled indicates whether the circuit breaker allows trade execution.
	CircuitBreakerEnabled = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "polymarket_circuit_breaker_enabled",
		Help: "Whether circuit breaker allows trade execution (1=enabled, 0=disabled)",
	})

	// CircuitBreakerBalance tracks the last checked USDC balance.
	CircuitBreakerBalance = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "polymarket_circuit_breaker_balance_usdc",
		Help: "Last checked USDC balance in the wallet",
	})

	// CircuitBreakerDisableThreshold tracks the current threshold for disabling execution.
	CircuitBreakerDisableThreshold = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "polymarket_circuit_breaker_disable_threshold_usdc",
		Help: "Current USDC balance threshold for disabling execution (dynamically calculated)",
	})

	// CircuitBreakerEnableThreshold tracks the current threshold for re-enabling execution.
	CircuitBreakerEnableThreshold = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "polymarket_circuit_breaker_enable_threshold_usdc",
		Help: "Current USDC balance threshold for re-enabling execution (with hysteresis)",
	})

	// CircuitBreakerAvgTradeSize tracks the rolling average trade size.
	CircuitBreakerAvgTradeSize = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "polymarket_circuit_breaker_avg_trade_size_usdc",
		Help: "Rolling average trade size from recent trades (used for threshold calculation)",
	})

	// CircuitBreakerStateChanges tracks the number of times the circuit breaker changed state.
	CircuitBreakerStateChanges = promauto.NewCounter(prometheus.CounterOpts{
		Name: "polymarket_circuit_breaker_state_changes_total",
		Help: "Total number of times circuit breaker changed state (enabled/disabled)",
	})

	// CircuitBreakerCheckDuration tracks the time taken to check balance.
	CircuitBreakerCheckDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "polymarket_circuit_breaker_check_duration_seconds",
		Help:    "Time taken to check wallet balance",
		Buckets: prometheus.DefBuckets,
	})
)
