package orderbook

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/mselser95/polymarket-arb/pkg/types"
	"go.uber.org/zap"
)

// TestManager_HandleBookMessage_ValidSnapshot tests full book snapshot handling
func TestManager_HandleBookMessage_ValidSnapshot(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	msgChan := make(chan *types.OrderbookMessage, 10)
	cfg := &Config{
		Logger:         logger,
		MessageChannel: msgChan,
	}

	mgr := New(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := mgr.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start manager: %v", err)
	}

	// Send valid book message
	msg := &types.OrderbookMessage{
		EventType: "book",
		AssetID:   "token1",
		Market:    "market1",
		Timestamp: time.Now().UnixMilli(),
		Bids: []types.PriceLevel{
			{Price: "0.50", Size: "100"},
		},
		Asks: []types.PriceLevel{
			{Price: "0.51", Size: "100"},
		},
	}

	msgChan <- msg

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Verify snapshot was created
	snapshot, exists := mgr.GetSnapshot("token1")
	if !exists {
		t.Fatal("expected snapshot to exist")
	}

	if snapshot.BestBidPrice != 0.50 {
		t.Errorf("expected best bid 0.50, got %f", snapshot.BestBidPrice)
	}

	if snapshot.BestAskPrice != 0.51 {
		t.Errorf("expected best ask 0.51, got %f", snapshot.BestAskPrice)
	}

	cancel() // Must cancel context before Close() so goroutines can exit
	mgr.Close()
}

// TestManager_HandleBookMessage_EmptyOrderbook tests handling of empty orderbooks
func TestManager_HandleBookMessage_EmptyOrderbook(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	msgChan := make(chan *types.OrderbookMessage, 10)
	cfg := &Config{
		Logger:         logger,
		MessageChannel: msgChan,
	}

	mgr := New(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := mgr.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start manager: %v", err)
	}

	// Send book message with empty bids
	msg := &types.OrderbookMessage{
		EventType: "book",
		AssetID:   "token1",
		Market:    "market1",
		Timestamp: time.Now().UnixMilli(),
		Bids:      []types.PriceLevel{}, // Empty
		Asks: []types.PriceLevel{
			{Price: "0.51", Size: "100"},
		},
	}

	msgChan <- msg

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Snapshot should not be created due to missing bid
	_, exists := mgr.GetSnapshot("token1")
	if exists {
		t.Error("expected no snapshot for orderbook with empty bids")
	}

	cancel() // Must cancel context before Close() so goroutines can exit
	mgr.Close()
}

// TestManager_HandlePriceChange_ExistingSnapshot tests incremental updates
func TestManager_HandlePriceChange_ExistingSnapshot(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	msgChan := make(chan *types.OrderbookMessage, 10)
	cfg := &Config{
		Logger:         logger,
		MessageChannel: msgChan,
	}

	mgr := New(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := mgr.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start manager: %v", err)
	}

	// Create initial snapshot
	bookMsg := &types.OrderbookMessage{
		EventType: "book",
		AssetID:   "token1",
		Market:    "market1",
		Timestamp: time.Now().UnixMilli(),
		Bids: []types.PriceLevel{
			{Price: "0.50", Size: "100"},
		},
		Asks: []types.PriceLevel{
			{Price: "0.51", Size: "100"},
		},
	}

	msgChan <- bookMsg
	time.Sleep(100 * time.Millisecond)

	// Send price change update
	priceChangeMsg := &types.OrderbookMessage{
		EventType: "price_change",
		AssetID:   "token1",
		Market:    "market1",
		Timestamp: time.Now().UnixMilli(),
		Bids: []types.PriceLevel{
			{Price: "0.52", Size: "0"}, // Size 0 means preserve existing size
		},
		Asks: []types.PriceLevel{},
	}

	msgChan <- priceChangeMsg
	time.Sleep(100 * time.Millisecond)

	// Verify price was updated
	snapshot, exists := mgr.GetSnapshot("token1")
	if !exists {
		t.Fatal("expected snapshot to exist")
	}

	if snapshot.BestBidPrice != 0.52 {
		t.Errorf("expected bid price updated to 0.52, got %f", snapshot.BestBidPrice)
	}

	// Size should be preserved (100 from original)
	if snapshot.BestBidSize != 100 {
		t.Errorf("expected bid size preserved at 100, got %f", snapshot.BestBidSize)
	}

	cancel() // Must cancel context before Close() so goroutines can exit
	mgr.Close()
}

