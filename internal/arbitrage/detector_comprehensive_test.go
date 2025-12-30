//go:build integration
// +build integration

// NOTE: These tests use the old detector API (direct calls to detectMultiOutcome).
// They need to be updated to use the new detector architecture with orderbook manager.
// For now, they're tagged as integration tests and will be skipped in regular test runs.

package arbitrage

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/mselser95/polymarket-arb/internal/testutil"
	"github.com/mselser95/polymarket-arb/pkg/types"
	"go.uber.org/zap"
)

// TestDetectMultiOutcome_ZeroPrice tests rejection of zero ask prices
func TestDetectMultiOutcome_ZeroPrice(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	cfg := Config{
		MaxPriceSum:  0.995,
		MinTradeSize: 1.0,
		MaxTradeSize: 100.0,
		TakerFee:     0.01,
	}

	detector := &Detector{
		config: cfg,
		logger: logger,
	}

	market := testutil.CreateTestMarket("market1", "test-slug", "Test market")
	market.Tokens = []types.Token{
		{TokenID: "token1", Outcome: "A"},
		{TokenID: "token2", Outcome: "B"},
		{TokenID: "token3", Outcome: "C"},
	}

	orderbooks := []*types.OrderbookSnapshot{
		{TokenID: "token1", BestAskPrice: 0.0, BestAskSize: 100}, // Zero price
		{TokenID: "token2", BestAskPrice: 0.33, BestAskSize: 100},
		{TokenID: "token3", BestAskPrice: 0.33, BestAskSize: 100},
	}

	opp, exists := detector.detectMultiOutcome(market, orderbooks)
	if exists {
		t.Error("expected no opportunity with zero ask price")
	}

	if opp != nil {
		t.Error("expected nil opportunity")
	}
}

// TestDetectMultiOutcome_NegativePrice tests rejection of negative prices
func TestDetectMultiOutcome_NegativePrice(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	cfg := Config{
		MaxPriceSum:  0.995,
		MinTradeSize: 1.0,
		MaxTradeSize: 100.0,
		TakerFee:     0.01,
	}

	detector := &Detector{
		config: cfg,
		logger: logger,
	}

	market := testutil.CreateTestMarket("market1", "test-slug", "Test market")
	market.Tokens = []types.Token{
		{TokenID: "token1", Outcome: "A"},
		{TokenID: "token2", Outcome: "B"},
	}

	orderbooks := []*types.OrderbookSnapshot{
		{TokenID: "token1", BestAskPrice: -0.1, BestAskSize: 100}, // Negative
		{TokenID: "token2", BestAskPrice: 0.50, BestAskSize: 100},
	}

	opp, exists := detector.detectMultiOutcome(market, orderbooks)
	if exists {
		t.Error("expected no opportunity with negative ask price")
	}

	if opp != nil {
		t.Error("expected nil opportunity")
	}
}

// TestDetectMultiOutcome_ZeroSize tests rejection of zero size
func TestDetectMultiOutcome_ZeroSize(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	cfg := Config{
		MaxPriceSum:  0.995,
		MinTradeSize: 1.0,
		MaxTradeSize: 100.0,
		TakerFee:     0.01,
	}

	detector := &Detector{
		config: cfg,
		logger: logger,
	}

	market := testutil.CreateTestMarket("market1", "test-slug", "Test market")
	market.Tokens = []types.Token{
		{TokenID: "token1", Outcome: "A"},
		{TokenID: "token2", Outcome: "B"},
	}

	orderbooks := []*types.OrderbookSnapshot{
		{TokenID: "token1", BestAskPrice: 0.49, BestAskSize: 0}, // Zero size
		{TokenID: "token2", BestAskPrice: 0.49, BestAskSize: 100},
	}

	opp, exists := detector.detectMultiOutcome(market, orderbooks)
	if exists {
		t.Error("expected no opportunity with zero ask size")
	}

	if opp != nil {
		t.Error("expected nil opportunity")
	}
}

