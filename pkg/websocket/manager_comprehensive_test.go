package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/mselser95/polymarket-arb/pkg/types"
	"go.uber.org/zap"
)

// TestManager_ParseMessage_ArrayOfOrderbooks tests parsing array format
func TestManager_ParseMessage_ArrayOfOrderbooks(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := Config{
		URL:                   "wss://test.com",
		DialTimeout:           10 * time.Second,
		PongTimeout:           15 * time.Second,
		PingInterval:          10 * time.Second,
		ReconnectInitialDelay: 1 * time.Second,
		ReconnectMaxDelay:     30 * time.Second,
		ReconnectBackoffMult:  2.0,
		MessageBufferSize:     100,
		Logger:                logger,
	}

	_ = New(cfg)

	// Test array of orderbook messages (primary format)
	jsonData := `[{
		"event_type": "book",
		"market": "market1",
		"asset_id": "token1",
		"timestamp": "1234567890",
		"hash": "hash1",
		"bids": [{"price": "0.50", "size": "100"}],
		"asks": [{"price": "0.51", "size": "100"}]
	}]`

	var messages []types.OrderbookMessage
	err := json.Unmarshal([]byte(jsonData), &messages)
	if err != nil {
		t.Fatalf("failed to unmarshal test data: %v", err)
	}

	if len(messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(messages))
	}

	if messages[0].EventType != "book" {
		t.Errorf("expected event_type 'book', got '%s'", messages[0].EventType)
	}

	if messages[0].AssetID != "token1" {
		t.Errorf("expected asset_id 'token1', got '%s'", messages[0].AssetID)
	}
}

// TestManager_ParseMessage_SingleBook tests single message fallback format
func TestManager_ParseMessage_SingleBook(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := Config{
		URL:                   "wss://test.com",
		DialTimeout:           10 * time.Second,
		PongTimeout:           15 * time.Second,
		PingInterval:          10 * time.Second,
		ReconnectInitialDelay: 1 * time.Second,
		ReconnectMaxDelay:     30 * time.Second,
		ReconnectBackoffMult:  2.0,
		MessageBufferSize:     100,
		Logger:                logger,
	}

	_ = New(cfg)

	// Test single book message format
	jsonData := `{
		"event_type": "book",
		"market": "market1",
		"asset_id": "token1",
		"timestamp": "1234567890",
		"hash": "hash1",
		"bids": [{"price": "0.50", "size": "100"}],
		"asks": [{"price": "0.51", "size": "100"}]
	}`

	var message types.OrderbookMessage
	err := json.Unmarshal([]byte(jsonData), &message)
	if err != nil {
		t.Fatalf("failed to unmarshal test data: %v", err)
	}

	if message.EventType != "book" {
		t.Errorf("expected event_type 'book', got '%s'", message.EventType)
	}
}

// TestManager_ParseMessage_PriceChange tests price change message parsing
func TestManager_ParseMessage_PriceChange(t *testing.T) {
	tests := []struct {
		name     string
		jsonData string
		wantType string
	}{
		{
			name: "price_change with bids and asks",
			jsonData: `{
				"event_type": "price_change",
				"market": "market1",
				"asset_id": "token1",
				"timestamp": "1234567890",
				"hash": "hash1",
				"changes": {
					"bids": [{"price": "0.49", "size": "0"}],
					"asks": [{"price": "0.52", "size": "0"}]
				}
			}`,
			wantType: "price_change",
		},
		{
			name: "price_change with only bids",
			jsonData: `{
				"event_type": "price_change",
				"market": "market1",
				"asset_id": "token1",
				"timestamp": "1234567890",
				"changes": {
					"bids": [{"price": "0.49", "size": "0"}]
				}
			}`,
			wantType: "price_change",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var message types.OrderbookMessage
			err := json.Unmarshal([]byte(tt.jsonData), &message)
			if err != nil {
				t.Fatalf("failed to unmarshal: %v", err)
			}

			if message.EventType != tt.wantType {
				t.Errorf("expected event_type '%s', got '%s'", tt.wantType, message.EventType)
			}
		})
	}
}

