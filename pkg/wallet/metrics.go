package wallet

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

//nolint:gochecknoglobals // Prometheus metrics
var (
	// MATICBalance tracks the current MATIC balance for gas fees.
	MATICBalance = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "polymarket_wallet_matic_balance",
		Help: "Current MATIC balance in wallet (native units)",
	})

	// USDCBalance tracks the current USDC balance for trading.
	USDCBalance = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "polymarket_wallet_usdc_balance",
		Help: "Current USDC balance in wallet (USD)",
	})

	// USDCAllowance tracks the USDC allowance approved to CTF Exchange.
	USDCAllowance = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "polymarket_wallet_usdc_allowance",
		Help: "USDC allowance approved to CTF Exchange (USD)",
	})

	// ActivePositions tracks the number of open positions.
	ActivePositions = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "polymarket_wallet_active_positions",
		Help: "Number of open positions",
	})

	// TotalPositionValue tracks the sum of all position current values.
	TotalPositionValue = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "polymarket_wallet_total_position_value",
		Help: "Sum of all position current values (USD)",
	})

	// TotalPositionCost tracks the sum of all position initial costs.
	TotalPositionCost = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "polymarket_wallet_total_position_cost",
		Help: "Sum of all position initial costs (USD)",
	})

	// UnrealizedPnL tracks the total unrealized profit/loss from positions.
	UnrealizedPnL = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "polymarket_wallet_unrealized_pnl",
		Help: "Total unrealized P&L from positions (USD)",
	})

	// UnrealizedPnLPercent tracks the total unrealized P&L as a percentage.
	UnrealizedPnLPercent = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "polymarket_wallet_unrealized_pnl_percent",
		Help: "Total unrealized P&L as percentage",
	})

	// PortfolioValue tracks the total portfolio value (USDC + positions).
	PortfolioValue = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "polymarket_wallet_portfolio_value",
		Help: "Total portfolio value: USDC + positions (USD)",
	})

	// UpdateErrorsTotal tracks the number of failed update attempts.
	UpdateErrorsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "polymarket_wallet_update_errors_total",
		Help: "Total number of failed wallet update attempts",
	})

	// UpdateDuration tracks the time taken to fetch wallet data.
	UpdateDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "polymarket_wallet_update_duration_seconds",
		Help:    "Time taken to fetch wallet data (seconds)",
		Buckets: prometheus.DefBuckets,
	})

	// LastUpdateTimestamp tracks the Unix timestamp of the last successful update.
	LastUpdateTimestamp = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "polymarket_wallet_last_update_timestamp",
		Help: "Unix timestamp of last successful wallet update",
	})
)
