package websocket

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/mselser95/polymarket-arb/pkg/types"
	"go.uber.org/zap"
)

func TestNew(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := Config{
		URL:                   "wss://ws-subscriptions-clob.polymarket.com/ws/market",
		DialTimeout:           10 * time.Second,
		PongTimeout:           15 * time.Second,
		PingInterval:          10 * time.Second,
		ReconnectInitialDelay: 1 * time.Second,
		ReconnectMaxDelay:     30 * time.Second,
		ReconnectBackoffMult:  2.0,
		MessageBufferSize:     1000,
		Logger:                logger,
	}

	mgr := New(cfg)

	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}

	if mgr.url != cfg.URL {
		t.Errorf("expected URL %q, got %q", cfg.URL, mgr.url)
	}

	if mgr.logger == nil {
		t.Error("expected non-nil logger")
	}

	if mgr.reconnectMgr == nil {
		t.Error("expected non-nil reconnect manager")
	}

	if mgr.messageChan == nil {
		t.Error("expected non-nil message channel")
	}

	if cap(mgr.messageChan) != cfg.MessageBufferSize {
		t.Errorf("expected message channel capacity %d, got %d", cfg.MessageBufferSize, cap(mgr.messageChan))
	}

	if mgr.subscribed == nil {
		t.Error("expected non-nil subscribed map")
	}
}

func TestSubscribe_EmptyTokens(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := Config{
		URL:                   "wss://ws-subscriptions-clob.polymarket.com/ws/market",
		DialTimeout:           10 * time.Second,
		PongTimeout:           15 * time.Second,
		PingInterval:          10 * time.Second,
		ReconnectInitialDelay: 1 * time.Second,
		ReconnectMaxDelay:     30 * time.Second,
		ReconnectBackoffMult:  2.0,
		MessageBufferSize:     1000,
		Logger:                logger,
	}

	mgr := New(cfg)
	ctx := context.Background()

	err := mgr.Subscribe(ctx, []string{})
	if err != nil {
		t.Errorf("expected no error for empty tokens, got %v", err)
	}
}

func TestSubscribe_DuplicateTokens(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := Config{
		URL:                   "wss://ws-subscriptions-clob.polymarket.com/ws/market",
		DialTimeout:           10 * time.Second,
		PongTimeout:           15 * time.Second,
		PingInterval:          10 * time.Second,
		ReconnectInitialDelay: 1 * time.Second,
		ReconnectMaxDelay:     30 * time.Second,
		ReconnectBackoffMult:  2.0,
		MessageBufferSize:     1000,
		Logger:                logger,
	}

	mgr := New(cfg)

	// Manually mark tokens as subscribed
	mgr.mu.Lock()
	mgr.subscribed["token1"] = true
	mgr.subscribed["token2"] = true
	mgr.mu.Unlock()

	ctx := context.Background()

	// Try to subscribe to already subscribed tokens
	err := mgr.Subscribe(ctx, []string{"token1", "token2"})
	if err != nil {
		t.Errorf("expected no error for duplicate tokens, got %v", err)
	}

	// Verify no change in subscription count
	mgr.mu.RLock()
	count := len(mgr.subscribed)
	mgr.mu.RUnlock()

	if count != 2 {
		t.Errorf("expected 2 subscribed tokens, got %d", count)
	}
}

func TestMessageChan(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := Config{
		URL:                   "wss://ws-subscriptions-clob.polymarket.com/ws/market",
		DialTimeout:           10 * time.Second,
		PongTimeout:           15 * time.Second,
		PingInterval:          10 * time.Second,
		ReconnectInitialDelay: 1 * time.Second,
		ReconnectMaxDelay:     30 * time.Second,
		ReconnectBackoffMult:  2.0,
		MessageBufferSize:     1000,
		Logger:                logger,
	}

	mgr := New(cfg)

	msgChan := mgr.MessageChan()
	if msgChan == nil {
		t.Fatal("expected non-nil message channel")
	}

	// Verify it's the same channel
	if msgChan != mgr.messageChan {
		t.Error("MessageChan() returned different channel")
	}
}

