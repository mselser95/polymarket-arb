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
// Supports both binary (2 outcomes) and multi-outcome (3+) markets.
// All orders are submitted atomically via the batch API endpoint.
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

	// Validate all outcomes have token IDs
	for i, outcome := range opp.Outcomes {
		if outcome.TokenID == "" {
			e.logger.Error("missing-token-id",
				zap.String("opportunity-id", opp.ID),
				zap.Int("outcome-index", i),
				zap.String("outcome", outcome.Outcome))
			return &types.ExecutionResult{
				OpportunityID: opp.ID,
				MarketSlug:    opp.MarketSlug,
				ExecutedAt:    now,
				Success:       false,
				Error:         fmt.Errorf("missing token ID for outcome %d (%s)", i, outcome.Outcome),
			}
		}
	}

	// Build log fields for all outcomes
	outcomeLogFields := make([]zap.Field, 0, len(opp.Outcomes)*2)
	for i, outcome := range opp.Outcomes {
		outcomeLogFields = append(outcomeLogFields,
			zap.String(fmt.Sprintf("outcome%d", i+1), outcome.Outcome),
			zap.Float64(fmt.Sprintf("price%d", i+1), outcome.AskPrice))
	}

	e.logger.Info("placing-multi-outcome-orders",
		append([]zap.Field{
			zap.String("market-slug", opp.MarketSlug),
			zap.Int("outcome-count", len(opp.Outcomes)),
			zap.Float64("size", opp.MaxTradeSize),
		}, outcomeLogFields...)...)

	// Build outcome parameters for order client
	outcomeParams := make([]OutcomeOrderParams, len(opp.Outcomes))
	for i, outcome := range opp.Outcomes {
		outcomeParams[i] = OutcomeOrderParams{
			TokenID:  outcome.TokenID,
			Price:    outcome.AskPrice,
			TickSize: outcome.TickSize,
			MinSize:  outcome.MinSize,
		}
	}

	// Place orders using batch endpoint for atomic submission
	ctx, cancel := context.WithTimeout(e.ctx, 30*time.Second)
	defer cancel()

	responses, err := e.orderClient.PlaceOrdersMultiOutcome(
		ctx,
		outcomeParams,
		opp.MaxTradeSize,
	)

	if err != nil {
		// Log detailed error information
		e.logger.Error("multi-outcome-order-placement-failed",
			zap.String("opportunity-id", opp.ID),
			zap.String("market-slug", opp.MarketSlug),
			zap.Int("outcome-count", len(opp.Outcomes)),
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

	// Verify all orders succeeded
	var failedOutcomes []string
	for i, resp := range responses {
		if !resp.Success {
			failedOutcomes = append(failedOutcomes,
				fmt.Sprintf("%s: %s", opp.Outcomes[i].Outcome, resp.ErrorMsg))
		}
	}

	if len(failedOutcomes) > 0 {
		errorMsg := strings.Join(failedOutcomes, "; ")
		e.logger.Error("some-orders-failed",
			zap.String("opportunity-id", opp.ID),
			zap.String("market-slug", opp.MarketSlug),
			zap.Strings("failed-outcomes", failedOutcomes))
		ExecutionErrorsTotal.Inc()
		return &types.ExecutionResult{
			OpportunityID: opp.ID,
			MarketSlug:    opp.MarketSlug,
			ExecutedAt:    now,
			Success:       false,
			Error:         fmt.Errorf("order failures: %s", errorMsg),
		}
	}

	// Create trade records for all outcomes
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
		TradesTotal.WithLabelValues("live", outcome.Outcome).Inc()
	}

	// Calculate realized profit
	realizedProfit := opp.MaxTradeSize * opp.ProfitMargin

	// Update metrics
	ProfitRealizedUSD.WithLabelValues("live").Add(realizedProfit)

	// Update cumulative profit
	e.mu.Lock()
	e.cumulativeProfit += realizedProfit
	cumulativeProfit := e.cumulativeProfit
	e.mu.Unlock()

	// Build log fields for order IDs and statuses
	orderLogFields := make([]zap.Field, 0, len(responses)*3)
	for i, resp := range responses {
		orderLogFields = append(orderLogFields,
			zap.String(fmt.Sprintf("outcome%d", i+1), opp.Outcomes[i].Outcome),
			zap.String(fmt.Sprintf("order-id%d", i+1), resp.OrderID),
			zap.String(fmt.Sprintf("status%d", i+1), resp.Status))
	}

	e.logger.Info("live-multi-outcome-trade-executed",
		append([]zap.Field{
			zap.String("market-slug", opp.MarketSlug),
			zap.String("question", opp.MarketQuestion),
			zap.Int("outcome-count", len(opp.Outcomes)),
			zap.Float64("size", opp.MaxTradeSize),
			zap.Int("profit-bps", opp.ProfitBPS),
			zap.Float64("profit-usd", realizedProfit),
			zap.Float64("cumulative-profit-usd", cumulativeProfit),
		}, orderLogFields...)...)

	// Create execution result
	result := &types.ExecutionResult{
		OpportunityID:  opp.ID,
		MarketSlug:     opp.MarketSlug,
		ExecutedAt:     now,
		AllTrades:      trades,
		RealizedProfit: realizedProfit,
		Success:        true,
		Error:          nil,
	}

	// For backward compatibility with binary markets, set YesTrade/NoTrade
	if len(trades) == 2 {
		result.YesTrade = trades[0]
		result.NoTrade = trades[1]
	}

	return result
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
