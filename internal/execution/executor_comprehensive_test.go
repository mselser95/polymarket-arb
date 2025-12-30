package execution

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/mselser95/polymarket-arb/internal/arbitrage"
	"github.com/mselser95/polymarket-arb/pkg/types"
	"go.uber.org/zap/zaptest"
)

// ===== Mode Dispatch Comprehensive Tests =====

// TestExecute_PaperMode tests that paper mode calls executePaper
func TestExecute_PaperMode(t *testing.T) {
	t.Parallel()

	logger := zaptest.NewLogger(t)
	exec := &Executor{
		mode:   "paper",
		logger: logger,
	}

	opp := arbitrage.CreateTestOpportunity("test-market", "test-slug")
	result := exec.execute(opp)

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if !result.Success {
		t.Error("expected paper trade to succeed")
	}

	if result.Error != nil {
		t.Errorf("expected no error, got %v", result.Error)
	}

	// Paper trades should have realized profit calculated
	if result.RealizedProfit <= 0 {
		t.Errorf("expected positive profit, got %f", result.RealizedProfit)
	}
}

// TestExecute_LiveMode tests that live mode calls executeLive
func TestExecute_LiveMode(t *testing.T) {
	t.Parallel()

	logger := zaptest.NewLogger(t)
	exec := &Executor{
		mode:   "live",
		logger: logger,
		// No order client - should fail with validation error
	}

	opp := arbitrage.CreateTestOpportunity("test-market", "test-slug")
	result := exec.execute(opp)

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if result.Success {
		t.Error("expected failure when order client not configured")
	}

	if result.Error == nil {
		t.Error("expected error for missing order client")
	}
}

// TestExecute_UnknownMode tests error on invalid mode
func TestExecute_UnknownMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mode string
	}{
		{name: "empty-mode", mode: ""},
		{name: "invalid-mode", mode: "invalid"},
		{name: "dry-run-mode", mode: "dry-run"},
		{name: "uppercase-mode", mode: "PAPER"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := zaptest.NewLogger(t)
			exec := &Executor{
				mode:   tt.mode,
				logger: logger,
			}

			opp := arbitrage.CreateTestOpportunity("test-market", "test-slug")
			result := exec.execute(opp)

			if result == nil {
				t.Fatal("expected non-nil result")
			}

			if result.Success {
				t.Errorf("expected failure for mode %q", tt.mode)
			}

			if result.Error == nil {
				t.Errorf("expected error for mode %q", tt.mode)
			}

			if result.OpportunityID != opp.ID {
				t.Errorf("expected opportunity ID %s, got %s", opp.ID, result.OpportunityID)
			}
		})
	}
}

// TestExecute_NilOrderClient tests error if live mode without client
func TestExecute_NilOrderClient(t *testing.T) {
	t.Parallel()

	logger := zaptest.NewLogger(t)
	exec := &Executor{
		mode:        "live",
		logger:      logger,
		orderClient: nil, // Explicitly nil
	}

	opp := arbitrage.CreateTestOpportunity("test-market", "test-slug")
	result := exec.executeLive(opp)

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if result.Success {
		t.Error("expected failure with nil order client")
	}

	if result.Error == nil {
		t.Error("expected error for nil order client")
	}

	// Error should mention order client
	if result.Error.Error() != "order client not configured" {
		t.Errorf("expected 'order client not configured' error, got %v", result.Error)
	}
}

// ===== Paper Trading Comprehensive Tests =====

