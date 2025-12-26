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

// FillStatus represents the fill verification result for a single order.
type FillStatus struct {
	OrderID      string
	Outcome      string
	Status       string  // "matched", "live", "unmatched"
	OriginalSize float64
	SizeFilled   float64
	ActualPrice  float64
	FullyFilled  bool // true if SizeFilled == OriginalSize
	VerifiedAt   time.Time
	Error        error
}

// ExecutionResult contains the result of executing an arbitrage opportunity.
type ExecutionResult struct {
	OpportunityID  string
	MarketSlug     string
	ExecutedAt     time.Time
	YesTrade       *Trade    // For binary markets (backward compatibility)
	NoTrade        *Trade    // For binary markets (backward compatibility)
	AllTrades      []*Trade  // For all markets (binary + multi-outcome)
	RealizedProfit float64   // ACTUAL profit after fills verified
	Success        bool
	Error          error

	// Fill verification data
	OrderIDs        []string      // Order IDs for tracking
	FillStatuses    []FillStatus  // Fill verification results
	AllOrdersFilled bool          // true if all 100% filled
	ExpectedProfit  float64       // Expected profit at order time
	VerifiedAt      time.Time     // When fills were verified
	PriceAdjustment float64       // How much above ask we placed orders
}
