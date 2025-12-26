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
	YesTrade       *Trade    // For binary markets (backward compatibility)
	NoTrade        *Trade    // For binary markets (backward compatibility)
	AllTrades      []*Trade  // For all markets (binary + multi-outcome)
	RealizedProfit float64
	Success        bool
	Error          error
}