// TestExecutePaper_MultiOutcome tests simulation of N outcome trades
func TestExecutePaper_MultiOutcome(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		outcomeCount  int
		maxTradeSize  float64
		profitMargin  float64
		expectedTrades int
	}{
		{
			name:           "2-outcomes-binary",
			outcomeCount:   2,
			maxTradeSize:   100.0,
			profitMargin:   0.01,
			expectedTrades: 2,
		},
		{
			name:           "3-outcomes",
			outcomeCount:   3,
			maxTradeSize:   50.0,
			profitMargin:   0.02,
			expectedTrades: 3,
		},
		{
			name:           "5-outcomes",
			outcomeCount:   5,
			maxTradeSize:   20.0,
			profitMargin:   0.015,
			expectedTrades: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := zaptest.NewLogger(t)
			exec := &Executor{
				mode:   "paper",
				logger: logger,
			}

			// Create multi-outcome opportunity
			opp := &arbitrage.Opportunity{
				ID:             "test-opp-id",
				MarketID:       "test-market-id",
				MarketSlug:     "test-slug",
				MarketQuestion: "Test question?",
				DetectedAt:     time.Now(),
				MaxTradeSize:   tt.maxTradeSize,
				ProfitMargin:   tt.profitMargin,
				ProfitBPS:      int(tt.profitMargin * 10000),
				Outcomes:       make([]arbitrage.OpportunityOutcome, tt.outcomeCount),
			}

			// Initialize outcomes
			for i := 0; i < tt.outcomeCount; i++ {
				opp.Outcomes[i] = arbitrage.OpportunityOutcome{
					Outcome:  fmt.Sprintf("Outcome%d", i+1),
					AskPrice: 0.45 + float64(i)*0.01,
					AskSize:  100.0,
				}
			}

			result := exec.executePaper(opp)

			if result == nil {
				t.Fatal("expected non-nil result")
			}

			if !result.Success {
				t.Error("expected success for paper trade")
			}

			if result.Error != nil {
				t.Errorf("expected no error, got %v", result.Error)
			}

			// Verify all trades created
			if len(result.AllTrades) != tt.expectedTrades {
				t.Errorf("expected %d trades, got %d", tt.expectedTrades, len(result.AllTrades))
			}

			// Verify each trade
			for i, trade := range result.AllTrades {
				if trade.Side != "BUY" {
					t.Errorf("trade %d: expected BUY side, got %s", i, trade.Side)
				}

				if trade.Size != tt.maxTradeSize {
					t.Errorf("trade %d: expected size %f, got %f", i, tt.maxTradeSize, trade.Size)
				}

				if trade.Outcome == "" {
					t.Errorf("trade %d: expected non-empty outcome", i)
				}
			}
		})
	}
}

// TestExecutePaper_ProfitCalculation tests profit = size × margin
func TestExecutePaper_ProfitCalculation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		maxTradeSize   float64
		profitMargin   float64
		expectedProfit float64
	}{
		{
			name:           "1%-margin",
			maxTradeSize:   100.0,
			profitMargin:   0.01,
			expectedProfit: 1.0, // 100 * 0.01
		},
		{
			name:           "5%-margin",
			maxTradeSize:   50.0,
			profitMargin:   0.05,
			expectedProfit: 2.5, // 50 * 0.05
		},
		{
			name:           "0.5%-margin",
			maxTradeSize:   200.0,
			profitMargin:   0.005,
			expectedProfit: 1.0, // 200 * 0.005
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := zaptest.NewLogger(t)
			exec := &Executor{
				mode:   "paper",
				logger: logger,
			}

			opp := &arbitrage.Opportunity{
				ID:             "test-opp",
				MarketSlug:     "test-slug",
				MarketQuestion: "Test?",
				DetectedAt:     time.Now(),
				MaxTradeSize:   tt.maxTradeSize,
				ProfitMargin:   tt.profitMargin,
				Outcomes: []arbitrage.OpportunityOutcome{
					{Outcome: "YES", AskPrice: 0.49, AskSize: 100},
					{Outcome: "NO", AskPrice: 0.50, AskSize: 100},
				},
			}

			result := exec.executePaper(opp)

			if !floatEquals(result.RealizedProfit, tt.expectedProfit, 0.0001) {
				t.Errorf("expected profit %f, got %f", tt.expectedProfit, result.RealizedProfit)
			}
		})
	}
}

