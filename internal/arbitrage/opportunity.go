package arbitrage

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// OpportunityOutcome represents a single outcome in an arbitrage opportunity.
type OpportunityOutcome struct {
	TokenID  string  // CLOB token ID for this outcome
	Outcome  string  // Human-readable outcome name ("YES", "NO", "Candidate A", etc.)
	AskPrice float64 // Price to BUY this outcome
	AskSize  float64 // Size available to BUY this outcome
	TickSize float64 // Price tick size for this outcome (from market metadata)
	MinSize  float64 // Minimum order size for this outcome (from market metadata)
}

// Opportunity represents an arbitrage opportunity.
// Supports both binary (2 outcomes) and multi-outcome (3+) markets.
type Opportunity struct {
	ID              string
	MarketID        string
	MarketSlug      string
	MarketQuestion  string
	Outcomes        []OpportunityOutcome // All outcomes in this opportunity (2+)
	DetectedAt      time.Time
	TotalPriceSum   float64 // Sum of all outcome ask prices
	ProfitMargin    float64 // 1.0 - TotalPriceSum
	ProfitBPS       int     // Profit margin in basis points
	MaxTradeSize    float64 // Minimum size across all outcomes
	EstimatedProfit float64 // Gross profit (before fees)
	TotalFees       float64 // Total taker fees for all outcomes
	NetProfit       float64 // Net profit after fees
	NetProfitBPS    int     // Net profit in basis points
	ConfigThreshold float64 // Configured threshold for detection
}

// NewOpportunity creates a new arbitrage opportunity with fee accounting.
// This is a backward-compatible wrapper for binary markets.
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
	// Convert binary parameters to multi-outcome format
	outcomes := []OpportunityOutcome{
		{
			TokenID:  yesTokenID,
			Outcome:  "YES",
			AskPrice: yesAskPrice,
			AskSize:  yesAskSize,
		},
		{
			TokenID:  noTokenID,
			Outcome:  "NO",
			AskPrice: noAskPrice,
			AskSize:  noAskSize,
		},
	}

	// Calculate maxTradeSize (minimum of both sides)
	maxTradeSize := yesAskSize
	if noAskSize < maxTradeSize {
		maxTradeSize = noAskSize
	}

	return NewMultiOutcomeOpportunity(marketID, marketSlug, marketQuestion, outcomes, maxTradeSize, threshold, takerFee)
}

// NewMultiOutcomeOpportunity creates an arbitrage opportunity for N-outcome markets.
// Works for both binary (2 outcomes) and multi-outcome (3+) markets.
// The maxTradeSize parameter is pre-calculated by the detector and includes all constraints.
func NewMultiOutcomeOpportunity(
	marketID string,
	marketSlug string,
	marketQuestion string,
	outcomes []OpportunityOutcome,
	maxTradeSize float64, // Pre-calculated by detector (includes min/max/metadata constraints)
	threshold float64,
	takerFee float64,
) *Opportunity {
	// Calculate sum of all outcome prices
	priceSum := 0.0
	for _, outcome := range outcomes {
		priceSum += outcome.AskPrice
	}

	profitMargin := 1.0 - priceSum

	// Use maxTradeSize from detector (already constrained)
	maxSize := maxTradeSize

	// Calculate fees (taker fee on all outcomes since we're taking liquidity)
	totalCost := priceSum * maxSize
	totalFees := totalCost * takerFee
	grossProfit := profitMargin * maxSize
	netProfit := grossProfit - totalFees

	// Calculate net profit BPS (avoid division by zero)
	netProfitBPS := 0
	if maxSize > 0 {
		netProfitBPS = int((netProfit / maxSize) * 10000)
	}

	return &Opportunity{
		ID:              uuid.New().String(),
		MarketID:        marketID,
		MarketSlug:      marketSlug,
		MarketQuestion:  marketQuestion,
		Outcomes:        outcomes,
		DetectedAt:      time.Now(),
		TotalPriceSum:   priceSum,
		ProfitMargin:    profitMargin,
		ProfitBPS:       int(profitMargin * 10000),
		MaxTradeSize:    maxSize,
		EstimatedProfit: grossProfit,
		TotalFees:       totalFees,
		NetProfit:       netProfit,
		NetProfitBPS:    netProfitBPS,
		ConfigThreshold: threshold,
	}
}

// String returns a human-readable representation of the opportunity.
func (o *Opportunity) String() string {
	// For binary markets, use concise format
	if len(o.Outcomes) == 2 {
		return fmt.Sprintf(
			"Opportunity[%s] Market=%s %s=%.4f %s=%.4f Sum=%.4f Profit=%dbps Size=%.2f Est=$%.2f",
			o.ID[:8],
			o.MarketSlug,
			o.Outcomes[0].Outcome,
			o.Outcomes[0].AskPrice,
			o.Outcomes[1].Outcome,
			o.Outcomes[1].AskPrice,
			o.TotalPriceSum,
			o.ProfitBPS,
			o.MaxTradeSize,
			o.EstimatedProfit,
		)
	}

	// For multi-outcome markets, show outcome count
	return fmt.Sprintf(
		"Opportunity[%s] Market=%s Outcomes=%d Sum=%.4f Profit=%dbps Size=%.2f Est=$%.2f",
		o.ID[:8],
		o.MarketSlug,
		len(o.Outcomes),
		o.TotalPriceSum,
		o.ProfitBPS,
		o.MaxTradeSize,
		o.EstimatedProfit,
	)
}