func TestManager_ConcurrentSubscribe(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := Config{
		URL:                   "wss://ws-subscriptions-clob.polymarket.com/ws/market",
		DialTimeout:           10 * time.Second,
		PongTimeout:           15 * time.Second,
		PingInterval:          10 * time.Second,
		ReconnectInitialDelay: 1 * time.Second,
		ReconnectMaxDelay:     30 * time.Second,
		ReconnectBackoffMult:  2.0,
		MessageBufferSize:     1000,
		Logger:                logger,
	}

	mgr := New(cfg)

	// Test concurrent subscription tracking with pre-subscribed tokens
	// We're testing for race conditions, not actual network operations
	ctx := context.Background()
	var wg sync.WaitGroup

	// Pre-populate with tokens so Subscribe() returns early without network I/O
	mgr.mu.Lock()
	for i := 0; i < 10; i++ {
		mgr.subscribed["token-"+string(rune('A'+i))] = true
	}
	mgr.mu.Unlock()

	// Simulate concurrent subscribe calls to already-subscribed tokens
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			// These will return early since tokens are already subscribed
			_ = mgr.Subscribe(ctx, []string{"token-" + string(rune('A'+idx))})
		}(i)
	}

	wg.Wait()

	// Verify no race conditions (if test runs with -race flag)
	mgr.mu.RLock()
	count := len(mgr.subscribed)
	mgr.mu.RUnlock()

	if count != 10 {
		t.Errorf("expected 10 subscribed tokens, got %d", count)
	}
}

func TestReconnectManager_Config(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	cfg := ReconnectConfig{
		InitialDelay:      1 * time.Second,
		MaxDelay:          30 * time.Second,
		BackoffMultiplier: 2.0,
		JitterPercent:     0.2,
	}

	rm := NewReconnectManager(cfg, logger)

	if rm == nil {
		t.Fatal("expected non-nil reconnect manager")
	}

	if rm.config.InitialDelay != cfg.InitialDelay {
		t.Errorf("expected InitialDelay %v, got %v", cfg.InitialDelay, rm.config.InitialDelay)
	}

	if rm.config.MaxDelay != cfg.MaxDelay {
		t.Errorf("expected MaxDelay %v, got %v", cfg.MaxDelay, rm.config.MaxDelay)
	}

	if rm.config.BackoffMultiplier != cfg.BackoffMultiplier {
		t.Errorf("expected BackoffMultiplier %v, got %v", cfg.BackoffMultiplier, rm.config.BackoffMultiplier)
	}
}

func TestReconnectManager_Backoff(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	cfg := ReconnectConfig{
		InitialDelay:      1 * time.Second,
		MaxDelay:          8 * time.Second,
		BackoffMultiplier: 2.0,
		JitterPercent:     0,
	}

	rm := NewReconnectManager(cfg, logger)

	// Test backoff progression with a mock connect function that always fails
	attemptCount := 0
	maxAttempts := 4

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	connectFunc := func(_ context.Context) error {
		attemptCount++
		if attemptCount >= maxAttempts {
			cancel() // Stop after maxAttempts
		}
		return context.Canceled // Simulate connection failure
	}

	// Start reconnection in a goroutine
	done := make(chan error, 1)
	go func() {
		done <- rm.Reconnect(ctx, connectFunc)
	}()

	// Wait for completion or timeout
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("reconnection test timed out")
	}

	if attemptCount < maxAttempts {
		t.Errorf("expected at least %d attempts, got %d", maxAttempts, attemptCount)
	}
}

func TestReconnectManager_Reset(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	cfg := ReconnectConfig{
		InitialDelay:      1 * time.Second,
		MaxDelay:          30 * time.Second,
		BackoffMultiplier: 2.0,
		JitterPercent:     0,
	}

	rm := NewReconnectManager(cfg, logger)

	// Test that Reset doesn't panic
	rm.Reset()

	// Test successful reconnection after reset
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	connected := false
	connectFunc := func(_ context.Context) error {
		connected = true
		return nil // Successful connection
	}

	err := rm.Reconnect(ctx, connectFunc)
	if err != nil {
		t.Errorf("expected successful reconnection, got error: %v", err)
	}

	if !connected {
		t.Error("expected connectFunc to be called")
	}
}

func TestManager_ConnectionState(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := Config{
		URL:                   "wss://ws-subscriptions-clob.polymarket.com/ws/market",
		DialTimeout:           10 * time.Second,
		PongTimeout:           15 * time.Second,
		PingInterval:          10 * time.Second,
		ReconnectInitialDelay: 1 * time.Second,
		ReconnectMaxDelay:     30 * time.Second,
		ReconnectBackoffMult:  2.0,
		MessageBufferSize:     1000,
		Logger:                logger,
	}

	mgr := New(cfg)

	// Initially not connected
	if mgr.connected.Load() {
		t.Error("expected manager to not be connected initially")
	}

	// Simulate connection
	mgr.connected.Store(true)

	if !mgr.connected.Load() {
		t.Error("expected manager to be connected after setting state")
	}

	// Simulate disconnection
	mgr.connected.Store(false)

	if mgr.connected.Load() {
		t.Error("expected manager to be disconnected after clearing state")
	}
}

