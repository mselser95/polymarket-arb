package arbitrage

import (
	"testing"
	"time"

	"github.com/mselser95/polymarket-arb/pkg/types"
	"go.uber.org/zap"
)

// TestDetectMultiOutcome_3Outcome tests arbitrage detection for 3-outcome markets
func TestDetectMultiOutcome_3Outcome(t *testing.T) {
	takerFee := 0.01 // 1% taker fee

	tests := []struct {
		name               string
		threshold          float64
		minTradeSize       float64
		maxTradeSize       float64
		prices             []float64 // Prices for each outcome
		sizes              []float64 // Sizes for each outcome
		expectOpp          bool
		expectNetProfitBPS int
	}{
		{
			name:         "3-outcome-arbitrage-exists",
			threshold:    0.995,
			minTradeSize: 1.0,
			maxTradeSize: 1000.0,
			prices:       []float64{0.32, 0.32, 0.32}, // Sum = 0.96 < 0.995
			sizes:        []float64{100.0, 100.0, 100.0},
			expectOpp:    true,
			expectNetProfitBPS: 304, // Gross: 400bps, Fees: ~96bps, Net: ~304bps
		},
		{
			name:         "3-outcome-no-arbitrage",
			threshold:    0.995,
			minTradeSize: 1.0,
			maxTradeSize: 1000.0,
			prices:       []float64{0.35, 0.35, 0.35}, // Sum = 1.05 > 0.995
			sizes:        []float64{100.0, 100.0, 100.0},
			expectOpp:    false,
		},
		{
			name:         "3-outcome-at-threshold-boundary",
			threshold:    0.995,
			minTradeSize: 1.0,
			maxTradeSize: 1000.0,
			prices:       []float64{0.332, 0.332, 0.331}, // Sum = 0.995 (exactly at threshold)
			sizes:        []float64{100.0, 100.0, 100.0},
			expectOpp:    false, // At threshold, not below
		},
		{
			name:         "3-outcome-asymmetric-liquidity",
			threshold:    0.995,
			minTradeSize: 1.0,
			maxTradeSize: 1000.0,
			prices:       []float64{0.32, 0.32, 0.32},
			sizes:        []float64{50.0, 200.0, 150.0}, // Min is 50
			expectOpp:    true,
			expectNetProfitBPS: 304, // Still profitable, but limited to 50 size
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create 3-outcome market
			market := create3OutcomeMarket("test-market", "test-slug")

			// Create orderbook snapshots
			orderbooks := createOrderbooksFromPrices(market, tt.prices, tt.sizes)

			// Create detector
			logger, _ := zap.NewDevelopment()
			detector := &Detector{
				config: Config{
					MaxPriceSum:    tt.threshold,
					MinTradeSize: tt.minTradeSize,
					MaxTradeSize: tt.maxTradeSize,
					TakerFee:     takerFee,
				},
				logger: logger,
			}

			// Run detection
			opp, exists := detector.detectMultiOutcome(market, orderbooks)

			// Assertions
			if exists != tt.expectOpp {
				t.Errorf("expected opportunity=%v, got=%v", tt.expectOpp, exists)
			}

			if exists && opp != nil {
				// Verify profit within tolerance (±2 bps for floating point)
				if opp.NetProfitBPS < tt.expectNetProfitBPS-2 || opp.NetProfitBPS > tt.expectNetProfitBPS+2 {
					t.Errorf("expected net_profit_bps=%d, got=%d", tt.expectNetProfitBPS, opp.NetProfitBPS)
				}

				// Verify trade size is minimum of all sides
				expectedSize := tt.sizes[0]
				for _, size := range tt.sizes {
					if size < expectedSize {
						expectedSize = size
					}
				}

				if opp.MaxTradeSize != expectedSize {
					t.Errorf("expected max_trade_size=%.2f, got=%.2f", expectedSize, opp.MaxTradeSize)
				}

				// Verify price sum
				expectedSum := 0.0
				for _, price := range tt.prices {
					expectedSum += price
				}

				if opp.TotalPriceSum != expectedSum {
					t.Errorf("expected total_price_sum=%.4f, got=%.4f", expectedSum, opp.TotalPriceSum)
				}
			}
		})
	}
}

