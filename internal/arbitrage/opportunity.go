package arbitrage

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Opportunity represents an arbitrage opportunity.
type Opportunity struct {
	ID              string
	MarketID        string
	MarketSlug      string
	MarketQuestion  string
	YesTokenID      string  // Token ID for YES outcome
	NoTokenID       string  // Token ID for NO outcome
	DetectedAt      time.Time
	YesAskPrice     float64 // Price to BUY YES (was YesBidPrice)
	YesAskSize      float64 // Size available to BUY YES (was YesBidSize)
	NoAskPrice      float64 // Price to BUY NO (was NoBidPrice)
	NoAskSize       float64 // Size available to BUY NO (was NoBidSize)
	PriceSum        float64
	ProfitMargin    float64
	ProfitBPS       int
	MaxTradeSize    float64
	EstimatedProfit float64
	TotalFees       float64
	NetProfit       float64
	NetProfitBPS    int
	ConfigThreshold float64
}

// NewOpportunity creates a new arbitrage opportunity with fee accounting.
// Parameters are ASK prices and sizes (the prices you PAY to BUY).
func NewOpportunity(
	marketID string,
	marketSlug string,
	marketQuestion string,
	yesTokenID string,
	noTokenID string,
	yesAskPrice float64,
	yesAskSize float64,
	noAskPrice float64,
	noAskSize float64,
	threshold float64,
	takerFee float64,
) *Opportunity {
	priceSum := yesAskPrice + noAskPrice
	profitMargin := 1.0 - priceSum

	maxSize := yesAskSize
	if noAskSize < maxSize {
		maxSize = noAskSize
	}

	// Calculate fees (taker fee on both sides since we're taking liquidity)
	totalCost := (yesAskPrice + noAskPrice) * maxSize
	totalFees := totalCost * takerFee
	grossProfit := profitMargin * maxSize
	netProfit := grossProfit - totalFees

	return &Opportunity{
		ID:              uuid.New().String(),
		MarketID:        marketID,
		MarketSlug:      marketSlug,
		MarketQuestion:  marketQuestion,
		YesTokenID:      yesTokenID,
		NoTokenID:       noTokenID,
		DetectedAt:      time.Now(),
		YesAskPrice:     yesAskPrice,
		YesAskSize:      yesAskSize,
		NoAskPrice:      noAskPrice,
		NoAskSize:       noAskSize,
		PriceSum:        priceSum,
		ProfitMargin:    profitMargin,
		ProfitBPS:       int(profitMargin * 10000),
		MaxTradeSize:    maxSize,
		EstimatedProfit: grossProfit,
		TotalFees:       totalFees,
		NetProfit:       netProfit,
		NetProfitBPS:    int((netProfit / maxSize) * 10000),
		ConfigThreshold: threshold,
	}
}

// String returns a human-readable representation of the opportunity.
func (o *Opportunity) String() string {
	return fmt.Sprintf(
		"Opportunity[%s] Market=%s YES=%.4f NO=%.4f Sum=%.4f Profit=%dbps Size=%.2f Est=$%.2f",
		o.ID[:8],
		o.MarketSlug,
		o.YesAskPrice,
		o.NoAskPrice,
		o.PriceSum,
		o.ProfitBPS,
		o.MaxTradeSize,
		o.EstimatedProfit,
	)
}