func TestManager_PongTime(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := Config{
		URL:                   "wss://ws-subscriptions-clob.polymarket.com/ws/market",
		DialTimeout:           10 * time.Second,
		PongTimeout:           15 * time.Second,
		PingInterval:          10 * time.Second,
		ReconnectInitialDelay: 1 * time.Second,
		ReconnectMaxDelay:     30 * time.Second,
		ReconnectBackoffMult:  2.0,
		MessageBufferSize:     1000,
		Logger:                logger,
	}

	mgr := New(cfg)

	// Initially zero
	if mgr.lastPongTime.Load() != 0 {
		t.Error("expected lastPongTime to be zero initially")
	}

	// Set pong time
	now := time.Now().Unix()
	mgr.lastPongTime.Store(now)

	if mgr.lastPongTime.Load() != now {
		t.Errorf("expected lastPongTime to be %d, got %d", now, mgr.lastPongTime.Load())
	}
}

func TestManager_SubscribedTracking(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := Config{
		URL:                   "wss://ws-subscriptions-clob.polymarket.com/ws/market",
		DialTimeout:           10 * time.Second,
		PongTimeout:           15 * time.Second,
		PingInterval:          10 * time.Second,
		ReconnectInitialDelay: 1 * time.Second,
		ReconnectMaxDelay:     30 * time.Second,
		ReconnectBackoffMult:  2.0,
		MessageBufferSize:     1000,
		Logger:                logger,
	}

	mgr := New(cfg)

	tokens := []string{"token1", "token2", "token3"}

	// Add tokens to subscribed map
	mgr.mu.Lock()
	for _, token := range tokens {
		mgr.subscribed[token] = true
	}
	mgr.mu.Unlock()

	// Verify tracking
	mgr.mu.RLock()
	for _, token := range tokens {
		if !mgr.subscribed[token] {
			t.Errorf("expected token %s to be tracked", token)
		}
	}

	if len(mgr.subscribed) != len(tokens) {
		t.Errorf("expected %d subscribed tokens, got %d", len(tokens), len(mgr.subscribed))
	}
	mgr.mu.RUnlock()
}

func TestManager_Close(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := Config{
		URL:                   "wss://ws-subscriptions-clob.polymarket.com/ws/market",
		DialTimeout:           10 * time.Second,
		PongTimeout:           15 * time.Second,
		PingInterval:          10 * time.Second,
		ReconnectInitialDelay: 1 * time.Second,
		ReconnectMaxDelay:     30 * time.Second,
		ReconnectBackoffMult:  2.0,
		MessageBufferSize:     1000,
		Logger:                logger,
	}

	mgr := New(cfg)

	// Close should not panic even without Start()
	err := mgr.Close()
	if err != nil {
		t.Errorf("expected no error on close, got %v", err)
	}

	// Verify message channel is closed
	_, ok := <-mgr.messageChan
	if ok {
		t.Error("expected message channel to be closed")
	}
}

func TestResubscribeAll_EmptySubscriptions(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := Config{
		URL:                   "wss://ws-subscriptions-clob.polymarket.com/ws/market",
		DialTimeout:           10 * time.Second,
		PongTimeout:           15 * time.Second,
		PingInterval:          10 * time.Second,
		ReconnectInitialDelay: 1 * time.Second,
		ReconnectMaxDelay:     30 * time.Second,
		ReconnectBackoffMult:  2.0,
		MessageBufferSize:     1000,
		Logger:                logger,
	}

	mgr := New(cfg)
	ctx := context.Background()

	// Should not error with empty subscriptions
	err := mgr.resubscribeAll(ctx)
	if err != nil {
		t.Errorf("expected no error with empty subscriptions, got %v", err)
	}
}

func TestManager_MessageProcessing(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := Config{
		URL:                   "wss://ws-subscriptions-clob.polymarket.com/ws/market",
		DialTimeout:           10 * time.Second,
		PongTimeout:           15 * time.Second,
		PingInterval:          10 * time.Second,
		ReconnectInitialDelay: 1 * time.Second,
		ReconnectMaxDelay:     30 * time.Second,
		ReconnectBackoffMult:  2.0,
		MessageBufferSize:     10,
		Logger:                logger,
	}

	mgr := New(cfg)

	// Test message channel capacity
	for i := 0; i < 10; i++ {
		msg := &types.OrderbookMessage{
			EventType: "test",
			AssetID:   "test-asset",
		}

		select {
		case mgr.messageChan <- msg:
			// Success
		default:
			t.Errorf("message channel full at %d messages (capacity %d)", i, cap(mgr.messageChan))
		}
	}

	// 11th message should not block (using select with default)
	msg := &types.OrderbookMessage{
		EventType: "test",
		AssetID:   "test-asset",
	}

	select {
	case mgr.messageChan <- msg:
		t.Error("expected message channel to be full")
	default:
		// Expected - channel is full
	}
}