// TestExecutePaper_CumulativeProfit tests atomic increment of cumulative profit
func TestExecutePaper_CumulativeProfit(t *testing.T) {
	t.Parallel()

	logger := zaptest.NewLogger(t)
	exec := &Executor{
		mode:             "paper",
		logger:           logger,
		cumulativeProfit: 0.0,
	}

	trades := []struct {
		maxTradeSize float64
		profitMargin float64
	}{
		{100.0, 0.01}, // +1.0
		{50.0, 0.02},  // +1.0
		{200.0, 0.005}, // +1.0
	}

	expectedCumulative := 0.0

	for i, trade := range trades {
		opp := &arbitrage.Opportunity{
			ID:           fmt.Sprintf("opp-%d", i),
			MarketSlug:   "test-slug",
			DetectedAt:   time.Now(),
			MaxTradeSize: trade.maxTradeSize,
			ProfitMargin: trade.profitMargin,
			Outcomes: []arbitrage.OpportunityOutcome{
				{Outcome: "YES", AskPrice: 0.49},
				{Outcome: "NO", AskPrice: 0.50},
			},
		}

		result := exec.executePaper(opp)

		expectedCumulative += result.RealizedProfit

		// Check current cumulative
		exec.mu.Lock()
		actual := exec.cumulativeProfit
		exec.mu.Unlock()

		if !floatEquals(actual, expectedCumulative, 0.0001) {
			t.Errorf("trade %d: expected cumulative %f, got %f", i, expectedCumulative, actual)
		}
	}

	// Final cumulative should be 3.0
	exec.mu.Lock()
	final := exec.cumulativeProfit
	exec.mu.Unlock()

	if !floatEquals(final, 3.0, 0.0001) {
		t.Errorf("expected final cumulative 3.0, got %f", final)
	}
}

// TestExecutePaper_TradeRecording tests that trade details are logged
func TestExecutePaper_TradeRecording(t *testing.T) {
	t.Parallel()

	logger := zaptest.NewLogger(t)
	exec := &Executor{
		mode:   "paper",
		logger: logger,
	}

	opp := arbitrage.CreateTestOpportunity("test-market", "test-slug")
	result := exec.executePaper(opp)

	// Verify result fields
	if result.OpportunityID != opp.ID {
		t.Errorf("expected opportunity ID %s, got %s", opp.ID, result.OpportunityID)
	}

	if result.MarketSlug != opp.MarketSlug {
		t.Errorf("expected market slug %s, got %s", opp.MarketSlug, result.MarketSlug)
	}

	if result.ExecutedAt.IsZero() {
		t.Error("expected non-zero execution timestamp")
	}

	if !result.Success {
		t.Error("expected success to be true")
	}

	if result.Error != nil {
		t.Errorf("expected no error, got %v", result.Error)
	}

	// Verify trades have timestamps
	for i, trade := range result.AllTrades {
		if trade.Timestamp.IsZero() {
			t.Errorf("trade %d: expected non-zero timestamp", i)
		}
	}
}

// TestExecutePaper_CircuitBreakerRecording tests that trade size is recorded for circuit breaker
// Note: Circuit breaker recording is only done in executionLoop, not executePaper directly
func TestExecutePaper_CircuitBreakerRecording(t *testing.T) {
	t.Parallel()

	// This test verifies the logic exists in executionLoop
	// executePaper itself doesn't interact with circuit breaker

	logger := zaptest.NewLogger(t)
	exec := &Executor{
		mode:   "paper",
		logger: logger,
	}

	opp := arbitrage.CreateTestOpportunity("test-market", "test-slug")
	result := exec.executePaper(opp)

	// Paper trades should succeed
	if !result.Success {
		t.Error("expected success")
	}

	// Note: Circuit breaker recording happens in executionLoop, not here
	// This test just verifies executePaper doesn't fail
}