// TestManager_HandlePriceChange_MissingSnapshot tests fallback to full book
func TestManager_HandlePriceChange_MissingSnapshot(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	msgChan := make(chan *types.OrderbookMessage, 10)
	cfg := &Config{
		Logger:         logger,
		MessageChannel: msgChan,
	}

	mgr := New(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := mgr.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start manager: %v", err)
	}

	// Send price_change without existing snapshot
	// This should trigger fallback to handleBookMessage
	msg := &types.OrderbookMessage{
		EventType: "price_change",
		AssetID:   "token1",
		Market:    "market1",
		Timestamp: time.Now().UnixMilli(),
		Bids: []types.PriceLevel{
			{Price: "0.50", Size: "100"},
		},
		Asks: []types.PriceLevel{
			{Price: "0.51", Size: "100"},
		},
	}

	msgChan <- msg
	time.Sleep(100 * time.Millisecond)

	// Snapshot should be created via fallback
	snapshot, exists := mgr.GetSnapshot("token1")
	if !exists {
		t.Fatal("expected snapshot to be created via fallback")
	}

	if snapshot.BestBidPrice != 0.50 {
		t.Errorf("expected best bid 0.50, got %f", snapshot.BestBidPrice)
	}

	cancel() // Must cancel context before Close() so goroutines can exit
	mgr.Close()
}

// TestManager_HandlePriceChange_PartialUpdate tests updates with only bid or ask
func TestManager_HandlePriceChange_PartialUpdate(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	msgChan := make(chan *types.OrderbookMessage, 10)
	cfg := &Config{
		Logger:         logger,
		MessageChannel: msgChan,
	}

	mgr := New(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := mgr.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start manager: %v", err)
	}

	// Create initial snapshot
	bookMsg := &types.OrderbookMessage{
		EventType: "book",
		AssetID:   "token1",
		Market:    "market1",
		Timestamp: time.Now().UnixMilli(),
		Bids: []types.PriceLevel{
			{Price: "0.50", Size: "100"},
		},
		Asks: []types.PriceLevel{
			{Price: "0.51", Size: "100"},
		},
	}

	msgChan <- bookMsg
	time.Sleep(100 * time.Millisecond)

	// Update only ask price
	priceChangeMsg := &types.OrderbookMessage{
		EventType: "price_change",
		AssetID:   "token1",
		Market:    "market1",
		Timestamp: time.Now().UnixMilli(),
		Bids:      []types.PriceLevel{}, // Empty - no bid update
		Asks: []types.PriceLevel{
			{Price: "0.52", Size: "0"},
		},
	}

	msgChan <- priceChangeMsg
	time.Sleep(100 * time.Millisecond)

	// Verify only ask was updated
	snapshot, exists := mgr.GetSnapshot("token1")
	if !exists {
		t.Fatal("expected snapshot to exist")
	}

	if snapshot.BestBidPrice != 0.50 {
		t.Errorf("expected bid price unchanged at 0.50, got %f", snapshot.BestBidPrice)
	}

	if snapshot.BestAskPrice != 0.52 {
		t.Errorf("expected ask price updated to 0.52, got %f", snapshot.BestAskPrice)
	}

	cancel() // Must cancel context before Close() so goroutines can exit
	mgr.Close()
}

// TestExtractBestLevel_ValidData tests normal price/size extraction
func TestExtractBestLevel_ValidData(t *testing.T) {
	tests := []struct {
		name          string
		levels        []types.PriceLevel
		expectedPrice float64
		expectedSize  float64
	}{
		{
			name: "single level",
			levels: []types.PriceLevel{
				{Price: "0.50", Size: "100"},
			},
			expectedPrice: 0.50,
			expectedSize:  100,
		},
		{
			name: "multiple levels - first is best",
			levels: []types.PriceLevel{
				{Price: "0.50", Size: "100"},
				{Price: "0.51", Size: "200"},
			},
			expectedPrice: 0.50,
			expectedSize:  100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			price, size, err := extractBestLevel(tt.levels)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if price != tt.expectedPrice {
				t.Errorf("expected price %f, got %f", tt.expectedPrice, price)
			}

			if size != tt.expectedSize {
				t.Errorf("expected size %f, got %f", tt.expectedSize, size)
			}
		})
	}
}

// TestExtractBestLevel_EmptyLevels tests error on empty levels
func TestExtractBestLevel_EmptyLevels(t *testing.T) {
	_, _, err := extractBestLevel([]types.PriceLevel{})
	if err == nil {
		t.Error("expected error for empty levels, got nil")
	}

	expectedMsg := "no price levels"
	if err.Error() != expectedMsg {
		t.Errorf("expected error message '%s', got '%s'", expectedMsg, err.Error())
	}
}