// TestDetectMultiOutcome_InvalidPrices tests edge cases with invalid prices
func TestDetectMultiOutcome_InvalidPrices(t *testing.T) {
	tests := []struct {
		name   string
		prices []float64
		sizes  []float64
	}{
		{
			name:   "one-outcome-zero-price",
			prices: []float64{0.32, 0.0, 0.32},  // Invalid: zero price
			sizes:  []float64{100.0, 100.0, 100.0},
		},
		{
			name:   "one-outcome-negative-price",
			prices: []float64{0.32, -0.1, 0.32}, // Invalid: negative price
			sizes:  []float64{100.0, 100.0, 100.0},
		},
		{
			name:   "all-zero-prices",
			prices: []float64{0.0, 0.0, 0.0},    // Invalid: all zero
			sizes:  []float64{100.0, 100.0, 100.0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			market := create3OutcomeMarket("test-market", "test-slug")
			orderbooks := createOrderbooksFromPrices(market, tt.prices, tt.sizes)

			logger, _ := zap.NewDevelopment()
			detector := &Detector{
				config: Config{
					MaxPriceSum:    0.995,
					MinTradeSize: 1.0,
					TakerFee:     0.01,
				},
				logger: logger,
			}

			_, exists := detector.detectMultiOutcome(market, orderbooks)
			if exists {
				t.Error("expected no opportunity with invalid prices")
			}
		})
	}
}

// TestDetectMultiOutcome_InvalidSizes tests edge cases with invalid sizes
func TestDetectMultiOutcome_InvalidSizes(t *testing.T) {
	tests := []struct {
		name   string
		prices []float64
		sizes  []float64
	}{
		{
			name:   "one-outcome-zero-size",
			prices: []float64{0.32, 0.32, 0.32},
			sizes:  []float64{100.0, 0.0, 100.0}, // Invalid: zero size
		},
		{
			name:   "one-outcome-negative-size",
			prices: []float64{0.32, 0.32, 0.32},
			sizes:  []float64{100.0, -10.0, 100.0}, // Invalid: negative size
		},
		{
			name:   "all-zero-sizes",
			prices: []float64{0.32, 0.32, 0.32},
			sizes:  []float64{0.0, 0.0, 0.0}, // Invalid: all zero
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			market := create3OutcomeMarket("test-market", "test-slug")
			orderbooks := createOrderbooksFromPrices(market, tt.prices, tt.sizes)

			logger, _ := zap.NewDevelopment()
			detector := &Detector{
				config: Config{
					MaxPriceSum:    0.995,
					MinTradeSize: 1.0,
					TakerFee:     0.01,
				},
				logger: logger,
			}

			_, exists := detector.detectMultiOutcome(market, orderbooks)
			if exists {
				t.Error("expected no opportunity with invalid sizes")
			}
		})
	}
}

// TestDetectMultiOutcome_MissingOrderbook tests when orderbooks are incomplete
func TestDetectMultiOutcome_MissingOrderbook(t *testing.T) {
	market := create3OutcomeMarket("test-market", "test-slug")

	logger, _ := zap.NewDevelopment()
	detector := &Detector{
		config: Config{
			MaxPriceSum:    0.995,
			MinTradeSize: 1.0,
			TakerFee:     0.01,
		},
		logger: logger,
	}

	// Test with only 2 orderbooks (missing one)
	orderbooks := []*types.OrderbookSnapshot{
		{
			MarketID:     "test-market",
			TokenID:      market.Outcomes[0].TokenID,
			Outcome:      "Candidate A",
			BestAskPrice: 0.32,
			BestAskSize:  100.0,
			LastUpdated:  time.Now(),
		},
		{
			MarketID:     "test-market",
			TokenID:      market.Outcomes[1].TokenID,
			Outcome:      "Candidate B",
			BestAskPrice: 0.32,
			BestAskSize:  100.0,
			LastUpdated:  time.Now(),
		},
		// Missing third orderbook
	}

	_, exists := detector.detectMultiOutcome(market, orderbooks)
	if exists {
		t.Error("expected no opportunity with incomplete orderbooks")
	}
}

