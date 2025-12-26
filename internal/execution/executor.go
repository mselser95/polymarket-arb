package execution

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/mselser95/polymarket-arb/internal/arbitrage"
	"github.com/mselser95/polymarket-arb/internal/circuitbreaker"
	"github.com/mselser95/polymarket-arb/pkg/types"
	"go.uber.org/zap"
)

// Executor executes trades for arbitrage opportunities.
type Executor struct {
	mode             string // "paper" or "live"
	logger           *zap.Logger
	opportunityChan  <-chan *arbitrage.Opportunity
	ctx              context.Context
	wg               sync.WaitGroup
	cumulativeProfit float64
	mu               sync.Mutex
	orderClient      *OrderClient // For live trading
	circuitBreaker   *circuitbreaker.BalanceCircuitBreaker
}

// Config holds executor configuration.
type Config struct {
	Mode               string
	MaxPositionSize    float64
	Logger             *zap.Logger
	OpportunityChannel <-chan *arbitrage.Opportunity
	OrderClient        *OrderClient                          // Optional: for live trading
	CircuitBreaker     *circuitbreaker.BalanceCircuitBreaker // Optional: for balance monitoring
}

// New creates a new trade executor.
func New(cfg *Config) *Executor {
	return &Executor{
		mode:            cfg.Mode,
		logger:          cfg.Logger,
		opportunityChan: cfg.OpportunityChannel,
		orderClient:     cfg.OrderClient,
		circuitBreaker:  cfg.CircuitBreaker,
	}
}

// Start starts the executor.
func (e *Executor) Start(ctx context.Context) error {
	e.ctx = ctx
	e.logger.Info("executor-starting", zap.String("mode", e.mode))

	e.wg.Add(1)
	go e.executionLoop()

	return nil
}

// executionLoop processes opportunities.
func (e *Executor) executionLoop() {
	defer e.wg.Done()

	for {
		select {
		case <-e.ctx.Done():
			e.logger.Info("executor-stopping")
			return
		case opp, ok := <-e.opportunityChan:
			if !ok {
				e.logger.Info("opportunity-channel-closed")
				return
			}

			// Track opportunity received
			OpportunitiesReceived.Inc()

			// Check circuit breaker before executing
			if e.circuitBreaker != nil && !e.circuitBreaker.IsEnabled() {
				e.logger.Warn("skipping-opportunity-circuit-breaker-disabled",
					zap.String("opportunity-id", opp.ID),
					zap.String("market-slug", opp.MarketSlug),
					zap.Float64("spread", opp.ProfitMargin))
				OpportunitiesSkippedTotal.WithLabelValues("circuit_breaker").Inc()
				continue
			}

			start := time.Now()
			result := e.execute(opp)
			ExecutionDurationSeconds.Observe(time.Since(start).Seconds())

			if result.Error != nil {
				e.logger.Error("execution-failed",
					zap.String("opportunity-id", opp.ID),
					zap.Error(result.Error))

				// Classify error type
				errorType := classifyError(result.Error)
				ExecutionErrorsTotal.Inc()
				ExecutionErrorsByType.WithLabelValues(errorType).Inc()
			} else {
				// Track successful execution
				OpportunitiesExecuted.Inc()

				e.logger.Info("execution-successful",
					zap.String("opportunity-id", opp.ID),
					zap.String("market-slug", opp.MarketSlug),
					zap.Float64("profit", result.RealizedProfit))

				// Record successful trade for circuit breaker threshold calculation
				if e.circuitBreaker != nil && e.mode == "live" {
					e.circuitBreaker.RecordTrade(opp.MaxTradeSize)
				}
			}
		}
	}
}

// execute executes an arbitrage opportunity.
func (e *Executor) execute(opp *arbitrage.Opportunity) *types.ExecutionResult {
	switch e.mode {
	case "paper":
		return e.executePaper(opp)
	case "live":
		return e.executeLive(opp)
	default:
		return &types.ExecutionResult{
			OpportunityID: opp.ID,
			MarketSlug:    opp.MarketSlug,
			ExecutedAt:    time.Now(),
			Success:       false,
			Error:         fmt.Errorf("unknown execution mode: %s", e.mode),
		}
	}
}