// TestExtractBestLevel_InvalidPrice tests parse error handling
func TestExtractBestLevel_InvalidPrice(t *testing.T) {
	levels := []types.PriceLevel{
		{Price: "invalid", Size: "100"},
	}

	_, _, err := extractBestLevel(levels)
	if err == nil {
		t.Error("expected error for invalid price, got nil")
	}
}

// TestExtractBestLevel_InvalidSize tests parse error handling
func TestExtractBestLevel_InvalidSize(t *testing.T) {
	levels := []types.PriceLevel{
		{Price: "0.50", Size: "invalid"},
	}

	_, _, err := extractBestLevel(levels)
	if err == nil {
		t.Error("expected error for invalid size, got nil")
	}
}

// TestManager_ConcurrentReads tests multiple GetSnapshot calls
func TestManager_ConcurrentReads(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	msgChan := make(chan *types.OrderbookMessage, 10)
	cfg := &Config{
		Logger:         logger,
		MessageChannel: msgChan,
	}

	mgr := New(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := mgr.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start manager: %v", err)
	}

	// Create initial snapshot
	bookMsg := &types.OrderbookMessage{
		EventType: "book",
		AssetID:   "token1",
		Market:    "market1",
		Timestamp: time.Now().UnixMilli(),
		Bids: []types.PriceLevel{
			{Price: "0.50", Size: "100"},
		},
		Asks: []types.PriceLevel{
			{Price: "0.51", Size: "100"},
		},
	}

	msgChan <- bookMsg
	time.Sleep(100 * time.Millisecond)

	// Concurrent reads
	var wg sync.WaitGroup
	numReaders := 50

	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			snapshot, exists := mgr.GetSnapshot("token1")
			if !exists {
				t.Error("expected snapshot to exist")
				return
			}

			if snapshot.BestBidPrice != 0.50 {
				t.Errorf("expected best bid 0.50, got %f", snapshot.BestBidPrice)
			}
		}()
	}

	wg.Wait()
	cancel() // Must cancel context before Close() so goroutines can exit
	mgr.Close()
}

// TestManager_ConcurrentWrites tests concurrent handleBookMessage
func TestManager_ConcurrentWrites(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	msgChan := make(chan *types.OrderbookMessage, 1000)
	cfg := &Config{
		Logger:         logger,
		MessageChannel: msgChan,
	}

	mgr := New(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := mgr.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start manager: %v", err)
	}

	// Send many book messages concurrently
	numWrites := 100

	for i := 0; i < numWrites; i++ {
		msg := &types.OrderbookMessage{
			EventType: "book",
			AssetID:   fmt.Sprintf("token%d", i),
			Market:    "market1",
			Timestamp: time.Now().UnixMilli(),
			Bids: []types.PriceLevel{
				{Price: "0.50", Size: "100"},
			},
			Asks: []types.PriceLevel{
				{Price: "0.51", Size: "100"},
			},
		}
		msgChan <- msg
	}

	// Wait for processing
	time.Sleep(500 * time.Millisecond)

	// Verify all snapshots created
	snapshots := mgr.GetAllSnapshots()
	if len(snapshots) != numWrites {
		t.Errorf("expected %d snapshots, got %d", numWrites, len(snapshots))
	}

	cancel() // Must cancel context before Close() so goroutines can exit
	mgr.Close()
}

// TestManager_ReadDuringWrite tests GetSnapshot during concurrent updates
func TestManager_ReadDuringWrite(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	msgChan := make(chan *types.OrderbookMessage, 100)
	cfg := &Config{
		Logger:         logger,
		MessageChannel: msgChan,
	}

	mgr := New(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := mgr.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start manager: %v", err)
	}

	// Create initial snapshot
	bookMsg := &types.OrderbookMessage{
		EventType: "book",
		AssetID:   "token1",
		Market:    "market1",
		Timestamp: time.Now().UnixMilli(),
		Bids: []types.PriceLevel{
			{Price: "0.50", Size: "100"},
		},
		Asks: []types.PriceLevel{
			{Price: "0.51", Size: "100"},
		},
	}

	msgChan <- bookMsg
	time.Sleep(100 * time.Millisecond)

	// Concurrent reads and writes
	var wg sync.WaitGroup

	// Writers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for j := 0; j < 10; j++ {
				msg := &types.OrderbookMessage{
					EventType: "price_change",
					AssetID:   "token1",
					Market:    "market1",
					Timestamp: time.Now().UnixMilli(),
					Bids: []types.PriceLevel{
						{Price: fmt.Sprintf("0.%d", 50+j), Size: "0"},
					},
					Asks: []types.PriceLevel{},
				}
				msgChan <- msg
				time.Sleep(1 * time.Millisecond)
			}
		}()
	}

	// Readers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for j := 0; j < 10; j++ {
				_, exists := mgr.GetSnapshot("token1")
				if !exists {
					t.Error("expected snapshot to exist during concurrent access")
				}
				time.Sleep(1 * time.Millisecond)
			}
		}()
	}

	wg.Wait()
	cancel() // Must cancel context before Close() so goroutines can exit
	mgr.Close()
}