// TestDetectMultiOutcome_SizeConstraints tests min/max size constraints
func TestDetectMultiOutcome_SizeConstraints(t *testing.T) {
	tests := []struct {
		name         string
		minTradeSize float64
		maxTradeSize float64
		sizes        []float64
		expectOpp    bool
	}{
		{
			name:         "all-below-min-size",
			minTradeSize: 100.0,
			maxTradeSize: 1000.0,
			sizes:        []float64{50.0, 50.0, 50.0}, // All below min
			expectOpp:    false,
		},
		{
			name:         "one-below-min-size",
			minTradeSize: 100.0,
			maxTradeSize: 1000.0,
			sizes:        []float64{200.0, 50.0, 200.0}, // One below min (limits trade)
			expectOpp:    false,
		},
		{
			name:         "above-max-size-caps-correctly",
			minTradeSize: 1.0,
			maxTradeSize: 50.0,
			sizes:        []float64{100.0, 100.0, 100.0}, // All above max
			expectOpp:    true, // Should cap at maxTradeSize
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			market := create3OutcomeMarket("test-market", "test-slug")
			prices := []float64{0.32, 0.32, 0.32} // Arbitrage exists
			orderbooks := createOrderbooksFromPrices(market, prices, tt.sizes)

			logger, _ := zap.NewDevelopment()
			detector := &Detector{
				config: Config{
					MaxPriceSum:    0.995,
					MinTradeSize: tt.minTradeSize,
					MaxTradeSize: tt.maxTradeSize,
					TakerFee:     0.01,
				},
				logger: logger,
			}

			opp, exists := detector.detectMultiOutcome(market, orderbooks)

			if exists != tt.expectOpp {
				t.Errorf("expected opportunity=%v, got=%v", tt.expectOpp, exists)
			}

			// If above-max-size test, verify it's capped at maxTradeSize
			if exists && opp != nil && tt.name == "above-max-size-caps-correctly" {
				if opp.MaxTradeSize != tt.maxTradeSize {
					t.Errorf("expected trade size capped at %.2f, got %.2f", tt.maxTradeSize, opp.MaxTradeSize)
				}
			}
		})
	}
}

// TestDetectMultiOutcome_FeesEliminateProfit tests when fees make arbitrage unprofitable
func TestDetectMultiOutcome_FeesEliminateProfit(t *testing.T) {
	tests := []struct {
		name      string
		prices    []float64
		takerFee  float64
		expectOpp bool
	}{
		{
			name:      "high-fees-eliminate-profit",
			prices:    []float64{0.33, 0.33, 0.33}, // Sum = 0.99, gross profit = 1%
			takerFee:  0.02,                        // 2% fee > 1% gross profit
			expectOpp: false, // Should reject due to negative net profit
		},
		{
			name:      "marginal-profit-after-fees",
			prices:    []float64{0.32, 0.32, 0.32}, // Sum = 0.96, gross profit = 4%
			takerFee:  0.03,                        // 3% fee, leaves ~1% net profit
			expectOpp: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			market := create3OutcomeMarket("test-market", "test-slug")
			sizes := []float64{100.0, 100.0, 100.0}
			orderbooks := createOrderbooksFromPrices(market, tt.prices, sizes)

			logger, _ := zap.NewDevelopment()
			detector := &Detector{
				config: Config{
					MaxPriceSum:    0.995,
					MinTradeSize: 1.0,
					MaxTradeSize: 1000.0,
					TakerFee:     tt.takerFee,
				},
				logger: logger,
			}

			_, exists := detector.detectMultiOutcome(market, orderbooks)

			if exists != tt.expectOpp {
				t.Errorf("expected opportunity=%v, got=%v", tt.expectOpp, exists)
			}
		})
	}
}

// TestDetectMultiOutcome_LargeMarkets tests with 4, 5, and 10 outcomes
func TestDetectMultiOutcome_LargeMarkets(t *testing.T) {
	tests := []struct {
		name               string
		numOutcomes        int
		pricePerOutcome    float64
		expectOpp          bool
		expectNetProfitBPS int
	}{
		{
			name:               "4-outcome-arbitrage",
			numOutcomes:        4,
			pricePerOutcome:    0.24,                // Sum = 0.96 < 0.995
			expectOpp:          true,
			expectNetProfitBPS: 320, // Approx, with 1% fees
		},
		{
			name:               "5-outcome-arbitrage",
			numOutcomes:        5,
			pricePerOutcome:    0.19,                // Sum = 0.95 < 0.995
			expectOpp:          true,
			expectNetProfitBPS: 400, // Approx, with 1% fees
		},
		{
			name:               "10-outcome-arbitrage",
			numOutcomes:        10,
			pricePerOutcome:    0.09,                 // Sum = 0.90 < 0.995
			expectOpp:          true,
			expectNetProfitBPS: 900, // Approx, with 1% fees
		},
		{
			name:            "10-outcome-no-arbitrage",
			numOutcomes:     10,
			pricePerOutcome: 0.11,  // Sum = 1.10 > 0.995
			expectOpp:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			market := createNOutcomeMarket("test-market", "test-slug", tt.numOutcomes)

			// Create equal prices and sizes for all outcomes
			prices := make([]float64, tt.numOutcomes)
			sizes := make([]float64, tt.numOutcomes)
			for i := 0; i < tt.numOutcomes; i++ {
				prices[i] = tt.pricePerOutcome
				sizes[i] = 100.0
			}

			orderbooks := createOrderbooksFromPrices(market, prices, sizes)

			logger, _ := zap.NewDevelopment()
			detector := &Detector{
				config: Config{
					MaxPriceSum:    0.995,
					MinTradeSize: 1.0,
					MaxTradeSize: 1000.0,
					TakerFee:     0.01,
				},
				logger: logger,
			}

			opp, exists := detector.detectMultiOutcome(market, orderbooks)

			if exists != tt.expectOpp {
				t.Errorf("expected opportunity=%v, got=%v", tt.expectOpp, exists)
			}

			if exists && opp != nil {
				// Verify we're within ±5% of expected profit (due to fee calculation)
				tolerance := tt.expectNetProfitBPS / 20 // 5%
				if opp.NetProfitBPS < tt.expectNetProfitBPS-tolerance || opp.NetProfitBPS > tt.expectNetProfitBPS+tolerance {
					t.Errorf("expected net_profit_bps≈%d (±%d), got=%d",
						tt.expectNetProfitBPS, tolerance, opp.NetProfitBPS)
				}
			}
		})
	}
}

