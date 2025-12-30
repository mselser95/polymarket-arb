package arbitrage

import (
	"context"
	"sync"
)

// MockStorage is an in-memory storage implementation for testing arbitrage detection.
// This mock lives in the arbitrage package to avoid import cycles.
type MockStorage struct {
	Opportunities []*Opportunity
	mu            sync.Mutex
}

// NewMockStorage creates a new mock storage for arbitrage tests.
func NewMockStorage() *MockStorage {
	return &MockStorage{
		Opportunities: make([]*Opportunity, 0),
	}
}

// StoreOpportunity stores an opportunity in memory.
func (m *MockStorage) StoreOpportunity(ctx context.Context, opp *Opportunity) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Opportunities = append(m.Opportunities, opp)
	return nil
}

// Close is a no-op for mock storage.
func (m *MockStorage) Close() error {
	return nil
}

// GetOpportunities returns all stored opportunities.
func (m *MockStorage) GetOpportunities() []*Opportunity {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Return a copy to avoid race conditions
	result := make([]*Opportunity, len(m.Opportunities))
	copy(result, m.Opportunities)
	return result
}

// Clear clears all stored opportunities.
func (m *MockStorage) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Opportunities = make([]*Opportunity, 0)
}