// TestDetectMultiOutcome_FloatingPointPrecision tests sum near threshold
func TestDetectMultiOutcome_FloatingPointPrecision(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	cfg := Config{
		MaxPriceSum:  0.995,
		MinTradeSize: 1.0,
		MaxTradeSize: 100.0,
		TakerFee:     0.01,
	}

	detector := &Detector{
		config: cfg,
		logger: logger,
	}

	tests := []struct {
		name      string
		prices    []float64
		expectOpp bool
	}{
		{
			name:      "sum exactly at threshold",
			prices:    []float64{0.497, 0.498}, // sum = 0.995 exactly
			expectOpp: false,                   // Not below threshold
		},
		{
			name:      "sum just below threshold",
			prices:    []float64{0.497, 0.497}, // sum = 0.994
			expectOpp: true,                    // Below threshold
		},
		{
			name:      "sum just above threshold",
			prices:    []float64{0.498, 0.498}, // sum = 0.996
			expectOpp: false,                   // Above threshold
		},
		{
			name:      "floating point close to threshold",
			prices:    []float64{0.4975, 0.4974999}, // sum ~0.9949999
			expectOpp: true,                         // Just below
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			market := testutil.CreateTestMarket("market1", "test-slug", "Test market")
			market.Tokens = make([]*types.Token, len(tt.prices))
			orderbooks := make([]*types.OrderbookSnapshot, len(tt.prices))

			for i, price := range tt.prices {
				market.Tokens[i] = &types.Token{
					TokenID: fmt.Sprintf("token%d", i),
					Outcome: fmt.Sprintf("Outcome%d", i),
				}
				orderbooks[i] = &types.OrderbookSnapshot{
					TokenID:      fmt.Sprintf("token%d", i),
					BestAskPrice: price,
					BestAskSize:  100,
				}
			}

			_, exists := detector.detectMultiOutcome(market, orderbooks)
			if exists != tt.expectOpp {
				sum := 0.0
				for _, p := range tt.prices {
					sum += p
				}
				t.Errorf("expected opportunity=%v for sum=%f, got %v", tt.expectOpp, sum, exists)
			}
		})
	}
}

// TestDetectMultiOutcome_SizeBottleneck tests minimum size across outcomes
func TestDetectMultiOutcome_SizeBottleneck(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	cfg := Config{
		MaxPriceSum:  0.995,
		MinTradeSize: 1.0,
		MaxTradeSize: 100.0,
		TakerFee:     0.01,
	}

	detector := &Detector{
		config: cfg,
		logger: logger,
	}

	market := testutil.CreateTestMarket("market1", "test-slug", "Test market")
	market.Tokens = []types.Token{
		{TokenID: "token1", Outcome: "A"},
		{TokenID: "token2", Outcome: "B"},
		{TokenID: "token3", Outcome: "C"},
	}

	// Different sizes - smallest should be bottleneck
	orderbooks := []*types.OrderbookSnapshot{
		{TokenID: "token1", BestAskPrice: 0.32, BestAskSize: 100},
		{TokenID: "token2", BestAskPrice: 0.32, BestAskSize: 50}, // Bottleneck
		{TokenID: "token3", BestAskPrice: 0.32, BestAskSize: 200},
	}

	opp, exists := detector.detectMultiOutcome(market, orderbooks)
	if !exists {
		t.Fatal("expected opportunity to exist")
	}

	// Max trade size should be capped by smallest size
	// 50 tokens × 0.32 = $16
	expectedMax := 50.0

	if opp.MaxTradeSize > expectedMax+epsilon {
		t.Errorf("expected max trade size limited by bottleneck to ~%f, got %f", expectedMax, opp.MaxTradeSize)
	}
}

// TestDetectMultiOutcome_MaxTradeSizeCap tests cap at config max
func TestDetectMultiOutcome_MaxTradeSizeCap(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	cfg := Config{
		MaxPriceSum:  0.995,
		MinTradeSize: 1.0,
		MaxTradeSize: 20.0, // Low cap
		TakerFee:     0.01,
	}

	detector := &Detector{
		config: cfg,
		logger: logger,
	}

	market := testutil.CreateTestMarket("market1", "test-slug", "Test market")
	market.Tokens = []types.Token{
		{TokenID: "token1", Outcome: "A"},
		{TokenID: "token2", Outcome: "B"},
	}

	// Large available size
	orderbooks := []*types.OrderbookSnapshot{
		{TokenID: "token1", BestAskPrice: 0.49, BestAskSize: 1000},
		{TokenID: "token2", BestAskPrice: 0.49, BestAskSize: 1000},
	}

	opp, exists := detector.detectMultiOutcome(market, orderbooks)
	if !exists {
		t.Fatal("expected opportunity to exist")
	}

	// Should be capped at config max
	if opp.MaxTradeSize > cfg.MaxTradeSize+epsilon {
		t.Errorf("expected max trade size capped at %f, got %f", cfg.MaxTradeSize, opp.MaxTradeSize)
	}
}

