package testutil

import (
	"time"

	"github.com/mselser95/polymarket-arb/internal/arbitrage"
	"github.com/mselser95/polymarket-arb/pkg/types"
)

// CreateTestMarket creates a test market with YES and NO tokens.
func CreateTestMarket(id string, slug string, question string) *types.Market {
	return &types.Market{
		ID:          id,
		Slug:        slug,
		Question:    question,
		Closed:      false,
		Active:      true,
		Outcomes:    `["Yes", "No"]`,              // API format
		ClobTokens:  `["` + id + `-yes", "` + id + `-no"]`, // API format
		Tokens: []types.Token{
			{TokenID: id + "-yes", Outcome: "Yes", Price: 0.52},
			{TokenID: id + "-no", Outcome: "No", Price: 0.48},
		},
		CreatedAt:   time.Now(),
		Description: "Test market: " + question,
	}
}

// CreateTestOrderbookMessage creates a test orderbook message.
func CreateTestOrderbookMessage(eventType string, assetID string, marketID string) *types.OrderbookMessage {
	return &types.OrderbookMessage{
		EventType: eventType,
		Market:    marketID,
		AssetID:   assetID,
		Timestamp: time.Now().Unix(),
		Bids: []types.PriceLevel{
			{Price: "0.52", Size: "100.0"},
			{Price: "0.51", Size: "50.0"},
		},
		Asks: []types.PriceLevel{
			{Price: "0.53", Size: "100.0"},
			{Price: "0.54", Size: "50.0"},
		},
	}
}

// CreateTestBookMessage creates a "book" type orderbook message.
func CreateTestBookMessage(assetID string, marketID string) *types.OrderbookMessage {
	return CreateTestOrderbookMessage("book", assetID, marketID)
}

// CreateTestPriceChangeMessage creates a "price_change" type orderbook message.
func CreateTestPriceChangeMessage(assetID string, marketID string) *types.OrderbookMessage {
	return CreateTestOrderbookMessage("price_change", assetID, marketID)
}

// CreateTestOpportunity creates a test arbitrage opportunity (binary market).
func CreateTestOpportunity(marketID string, marketSlug string) *arbitrage.Opportunity {
	outcomes := []arbitrage.OpportunityOutcome{
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

	return &arbitrage.Opportunity{
		ID:              "test-opp-" + marketID,
		MarketID:        marketID,
		MarketSlug:      marketSlug,
		MarketQuestion:  "Test market: " + marketSlug,
		Outcomes:        outcomes,
		DetectedAt:      time.Now(),
		TotalPriceSum:   0.99,
		ProfitMargin:    0.01,
		ProfitBPS:       100,
		MaxTradeSize:    100.0,
		EstimatedProfit: 1.0,
		TotalFees:       0.2,
		NetProfit:       0.8,
		NetProfitBPS:    80,
		ConfigThreshold: 0.995,
	}
}

// CreateArbitrageOrderbooks creates YES and NO orderbook snapshots that form an arbitrage.
// Note: Arbitrage detection uses ASK prices (the price you pay to BUY).
// We set the ASK prices to create an arbitrage opportunity.
func CreateArbitrageOrderbooks(marketID string, yesTokenID string, noTokenID string) (*types.OrderbookSnapshot, *types.OrderbookSnapshot) {
	now := time.Now()

	yesBook := &types.OrderbookSnapshot{
		MarketID:     marketID,
		TokenID:      yesTokenID,
		BestAskPrice: 0.48, // Price to buy YES
		BestAskSize:  100.0,
		LastUpdated:  now,
	}

	noBook := &types.OrderbookSnapshot{
		MarketID:     marketID,
		TokenID:      noTokenID,
		BestAskPrice: 0.51, // Price to buy NO
		BestAskSize:  100.0,
		LastUpdated:  now,
	}

	return yesBook, noBook
}

// CreateMarketsResponse creates a test markets response from Gamma API.
func CreateMarketsResponse(markets ...*types.Market) *types.MarketsResponse {
	data := make([]types.Market, len(markets))
	for i, m := range markets {
		data[i] = *m
	}

	return &types.MarketsResponse{
		Data:   data,
		Count:  len(markets),
		Limit:  50,
		Offset: 0,
	}
}
