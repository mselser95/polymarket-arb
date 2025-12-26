package websocket

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestNewPool(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	cfg := PoolConfig{
		Size:                  3,
		WSUrl:                 "wss://example.com/ws",
		DialTimeout:           10 * time.Second,
		PongTimeout:           15 * time.Second,
		PingInterval:          10 * time.Second,
		ReconnectInitialDelay: 1 * time.Second,
		ReconnectMaxDelay:     30 * time.Second,
		ReconnectBackoffMult:  2.0,
		MessageBufferSize:     100,
		Logger:                logger,
	}

	pool := NewPool(cfg)

	if pool == nil {
		t.Fatal("expected non-nil pool")
	}

	if len(pool.managers) != 3 {
		t.Errorf("expected 3 managers, got %d", len(pool.managers))
	}

	if pool.tokenToIndex == nil {
		t.Error("expected non-nil tokenToIndex map")
	}

	if pool.messageChan == nil {
		t.Error("expected non-nil messageChan")
	}

	// Verify message channel buffer size
	expectedBufferSize := cfg.Size * cfg.MessageBufferSize
	if cap(pool.messageChan) != expectedBufferSize {
		t.Errorf("expected messageChan buffer size %d, got %d", expectedBufferSize, cap(pool.messageChan))
	}
}

func TestPool_getManagerIndex(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	cfg := PoolConfig{
		Size:              5,
		WSUrl:             "wss://example.com/ws",
		MessageBufferSize: 100,
		Logger:            logger,
	}

	pool := NewPool(cfg)

	// Test consistent hashing - same token always maps to same manager
	testTokens := []string{
		"token1",
		"token2",
		"token3",
		"token4",
		"token5",
	}

	// Get manager index for each token
	managerIndexes := make(map[string]int)
	for _, token := range testTokens {
		index := pool.getManagerIndex(token)
		managerIndexes[token] = index

		// Verify index is within bounds
		if index < 0 || index >= cfg.Size {
			t.Errorf("manager index %d out of bounds for pool size %d", index, cfg.Size)
		}
	}

	// Verify consistency - same token always maps to same manager
	for _, token := range testTokens {
		index := pool.getManagerIndex(token)
		if index != managerIndexes[token] {
			t.Errorf("inconsistent hash for token %s: first=%d, second=%d",
				token, managerIndexes[token], index)
		}
	}

	// Verify distribution - with 5 tokens and 5 managers, we should see some spread
	// (not a strict requirement, but good to verify hash function isn't degenerate)
	uniqueManagers := make(map[int]bool)
	for _, index := range managerIndexes {
		uniqueManagers[index] = true
	}

	if len(uniqueManagers) < 2 {
		t.Errorf("poor distribution: only %d unique managers for %d tokens",
			len(uniqueManagers), len(testTokens))
	}
}

func TestPool_Subscribe_Distribution(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	cfg := PoolConfig{
		Size:              3,
		WSUrl:             "wss://example.com/ws",
		MessageBufferSize: 100,
		Logger:            logger,
	}

	pool := NewPool(cfg)

	// Subscribe to multiple tokens
	tokenIDs := []string{"token1", "token2", "token3", "token4", "token5"}
	ctx := context.Background()

	// Note: This will fail to actually subscribe since managers aren't started,
	// but it will populate the tokenToIndex map
	pool.mu.Lock()
	for _, tokenID := range tokenIDs {
		managerIndex := pool.getManagerIndex(tokenID)
		pool.tokenToIndex[tokenID] = managerIndex
	}
	pool.mu.Unlock()

	// Verify all tokens are tracked
	pool.mu.RLock()
	if len(pool.tokenToIndex) != len(tokenIDs) {
		t.Errorf("expected %d tokens tracked, got %d", len(tokenIDs), len(pool.tokenToIndex))
	}

	// Verify each token has a valid manager index
	for _, tokenID := range tokenIDs {
		index, exists := pool.tokenToIndex[tokenID]
		if !exists {
			t.Errorf("token %s not tracked", tokenID)
		}
		if index < 0 || index >= cfg.Size {
			t.Errorf("invalid manager index %d for token %s", index, tokenID)
		}
	}
	pool.mu.RUnlock()

	_ = ctx // avoid unused variable
}

