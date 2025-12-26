package websocket

import (
	"context"
	"fmt"
	"hash/crc32"
	"reflect"
	"sync"
	"time"

	"github.com/mselser95/polymarket-arb/pkg/types"
	"go.uber.org/zap"
)

// PoolConfig holds WebSocket pool configuration.
type PoolConfig struct {
	Size                  int           // Number of WebSocket connections (default: 5)
	WSUrl                 string        // WebSocket URL
	DialTimeout           time.Duration // Connection timeout
	PongTimeout           time.Duration // Pong timeout
	PingInterval          time.Duration // Ping interval
	ReconnectInitialDelay time.Duration // Initial reconnect delay
	ReconnectMaxDelay     time.Duration // Max reconnect delay
	ReconnectBackoffMult  float64       // Reconnect backoff multiplier
	MessageBufferSize     int           // Per-connection buffer size
	Logger                *zap.Logger
}

// Pool manages multiple WebSocket connections for load distribution.
type Pool struct {
	cfg                PoolConfig
	managers           []*Manager                   // Array of WebSocket managers
	tokenToIndex       map[string]int               // Map token ID to manager index
	totalSubscriptions int                          // Total subscriptions across all managers
	mu                 sync.RWMutex                 // Protects tokenToIndex and totalSubscriptions
	messageChan        chan *types.OrderbookMessage // Multiplexed messages from all managers
	ctx                context.Context
	cancel             context.CancelFunc
	wg                 sync.WaitGroup
	logger             *zap.Logger
}

// NewPool creates a new WebSocket connection pool.
func NewPool(cfg PoolConfig) *Pool {
	ctx, cancel := context.WithCancel(context.Background())

	// Create multiplexed message channel (buffer: pool size × per-connection buffer)
	messageBufferSize := cfg.Size * cfg.MessageBufferSize

	pool := &Pool{
		cfg:          cfg,
		managers:     make([]*Manager, cfg.Size),
		tokenToIndex: make(map[string]int),
		messageChan:  make(chan *types.OrderbookMessage, messageBufferSize),
		ctx:          ctx,
		cancel:       cancel,
		logger:       cfg.Logger,
	}

	// Create manager instances
	for i := range cfg.Size {
		managerCfg := Config{
			URL:                   cfg.WSUrl,
			DialTimeout:           cfg.DialTimeout,
			PongTimeout:           cfg.PongTimeout,
			PingInterval:          cfg.PingInterval,
			ReconnectInitialDelay: cfg.ReconnectInitialDelay,
			ReconnectMaxDelay:     cfg.ReconnectMaxDelay,
			ReconnectBackoffMult:  cfg.ReconnectBackoffMult,
			MessageBufferSize:     cfg.MessageBufferSize,
			Logger:                cfg.Logger.With(zap.Int("manager-id", i)),
		}

		pool.managers[i] = New(managerCfg)
	}

	return pool
}

// Start starts all WebSocket managers in the pool.
func (p *Pool) Start() error {
	p.logger.Info("websocket-pool-starting", zap.Int("pool-size", p.cfg.Size))

	// Start all managers concurrently
	errChan := make(chan error, p.cfg.Size)
	var startWg sync.WaitGroup

	for i, mgr := range p.managers {
		startWg.Add(1)
		go func(index int, manager *Manager) {
			defer startWg.Done()

			err := manager.Start()
			if err != nil {
				p.logger.Error("manager-start-failed",
					zap.Int("manager-id", index),
					zap.Error(err))
				errChan <- fmt.Errorf("manager %d start failed: %w", index, err)
			}
		}(i, mgr)
	}

	startWg.Wait()
	close(errChan)

	// Check if any managers failed to start
	var startErrors []error
	for err := range errChan {
		startErrors = append(startErrors, err)
	}

	if len(startErrors) > 0 {
		return fmt.Errorf("failed to start %d managers: %v", len(startErrors), startErrors)
	}

	// Start message multiplexer goroutine
	p.wg.Add(1)
	go p.multiplexMessages()

	// Update pool metrics
	PoolActiveConnections.Set(float64(p.cfg.Size))

	p.logger.Info("websocket-pool-started", zap.Int("active-managers", p.cfg.Size))

	return nil
}