// TestManager_ParseMessage_Heartbeat tests heartbeat detection
func TestManager_ParseMessage_Heartbeat(t *testing.T) {
	tests := []struct {
		name        string
		jsonData    string
		isHeartbeat bool
	}{
		{
			name:        "empty array heartbeat",
			jsonData:    `[]`,
			isHeartbeat: true,
		},
		{
			name:        "non-empty array not heartbeat",
			jsonData:    `[{"event_type": "book"}]`,
			isHeartbeat: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var messages []types.OrderbookMessage
			err := json.Unmarshal([]byte(tt.jsonData), &messages)
			if err != nil {
				t.Fatalf("failed to unmarshal: %v", err)
			}

			isHeartbeat := len(messages) == 0
			if isHeartbeat != tt.isHeartbeat {
				t.Errorf("expected heartbeat=%v, got %v", tt.isHeartbeat, isHeartbeat)
			}
		})
	}
}

// TestManager_ParseMessage_MalformedJSON tests error handling for invalid JSON
func TestManager_ParseMessage_MalformedJSON(t *testing.T) {
	tests := []struct {
		name     string
		jsonData string
	}{
		{
			name:     "truncated json",
			jsonData: `{"event_type": "book`,
		},
		{
			name:     "invalid syntax",
			jsonData: `{event_type: book}`,
		},
		{
			name:     "non-json data",
			jsonData: `not json at all`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var messages []types.OrderbookMessage
			err := json.Unmarshal([]byte(tt.jsonData), &messages)
			if err == nil {
				t.Error("expected error for malformed JSON, got nil")
			}
		})
	}
}

// TestManager_MessageChannel_Overflow tests channel overflow handling
func TestManager_MessageChannel_Overflow(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := Config{
		URL:                   "wss://test.com",
		DialTimeout:           10 * time.Second,
		PongTimeout:           15 * time.Second,
		PingInterval:          10 * time.Second,
		ReconnectInitialDelay: 1 * time.Second,
		ReconnectMaxDelay:     30 * time.Second,
		ReconnectBackoffMult:  2.0,
		MessageBufferSize:     5, // Small buffer for testing
		Logger:                logger,
	}

	mgr := New(cfg)

	// Fill the channel completely
	for i := 0; i < 5; i++ {
		msg := &types.OrderbookMessage{
			EventType: "book",
			AssetID:   fmt.Sprintf("token%d", i),
		}
		mgr.messageChan <- msg
	}

	// Next message should not block (testing non-blocking behavior)
	msg := &types.OrderbookMessage{
		EventType: "book",
		AssetID:   "overflow-token",
	}

	done := make(chan bool, 1)
	go func() {
		select {
		case mgr.messageChan <- msg:
			done <- true
		case <-time.After(100 * time.Millisecond):
			done <- false
		}
	}()

	select {
	case sent := <-done:
		if sent {
			t.Error("message was sent to full channel (should have been dropped or blocked)")
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("test timed out")
	}
}

// TestManager_Subscribe_Initial tests first-time subscription
func TestManager_Subscribe_Initial(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := Config{
		URL:                   "wss://test.com",
		DialTimeout:           10 * time.Second,
		PongTimeout:           15 * time.Second,
		PingInterval:          10 * time.Second,
		ReconnectInitialDelay: 1 * time.Second,
		ReconnectMaxDelay:     30 * time.Second,
		ReconnectBackoffMult:  2.0,
		MessageBufferSize:     100,
		Logger:                logger,
	}

	mgr := New(cfg)
	ctx := context.Background()

	// First subscription (no connection, so will fail gracefully)
	tokens := []string{"token1", "token2", "token3"}

	// Since there's no real connection, this will error
	// but we can verify the subscription tracking
	_ = mgr.Subscribe(ctx, tokens)

	// Verify tokens are tracked (even if connection failed)
	mgr.mu.RLock()
	for _, token := range tokens {
		if !mgr.subscribed[token] {
			t.Errorf("expected token %s to be tracked after Subscribe", token)
		}
	}
	mgr.mu.RUnlock()
}

// TestManager_Subscribe_Dynamic tests adding tokens to existing connection
func TestManager_Subscribe_Dynamic(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := Config{
		URL:                   "wss://test.com",
		DialTimeout:           10 * time.Second,
		PongTimeout:           15 * time.Second,
		PingInterval:          10 * time.Second,
		ReconnectInitialDelay: 1 * time.Second,
		ReconnectMaxDelay:     30 * time.Second,
		ReconnectBackoffMult:  2.0,
		MessageBufferSize:     100,
		Logger:                logger,
	}

	mgr := New(cfg)
	ctx := context.Background()

	// Pre-populate with initial tokens
	mgr.mu.Lock()
	mgr.subscribed["token1"] = true
	mgr.subscribed["token2"] = true
	mgr.mu.Unlock()

	// Add new tokens dynamically
	newTokens := []string{"token3", "token4"}
	_ = mgr.Subscribe(ctx, newTokens)

	// Verify all tokens are tracked
	mgr.mu.RLock()
	if len(mgr.subscribed) != 4 {
		t.Errorf("expected 4 subscribed tokens, got %d", len(mgr.subscribed))
	}

	for _, token := range newTokens {
		if !mgr.subscribed[token] {
			t.Errorf("expected new token %s to be tracked", token)
		}
	}
	mgr.mu.RUnlock()
}

// TestManager_Unsubscribe_Success tests successful token unsubscription
func TestManager_Unsubscribe_Success(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := Config{
		URL:                   "wss://test.com",
		DialTimeout:           10 * time.Second,
		PongTimeout:           15 * time.Second,
		PingInterval:          10 * time.Second,
		ReconnectInitialDelay: 1 * time.Second,
		ReconnectMaxDelay:     30 * time.Second,
		ReconnectBackoffMult:  2.0,
		MessageBufferSize:     100,
		Logger:                logger,
	}

	mgr := New(cfg)
	ctx := context.Background()

	// Pre-populate with tokens
	mgr.mu.Lock()
	mgr.subscribed["token1"] = true
	mgr.subscribed["token2"] = true
	mgr.subscribed["token3"] = true
	mgr.mu.Unlock()

	// Unsubscribe from token2
	err := mgr.Unsubscribe(ctx, []string{"token2"})
	if err != nil {
		// Error is expected (no real connection), but verify tracking
	}

	// Verify token2 is removed
	mgr.mu.RLock()
	if mgr.subscribed["token2"] {
		t.Error("expected token2 to be unsubscribed")
	}

	// Verify other tokens remain
	if !mgr.subscribed["token1"] || !mgr.subscribed["token3"] {
		t.Error("expected token1 and token3 to remain subscribed")
	}
	mgr.mu.RUnlock()
}

// TestManager_ConcurrentReads tests concurrent GetSnapshot-like operations
func TestManager_ConcurrentReads(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := Config{
		URL:                   "wss://test.com",
		DialTimeout:           10 * time.Second,
		PongTimeout:           15 * time.Second,
		PingInterval:          10 * time.Second,
		ReconnectInitialDelay: 1 * time.Second,
		ReconnectMaxDelay:     30 * time.Second,
		ReconnectBackoffMult:  2.0,
		MessageBufferSize:     100,
		Logger:                logger,
	}

	mgr := New(cfg)

	// Pre-populate subscriptions
	mgr.mu.Lock()
	for i := 0; i < 100; i++ {
		mgr.subscribed[fmt.Sprintf("token%d", i)] = true
	}
	mgr.mu.Unlock()

	var wg sync.WaitGroup
	numReaders := 50

	// Concurrent reads
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mgr.mu.RLock()
			count := len(mgr.subscribed)
			mgr.mu.RUnlock()

			if count != 100 {
				t.Errorf("expected 100 subscribed tokens, got %d", count)
			}
		}()
	}

	wg.Wait()
}

