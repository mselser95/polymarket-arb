package storage

import (
	"context"

	"github.com/mselser95/polymarket-arb/internal/arbitrage"
)

// Storage is the interface for storing arbitrage opportunities.
type Storage interface {
	// StoreOpportunity stores an arbitrage opportunity.
	StoreOpportunity(ctx context.Context, opp *arbitrage.Opportunity) error

	// Close closes the storage connection.
	Close() error
}