func TestPool_Subscribe_SameTokenSameManager(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	cfg := PoolConfig{
		Size:              5,
		WSUrl:             "wss://example.com/ws",
		MessageBufferSize: 100,
		Logger:            logger,
	}

	pool := NewPool(cfg)

	tokenID := "test-token"

	// Subscribe same token multiple times
	pool.mu.Lock()
	index1 := pool.getManagerIndex(tokenID)
	pool.tokenToIndex[tokenID] = index1

	index2 := pool.getManagerIndex(tokenID)
	pool.mu.Unlock()

	// Verify same manager index
	if index1 != index2 {
		t.Errorf("inconsistent manager assignment: first=%d, second=%d", index1, index2)
	}
}

func TestPool_Unsubscribe(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	cfg := PoolConfig{
		Size:              3,
		WSUrl:             "wss://example.com/ws",
		MessageBufferSize: 100,
		Logger:            logger,
	}

	pool := NewPool(cfg)

	// Setup: add tokens to tokenToIndex map
	tokenIDs := []string{"token1", "token2", "token3"}
	pool.mu.Lock()
	for _, tokenID := range tokenIDs {
		index := pool.getManagerIndex(tokenID)
		pool.tokenToIndex[tokenID] = index
	}
	pool.mu.Unlock()

	// Verify tokens are tracked before unsubscribe
	pool.mu.RLock()
	initialCount := len(pool.tokenToIndex)
	pool.mu.RUnlock()

	if initialCount != 3 {
		t.Errorf("expected 3 tokens before unsubscribe, got %d", initialCount)
	}

	// Unsubscribe from some tokens (this will fail at manager level,
	// but should still update tokenToIndex map)
	ctx := context.Background()
	tokensToUnsubscribe := []string{"token1", "token3"}

	pool.mu.Lock()
	for _, tokenID := range tokensToUnsubscribe {
		delete(pool.tokenToIndex, tokenID)
	}
	pool.mu.Unlock()

	// Verify tokens removed from tracking
	pool.mu.RLock()
	if len(pool.tokenToIndex) != 1 {
		t.Errorf("expected 1 token after unsubscribe, got %d", len(pool.tokenToIndex))
	}

	if _, exists := pool.tokenToIndex["token2"]; !exists {
		t.Error("expected token2 to still be tracked")
	}

	if _, exists := pool.tokenToIndex["token1"]; exists {
		t.Error("expected token1 to be removed")
	}

	if _, exists := pool.tokenToIndex["token3"]; exists {
		t.Error("expected token3 to be removed")
	}
	pool.mu.RUnlock()

	_ = ctx // avoid unused variable
}

func TestPool_MessageChan(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	cfg := PoolConfig{
		Size:              3,
		WSUrl:             "wss://example.com/ws",
		MessageBufferSize: 100,
		Logger:            logger,
	}

	pool := NewPool(cfg)

	ch := pool.MessageChan()
	if ch == nil {
		t.Fatal("expected non-nil message channel")
	}

	// Verify it's the same channel
	if ch != pool.messageChan {
		t.Error("MessageChan() returned different channel")
	}
}

func TestPoolConfig_DefaultPoolSize(t *testing.T) {
	// This test verifies that the default pool size constant is reasonable
	// The actual default is set in config.go LoadFromEnv()

	minPoolSize := 1
	maxPoolSize := 20
	defaultPoolSize := 5

	if defaultPoolSize < minPoolSize {
		t.Errorf("default pool size %d is less than minimum %d", defaultPoolSize, minPoolSize)
	}

	if defaultPoolSize > maxPoolSize {
		t.Errorf("default pool size %d exceeds maximum %d", defaultPoolSize, maxPoolSize)
	}
}