// executePaper executes a paper trade (simulated).
// Supports both binary (2 outcomes) and multi-outcome (3+) markets.
func (e *Executor) executePaper(opp *arbitrage.Opportunity) *types.ExecutionResult {
	now := time.Now()

	// Simulate buying all outcomes
	trades := make([]*types.Trade, len(opp.Outcomes))
	for i, outcome := range opp.Outcomes {
		trades[i] = &types.Trade{
			Outcome:   outcome.Outcome,
			Side:      "BUY",
			Price:     outcome.AskPrice,
			Size:      opp.MaxTradeSize,
			Timestamp: now,
		}

		// Update metrics for each outcome
		TradesTotal.WithLabelValues("paper", outcome.Outcome).Inc()
	}

	// Calculate realized profit
	realizedProfit := opp.MaxTradeSize * opp.ProfitMargin

	// Update metrics
	ProfitRealizedUSD.WithLabelValues("paper").Add(realizedProfit)

	// Update cumulative profit
	e.mu.Lock()
	e.cumulativeProfit += realizedProfit
	cumulativeProfit := e.cumulativeProfit
	e.mu.Unlock()

	// Build log fields for all outcomes
	outcomeFields := make([]zap.Field, 0, len(opp.Outcomes))
	for i, outcome := range opp.Outcomes {
		outcomeFields = append(outcomeFields,
			zap.Float64(fmt.Sprintf("outcome%d-price", i+1), outcome.AskPrice),
			zap.String(fmt.Sprintf("outcome%d-name", i+1), outcome.Outcome))
	}

	baseFields := []zap.Field{
		zap.String("market-slug", opp.MarketSlug),
		zap.String("question", opp.MarketQuestion),
		zap.Int("outcome-count", len(opp.Outcomes)),
		zap.Float64("size", opp.MaxTradeSize),
		zap.Int("profit-bps", opp.ProfitBPS),
		zap.Float64("profit-usd", realizedProfit),
		zap.Float64("cumulative-profit-usd", cumulativeProfit),
	}

	e.logger.Info("paper-trade-executed", append(baseFields, outcomeFields...)...)

	// Create execution result
	result := &types.ExecutionResult{
		OpportunityID:  opp.ID,
		MarketSlug:     opp.MarketSlug,
		ExecutedAt:     now,
		RealizedProfit: realizedProfit,
		Success:        true,
		Error:          nil,
		AllTrades:      trades, // Store all trades
	}

	// For backward compatibility with binary markets, set YesTrade/NoTrade
	if len(trades) == 2 {
		result.YesTrade = trades[0]
		result.NoTrade = trades[1]
	}

	return result
}

