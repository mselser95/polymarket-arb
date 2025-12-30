package arbitrage

import "time"

// CreateTestOpportunity creates a test arbitrage opportunity (binary market).
// This is a test helper moved from testutil to avoid import cycles.
func CreateTestOpportunity(marketID string, marketSlug string) *Opportunity {
	outcomes := []OpportunityOutcome{
		{
			TokenID:  "test-yes-token-" + marketID,
			Outcome:  "YES",
			AskPrice: 0.48,
			AskSize:  100.0,
			TickSize: 0.01,
			MinSize:  5.0,
		},
		{
			TokenID:  "test-no-token-" + marketID,
			Outcome:  "NO",
			AskPrice: 0.51,
			AskSize:  100.0,
			TickSize: 0.01,
			MinSize:  5.0,
		},
	}

	return &Opportunity{
		ID:                "test-opp-" + marketID,
		MarketID:          marketID,
		MarketSlug:        marketSlug,
		MarketQuestion:    "Test market: " + marketSlug,
		Outcomes:          outcomes,
		DetectedAt:        time.Now(),
		TotalPriceSum:     0.99,
		ProfitMargin:      0.01,
		ProfitBPS:         100,
		MaxTradeSize:      100.0,
		EstimatedProfit:   1.0,
		TotalFees:         0.2,
		NetProfit:         0.8,
		NetProfitBPS:      80,
		ConfigMaxPriceSum: 0.995,
	}
}
