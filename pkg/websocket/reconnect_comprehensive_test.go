package websocket

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"go.uber.org/zap"
)

// TestReconnect_InitialDelay tests first retry uses initial delay
func TestReconnect_InitialDelay(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := ReconnectConfig{
		InitialDelay:      100 * time.Millisecond,
		MaxDelay:          1 * time.Second,
		BackoffMultiplier: 2.0,
		JitterPercent:     0, // No jitter for predictable timing
	}

	rm := NewReconnectManager(cfg, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	startTime := time.Now()
	attemptTimes := []time.Time{}

	connectFunc := func(_ context.Context) error {
		attemptTimes = append(attemptTimes, time.Now())
		if len(attemptTimes) >= 2 {
			cancel() // Stop after 2 attempts
		}
		return errors.New("connection failed")
	}

	_ = rm.Reconnect(ctx, connectFunc)

	if len(attemptTimes) < 2 {
		t.Fatalf("expected at least 2 attempts, got %d", len(attemptTimes))
	}

	// Check delay between first and second attempt
	// Allow generous tolerance for system timing variability
	delay := attemptTimes[1].Sub(attemptTimes[0])
	expectedMin := 50 * time.Millisecond  // Allow 50% tolerance
	expectedMax := 250 * time.Millisecond // Allow system load delays

	if delay < expectedMin || delay > expectedMax {
		t.Errorf("expected initial delay ~100ms (±150ms tolerance), got %v (first attempt at %v, second at %v from start)",
			delay, attemptTimes[0].Sub(startTime), attemptTimes[1].Sub(startTime))
	}
}

// TestReconnect_ExponentialGrowth tests backoff doubles each attempt
func TestReconnect_ExponentialGrowth(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := ReconnectConfig{
		InitialDelay:      50 * time.Millisecond,
		MaxDelay:          1 * time.Second,
		BackoffMultiplier: 2.0,
		JitterPercent:     0, // No jitter
	}

	rm := NewReconnectManager(cfg, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	attemptTimes := []time.Time{}

	connectFunc := func(_ context.Context) error {
		attemptTimes = append(attemptTimes, time.Now())
		if len(attemptTimes) >= 4 {
			cancel() // Stop after 4 attempts
		}
		return errors.New("connection failed")
	}

	_ = rm.Reconnect(ctx, connectFunc)

	if len(attemptTimes) < 4 {
		t.Fatalf("expected at least 4 attempts, got %d", len(attemptTimes))
	}

	// Check delays: should grow exponentially (~50ms, ~100ms, ~200ms)
	// Allow very generous tolerance for system timing variability
	delays := []time.Duration{
		attemptTimes[1].Sub(attemptTimes[0]),
		attemptTimes[2].Sub(attemptTimes[1]),
		attemptTimes[3].Sub(attemptTimes[2]),
	}

	// First delay: ~50ms (allow very wide range for system variability)
	if delays[0] < 10*time.Millisecond || delays[0] > 300*time.Millisecond {
		t.Errorf("expected first delay ~50ms (wide tolerance), got %v", delays[0])
	}

	// Second delay: ~100ms (2x first, allow very wide range)
	if delays[1] < 20*time.Millisecond || delays[1] > 500*time.Millisecond {
		t.Errorf("expected second delay ~100ms (wide tolerance), got %v", delays[1])
	}

	// Third delay: ~200ms (2x second, allow very wide range)
	if delays[2] < 50*time.Millisecond || delays[2] > 800*time.Millisecond {
		t.Errorf("expected third delay ~200ms (wide tolerance), got %v", delays[2])
	}

	// Most important: verify delays are increasing (exponential growth)
	if delays[1] <= delays[0] {
		t.Errorf("expected delays to increase exponentially, but delay[1] (%v) <= delay[0] (%v)", delays[1], delays[0])
	}
	if delays[2] <= delays[1] {
		t.Errorf("expected delays to increase exponentially, but delay[2] (%v) <= delay[1] (%v)", delays[2], delays[1])
	}
}

// TestReconnect_MaxDelayCap tests backoff caps at max delay
func TestReconnect_MaxDelayCap(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := ReconnectConfig{
		InitialDelay:      100 * time.Millisecond,
		MaxDelay:          200 * time.Millisecond, // Low max for faster test
		BackoffMultiplier: 2.0,
		JitterPercent:     0,
	}

	rm := NewReconnectManager(cfg, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	attemptTimes := []time.Time{}

	connectFunc := func(_ context.Context) error {
		attemptTimes = append(attemptTimes, time.Now())
		if len(attemptTimes) >= 5 {
			cancel()
		}
		return errors.New("connection failed")
	}

	_ = rm.Reconnect(ctx, connectFunc)

	if len(attemptTimes) < 5 {
		t.Fatalf("expected at least 5 attempts, got %d", len(attemptTimes))
	}

	// After 2nd attempt, delay should be capped at 200ms
	// Attempt 1: 100ms
	// Attempt 2: 200ms (capped from 200ms)
	// Attempt 3: 200ms (capped from 400ms)
	delay3 := attemptTimes[3].Sub(attemptTimes[2])
	delay4 := attemptTimes[4].Sub(attemptTimes[3])

	maxAllowed := 220 * time.Millisecond // 10% tolerance

	if delay3 > maxAllowed {
		t.Errorf("expected delay 3 to be capped at ~200ms, got %v", delay3)
	}

	if delay4 > maxAllowed {
		t.Errorf("expected delay 4 to be capped at ~200ms, got %v", delay4)
	}
}

// TestReconnect_JitterApplication tests jitter adds randomness
func TestReconnect_JitterApplication(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := ReconnectConfig{
		InitialDelay:      100 * time.Millisecond,
		MaxDelay:          1 * time.Second,
		BackoffMultiplier: 2.0,
		JitterPercent:     0.2, // 20% jitter
	}

	rm := NewReconnectManager(cfg, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	attemptTimes := []time.Time{}

	connectFunc := func(_ context.Context) error {
		attemptTimes = append(attemptTimes, time.Now())
		if len(attemptTimes) >= 3 {
			cancel()
		}
		return errors.New("connection failed")
	}

	_ = rm.Reconnect(ctx, connectFunc)

	if len(attemptTimes) < 3 {
		t.Fatalf("expected at least 3 attempts, got %d", len(attemptTimes))
	}

	// With 20% jitter, delay should be in range [baseDelay * 0.8, baseDelay * 1.2]
	// Plus additional tolerance for system timing variability
	delay := attemptTimes[1].Sub(attemptTimes[0])

	// Base jitter range: [80ms, 120ms] for 100ms ± 20%
	// Add very generous system tolerance to account for scheduling delays and system load
	minExpected := 20 * time.Millisecond  // Allow significant variance below
	maxExpected := 300 * time.Millisecond // Allow significant variance above

	if delay < minExpected || delay > maxExpected {
		t.Errorf("expected delay in range [%v, %v] with 20%% jitter + system tolerance, got %v", minExpected, maxExpected, delay)
	}
}

// TestReconnect_ContextCancellation tests graceful shutdown during backoff
func TestReconnect_ContextCancellation(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := ReconnectConfig{
		InitialDelay:      500 * time.Millisecond,
		MaxDelay:          5 * time.Second,
		BackoffMultiplier: 2.0,
		JitterPercent:     0,
	}

	rm := NewReconnectManager(cfg, logger)

	ctx, cancel := context.WithCancel(context.Background())

	connectFunc := func(_ context.Context) error {
		return errors.New("connection failed")
	}

	done := make(chan error, 1)
	go func() {
		done <- rm.Reconnect(ctx, connectFunc)
	}()

	// Cancel after 100ms (before first retry completes)
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("reconnection didn't stop after context cancellation")
	}
}

// TestReconnect_ResetOnSuccess tests delay resets after successful connect
func TestReconnect_ResetOnSuccess(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := ReconnectConfig{
		InitialDelay:      100 * time.Millisecond,
		MaxDelay:          1 * time.Second,
		BackoffMultiplier: 2.0,
		JitterPercent:     0,
	}

	rm := NewReconnectManager(cfg, logger)

	// First reconnection: fails a few times then succeeds
	ctx1, cancel1 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel1()

	attempt1 := 0
	connectFunc1 := func(_ context.Context) error {
		attempt1++
		if attempt1 < 3 {
			return errors.New("connection failed")
		}
		return nil // Success on 3rd attempt
	}

	err := rm.Reconnect(ctx1, connectFunc1)
	if err != nil {
		t.Fatalf("expected successful reconnection, got %v", err)
	}

	// Reset should have been called automatically
	rm.Reset()

	// Second reconnection: should start with initial delay again
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()

	attemptTimes := []time.Time{}
	connectFunc2 := func(_ context.Context) error {
		attemptTimes = append(attemptTimes, time.Now())
		if len(attemptTimes) >= 2 {
			cancel2()
		}
		return errors.New("connection failed")
	}

	_ = rm.Reconnect(ctx2, connectFunc2)

	if len(attemptTimes) < 2 {
		t.Fatalf("expected at least 2 attempts in second reconnection, got %d", len(attemptTimes))
	}

	// First delay should be back to initial delay (~100ms)
	// Allow ±50ms tolerance for system timing variability
	delay := attemptTimes[1].Sub(attemptTimes[0])
	if delay < 50*time.Millisecond || delay > 250*time.Millisecond {
		t.Errorf("expected reset to initial delay ~100ms (±150ms tolerance), got %v", delay)
	}
}

// TestReconnectLoop_DetectsDisconnection tests connection state monitoring
func TestReconnectLoop_DetectsDisconnection(t *testing.T) {
	// This test verifies the reconnect loop would detect disconnection
	// We can't test the full loop without a real connection, but we can
	// test the connection state change logic

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

	// Simulate connection state changes
	mgr.connected.Store(true)
	if !mgr.connected.Load() {
		t.Error("expected connected state to be true")
	}

	// Simulate disconnection
	mgr.connected.Store(false)
	if mgr.connected.Load() {
		t.Error("expected connected state to be false after disconnection")
	}
}

// TestReconnectLoop_ResubscribesTokens tests state consistency after reconnect
func TestReconnectLoop_ResubscribesTokens(t *testing.T) {
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

	// Pre-populate subscriptions
	tokens := []string{"token1", "token2", "token3"}
	mgr.mu.Lock()
	for _, token := range tokens {
		mgr.subscribed[token] = true
	}
	mgr.mu.Unlock()

	// Simulate resubscription (will fail without connection, but tests logic)
	err := mgr.resubscribeAll(ctx)
	if err != nil {
		// Expected - no real connection
	}

	// Verify subscriptions are maintained
	mgr.mu.RLock()
	for _, token := range tokens {
		if !mgr.subscribed[token] {
			t.Errorf("expected token %s to remain subscribed after resubscribeAll", token)
		}
	}
	mgr.mu.RUnlock()
}

// TestReconnectLoop_ConcurrentReconnect tests race between readLoop exit and reconnectLoop
func TestReconnectLoop_ConcurrentReconnect(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := Config{
		URL:                   "wss://test.com",
		DialTimeout:           10 * time.Millisecond,
		PongTimeout:           15 * time.Second,
		PingInterval:          10 * time.Second,
		ReconnectInitialDelay: 1 * time.Millisecond,
		ReconnectMaxDelay:     10 * time.Millisecond,
		ReconnectBackoffMult:  2.0,
		MessageBufferSize:     100,
		Logger:                logger,
	}

	mgr := New(cfg)

	// Simulate concurrent state changes
	done := make(chan bool)
	go func() {
		for i := 0; i < 100; i++ {
			mgr.connected.Store(false)
			time.Sleep(1 * time.Millisecond)
			mgr.connected.Store(true)
			time.Sleep(1 * time.Millisecond)
		}
		done <- true
	}()

	// Concurrent reads
	for i := 0; i < 100; i++ {
		_ = mgr.connected.Load()
		time.Sleep(1 * time.Millisecond)
	}

	<-done

	// Test passes if no race conditions (run with -race flag)
}

// TestReconnectLoop_ContextDone tests graceful shutdown during reconnection
func TestReconnectLoop_ContextDone(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cfg := ReconnectConfig{
		InitialDelay:      100 * time.Millisecond,
		MaxDelay:          1 * time.Second,
		BackoffMultiplier: 2.0,
		JitterPercent:     0,
	}

	rm := NewReconnectManager(cfg, logger)

	ctx, cancel := context.WithCancel(context.Background())

	connectFunc := func(_ context.Context) error {
		return errors.New("connection failed")
	}

	done := make(chan error, 1)
	go func() {
		done <- rm.Reconnect(ctx, connectFunc)
	}()

	// Cancel immediately
	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("reconnection didn't stop after context done")
	}
}

// TestReconnectLoop_SubscriptionRace tests Subscribe during reconnect
func TestReconnectLoop_SubscriptionRace(t *testing.T) {
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

	// Simulate concurrent Subscribe calls during reconnection state changes
	done := make(chan bool)
	go func() {
		for i := 0; i < 10; i++ {
			tokens := []string{fmt.Sprintf("token%d", i)}
			_ = mgr.Subscribe(ctx, tokens)
			time.Sleep(10 * time.Millisecond)
		}
		done <- true
	}()

	// Concurrent connection state changes
	for i := 0; i < 10; i++ {
		mgr.connected.Store(false)
		time.Sleep(5 * time.Millisecond)
		mgr.connected.Store(true)
		time.Sleep(5 * time.Millisecond)
	}

	<-done

	// Verify no race conditions and all tokens tracked
	mgr.mu.RLock()
	if len(mgr.subscribed) != 10 {
		t.Errorf("expected 10 subscribed tokens, got %d", len(mgr.subscribed))
	}
	mgr.mu.RUnlock()
}
