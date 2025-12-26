package arbitrage

import (
	"testing"
	"time"

	"github.com/mselser95/polymarket-arb/pkg/types"
	"go.uber.org/zap"
)

func TestDetect(t *testing.T) {
	takerFee := 0.01 // 1% taker fee

	tests := []struct {
		name               string
		threshold          float64
		minTradeSize       float64
		maxTradeSize       float64
		yesAsk             float64
		yesAskSize         float64
		noAsk              float64
		noAskSize          float64
		expectOpp          bool
		expectNetProfitBPS int
	}{
		{
			name:         "no-arbitrage-efficient-market",
			threshold:    0.995,
			minTradeSize: 10.0,
			maxTradeSize: 1000.0,
			yesAsk:       0.50,
			yesAskSize:   100.0,
			noAsk:        0.50,
			noAskSize:    100.0,
			expectOpp:    false,
		},
		{
			name:               "arbitrage-exists-after-fees",
			threshold:          0.995,
			minTradeSize:       10.0,
			maxTradeSize:       1000.0,
			yesAsk:             0.48,
			yesAskSize:         100.0,
			noAsk:              0.48,
			noAskSize:          100.0,
			expectOpp:          true,
			expectNetProfitBPS: 304, // Gross: 400bps, Fees: ~96bps, Net: ~304bps
		},
		{
			name:         "at-threshold-boundary-negative-profit",
			threshold:    0.995,
			minTradeSize: 10.0,
			maxTradeSize: 1000.0,
			yesAsk:       0.497,
			yesAskSize:   100.0,
			noAsk:        0.497,
			noAskSize:    100.0,
			expectOpp:    false, // 0.994 < 0.995, but net profit is negative after fees (rejected)
		},
		{
			name:         "below-min-trade-size",
			threshold:    0.995,
			minTradeSize: 100.0,
			maxTradeSize: 1000.0,
			yesAsk:       0.40,
			yesAskSize:   50.0, // Below min
			noAsk:        0.40,
			noAskSize:    50.0,
			expectOpp:    false,
		},
		{
			name:               "large-arbitrage",
			threshold:          0.995,
			minTradeSize:       10.0,
			maxTradeSize:       1000.0,
			yesAsk:             0.30,
			yesAskSize:         1000.0,
			noAsk:              0.30,
			noAskSize:          1000.0,
			expectOpp:          true,
			expectNetProfitBPS: 3940, // Gross: 4000bps, Fees: 60bps, Net: 3940bps
		},
		{
			name:               "asymmetric-sizes",
			threshold:          0.995,
			minTradeSize:       10.0,
			maxTradeSize:       1000.0,
			yesAsk:             0.45,
			yesAskSize:         50.0, // Smaller
			noAsk:              0.45,
			noAskSize:          200.0, // Larger
			expectOpp:          true,
			expectNetProfitBPS: 909, // After fees
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test market subscription with Outcomes
			market := &types.MarketSubscription{
				MarketID:   "test-market",
				MarketSlug: "test-slug",
				Question:   "Test question?",
				Outcomes: []types.OutcomeToken{
					{TokenID: "yes-token", Outcome: "YES"},
					{TokenID: "no-token", Outcome: "NO"},
				},
			}

			// Create test orderbook snapshots
			// Note: We use ASK prices since arbitrage involves BUYING at ask prices
			yesBook := &types.OrderbookSnapshot{
				MarketID:     "test-market",
				TokenID:      "yes-token",
				Outcome:      "YES",
				BestAskPrice: tt.yesAsk,
				BestAskSize:  tt.yesAskSize,
				LastUpdated:  time.Now(),
			}

			noBook := &types.OrderbookSnapshot{
				MarketID:     "test-market",
				TokenID:      "no-token",
				Outcome:      "NO",
				BestAskPrice: tt.noAsk,
				BestAskSize:  tt.noAskSize,
				LastUpdated:  time.Now(),
			}

			// Create detector (without starting it)
			logger, _ := zap.NewDevelopment()
			detector := &Detector{
				config: Config{
					Threshold:    tt.threshold,
					MinTradeSize: tt.minTradeSize,
					MaxTradeSize: tt.maxTradeSize,
					TakerFee:     takerFee,
				},
				logger: logger,
			}

			// Run detection
			opp, exists := detector.detect(market, yesBook, noBook)

			// Check results
			if exists != tt.expectOpp {
				t.Errorf("expected opportunity=%v, got=%v", tt.expectOpp, exists)
			}

			if tt.expectOpp && opp != nil {
				// Allow 1bps tolerance for floating point precision
				if opp.NetProfitBPS < tt.expectNetProfitBPS-1 || opp.NetProfitBPS > tt.expectNetProfitBPS+1 {
					t.Errorf("expected net_profit_bps=%d, got=%d", tt.expectNetProfitBPS, opp.NetProfitBPS)
				}

				// Verify trade size is minimum of both sides
				expectedSize := tt.yesAskSize
				if tt.noAskSize < expectedSize {
					expectedSize = tt.noAskSize
				}

				if opp.MaxTradeSize != expectedSize {
					t.Errorf("expected max_trade_size=%.2f, got=%.2f", expectedSize, opp.MaxTradeSize)
				}
			}
		})
	}
}

func TestDetectInvalidOrderbooks(t *testing.T) {
	market := &types.MarketSubscription{
		MarketID:   "test-market",
		MarketSlug: "test-slug",
		Question:   "Test?",
	}

	logger, _ := zap.NewDevelopment()
	detector := &Detector{
		config: Config{
			Threshold:    0.995,
			MinTradeSize: 10.0,
			TakerFee:     0.01,
		},
		logger: logger,
	}

	// Test zero prices
	yesBook := &types.OrderbookSnapshot{
		MarketID:     "test-market",
		BestAskPrice: 0.0, // Invalid
		BestAskSize:  100.0,
	}

	noBook := &types.OrderbookSnapshot{
		MarketID:     "test-market",
		BestAskPrice: 0.50,
		BestAskSize:  100.0,
	}

	_, exists := detector.detect(market, yesBook, noBook)
	if exists {
		t.Error("expected no opportunity with zero price")
	}

	// Test zero sizes
	yesBook.BestAskPrice = 0.48
	yesBook.BestAskSize = 0.0 // Invalid

	_, exists = detector.detect(market, yesBook, noBook)
	if exists {
		t.Error("expected no opportunity with zero size")
	}
}
