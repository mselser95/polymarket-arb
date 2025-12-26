package execution

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/mselser95/polymarket-arb/internal/arbitrage"
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
}

// Config holds executor configuration.
type Config struct {
	Mode               string
	MaxPositionSize    float64
	Logger             *zap.Logger
	OpportunityChannel <-chan *arbitrage.Opportunity
	OrderClient        *OrderClient // Optional: for live trading
}

// New creates a new trade executor.
func New(cfg *Config) *Executor {
	return &Executor{
		mode:            cfg.Mode,
		logger:          cfg.Logger,
		opportunityChan: cfg.OpportunityChannel,
		orderClient:     cfg.OrderClient,
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
func (e *Executor) executePaper(opp *arbitrage.Opportunity) *types.ExecutionResult {
	now := time.Now()

	// Simulate buying YES and NO tokens at bid prices
	yesTrade := &types.Trade{
		Outcome:   "YES",
		Side:      "BUY",
		Price:     opp.YesAskPrice,
		Size:      opp.MaxTradeSize,
		Timestamp: now,
	}

	noTrade := &types.Trade{
		Outcome:   "NO",
		Side:      "BUY",
		Price:     opp.NoAskPrice,
		Size:      opp.MaxTradeSize,
		Timestamp: now,
	}

	// Calculate realized profit
	// Cost: (yesBidPrice + noBidPrice) * size
	// Value when market resolves: 1.0 * size (one side wins)
	// Profit: size - (yesBidPrice + noBidPrice) * size = size * (1 - priceSum)
	realizedProfit := opp.MaxTradeSize * opp.ProfitMargin

	// Update metrics
	TradesTotal.WithLabelValues("paper", "YES").Inc()
	TradesTotal.WithLabelValues("paper", "NO").Inc()
	ProfitRealizedUSD.WithLabelValues("paper").Add(realizedProfit)

	// Update cumulative profit
	e.mu.Lock()
	e.cumulativeProfit += realizedProfit
	cumulativeProfit := e.cumulativeProfit
	e.mu.Unlock()

	e.logger.Info("paper-trade-executed",
		zap.String("market-slug", opp.MarketSlug),
		zap.String("question", opp.MarketQuestion),
		zap.Float64("yes-price", opp.YesAskPrice),
		zap.Float64("no-price", opp.NoAskPrice),
		zap.Float64("size", opp.MaxTradeSize),
		zap.Int("profit-bps", opp.ProfitBPS),
		zap.Float64("profit-usd", realizedProfit),
		zap.Float64("cumulative-profit-usd", cumulativeProfit))

	return &types.ExecutionResult{
		OpportunityID:  opp.ID,
		MarketSlug:     opp.MarketSlug,
		ExecutedAt:     now,
		YesTrade:       yesTrade,
		NoTrade:        noTrade,
		RealizedProfit: realizedProfit,
		Success:        true,
		Error:          nil,
	}
}

// executeLive executes a live trade via Polymarket CLOB API.
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

	// Get token IDs from opportunity
	if opp.YesTokenID == "" || opp.NoTokenID == "" {
		e.logger.Error("missing-token-ids")
		return &types.ExecutionResult{
			OpportunityID: opp.ID,
			MarketSlug:    opp.MarketSlug,
			ExecutedAt:    now,
			Success:       false,
			Error:         fmt.Errorf("missing token IDs"),
		}
	}

	yesTokenID := opp.YesTokenID
	noTokenID := opp.NoTokenID

	e.logger.Info("placing-orders",
		zap.String("market-slug", opp.MarketSlug),
		zap.Float64("yes-price", opp.YesAskPrice),
		zap.Float64("no-price", opp.NoAskPrice),
		zap.Float64("size", opp.MaxTradeSize))

	// Place orders using batch endpoint for atomic submission
	ctx, cancel := context.WithTimeout(e.ctx, 30*time.Second)
	defer cancel()

	yesResp, noResp, err := e.orderClient.PlaceOrdersBatch(
		ctx,
		yesTokenID,
		noTokenID,
		opp.MaxTradeSize,
		opp.YesAskPrice,
		opp.NoAskPrice,
		opp.YesTickSize,
		opp.YesMinSize,
		opp.NoTickSize,
		opp.NoMinSize,
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
	yesTrade := &types.Trade{
		Outcome:   "YES",
		Side:      "BUY",
		Price:     opp.YesAskPrice,     // Use opportunity price
		Size:      opp.MaxTradeSize,    // Use opportunity size
		Timestamp: now,
	}

	noTrade := &types.Trade{
		Outcome:   "NO",
		Side:      "BUY",
		Price:     opp.NoAskPrice,      // Use opportunity price
		Size:      opp.MaxTradeSize,    // Use opportunity size
		Timestamp: now,
	}

	// Calculate realized profit
	realizedProfit := opp.MaxTradeSize * opp.ProfitMargin

	// Update metrics
	TradesTotal.WithLabelValues("live", "YES").Inc()
	TradesTotal.WithLabelValues("live", "NO").Inc()
	ProfitRealizedUSD.WithLabelValues("live").Add(realizedProfit)

	// Update cumulative profit
	e.mu.Lock()
	e.cumulativeProfit += realizedProfit
	cumulativeProfit := e.cumulativeProfit
	e.mu.Unlock()

	e.logger.Info("live-trade-executed",
		zap.String("market-slug", opp.MarketSlug),
		zap.String("yes-order-id", yesResp.OrderID),
		zap.String("yes-status", yesResp.Status),
		zap.String("no-order-id", noResp.OrderID),
		zap.String("no-status", noResp.Status),
		zap.Float64("yes-price", opp.YesAskPrice),
		zap.Float64("no-price", opp.NoAskPrice),
		zap.Float64("size", opp.MaxTradeSize),
		zap.Int("profit-bps", opp.ProfitBPS),
		zap.Float64("profit-usd", realizedProfit),
		zap.Float64("cumulative-profit-usd", cumulativeProfit))

	return &types.ExecutionResult{
		OpportunityID:  opp.ID,
		MarketSlug:     opp.MarketSlug,
		ExecutedAt:     now,
		YesTrade:       yesTrade,
		NoTrade:        noTrade,
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
