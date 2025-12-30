package testutil

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/mselser95/polymarket-arb/pkg/types"
	"github.com/mselser95/polymarket-arb/pkg/wallet"
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
			_ = json.NewEncoder(w).Encode(mock.Markets) //nolint:errcheck // Test mock
			return
		}

		// Handle /markets/{slug} endpoint (single market)
		if len(r.URL.Path) > 9 && r.URL.Path[:9] == "/markets/" {
			slug := r.URL.Path[9:]
			for _, m := range mock.Markets {
				if m.Slug == slug {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(m) //nolint:errcheck // Test mock
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
	Opportunities []any
	mu            sync.Mutex
}

// NewMockStorage creates a new mock storage.
func NewMockStorage() *MockStorage {
	return &MockStorage{
		Opportunities: make([]any, 0),
	}
}

// StoreOpportunity stores an opportunity in memory.
func (m *MockStorage) StoreOpportunity(ctx context.Context, opp any) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Store the opportunity (already a pointer typically)
	m.Opportunities = append(m.Opportunities, opp)
	return nil
}

// Close is a no-op for mock storage.
func (m *MockStorage) Close() error {
	return nil
}

// GetOpportunities returns all stored opportunities.
func (m *MockStorage) GetOpportunities() []any {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Return a copy to avoid race conditions
	result := make([]any, len(m.Opportunities))
	copy(result, m.Opportunities)
	return result
}

// Clear clears all stored opportunities.
func (m *MockStorage) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Opportunities = make([]any, 0)
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

// MockWalletClient is a mock implementation of wallet.Client for testing.
type MockWalletClient struct {
	mu             sync.Mutex
	balances       *wallet.Balances
	positions      []*wallet.Position
	getBalancesErr error
	getPositionsErr error
}

// NewMockWalletClient creates a new mock wallet client.
func NewMockWalletClient() (client *MockWalletClient) {
	return &MockWalletClient{
		balances: &wallet.Balances{
			MATIC:         big.NewInt(0),
			USDC:          big.NewInt(0),
			USDCAllowance: big.NewInt(0),
		},
		positions: make([]*wallet.Position, 0),
	}
}

// GetBalances returns the configured mock balances.
func (m *MockWalletClient) GetBalances(ctx context.Context, address common.Address) (balances *wallet.Balances, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.getBalancesErr != nil {
		return nil, m.getBalancesErr
	}

	// Return a copy to avoid race conditions
	return &wallet.Balances{
		MATIC:         new(big.Int).Set(m.balances.MATIC),
		USDC:          new(big.Int).Set(m.balances.USDC),
		USDCAllowance: new(big.Int).Set(m.balances.USDCAllowance),
	}, nil
}

// GetPositions returns the configured mock positions.
func (m *MockWalletClient) GetPositions(ctx context.Context, address common.Address) (positions []*wallet.Position, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.getPositionsErr != nil {
		return nil, m.getPositionsErr
	}

	// Return a copy to avoid race conditions
	result := make([]*wallet.Position, len(m.positions))
	copy(result, m.positions)
	return result, nil
}

// SetBalances sets the mock balances that will be returned.
func (m *MockWalletClient) SetBalances(matic, usdc, allowance *big.Int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.balances = &wallet.Balances{
		MATIC:         matic,
		USDC:          usdc,
		USDCAllowance: allowance,
	}
}

// SetUSDCBalance sets only the USDC balance (convenience method).
func (m *MockWalletClient) SetUSDCBalance(usdc *big.Int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.balances.USDC = usdc
}

// SetPositions sets the mock positions that will be returned.
func (m *MockWalletClient) SetPositions(positions []*wallet.Position) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.positions = positions
}

// SetGetBalancesError sets an error to be returned by GetBalances.
func (m *MockWalletClient) SetGetBalancesError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.getBalancesErr = err
}

// SetGetPositionsError sets an error to be returned by GetPositions.
func (m *MockWalletClient) SetGetPositionsError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.getPositionsErr = err
}

// ResetErrors clears all error states.
func (m *MockWalletClient) ResetErrors() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.getBalancesErr = nil
	m.getPositionsErr = nil
}

// NewUSDCBigInt is a helper to create a *big.Int representing USDC amount.
// USDC has 6 decimals, so 1000000 = $1.00
func NewUSDCBigInt(dollars float64) (amount *big.Int) {
	usdcUnits := int64(dollars * 1e6)
	return big.NewInt(usdcUnits)
}
