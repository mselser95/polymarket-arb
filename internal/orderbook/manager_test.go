package orderbook

import (
	"testing"

	"github.com/mselser95/polymarket-arb/pkg/types"
	"go.uber.org/zap"
)

func TestHandleBookMessage(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	manager := &Manager{
		books:  make(map[string]*types.OrderbookSnapshot),
		logger: logger,
	}

	// Create test book message
	msg := &types.OrderbookMessage{
		EventType: "book",
		AssetID:   "test-token-1",
		Market:    "test-market",
		Bids: []types.PriceLevel{
			{Price: "0.52", Size: "100.5"},
			{Price: "0.51", Size: "200.0"},
		},
		Asks: []types.PriceLevel{
			{Price: "0.54", Size: "150.0"},
			{Price: "0.55", Size: "250.0"},
		},
		Timestamp: 1234567890000,
	}

	// Handle message
	err := manager.handleBookMessage(msg)
	if err != nil {
		t.Fatalf("handleBookMessage failed: %v", err)
	}

	// Verify snapshot was created
	snapshot, exists := manager.GetSnapshot("test-token-1")
	if !exists {
		t.Fatal("expected snapshot to exist")
	}

	// Verify best bid
	if snapshot.BestBidPrice != 0.52 {
		t.Errorf("expected best_bid_price=0.52, got=%.2f", snapshot.BestBidPrice)
	}

	if snapshot.BestBidSize != 100.5 {
		t.Errorf("expected best_bid_size=100.5, got=%.2f", snapshot.BestBidSize)
	}

	// Verify best ask
	if snapshot.BestAskPrice != 0.54 {
		t.Errorf("expected best_ask_price=0.54, got=%.2f", snapshot.BestAskPrice)
	}

	if snapshot.BestAskSize != 150.0 {
		t.Errorf("expected best_ask_size=150.0, got=%.2f", snapshot.BestAskSize)
	}

	// Verify market ID
	if snapshot.MarketID != "test-market" {
		t.Errorf("expected market_id=test-market, got=%s", snapshot.MarketID)
	}
}

func TestHandlePriceChangeMessage(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	manager := &Manager{
		books:  make(map[string]*types.OrderbookSnapshot),
		logger: logger,
	}

	// Create initial book
	initialMsg := &types.OrderbookMessage{
		EventType: "book",
		AssetID:   "test-token-1",
		Market:    "test-market",
		Bids: []types.PriceLevel{
			{Price: "0.50", Size: "100.0"},
		},
		Asks: []types.PriceLevel{
			{Price: "0.52", Size: "100.0"},
		},
		Timestamp: 1234567890000,
	}

	err := manager.handleBookMessage(initialMsg)
	if err != nil {
		t.Fatalf("initial book failed: %v", err)
	}

	// Send price change
	priceChangeMsg := &types.OrderbookMessage{
		EventType: "price_change",
		AssetID:   "test-token-1",
		Market:    "test-market",
		Bids: []types.PriceLevel{
			{Price: "0.51", Size: "120.0"}, // Updated
		},
		Timestamp: 1234567891000,
	}

	err = manager.handlePriceChangeMessage(priceChangeMsg)
	if err != nil {
		t.Fatalf("price change failed: %v", err)
	}

	// Verify snapshot was updated
	snapshot, exists := manager.GetSnapshot("test-token-1")
	if !exists {
		t.Fatal("expected snapshot to exist")
	}

	// Verify bid was updated
	if snapshot.BestBidPrice != 0.51 {
		t.Errorf("expected updated best_bid_price=0.51, got=%.2f", snapshot.BestBidPrice)
	}

	if snapshot.BestBidSize != 120.0 {
		t.Errorf("expected updated best_bid_size=120.0, got=%.2f", snapshot.BestBidSize)
	}

	// Verify ask remained unchanged (not in price_change message)
	if snapshot.BestAskPrice != 0.52 {
		t.Errorf("expected ask to remain 0.52, got=%.2f", snapshot.BestAskPrice)
	}
}

func TestExtractBestLevel(t *testing.T) {
	tests := []struct {
		name        string
		levels      []types.PriceLevel
		expectPrice float64
		expectSize  float64
		expectError bool
	}{
		{
			name: "valid-level",
			levels: []types.PriceLevel{
				{Price: "0.52", Size: "100.5"},
			},
			expectPrice: 0.52,
			expectSize:  100.5,
			expectError: false,
		},
		{
			name:        "empty-levels",
			levels:      []types.PriceLevel{},
			expectError: true,
		},
		{
			name: "invalid-price",
			levels: []types.PriceLevel{
				{Price: "invalid", Size: "100.0"},
			},
			expectError: true,
		},
		{
			name: "invalid-size",
			levels: []types.PriceLevel{
				{Price: "0.52", Size: "invalid"},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			price, size, err := extractBestLevel(tt.levels)

			if tt.expectError {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if price != tt.expectPrice {
				t.Errorf("expected price=%.2f, got=%.2f", tt.expectPrice, price)
			}

			if size != tt.expectSize {
				t.Errorf("expected size=%.2f, got=%.2f", tt.expectSize, size)
			}
		})
	}
}