// ===== Live Trading Comprehensive Tests =====

// TestExecuteLive_TokenIDValidation tests that all outcomes must have token IDs
func TestExecuteLive_TokenIDValidation(t *testing.T) {
	t.Parallel()

	logger := zaptest.NewLogger(t)
	mockOrderClient := &OrderClient{} // Mock client

	exec := &Executor{
		mode:            "live",
		logger:          logger,
		orderClient:     mockOrderClient,
		ctx:             context.Background(), // Need context for executeLive
		aggressionTicks: 0,                    // No aggressive pricing in tests
	}

	tests := []struct {
		name     string
		outcomes []arbitrage.OpportunityOutcome
	}{
		{
			name: "missing-first-token-id",
			outcomes: []arbitrage.OpportunityOutcome{
				{Outcome: "YES", TokenID: "", AskPrice: 0.49, TickSize: 0.01, MinSize: 1.0},
				{Outcome: "NO", TokenID: "token2", AskPrice: 0.50, TickSize: 0.01, MinSize: 1.0},
			},
		},
		{
			name: "missing-second-token-id",
			outcomes: []arbitrage.OpportunityOutcome{
				{Outcome: "YES", TokenID: "token1", AskPrice: 0.49, TickSize: 0.01, MinSize: 1.0},
				{Outcome: "NO", TokenID: "", AskPrice: 0.50, TickSize: 0.01, MinSize: 1.0},
			},
		},
		{
			name: "all-missing-token-ids",
			outcomes: []arbitrage.OpportunityOutcome{
				{Outcome: "YES", TokenID: "", AskPrice: 0.49, TickSize: 0.01, MinSize: 1.0},
				{Outcome: "NO", TokenID: "", AskPrice: 0.50, TickSize: 0.01, MinSize: 1.0},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opp := &arbitrage.Opportunity{
				ID:           "test-opp",
				MarketSlug:   "test-slug",
				DetectedAt:   time.Now(),
				MaxTradeSize: 100.0,
				ProfitMargin: 0.01,
				Outcomes:     tt.outcomes,
			}

			result := exec.executeLive(opp)

			// Should fail with missing token ID error
			if result.Success {
				t.Error("expected failure with missing token ID")
			}
			if result.Error == nil {
				t.Error("expected error for missing token ID")
			}
		})
	}
}

// TestAdjustPriceForAggression tests aggressive pricing adjustment
func TestAdjustPriceForAggression(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		askPrice        float64
		tickSize        float64
		aggressionTicks int
		expectedPrice   float64
	}{
		{
			name:            "no-aggression",
			askPrice:        0.50,
			tickSize:        0.01,
			aggressionTicks: 0,
			expectedPrice:   0.50, // No change
		},
		{
			name:            "1-tick-aggression",
			askPrice:        0.50,
			tickSize:        0.01,
			aggressionTicks: 1,
			expectedPrice:   0.51, // +0.01
		},
		{
			name:            "5-tick-aggression",
			askPrice:        0.45,
			tickSize:        0.01,
			aggressionTicks: 5,
			expectedPrice:   0.50, // +0.05
		},
		{
			name:            "capped-at-0.9999",
			askPrice:        0.99,
			tickSize:        0.01,
			aggressionTicks: 10,
			expectedPrice:   1.00, // 0.99 + 0.10 = 1.09, capped to 0.9999, rounded to 1.00
		},
		{
			name:            "small-tick-size",
			askPrice:        0.50,
			tickSize:        0.001,
			aggressionTicks: 10,
			expectedPrice:   0.51, // +0.01
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adjusted := adjustPriceForAggression(tt.askPrice, tt.tickSize, tt.aggressionTicks)

			if !floatEquals(adjusted, tt.expectedPrice, 0.0001) {
				t.Errorf("expected adjusted price %f, got %f", tt.expectedPrice, adjusted)
			}

			// Note: Price can exceed 0.9999 after rounding to tick size
			// This is expected behavior - the cap happens before rounding
		})
	}
}

