package wallet

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"go.uber.org/zap"
)

// Tracker periodically fetches wallet data and updates Prometheus metrics.
type Tracker struct {
	client       *Client
	address      common.Address
	pollInterval time.Duration
	logger       *zap.Logger
}

// Config holds tracker configuration.
type Config struct {
	RPCEndpoint  string
	Address      common.Address
	PollInterval time.Duration
	Logger       *zap.Logger
}

// New creates a new wallet tracker.
func New(cfg *Config) (t *Tracker, err error) {
	if cfg == nil {
		return nil, errors.New("config cannot be nil")
	}

	if cfg.Logger == nil {
		return nil, errors.New("logger cannot be nil")
	}

	if cfg.RPCEndpoint == "" {
		return nil, errors.New("RPC endpoint cannot be empty")
	}

	if cfg.PollInterval <= 0 {
		return nil, errors.New("poll interval must be positive")
	}

	client, err := NewClient(cfg.RPCEndpoint, cfg.Logger)
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}

	tracker := &Tracker{
		client:       client,
		address:      cfg.Address,
		pollInterval: cfg.PollInterval,
		logger:       cfg.Logger,
	}

	return tracker, nil
}

// Run starts the tracker polling loop (blocking).
func (t *Tracker) Run(ctx context.Context) (err error) {
	t.logger.Info("wallet-tracker-starting",
		zap.Duration("poll-interval", t.pollInterval),
		zap.String("address", t.address.Hex()))

	ticker := time.NewTicker(t.pollInterval)
	defer ticker.Stop()

	// Initial poll
	pollErr := t.poll(ctx)
	if pollErr != nil {
		t.logger.Error("initial-poll-failed", zap.Error(pollErr))
		UpdateErrorsTotal.Inc()
	}

	for {
		select {
		case <-ctx.Done():
			t.logger.Info("wallet-tracker-stopping")
			return ctx.Err()
		case <-ticker.C:
			pollErr = t.poll(ctx)
			if pollErr != nil {
				t.logger.Error("poll-failed", zap.Error(pollErr))
				UpdateErrorsTotal.Inc()
			}
		}
	}
}

// poll performs a single polling cycle.
func (t *Tracker) poll(ctx context.Context) (err error) {
	start := time.Now()
	defer func() {
		UpdateDuration.Observe(time.Since(start).Seconds())
	}()

	// Fetch balances with timeout
	balCtx, balCancel := context.WithTimeout(ctx, 15*time.Second)
	defer balCancel()

	balances, err := t.client.GetBalances(balCtx, t.address)
	if err != nil {
		return fmt.Errorf("get balances: %w", err)
	}

	// Fetch positions with timeout
	posCtx, posCancel := context.WithTimeout(ctx, 15*time.Second)
	defer posCancel()

	positions, err := t.client.GetPositions(posCtx, t.address.Hex())
	if err != nil {
		return fmt.Errorf("get positions: %w", err)
	}

	// Update metrics
	t.updateMetrics(balances, positions)
	LastUpdateTimestamp.Set(float64(time.Now().Unix()))

	t.logger.Debug("poll-complete",
		zap.Int("position-count", len(positions)),
		zap.Duration("duration", time.Since(start)))

	return nil
}

// updateMetrics updates Prometheus gauges with wallet data.
func (t *Tracker) updateMetrics(balances *Balances, positions []Position) {
	// Convert MATIC from wei to float64
	maticFloat := new(big.Float).Quo(
		new(big.Float).SetInt(balances.MATIC),
		big.NewFloat(1e18))
	maticVal, _ := maticFloat.Float64()
	MATICBalance.Set(maticVal)

	// Convert USDC from 6 decimals to float64
	usdcFloat := new(big.Float).Quo(
		new(big.Float).SetInt(balances.USDC),
		big.NewFloat(1e6))
	usdcVal, _ := usdcFloat.Float64()
	USDCBalance.Set(usdcVal)

	// Convert USDC allowance from 6 decimals to float64
	allowanceFloat := new(big.Float).Quo(
		new(big.Float).SetInt(balances.USDCAllowance),
		big.NewFloat(1e6))
	allowanceVal, _ := allowanceFloat.Float64()
	USDCAllowance.Set(allowanceVal)

	// Calculate position aggregates
	totalValue := 0.0
	totalCost := 0.0
	totalPnL := 0.0

	for _, pos := range positions {
		totalValue += pos.Value
		totalCost += pos.InitialValue
		totalPnL += pos.CashPnL
	}

	ActivePositions.Set(float64(len(positions)))
	TotalPositionValue.Set(totalValue)
	TotalPositionCost.Set(totalCost)
	UnrealizedPnL.Set(totalPnL)

	// Calculate percentage P&L
	pnlPct := 0.0
	if totalCost > 0 {
		pnlPct = (totalPnL / totalCost) * 100
	}
	UnrealizedPnLPercent.Set(pnlPct)

	// Portfolio value = USDC + positions
	portfolioVal := usdcVal + totalValue
	PortfolioValue.Set(portfolioVal)
}