// TestDetectMultiOutcome_BinaryCompatibility ensures binary markets still work
func TestDetectMultiOutcome_BinaryCompatibility(t *testing.T) {
	market := &types.MarketSubscription{
		MarketID:   "test-market",
		MarketSlug: "test-slug",
		Question:   "Binary market?",
		Outcomes: []types.OutcomeToken{
			{TokenID: "yes-token", Outcome: "YES"},
			{TokenID: "no-token", Outcome: "NO"},
		},
	}

	orderbooks := []*types.OrderbookSnapshot{
		{
			MarketID:     "test-market",
			TokenID:      "yes-token",
			Outcome:      "YES",
			BestAskPrice: 0.48,
			BestAskSize:  100.0,
			LastUpdated:  time.Now(),
		},
		{
			MarketID:     "test-market",
			TokenID:      "no-token",
			Outcome:      "NO",
			BestAskPrice: 0.51,
			BestAskSize:  100.0,
			LastUpdated:  time.Now(),
		},
	}

	logger, _ := zap.NewDevelopment()
	detector := &Detector{
		config: Config{
			MaxPriceSum:    0.995,
			MinTradeSize: 1.0,
			MaxTradeSize: 1000.0,
			TakerFee:     0.01,
		},
		logger: logger,
	}

	opp, exists := detector.detectMultiOutcome(market, orderbooks)

	if !exists {
		t.Fatal("expected binary arbitrage to be detected")
	}

	// Verify it's treated as 2-outcome arbitrage
	if len(opp.Outcomes) != 2 {
		t.Errorf("expected 2 outcomes, got %d", len(opp.Outcomes))
	}

	expectedSum := 0.99
	if opp.TotalPriceSum != expectedSum {
		t.Errorf("expected price sum %.2f, got %.2f", expectedSum, opp.TotalPriceSum)
	}
}

// Helper: Create 3-outcome market
func create3OutcomeMarket(marketID, slug string) *types.MarketSubscription {
	return &types.MarketSubscription{
		MarketID:   marketID,
		MarketSlug: slug,
		Question:   "Who will win?",
		Outcomes: []types.OutcomeToken{
			{TokenID: "token-a", Outcome: "Candidate A"},
			{TokenID: "token-b", Outcome: "Candidate B"},
			{TokenID: "token-c", Outcome: "Candidate C"},
		},
	}
}

// Helper: Create N-outcome market
func createNOutcomeMarket(marketID, slug string, n int) *types.MarketSubscription {
	outcomes := make([]types.OutcomeToken, n)
	for i := 0; i < n; i++ {
		outcomes[i] = types.OutcomeToken{
			TokenID: "token-" + string(rune('a'+i)),
			Outcome: "Candidate " + string(rune('A'+i)),
		}
	}

	return &types.MarketSubscription{
		MarketID:   marketID,
		MarketSlug: slug,
		Question:   "Who will win?",
		Outcomes:   outcomes,
	}
}

// Helper: Create orderbooks from prices and sizes
func createOrderbooksFromPrices(market *types.MarketSubscription, prices, sizes []float64) []*types.OrderbookSnapshot {
	if len(prices) != len(market.Outcomes) || len(sizes) != len(market.Outcomes) {
		panic("prices/sizes must match number of outcomes")
	}

	orderbooks := make([]*types.OrderbookSnapshot, len(market.Outcomes))
	for i, outcome := range market.Outcomes {
		orderbooks[i] = &types.OrderbookSnapshot{
			MarketID:     market.MarketID,
			TokenID:      outcome.TokenID,
			Outcome:      outcome.Outcome,
			BestAskPrice: prices[i],
			BestAskSize:  sizes[i],
			LastUpdated:  time.Now(),
		}
	}

	return orderbooks
}