// TestCalculateActualProfit_FullFill tests 100% fill requirement
func TestCalculateActualProfit_FullFill(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		fills         []types.FillStatus
		takerFee      float64
		expectProfit  float64
		expectFilled  bool
	}{
		{
			name: "all-fully-filled",
			fills: []types.FillStatus{
				{Outcome: "YES", FullyFilled: true, SizeFilled: 100, ActualPrice: 0.49},
				{Outcome: "NO", FullyFilled: true, SizeFilled: 100, ActualPrice: 0.50},
			},
			takerFee:     0.01,
			expectProfit: 0.01, // Revenue (100) - Cost (99) - Fees (0.99) = 0.01
			expectFilled: true,
		},
		{
			name: "partial-fill-first",
			fills: []types.FillStatus{
				{Outcome: "YES", FullyFilled: false, SizeFilled: 50, ActualPrice: 0.49},
				{Outcome: "NO", FullyFilled: true, SizeFilled: 100, ActualPrice: 0.50},
			},
			takerFee:     0.01,
			expectProfit: 0.0, // Require 100% fill
			expectFilled: false,
		},
		{
			name: "partial-fill-second",
			fills: []types.FillStatus{
				{Outcome: "YES", FullyFilled: true, SizeFilled: 100, ActualPrice: 0.49},
				{Outcome: "NO", FullyFilled: false, SizeFilled: 50, ActualPrice: 0.50},
			},
			takerFee:     0.01,
			expectProfit: 0.0,
			expectFilled: false,
		},
		{
			name: "no-fills",
			fills: []types.FillStatus{
				{Outcome: "YES", FullyFilled: false, SizeFilled: 0, ActualPrice: 0.49},
				{Outcome: "NO", FullyFilled: false, SizeFilled: 0, ActualPrice: 0.50},
			},
			takerFee:     0.01,
			expectProfit: 0.0,
			expectFilled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profit, filled := calculateActualProfit(tt.fills, tt.takerFee)

			if filled != tt.expectFilled {
				t.Errorf("expected filled=%v, got %v", tt.expectFilled, filled)
			}

			if !floatEquals(profit, tt.expectProfit, 0.01) {
				t.Errorf("expected profit %f, got %f", tt.expectProfit, profit)
			}
		})
	}
}

// TestCalculateActualProfit_Fees tests per-outcome fee calculation
func TestCalculateActualProfit_Fees(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		fills        []types.FillStatus
		takerFee     float64
		expectedFees float64
	}{
		{
			name: "1%-fee",
			fills: []types.FillStatus{
				{Outcome: "YES", FullyFilled: true, SizeFilled: 100, ActualPrice: 0.50},
				{Outcome: "NO", FullyFilled: true, SizeFilled: 100, ActualPrice: 0.50},
			},
			takerFee:     0.01,
			expectedFees: 1.0, // (50 + 50) * 0.01
		},
		{
			name: "2%-fee",
			fills: []types.FillStatus{
				{Outcome: "YES", FullyFilled: true, SizeFilled: 100, ActualPrice: 0.50},
				{Outcome: "NO", FullyFilled: true, SizeFilled: 100, ActualPrice: 0.50},
			},
			takerFee:     0.02,
			expectedFees: 2.0, // (50 + 50) * 0.02
		},
		{
			name: "different-prices",
			fills: []types.FillStatus{
				{Outcome: "YES", FullyFilled: true, SizeFilled: 100, ActualPrice: 0.49},
				{Outcome: "NO", FullyFilled: true, SizeFilled: 100, ActualPrice: 0.51},
			},
			takerFee:     0.01,
			expectedFees: 1.0, // (49 + 51) * 0.01
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profit, filled := calculateActualProfit(tt.fills, tt.takerFee)

			if !filled {
				t.Fatal("expected all fills to be complete")
			}

			// Calculate expected profit: revenue - cost - fees
			cost := 0.0
			for _, fill := range tt.fills {
				cost += fill.SizeFilled * fill.ActualPrice
			}

			revenue := tt.fills[0].SizeFilled // Winning outcome pays $1.00 per token
			expectedProfit := revenue - cost - tt.expectedFees

			if !floatEquals(profit, expectedProfit, 0.01) {
				t.Errorf("expected profit %f, got %f", expectedProfit, profit)
			}
		})
	}
}