// TestManager_ConcurrentWrites tests concurrent subscription updates
func TestManager_ConcurrentWrites(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := Config{
		URL:                   "wss://test.com",
		DialTimeout:           10 * time.Second,
		PongTimeout:           15 * time.Second,
		PingInterval:          10 * time.Second,
		ReconnectInitialDelay: 1 * time.Second,
		ReconnectMaxDelay:     30 * time.Second,
		ReconnectBackoffMult:  2.0,
		MessageBufferSize:     100,
		Logger:                logger,
	}

	mgr := New(cfg)
	ctx := context.Background()

	var wg sync.WaitGroup
	numWriters := 10

	// Concurrent subscription updates
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			tokens := []string{
				fmt.Sprintf("token-%d-1", id),
				fmt.Sprintf("token-%d-2", id),
			}

			// Will fail due to no connection, but tests race conditions
			_ = mgr.Subscribe(ctx, tokens)
		}(i)
	}

	wg.Wait()

	// Verify no race conditions (if run with -race flag)
	mgr.mu.RLock()
	count := len(mgr.subscribed)
	mgr.mu.RUnlock()

	if count != numWriters*2 {
		t.Errorf("expected %d subscribed tokens, got %d", numWriters*2, count)
	}
}

// TestManager_ResubscribeAll_WithTokens tests resubscription after reconnect
func TestManager_ResubscribeAll_WithTokens(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := Config{
		URL:                   "wss://test.com",
		DialTimeout:           10 * time.Second,
		PongTimeout:           15 * time.Second,
		PingInterval:          10 * time.Second,
		ReconnectInitialDelay: 1 * time.Second,
		ReconnectMaxDelay:     30 * time.Second,
		ReconnectBackoffMult:  2.0,
		MessageBufferSize:     100,
		Logger:                logger,
	}

	mgr := New(cfg)
	ctx := context.Background()

	// Pre-populate with tokens
	mgr.mu.Lock()
	mgr.subscribed["token1"] = true
	mgr.subscribed["token2"] = true
	mgr.subscribed["token3"] = true
	mgr.mu.Unlock()

	// Resubscribe all (will fail due to no connection, but tests logic)
	err := mgr.resubscribeAll(ctx)
	if err != nil {
		// Expected - no real connection
	}

	// Verify subscriptions are maintained
	mgr.mu.RLock()
	if len(mgr.subscribed) != 3 {
		t.Errorf("expected 3 subscribed tokens after resubscribe, got %d", len(mgr.subscribed))
	}
	mgr.mu.RUnlock()
}