// executeLive executes a live trade via Polymarket CLOB API.
// LIMITATION: Currently only supports binary markets (2 outcomes) due to PlaceOrdersBatch API.
// Multi-outcome markets would require either:
// - Sequential order placement (higher latency, partial fill risk)
// - Polymarket CLOB batch API for N orders (not yet implemented)
//
// Use paper mode for testing multi-outcome markets.
func (e *Executor) executeLive(opp *arbitrage.Opportunity) *types.ExecutionResult {
	now := time.Now()

	if e.orderClient == nil {
		e.logger.Error("order-client-not-configured")
		return &types.ExecutionResult{
			OpportunityID: opp.ID,
			MarketSlug:    opp.MarketSlug,
			ExecutedAt:    now,
			Success:       false,
			Error:         fmt.Errorf("order client not configured"),
		}
	}

	// Only execute binary markets (2 outcomes) in live mode
	if len(opp.Outcomes) != 2 {
		e.logger.Info("skipping-multi-outcome-live-execution",
			zap.String("opportunity-id", opp.ID),
			zap.String("market-slug", opp.MarketSlug),
			zap.Int("outcome-count", len(opp.Outcomes)),
			zap.Float64("net-profit-usd", opp.NetProfit),
			zap.String("reason", "live execution requires batch API for N>2 outcomes"))
		OpportunitiesSkippedTotal.WithLabelValues("multi_outcome_live").Inc()
		return &types.ExecutionResult{
			OpportunityID: opp.ID,
			MarketSlug:    opp.MarketSlug,
			ExecutedAt:    now,
			Success:       false,
			Error:         fmt.Errorf("live execution only supports binary markets (got %d outcomes) - use paper mode for multi-outcome", len(opp.Outcomes)),
		}
	}

	// Extract binary outcomes
	outcome1 := opp.Outcomes[0]
	outcome2 := opp.Outcomes[1]

	// Get token IDs from outcomes
	if outcome1.TokenID == "" || outcome2.TokenID == "" {
		e.logger.Error("missing-token-ids")
		return &types.ExecutionResult{
			OpportunityID: opp.ID,
			MarketSlug:    opp.MarketSlug,
			ExecutedAt:    now,
			Success:       false,
			Error:         fmt.Errorf("missing token IDs"),
		}
	}

	e.logger.Info("placing-orders",
		zap.String("market-slug", opp.MarketSlug),
		zap.Float64("outcome1-price", outcome1.AskPrice),
		zap.Float64("outcome2-price", outcome2.AskPrice),
		zap.Float64("size", opp.MaxTradeSize))

	// Place orders using batch endpoint for atomic submission
	ctx, cancel := context.WithTimeout(e.ctx, 30*time.Second)
	defer cancel()

	yesResp, noResp, err := e.orderClient.PlaceOrdersBatch(
		ctx,
		outcome1.TokenID,
		outcome2.TokenID,
		opp.MaxTradeSize,
		outcome1.AskPrice,
		outcome2.AskPrice,
		outcome1.TickSize,
		outcome1.MinSize,
		outcome2.TickSize,
		outcome2.MinSize,
	)

	if err != nil {
		// Log detailed error information
		errorMsg := err.Error()
		if yesResp != nil && !yesResp.Success {
			errorMsg = fmt.Sprintf("YES: %s", yesResp.ErrorMsg)
		}
		if noResp != nil && !noResp.Success {
			if yesResp != nil && !yesResp.Success {
				errorMsg = fmt.Sprintf("YES: %s, NO: %s", yesResp.ErrorMsg, noResp.ErrorMsg)
			} else {
				errorMsg = fmt.Sprintf("NO: %s", noResp.ErrorMsg)
			}
		}

		e.logger.Error("order-placement-failed",
			zap.String("opportunity-id", opp.ID),
			zap.String("error-msg", errorMsg),
			zap.Error(err))

		ExecutionErrorsTotal.Inc()

		return &types.ExecutionResult{
			OpportunityID: opp.ID,
			MarketSlug:    opp.MarketSlug,
			ExecutedAt:    now,
			Success:       false,
			Error:         err,
		}
	}

	// Verify both orders succeeded
	if !yesResp.Success {
		e.logger.Error("yes-order-failed",
			zap.String("opportunity-id", opp.ID),
			zap.String("error-msg", yesResp.ErrorMsg))
		ExecutionErrorsTotal.Inc()
		return &types.ExecutionResult{
			OpportunityID: opp.ID,
			MarketSlug:    opp.MarketSlug,
			ExecutedAt:    now,
			Success:       false,
			Error:         fmt.Errorf("YES order failed: %s", yesResp.ErrorMsg),
		}
	}

	if !noResp.Success {
		e.logger.Error("no-order-failed",
			zap.String("opportunity-id", opp.ID),
			zap.String("error-msg", noResp.ErrorMsg))
		ExecutionErrorsTotal.Inc()
		return &types.ExecutionResult{
			OpportunityID: opp.ID,
			MarketSlug:    opp.MarketSlug,
			ExecutedAt:    now,
			Success:       false,
			Error:         fmt.Errorf("NO order failed: %s", noResp.ErrorMsg),
		}
	}

	// Create trade records using opportunity data
	// Note: OrderSubmissionResponse doesn't include price/size, so we use the opportunity values
	trade1 := &types.Trade{
		Outcome:   outcome1.Outcome,
		Side:      "BUY",
		Price:     outcome1.AskPrice,
		Size:      opp.MaxTradeSize,
		Timestamp: now,
	}

	trade2 := &types.Trade{
		Outcome:   outcome2.Outcome,
		Side:      "BUY",
		Price:     outcome2.AskPrice,
		Size:      opp.MaxTradeSize,
		Timestamp: now,
	}

	// Calculate realized profit
	realizedProfit := opp.MaxTradeSize * opp.ProfitMargin

	// Update metrics
	TradesTotal.WithLabelValues("live", outcome1.Outcome).Inc()
	TradesTotal.WithLabelValues("live", outcome2.Outcome).Inc()
	ProfitRealizedUSD.WithLabelValues("live").Add(realizedProfit)

	// Update cumulative profit
	e.mu.Lock()
	e.cumulativeProfit += realizedProfit
	cumulativeProfit := e.cumulativeProfit
	e.mu.Unlock()

	e.logger.Info("live-trade-executed",
		zap.String("market-slug", opp.MarketSlug),
		zap.String("order1-id", yesResp.OrderID),
		zap.String("order1-status", yesResp.Status),
		zap.String("order2-id", noResp.OrderID),
		zap.String("order2-status", noResp.Status),
		zap.Float64("outcome1-price", outcome1.AskPrice),
		zap.Float64("outcome2-price", outcome2.AskPrice),
		zap.Float64("size", opp.MaxTradeSize),
		zap.Int("profit-bps", opp.ProfitBPS),
		zap.Float64("profit-usd", realizedProfit),
		zap.Float64("cumulative-profit-usd", cumulativeProfit))

	return &types.ExecutionResult{
		OpportunityID:  opp.ID,
		MarketSlug:     opp.MarketSlug,
		ExecutedAt:     now,
		YesTrade:       trade1,
		NoTrade:        trade2,
		RealizedProfit: realizedProfit,
		Success:        true,
		Error:          nil,
	}
}