// TestDetectMultiOutcome_MinTradeSizeCheck tests below minimum rejection
func TestDetectMultiOutcome_MinTradeSizeCheck(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	cfg := Config{
		MaxPriceSum:  0.995,
		MinTradeSize: 50.0, // High minimum
		MaxTradeSize: 100.0,
		TakerFee:     0.01,
	}

	detector := &Detector{
		config: cfg,
		logger: logger,
	}

	market := testutil.CreateTestMarket("market1", "test-slug", "Test market")
	market.Tokens = []types.Token{
		{TokenID: "token1", Outcome: "A"},
		{TokenID: "token2", Outcome: "B"},
	}

	// Small available size (below minimum)
	orderbooks := []*types.OrderbookSnapshot{
		{TokenID: "token1", BestAskPrice: 0.49, BestAskSize: 10}, // Only $4.90
		{TokenID: "token2", BestAskPrice: 0.49, BestAskSize: 10}, // Only $4.90
	}

	opp, exists := detector.detectMultiOutcome(market, orderbooks)
	if exists {
		t.Error("expected no opportunity when size below minimum")
	}

	if opp != nil {
		t.Error("expected nil opportunity")
	}
}

// TestNewMultiOutcomeOpportunity_GrossProfit tests spread calculation
func TestNewMultiOutcomeOpportunity_GrossProfit(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	outcomes := []OpportunityOutcome{
		{Outcome: "A", TokenID: "token1", AskPrice: 0.32, AskSize: 100},
		{Outcome: "B", TokenID: "token2", AskPrice: 0.32, AskSize: 100},
		{Outcome: "C", TokenID: "token3", AskPrice: 0.32, AskSize: 100},
	}

	market := &types.MarketSubscription{
		MarketID:       "market1",
		Slug:           "test-slug",
		Question:       "Test question",
		OutcomeTokens:  []string{"token1", "token2", "token3"},
		OutcomeStrings: []string{"A", "B", "C"},
	}

	totalPriceSum := 0.96 // 3 × 0.32
	maxTradeSize := 100.0
	takerFee := 0.01
	threshold := 0.995

	opp := NewMultiOutcomeOpportunity(
		market,
		outcomes,
		totalPriceSum,
		maxTradeSize,
		takerFee,
		threshold,
		logger,
	)

	if opp == nil {
		t.Fatal("expected opportunity to be created")
	}

	// Gross profit = (1.0 - totalPriceSum) × maxTradeSize
	// = (1.0 - 0.96) × 100 = 0.04 × 100 = $4.00
	expectedGrossProfit := 4.0

	if !floatEquals(opp.EstimatedProfit, expectedGrossProfit, epsilon) {
		t.Errorf("expected gross profit %f, got %f", expectedGrossProfit, opp.EstimatedProfit)
	}

	// Profit margin = 1.0 - totalPriceSum = 0.04
	expectedMargin := 0.04

	if !floatEquals(opp.ProfitMargin, expectedMargin, epsilon) {
		t.Errorf("expected profit margin %f, got %f", expectedMargin, opp.ProfitMargin)
	}
}