// Subscribe distributes token subscriptions across managers using hash-based sharding.
func (p *Pool) Subscribe(ctx context.Context, tokenIDs []string) error {
	if len(tokenIDs) == 0 {
		return nil
	}

	// Group tokens by manager index using hash-based distribution
	tokensByManager := make(map[int][]string)
	newTokensCount := 0

	p.mu.Lock()
	for _, tokenID := range tokenIDs {
		// Skip if already subscribed
		if _, exists := p.tokenToIndex[tokenID]; exists {
			continue
		}

		// Calculate manager index using CRC32 hash
		managerIndex := p.getManagerIndex(tokenID)

		// Track token → manager mapping
		p.tokenToIndex[tokenID] = managerIndex

		// Group tokens for batch subscription
		tokensByManager[managerIndex] = append(tokensByManager[managerIndex], tokenID)
		newTokensCount++
	}
	p.mu.Unlock()

	// Subscribe tokens to their assigned managers
	errChan := make(chan error, len(tokensByManager))
	var subWg sync.WaitGroup

	for managerIndex, tokens := range tokensByManager {
		subWg.Add(1)
		go func(idx int, toks []string) {
			defer subWg.Done()

			err := p.managers[idx].Subscribe(ctx, toks)
			if err != nil {
				p.logger.Error("manager-subscribe-failed",
					zap.Int("manager-id", idx),
					zap.Int("token-count", len(toks)),
					zap.Error(err))
				errChan <- fmt.Errorf("manager %d subscribe failed: %w", idx, err)
			}
		}(managerIndex, tokens)
	}

	subWg.Wait()
	close(errChan)

	// Collect errors
	var subscribeErrors []error
	for err := range errChan {
		subscribeErrors = append(subscribeErrors, err)
	}

	if len(subscribeErrors) > 0 {
		return fmt.Errorf("failed to subscribe on %d managers: %v", len(subscribeErrors), subscribeErrors)
	}

	// Update total subscription count and metrics
	p.mu.Lock()
	p.totalSubscriptions += newTokensCount
	totalSubs := p.totalSubscriptions
	p.mu.Unlock()

	// Update Prometheus metric
	SubscriptionCount.Set(float64(totalSubs))

	// Update distribution metrics
	p.updateDistributionMetrics()

	p.logger.Info("pool-subscribed-to-tokens",
		zap.Int("new-tokens", newTokensCount),
		zap.Int("total-subscriptions", totalSubs),
		zap.Int("managers-used", len(tokensByManager)))

	return nil
}

// Unsubscribe removes token subscriptions from their assigned managers.
func (p *Pool) Unsubscribe(ctx context.Context, tokenIDs []string) error {
	if len(tokenIDs) == 0 {
		return nil
	}

	// Group tokens by their assigned manager
	tokensByManager := make(map[int][]string)
	removedTokensCount := 0

	p.mu.Lock()
	for _, tokenID := range tokenIDs {
		if managerIndex, exists := p.tokenToIndex[tokenID]; exists {
			tokensByManager[managerIndex] = append(tokensByManager[managerIndex], tokenID)
			delete(p.tokenToIndex, tokenID)
			removedTokensCount++
		}
	}
	p.mu.Unlock()

	// Unsubscribe tokens from their managers
	errChan := make(chan error, len(tokensByManager))
	var unsubWg sync.WaitGroup

	for managerIndex, tokens := range tokensByManager {
		unsubWg.Add(1)
		go func(idx int, toks []string) {
			defer unsubWg.Done()

			err := p.managers[idx].Unsubscribe(ctx, toks)
			if err != nil {
				p.logger.Error("manager-unsubscribe-failed",
					zap.Int("manager-id", idx),
					zap.Int("token-count", len(toks)),
					zap.Error(err))
				errChan <- fmt.Errorf("manager %d unsubscribe failed: %w", idx, err)
			}
		}(managerIndex, tokens)
	}

	unsubWg.Wait()
	close(errChan)

	// Collect errors
	var unsubscribeErrors []error
	for err := range errChan {
		unsubscribeErrors = append(unsubscribeErrors, err)
	}

	if len(unsubscribeErrors) > 0 {
		return fmt.Errorf("failed to unsubscribe on %d managers: %v", len(unsubscribeErrors), unsubscribeErrors)
	}

	// Update total subscription count and metrics
	p.mu.Lock()
	p.totalSubscriptions -= removedTokensCount
	totalSubs := p.totalSubscriptions
	p.mu.Unlock()

	// Update Prometheus metric
	SubscriptionCount.Set(float64(totalSubs))

	p.logger.Info("pool-unsubscribed-from-tokens",
		zap.Int("removed-tokens", removedTokensCount),
		zap.Int("total-subscriptions", totalSubs),
		zap.Int("managers-used", len(tokensByManager)))

	return nil
}

