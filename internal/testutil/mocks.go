package testutil

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"

	"github.com/mselser95/polymarket-arb/internal/arbitrage"
	"github.com/mselser95/polymarket-arb/pkg/types"
)

// MockGammaAPI is a mock HTTP server that simulates the Polymarket Gamma API.
type MockGammaAPI struct {
	*httptest.Server
	Markets []*types.Market
	mu      sync.RWMutex
}

// NewMockGammaAPI creates a new mock Gamma API server.
func NewMockGammaAPI(markets []*types.Market) *MockGammaAPI {
	mock := &MockGammaAPI{
		Markets: markets,
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mock.mu.RLock()
		defer mock.mu.RUnlock()

		// Handle /markets endpoint (list markets)
		// Gamma API returns a direct array, not wrapped in an object
		if r.URL.Path == "/markets" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mock.Markets)
			return
		}

		// Handle /markets/{slug} endpoint (single market)
		if len(r.URL.Path) > 9 && r.URL.Path[:9] == "/markets/" {
			slug := r.URL.Path[9:]
			for _, m := range mock.Markets {
				if m.Slug == slug {
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(m)
					return
				}
			}
			http.NotFound(w, r)
			return
		}

		http.NotFound(w, r)
	})

	mock.Server = httptest.NewServer(handler)
	return mock
}

// AddMarket adds a market to the mock API.
func (m *MockGammaAPI) AddMarket(market *types.Market) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Markets = append(m.Markets, market)
}

// MockStorage is an in-memory storage implementation for testing.
type MockStorage struct {
	Opportunities []*arbitrage.Opportunity
	mu            sync.Mutex
}

// NewMockStorage creates a new mock storage.
func NewMockStorage() *MockStorage {
	return &MockStorage{
		Opportunities: make([]*arbitrage.Opportunity, 0),
	}
}

// StoreOpportunity stores an opportunity in memory.
func (m *MockStorage) StoreOpportunity(ctx context.Context, opp *arbitrage.Opportunity) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Store a copy to avoid race conditions
	oppCopy := *opp
	m.Opportunities = append(m.Opportunities, &oppCopy)
	return nil
}

// Close is a no-op for mock storage.
func (m *MockStorage) Close() error {
	return nil
}

// GetOpportunities returns all stored opportunities.
func (m *MockStorage) GetOpportunities() []*arbitrage.Opportunity {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Return a copy to avoid race conditions
	result := make([]*arbitrage.Opportunity, len(m.Opportunities))
	copy(result, m.Opportunities)
	return result
}

// Clear clears all stored opportunities.
func (m *MockStorage) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Opportunities = make([]*arbitrage.Opportunity, 0)
}

// MockWebSocket simulates a WebSocket connection for testing.
type MockWebSocket struct {
	Messages      chan *types.OrderbookMessage
	Subscriptions []string
	Connected     bool
	mu            sync.Mutex
}

// NewMockWebSocket creates a new mock WebSocket.
func NewMockWebSocket(bufferSize int) *MockWebSocket {
	return &MockWebSocket{
		Messages:      make(chan *types.OrderbookMessage, bufferSize),
		Subscriptions: make([]string, 0),
		Connected:     false,
	}
}

// Connect simulates a WebSocket connection.
func (m *MockWebSocket) Connect() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Connected = true
}

// Disconnect simulates a WebSocket disconnection.
func (m *MockWebSocket) Disconnect() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Connected = false
}

// Subscribe simulates subscribing to token IDs.
func (m *MockWebSocket) Subscribe(tokenIDs []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Subscriptions = append(m.Subscriptions, tokenIDs...)
}

// SendMessage simulates receiving a WebSocket message.
func (m *MockWebSocket) SendMessage(msg *types.OrderbookMessage) {
	select {
	case m.Messages <- msg:
	default:
		// Drop message if buffer is full
	}
}

// IsConnected returns whether the mock WebSocket is connected.
func (m *MockWebSocket) IsConnected() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Connected
}

// GetSubscriptions returns all subscriptions.
func (m *MockWebSocket) GetSubscriptions() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]string, len(m.Subscriptions))
	copy(result, m.Subscriptions)
	return result
}

// Close closes the mock WebSocket.
func (m *MockWebSocket) Close() {
	close(m.Messages)
}