// TestManager_GetAllSnapshots_Isolation tests map iteration safety
func TestManager_GetAllSnapshots_Isolation(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	msgChan := make(chan *types.OrderbookMessage, 100)
	cfg := &Config{
		Logger:         logger,
		MessageChannel: msgChan,
	}

	mgr := New(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := mgr.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start manager: %v", err)
	}

	// Create multiple snapshots
	for i := 0; i < 10; i++ {
		msg := &types.OrderbookMessage{
			EventType: "book",
			AssetID:   fmt.Sprintf("token%d", i),
			Market:    "market1",
			Timestamp: time.Now().UnixMilli(),
			Bids: []types.PriceLevel{
				{Price: "0.50", Size: "100"},
			},
			Asks: []types.PriceLevel{
				{Price: "0.51", Size: "100"},
			},
		}
		msgChan <- msg
	}

	time.Sleep(200 * time.Millisecond)

	// Get all snapshots
	snapshots1 := mgr.GetAllSnapshots()

	// Modify returned map (should not affect internal state)
	for tokenID := range snapshots1 {
		delete(snapshots1, tokenID)
	}

	// Get snapshots again - should still have all 10
	snapshots2 := mgr.GetAllSnapshots()
	if len(snapshots2) != 10 {
		t.Errorf("expected 10 snapshots after modifying returned map, got %d", len(snapshots2))
	}

	cancel() // Must cancel context before Close() so goroutines can exit
	mgr.Close()
}

// TestManager_UpdateChannel_Normal tests successful channel sends
func TestManager_UpdateChannel_Normal(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	msgChan := make(chan *types.OrderbookMessage, 10)
	cfg := &Config{
		Logger:         logger,
		MessageChannel: msgChan,
	}

	mgr := New(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := mgr.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start manager: %v", err)
	}

	// Subscribe to updates
	updateChan := mgr.UpdateChan()

	// Send book message
	msg := &types.OrderbookMessage{
		EventType: "book",
		AssetID:   "token1",
		Market:    "market1",
		Timestamp: time.Now().UnixMilli(),
		Bids: []types.PriceLevel{
			{Price: "0.50", Size: "100"},
		},
		Asks: []types.PriceLevel{
			{Price: "0.51", Size: "100"},
		},
	}

	msgChan <- msg

	// Wait for update
	select {
	case update := <-updateChan:
		if update.TokenID != "token1" {
			t.Errorf("expected token1 update, got %s", update.TokenID)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for orderbook update")
	}

	cancel() // Must cancel context before Close() so goroutines can exit
	mgr.Close()
}

// TestManager_UpdateChannel_SlowConsumer tests backpressure handling
func TestManager_UpdateChannel_SlowConsumer(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	msgChan := make(chan *types.OrderbookMessage, 1000)
	cfg := &Config{
		Logger:         logger,
		MessageChannel: msgChan,
	}

	mgr := New(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := mgr.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start manager: %v", err)
	}

	// Don't consume from update channel - simulate slow consumer
	// Just verify manager doesn't block

	// Send many messages quickly
	for i := 0; i < 100; i++ {
		msg := &types.OrderbookMessage{
			EventType: "book",
			AssetID:   fmt.Sprintf("token%d", i),
			Market:    "market1",
			Timestamp: time.Now().UnixMilli(),
			Bids: []types.PriceLevel{
				{Price: "0.50", Size: "100"},
			},
			Asks: []types.PriceLevel{
				{Price: "0.51", Size: "100"},
			},
		}
		msgChan <- msg
	}

	// Wait briefly - manager should not block even with slow consumer
	time.Sleep(200 * time.Millisecond)

	// If we reach here without hanging, test passes
	cancel() // Must cancel context before Close() so goroutines can exit
	mgr.Close()
}