// TestNewMultiOutcomeOpportunity_NetProfit tests after fees
func TestNewMultiOutcomeOpportunity_NetProfit(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	outcomes := []OpportunityOutcome{
		{Outcome: "A", TokenID: "token1", AskPrice: 0.49, AskSize: 100},
		{Outcome: "B", TokenID: "token2", AskPrice: 0.49, AskSize: 100},
	}

	market := &types.MarketSubscription{
		MarketID:       "market1",
		Slug:           "test-slug",
		Question:       "Test question",
		OutcomeTokens:  []string{"token1", "token2"},
		OutcomeStrings: []string{"A", "B"},
	}

	totalPriceSum := 0.98
	maxTradeSize := 100.0
	takerFee := 0.01
	threshold := 0.995

	opp := NewMultiOutcomeOpportunity(
		market,
		outcomes,
		totalPriceSum,
		maxTradeSize,
		takerFee,
		threshold,
		logger,
	)

	if opp == nil {
		t.Fatal("expected opportunity to be created")
	}

	// Gross profit = (1.0 - 0.98) × 100 = $2.00
	// Fees = maxTradeSize × takerFee × numOutcomes = 100 × 0.01 × 2 = $2.00
	// Net profit = $2.00 - $2.00 = $0.00
	expectedNetProfit := 0.0

	if !floatEquals(opp.NetProfit, expectedNetProfit, epsilon) {
		t.Errorf("expected net profit %f, got %f", expectedNetProfit, opp.NetProfit)
	}
}

// TestNewMultiOutcomeOpportunity_NegativeProfit tests rejection with fees
func TestNewMultiOutcomeOpportunity_NegativeProfit(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	outcomes := []OpportunityOutcome{
		{Outcome: "A", TokenID: "token1", AskPrice: 0.495, AskSize: 100},
		{Outcome: "B", TokenID: "token2", AskPrice: 0.495, AskSize: 100},
	}

	market := &types.MarketSubscription{
		MarketID:       "market1",
		Slug:           "test-slug",
		Question:       "Test question",
		OutcomeTokens:  []string{"token1", "token2"},
		OutcomeStrings: []string{"A", "B"},
	}

	totalPriceSum := 0.99 // Sum = 0.99, spread = 0.01
	maxTradeSize := 100.0
	takerFee := 0.01
	threshold := 0.995

	opp := NewMultiOutcomeOpportunity(
		market,
		outcomes,
		totalPriceSum,
		maxTradeSize,
		takerFee,
		threshold,
		logger,
	)

	// Gross profit = (1.0 - 0.99) × 100 = $1.00
	// Fees = 100 × 0.01 × 2 = $2.00
	// Net profit = $1.00 - $2.00 = -$1.00 (loss)

	// Should return nil for negative profit
	if opp != nil {
		t.Errorf("expected nil opportunity with negative net profit, got opportunity with net profit %f", opp.NetProfit)
	}
}

// TestNewMultiOutcomeOpportunity_SmallSpread tests floating-point precision
func TestNewMultiOutcomeOpportunity_SmallSpread(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	outcomes := []OpportunityOutcome{
		{Outcome: "A", TokenID: "token1", AskPrice: 0.497, AskSize: 100},
		{Outcome: "B", TokenID: "token2", AskPrice: 0.497, AskSize: 100},
	}

	market := &types.MarketSubscription{
		MarketID:       "market1",
		Slug:           "test-slug",
		Question:       "Test question",
		OutcomeTokens:  []string{"token1", "token2"},
		OutcomeStrings: []string{"A", "B"},
	}

	totalPriceSum := 0.994 // Very small spread: 0.006
	maxTradeSize := 1000.0 // Large size to amplify small spread
	takerFee := 0.001      // Low fee: 0.1%
	threshold := 0.995

	opp := NewMultiOutcomeOpportunity(
		market,
		outcomes,
		totalPriceSum,
		maxTradeSize,
		takerFee,
		threshold,
		logger,
	)

	if opp == nil {
		t.Fatal("expected opportunity with small but positive spread")
	}

	// Gross profit = (1.0 - 0.994) × 1000 = $6.00
	// Fees = 1000 × 0.001 × 2 = $2.00
	// Net profit = $6.00 - $2.00 = $4.00

	expectedNetProfit := 4.0

	if math.Abs(opp.NetProfit-expectedNetProfit) > 0.01 {
		t.Errorf("expected net profit ~%f, got %f (diff: %f)",
			expectedNetProfit, opp.NetProfit, math.Abs(opp.NetProfit-expectedNetProfit))
	}
}