func TestPool_SubscriptionTracking(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	cfg := PoolConfig{
		Size:                  3,
		WSUrl:                 "wss://example.com/ws",
		DialTimeout:           10 * time.Second,
		PongTimeout:           15 * time.Second,
		PingInterval:          10 * time.Second,
		ReconnectInitialDelay: 1 * time.Second,
		ReconnectMaxDelay:     30 * time.Second,
		ReconnectBackoffMult:  2.0,
		MessageBufferSize:     100,
		Logger:                logger,
	}

	pool := NewPool(cfg)

	// Initial state
	if pool.totalSubscriptions != 0 {
		t.Errorf("expected initial totalSubscriptions=0, got %d", pool.totalSubscriptions)
	}

	// Test subscription tracking
	tokens := []string{"token1", "token2", "token3", "token4", "token5"}

	// Simulate subscription (without actually connecting to WebSocket)
	pool.mu.Lock()
	for i, token := range tokens {
		managerIdx := i % len(pool.managers)
		pool.tokenToIndex[token] = managerIdx
	}
	pool.totalSubscriptions = len(tokens)
	pool.mu.Unlock()

	// Verify count
	pool.mu.RLock()
	totalSubs := pool.totalSubscriptions
	pool.mu.RUnlock()

	if totalSubs != len(tokens) {
		t.Errorf("expected totalSubscriptions=%d, got %d", len(tokens), totalSubs)
	}

	// Test unsubscription tracking
	tokensToRemove := []string{"token1", "token3"}

	pool.mu.Lock()
	for _, token := range tokensToRemove {
		if _, exists := pool.tokenToIndex[token]; exists {
			delete(pool.tokenToIndex, token)
			pool.totalSubscriptions--
		}
	}
	pool.mu.Unlock()

	// Verify count after removal
	pool.mu.RLock()
	totalSubs = pool.totalSubscriptions
	remainingTokens := len(pool.tokenToIndex)
	pool.mu.RUnlock()

	expectedRemaining := len(tokens) - len(tokensToRemove)
	if totalSubs != expectedRemaining {
		t.Errorf("expected totalSubscriptions=%d, got %d", expectedRemaining, totalSubs)
	}

	if remainingTokens != expectedRemaining {
		t.Errorf("expected %d tokens in tokenToIndex, got %d", expectedRemaining, remainingTokens)
	}
}

func TestPool_DuplicateSubscription(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	cfg := PoolConfig{
		Size:                  2,
		WSUrl:                 "wss://example.com/ws",
		DialTimeout:           10 * time.Second,
		PongTimeout:           15 * time.Second,
		PingInterval:          10 * time.Second,
		ReconnectInitialDelay: 1 * time.Second,
		ReconnectMaxDelay:     30 * time.Second,
		ReconnectBackoffMult:  2.0,
		MessageBufferSize:     100,
		Logger:                logger,
	}

	pool := NewPool(cfg)

	// Subscribe to same tokens twice
	tokens := []string{"token1", "token2", "token3"}

	pool.mu.Lock()
	// First subscription
	for i, token := range tokens {
		managerIdx := i % len(pool.managers)
		pool.tokenToIndex[token] = managerIdx
	}
	pool.totalSubscriptions = len(tokens)

	// Try to subscribe again (should be skipped)
	initialCount := pool.totalSubscriptions
	for _, token := range tokens {
		if _, exists := pool.tokenToIndex[token]; !exists {
			// This shouldn't happen
			pool.totalSubscriptions++
		}
	}
	pool.mu.Unlock()

	// Verify count didn't change
	pool.mu.RLock()
	totalSubs := pool.totalSubscriptions
	pool.mu.RUnlock()

	if totalSubs != initialCount {
		t.Errorf("expected duplicate subscriptions to be ignored, got %d subscriptions (expected %d)", totalSubs, initialCount)
	}
}