// MessageChan returns the multiplexed message channel receiving from all managers.
func (p *Pool) MessageChan() <-chan *types.OrderbookMessage {
	return p.messageChan
}

// Close gracefully closes all WebSocket managers in the pool.
func (p *Pool) Close() error {
	p.logger.Info("closing-websocket-pool")

	// Cancel context to stop multiplexer
	p.cancel()

	// Close all managers concurrently
	var closeWg sync.WaitGroup
	for i, mgr := range p.managers {
		closeWg.Add(1)
		go func(index int, manager *Manager) {
			defer closeWg.Done()

			err := manager.Close()
			if err != nil {
				p.logger.Error("manager-close-failed",
					zap.Int("manager-id", index),
					zap.Error(err))
			}
		}(i, mgr)
	}

	closeWg.Wait()

	// Wait for multiplexer to finish
	p.wg.Wait()

	// Close multiplexed message channel
	close(p.messageChan)

	// Update pool metrics
	PoolActiveConnections.Set(0)

	p.logger.Info("websocket-pool-closed")

	return nil
}

// multiplexMessages receives messages from all managers and forwards to pool's message channel.
func (p *Pool) multiplexMessages() {
	defer p.wg.Done()

	// Create select cases for all manager channels
	cases := make([]reflect.SelectCase, len(p.managers)+1)

	// Case 0: context cancellation
	cases[0] = reflect.SelectCase{
		Dir:  reflect.SelectRecv,
		Chan: reflect.ValueOf(p.ctx.Done()),
	}

	// Cases 1-N: manager message channels
	for i, mgr := range p.managers {
		cases[i+1] = reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(mgr.MessageChan()),
		}
	}

	p.logger.Info("message-multiplexer-started", zap.Int("manager-count", len(p.managers)))

	for {
		chosen, value, ok := reflect.Select(cases)

		if chosen == 0 {
			// Context cancelled
			p.logger.Info("message-multiplexer-stopped")
			return
		}

		if !ok {
			// Channel closed, remove from cases by replacing with a nil channel
			// reflect.Select will ignore cases with nil channels
			p.logger.Warn("manager-channel-closed", zap.Int("manager-id", chosen-1))
			cases[chosen].Chan = reflect.ValueOf(make(chan *types.OrderbookMessage))
			continue
		}

		msg, ok := value.Interface().(*types.OrderbookMessage)
		if !ok {
			p.logger.Error("invalid-message-type",
				zap.Int("manager-id", chosen-1),
				zap.String("type", fmt.Sprintf("%T", value.Interface())))
			continue
		}

		// Non-blocking send to output channel
		select {
		case p.messageChan <- msg:
			// Message sent successfully
		default:
			// Drop message if buffer full
			p.logger.Warn("dropped-message-from-multiplexer",
				zap.Int("manager-id", chosen-1),
				zap.String("asset-id", msg.AssetID))
		}
	}
}

// getManagerIndex calculates the manager index for a token ID using CRC32 hash.
// Must be called with p.mu held.
func (p *Pool) getManagerIndex(tokenID string) int {
	hash := crc32.ChecksumIEEE([]byte(tokenID))
	return int(hash) % p.cfg.Size
}

// updateDistributionMetrics updates Prometheus metrics for subscription distribution.
func (p *Pool) updateDistributionMetrics() {
	// Count subscriptions per manager
	subscriptionsPerManager := make(map[int]int)

	p.mu.RLock()
	for _, managerIndex := range p.tokenToIndex {
		subscriptionsPerManager[managerIndex]++
	}
	p.mu.RUnlock()

	// Record distribution histogram
	for _, count := range subscriptionsPerManager {
		PoolSubscriptionDistribution.Observe(float64(count))
	}
}