// Close gracefully closes the executor.
func (e *Executor) Close() error {
	e.logger.Info("closing-executor")
	e.wg.Wait()

	e.mu.Lock()
	finalProfit := e.cumulativeProfit
	e.mu.Unlock()

	e.logger.Info("executor-closed",
		zap.Float64("total-profit-usd", finalProfit),
		zap.String("mode", e.mode))

	return nil
}

// classifyError classifies an execution error by type.
func classifyError(err error) string {
	if err == nil {
		return "unknown"
	}

	errMsg := strings.ToLower(err.Error())

	// Network errors
	if strings.Contains(errMsg, "connection refused") ||
		strings.Contains(errMsg, "timeout") ||
		strings.Contains(errMsg, "dial") ||
		strings.Contains(errMsg, "eof") ||
		strings.Contains(errMsg, "network") {
		return "network"
	}

	// API/validation errors
	if strings.Contains(errMsg, "api error") ||
		strings.Contains(errMsg, "invalid") ||
		strings.Contains(errMsg, "bad request") ||
		strings.Contains(errMsg, "400") ||
		strings.Contains(errMsg, "403") ||
		strings.Contains(errMsg, "404") ||
		strings.Contains(errMsg, "500") {
		return "api"
	}

	// Validation errors (client-side)
	if strings.Contains(errMsg, "missing") ||
		strings.Contains(errMsg, "required") ||
		strings.Contains(errMsg, "not configured") {
		return "validation"
	}

	// Insufficient funds
	if strings.Contains(errMsg, "insufficient") ||
		strings.Contains(errMsg, "balance") ||
		strings.Contains(errMsg, "funds") {
		return "funds"
	}

	return "unknown"
}