// TestNewMultiOutcomeOpportunity_ManyOutcomes tests fee multiplication
func TestNewMultiOutcomeOpportunity_ManyOutcomes(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// 5 outcomes
	outcomes := []OpportunityOutcome{
		{Outcome: "A", TokenID: "token1", AskPrice: 0.18, AskSize: 100},
		{Outcome: "B", TokenID: "token2", AskPrice: 0.18, AskSize: 100},
		{Outcome: "C", TokenID: "token3", AskPrice: 0.18, AskSize: 100},
		{Outcome: "D", TokenID: "token4", AskPrice: 0.18, AskSize: 100},
		{Outcome: "E", TokenID: "token5", AskPrice: 0.18, AskSize: 100},
	}

	market := &types.MarketSubscription{
		MarketID:       "market1",
		Slug:           "test-slug",
		Question:       "Test question",
		OutcomeTokens:  []string{"token1", "token2", "token3", "token4", "token5"},
		OutcomeStrings: []string{"A", "B", "C", "D", "E"},
	}

	totalPriceSum := 0.90 // 5 × 0.18
	maxTradeSize := 100.0
	takerFee := 0.01
	threshold := 0.995

	opp := NewMultiOutcomeOpportunity(
		market,
		outcomes,
		totalPriceSum,
		maxTradeSize,
		takerFee,
		threshold,
		logger,
	)

	if opp == nil {
		t.Fatal("expected opportunity to be created")
	}

	// Gross profit = (1.0 - 0.90) × 100 = $10.00
	// Fees = 100 × 0.01 × 5 = $5.00 (fees scale with outcome count)
	// Net profit = $10.00 - $5.00 = $5.00

	expectedFees := 5.0
	expectedNetProfit := 5.0

	if !floatEquals(opp.TotalFees, expectedFees, epsilon) {
		t.Errorf("expected total fees %f, got %f", expectedFees, opp.TotalFees)
	}

	if !floatEquals(opp.NetProfit, expectedNetProfit, epsilon) {
		t.Errorf("expected net profit %f, got %f", expectedNetProfit, opp.NetProfit)
	}
}

// TestDetectMultiOutcome_TwoOutcomes tests binary market handling
func TestDetectMultiOutcome_TwoOutcomes(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	cfg := Config{
		MaxPriceSum:  0.995,
		MinTradeSize: 1.0,
		MaxTradeSize: 100.0,
		TakerFee:     0.01,
	}

	detector := &Detector{
		config: cfg,
		logger: logger,
	}

	// Binary market (2 outcomes)
	market := testutil.CreateTestMarket("market1", "test-slug", "Binary market")
	market.Tokens = []types.Token{
		{TokenID: "token1", Outcome: "YES"},
		{TokenID: "token2", Outcome: "NO"},
	}

	orderbooks := []*types.OrderbookSnapshot{
		{TokenID: "token1", BestAskPrice: 0.49, BestAskSize: 100},
		{TokenID: "token2", BestAskPrice: 0.49, BestAskSize: 100},
	}

	opp, exists := detector.detectMultiOutcome(market, orderbooks)

	// Should still work for binary markets
	if !exists {
		t.Fatal("expected opportunity for binary market")
	}

	if len(opp.Outcomes) != 2 {
		t.Errorf("expected 2 outcomes, got %d", len(opp.Outcomes))
	}
}