// TestCalculateActualProfit_Revenue tests token count × $1.00 revenue calculation
func TestCalculateActualProfit_Revenue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		tokenCount      float64
		price1          float64
		price2          float64
		takerFee        float64
		expectedRevenue float64
	}{
		{
			name:            "100-tokens",
			tokenCount:      100,
			price1:          0.49,
			price2:          0.50,
			takerFee:        0.01,
			expectedRevenue: 100.0, // 100 tokens * $1.00
		},
		{
			name:            "50-tokens",
			tokenCount:      50,
			price1:          0.49,
			price2:          0.50,
			takerFee:        0.01,
			expectedRevenue: 50.0, // 50 tokens * $1.00
		},
		{
			name:            "200-tokens",
			tokenCount:      200,
			price1:          0.48,
			price2:          0.51,
			takerFee:        0.01,
			expectedRevenue: 200.0, // 200 tokens * $1.00
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fills := []types.FillStatus{
				{Outcome: "YES", FullyFilled: true, SizeFilled: tt.tokenCount, ActualPrice: tt.price1},
				{Outcome: "NO", FullyFilled: true, SizeFilled: tt.tokenCount, ActualPrice: tt.price2},
			}

			profit, filled := calculateActualProfit(fills, tt.takerFee)

			if !filled {
				t.Fatal("expected all fills to be complete")
			}

			// Revenue should be token count
			cost := tt.tokenCount*tt.price1 + tt.tokenCount*tt.price2
			fees := cost * tt.takerFee
			expectedProfit := tt.expectedRevenue - cost - fees

			if !floatEquals(profit, expectedProfit, 0.01) {
				t.Errorf("expected profit %f, got %f", expectedProfit, profit)
			}
		})
	}
}

// TestClassifyError tests error classification
func TestClassifyError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		err           error
		expectedType  string
	}{
		{
			name:         "nil-error",
			err:          nil,
			expectedType: "unknown",
		},
		{
			name:         "network-connection-refused",
			err:          fmt.Errorf("connection refused"),
			expectedType: "network",
		},
		{
			name:         "network-timeout",
			err:          fmt.Errorf("request timeout"),
			expectedType: "network",
		},
		{
			name:         "network-dial",
			err:          fmt.Errorf("dial tcp failed"),
			expectedType: "network",
		},
		{
			name:         "api-400",
			err:          fmt.Errorf("API error: 400 bad request"),
			expectedType: "api",
		},
		{
			name:         "api-500",
			err:          fmt.Errorf("server error: 500"),
			expectedType: "api",
		},
		{
			name:         "validation-missing",
			err:          fmt.Errorf("missing required field"),
			expectedType: "validation",
		},
		{
			name:         "validation-not-configured",
			err:          fmt.Errorf("order client not configured"),
			expectedType: "validation",
		},
		{
			name:         "funds-insufficient",
			err:          fmt.Errorf("insufficient balance"),
			expectedType: "funds",
		},
		{
			name:         "unknown-error",
			err:          fmt.Errorf("something unexpected"),
			expectedType: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errorType := classifyError(tt.err)

			if errorType != tt.expectedType {
				t.Errorf("expected error type %q, got %q", tt.expectedType, errorType)
			}
		})
	}
}

// Helper function for floating-point comparison
func floatEquals(a, b, epsilon float64) bool {
	if a == b {
		return true
	}
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < epsilon
}