// TestManager_ConnectionState_Atomicity tests atomic connection state updates
func TestManager_ConnectionState_Atomicity(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := Config{
		URL:                   "wss://test.com",
		DialTimeout:           10 * time.Second,
		PongTimeout:           15 * time.Second,
		PingInterval:          10 * time.Second,
		ReconnectInitialDelay: 1 * time.Second,
		ReconnectMaxDelay:     30 * time.Second,
		ReconnectBackoffMult:  2.0,
		MessageBufferSize:     100,
		Logger:                logger,
	}

	mgr := New(cfg)

	var wg sync.WaitGroup
	numGoroutines := 100

	// Concurrent connection state updates
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			if id%2 == 0 {
				mgr.connected.Store(true)
			} else {
				mgr.connected.Store(false)
			}

			// Read the state
			_ = mgr.connected.Load()
		}(i)
	}

	wg.Wait()

	// Verify no race conditions (if run with -race flag)
	state := mgr.connected.Load()
	_ = state // Use the value
}

// TestManager_LastPongTime_Atomicity tests atomic pong time updates
func TestManager_LastPongTime_Atomicity(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := Config{
		URL:                   "wss://test.com",
		DialTimeout:           10 * time.Second,
		PongTimeout:           15 * time.Second,
		PingInterval:          10 * time.Second,
		ReconnectInitialDelay: 1 * time.Second,
		ReconnectMaxDelay:     30 * time.Second,
		ReconnectBackoffMult:  2.0,
		MessageBufferSize:     100,
		Logger:                logger,
	}

	mgr := New(cfg)

	var wg sync.WaitGroup
	numGoroutines := 100

	// Concurrent pong time updates
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			now := time.Now().Unix()
			mgr.lastPongTime.Store(now)

			// Read the time
			_ = mgr.lastPongTime.Load()
		}()
	}

	wg.Wait()

	// Verify no race conditions (if run with -race flag)
	pongTime := mgr.lastPongTime.Load()
	if pongTime == 0 {
		t.Error("expected non-zero pong time after updates")
	}
}

// TestManager_ParseMessage_LastTradePrice tests last trade price parsing
func TestManager_ParseMessage_LastTradePrice(t *testing.T) {
	jsonData := `{
		"event_type": "last_trade_price",
		"market": "market1",
		"asset_id": "token1",
		"timestamp": "1234567890",
		"price": "0.55"
	}`

	var message types.OrderbookMessage
	err := json.Unmarshal([]byte(jsonData), &message)
	if err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if message.EventType != "last_trade_price" {
		t.Errorf("expected event_type 'last_trade_price', got '%s'", message.EventType)
	}

	if message.AssetID != "token1" {
		t.Errorf("expected asset_id 'token1', got '%s'", message.AssetID)
	}
}

// TestManager_ParseMessage_TickSizeChange tests tick size change parsing
func TestManager_ParseMessage_TickSizeChange(t *testing.T) {
	jsonData := `{
		"event_type": "tick_size_change",
		"market": "market1",
		"asset_id": "token1",
		"timestamp": "1234567890",
		"tick_size": "0.01"
	}`

	var message types.OrderbookMessage
	err := json.Unmarshal([]byte(jsonData), &message)
	if err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if message.EventType != "tick_size_change" {
		t.Errorf("expected event_type 'tick_size_change', got '%s'", message.EventType)
	}
}