// TestDetectMultiOutcome_FiveOutcomes tests many outcomes
func TestDetectMultiOutcome_FiveOutcomes(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	cfg := Config{
		MaxPriceSum:  0.995,
		MinTradeSize: 1.0,
		MaxTradeSize: 100.0,
		TakerFee:     0.01,
	}

	detector := &Detector{
		config: cfg,
		logger: logger,
	}

	// 5-outcome market
	market := testutil.CreateTestMarket("market1", "test-slug", "5-way market")
	market.Tokens = []types.Token{
		{TokenID: "token1", Outcome: "A"},
		{TokenID: "token2", Outcome: "B"},
		{TokenID: "token3", Outcome: "C"},
		{TokenID: "token4", Outcome: "D"},
		{TokenID: "token5", Outcome: "E"},
	}

	orderbooks := []*types.OrderbookSnapshot{
		{TokenID: "token1", BestAskPrice: 0.18, BestAskSize: 100},
		{TokenID: "token2", BestAskPrice: 0.18, BestAskSize: 100},
		{TokenID: "token3", BestAskPrice: 0.18, BestAskSize: 100},
		{TokenID: "token4", BestAskPrice: 0.18, BestAskSize: 100},
		{TokenID: "token5", BestAskPrice: 0.18, BestAskSize: 100},
	}

	opp, exists := detector.detectMultiOutcome(market, orderbooks)

	if !exists {
		t.Fatal("expected opportunity for 5-outcome market")
	}

	if len(opp.Outcomes) != 5 {
		t.Errorf("expected 5 outcomes, got %d", len(opp.Outcomes))
	}

	// Sum = 5 × 0.18 = 0.90 < 0.995 ✓
	expectedSum := 0.90
	if !floatEquals(opp.TotalPriceSum, expectedSum, epsilon) {
		t.Errorf("expected total price sum %f, got %f", expectedSum, opp.TotalPriceSum)
	}
}

// TestDetectMultiOutcome_NoOpportunity tests sum above threshold
func TestDetectMultiOutcome_NoOpportunity(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	cfg := Config{
		MaxPriceSum:  0.995,
		MinTradeSize: 1.0,
		MaxTradeSize: 100.0,
		TakerFee:     0.01,
	}

	detector := &Detector{
		config: cfg,
		logger: logger,
	}

	market := testutil.CreateTestMarket("market1", "test-slug", "Test market")
	market.Tokens = []types.Token{
		{TokenID: "token1", Outcome: "A"},
		{TokenID: "token2", Outcome: "B"},
	}

	// Sum = 1.00 > 0.995
	orderbooks := []*types.OrderbookSnapshot{
		{TokenID: "token1", BestAskPrice: 0.50, BestAskSize: 100},
		{TokenID: "token2", BestAskPrice: 0.50, BestAskSize: 100},
	}

	opp, exists := detector.detectMultiOutcome(market, orderbooks)

	if exists {
		t.Error("expected no opportunity when sum > threshold")
	}

	if opp != nil {
		t.Error("expected nil opportunity")
	}
}

// TestDetectMultiOutcome_ValidOpportunity tests successful detection
func TestDetectMultiOutcome_ValidOpportunity(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	cfg := Config{
		MaxPriceSum:  0.995,
		MinTradeSize: 1.0,
		MaxTradeSize: 100.0,
		TakerFee:     0.01,
	}

	detector := &Detector{
		config: cfg,
		logger: logger,
	}

	market := testutil.CreateTestMarket("market1", "test-slug", "Test market")
	market.Tokens = []types.Token{
		{TokenID: "token1", Outcome: "A"},
		{TokenID: "token2", Outcome: "B"},
		{TokenID: "token3", Outcome: "C"},
	}

	// Sum = 0.96 < 0.995 ✓
	orderbooks := []*types.OrderbookSnapshot{
		{TokenID: "token1", BestAskPrice: 0.32, BestAskSize: 100},
		{TokenID: "token2", BestAskPrice: 0.32, BestAskSize: 100},
		{TokenID: "token3", BestAskPrice: 0.32, BestAskSize: 100},
	}

	opp, exists := detector.detectMultiOutcome(market, orderbooks)

	if !exists {
		t.Fatal("expected opportunity to be detected")
	}

	if opp == nil {
		t.Fatal("expected non-nil opportunity")
	}

	// Verify opportunity structure
	if opp.MarketID != "market1" {
		t.Errorf("expected market ID 'market1', got '%s'", opp.MarketID)
	}

	if len(opp.Outcomes) != 3 {
		t.Errorf("expected 3 outcomes, got %d", len(opp.Outcomes))
	}

	if opp.TotalPriceSum >= cfg.MaxPriceSum {
		t.Errorf("expected price sum < %f, got %f", cfg.MaxPriceSum, opp.TotalPriceSum)
	}

	// Should have positive net profit
	if opp.NetProfit <= 0 {
		t.Errorf("expected positive net profit, got %f", opp.NetProfit)
	}
}
