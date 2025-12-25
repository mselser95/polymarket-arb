package types

import "time"

// Trade represents a single trade execution.
type Trade struct {
	TokenID   string
	Outcome   string // "YES" or "NO"
	Side      string // "BUY" or "SELL"
	Price     float64
	Size      float64
	Timestamp time.Time
}

// ExecutionResult contains the result of executing an arbitrage opportunity.
type ExecutionResult struct {
	OpportunityID  string
	MarketSlug     string
	ExecutedAt     time.Time
	YesTrade       *Trade
	NoTrade        *Trade
	RealizedProfit float64
	Success        bool
	Error          error
}
