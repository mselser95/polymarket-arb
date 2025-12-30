package testutil

import (
	"context"
	"fmt"
	"sync"

	"github.com/mselser95/polymarket-arb/pkg/types"
)

// MockOrderClient simulates order placement for testing.
type MockOrderClient struct {
	mu             sync.Mutex
	placedOrders   []MockPlacedOrder
	shouldFail     bool
	failureMessage string
	orderIDCounter int
}

// MockPlacedOrder records details of a placed order for verification.
type MockPlacedOrder struct {
	TokenID    string
	Price      float64
	TokenCount float64
	Outcome    string
	OrderID    string
}

// NewMockOrderClient creates a mock order client.
func NewMockOrderClient() *MockOrderClient {
	return &MockOrderClient{
		placedOrders:   make([]MockPlacedOrder, 0),
		shouldFail:     false,
		orderIDCounter: 1,
	}
}

// PlaceOrdersMultiOutcome simulates placing multiple orders.
// CRITICAL: All orders use the SAME tokenCount (arbitrage requirement).
// The interface enforces this by taking a single tokenCount parameter.
func (m *MockOrderClient) PlaceOrdersMultiOutcome(
	ctx context.Context,
	outcomes []types.OutcomeOrderParams,
	tokenCount float64, // SAME count for ALL outcomes
) ([]*types.OrderSubmissionResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldFail {
		return nil, fmt.Errorf("mock order placement failed: %s", m.failureMessage)
	}

	responses := make([]*types.OrderSubmissionResponse, len(outcomes))
	for i, outcome := range outcomes {
		orderID := fmt.Sprintf("mock-order-%d", m.orderIDCounter)
		m.orderIDCounter++

		// Record the order - note ALL orders get the same tokenCount
		m.placedOrders = append(m.placedOrders, MockPlacedOrder{
			TokenID:    outcome.TokenID,
			Price:      outcome.Price,
			TokenCount: tokenCount, // Same for all (arbitrage requirement)
			Outcome:    "outcome",  // Would need to extract from outcomes
			OrderID:    orderID,
		})

		responses[i] = &types.OrderSubmissionResponse{
			OrderID:  orderID,
			Success:  true,
			ErrorMsg: "",
		}
	}

	return responses, nil
}

// GetPlacedOrders returns all orders placed during the test.
func (m *MockOrderClient) GetPlacedOrders() []MockPlacedOrder {
	m.mu.Lock()
	defer m.mu.Unlock()

	orders := make([]MockPlacedOrder, len(m.placedOrders))
	copy(orders, m.placedOrders)
	return orders
}

// SetFailure configures the mock to fail order placement.
func (m *MockOrderClient) SetFailure(shouldFail bool, message string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.shouldFail = shouldFail
	m.failureMessage = message
}

// Reset clears all recorded orders.
func (m *MockOrderClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.placedOrders = make([]MockPlacedOrder, 0)
	m.orderIDCounter = 1
}
