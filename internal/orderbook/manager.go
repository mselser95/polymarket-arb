package orderbook

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/mselser95/polymarket-arb/pkg/types"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// Manager manages orderbook state for all subscribed tokens.
type Manager struct {
	books      map[string]*types.OrderbookSnapshot // key: token_id
	mu         sync.RWMutex
	logger     *zap.Logger
	msgChan    <-chan *types.OrderbookMessage
	updateChan chan *types.OrderbookSnapshot
	ctx        context.Context
	wg         sync.WaitGroup
}

// Config holds orderbook manager configuration.
type Config struct {
	Logger         *zap.Logger
	MessageChannel <-chan *types.OrderbookMessage
}

// New creates a new orderbook manager.
func New(cfg *Config) *Manager {
	return &Manager{
		books:      make(map[string]*types.OrderbookSnapshot),
		logger:     cfg.Logger,
		msgChan:    cfg.MessageChannel,
		updateChan: make(chan *types.OrderbookSnapshot, 100000), // Buffer for high update rate
	}
}

// Start starts the orderbook manager.
func (m *Manager) Start(ctx context.Context) error {
	m.ctx = ctx
	m.logger.Info("orderbook-manager-starting")

	m.wg.Add(1)
	go m.processMessages()

	return nil
}

// processMessages processes incoming orderbook messages.
func (m *Manager) processMessages() {
	defer m.wg.Done()

	for {
		select {
		case <-m.ctx.Done():
			m.logger.Info("orderbook-manager-stopping")
			return
		case msg, ok := <-m.msgChan:
			if !ok {
				m.logger.Info("message-channel-closed")
				return
			}

			err := m.handleMessage(msg)
			if err != nil {
				// Log empty orderbooks at debug level (common for illiquid markets)
				if err.Error() == "extract best bid: no price levels" || err.Error() == "extract best ask: no price levels" {
					m.logger.Debug("orderbook-empty",
						zap.String("event-type", msg.EventType),
						zap.String("asset-id", msg.AssetID))
				} else {
					m.logger.Warn("handle-message-error",
						zap.Error(err),
						zap.String("event-type", msg.EventType),
						zap.String("asset-id", msg.AssetID))
				}
			}
		}
	}
}

// handleMessage processes a single orderbook message.
func (m *Manager) handleMessage(msg *types.OrderbookMessage) error {
	timer := prometheus.NewTimer(UpdateProcessingDuration)
	defer timer.ObserveDuration()

	UpdatesTotal.WithLabelValues(msg.EventType).Inc()

	switch msg.EventType {
	case "book":
		return m.handleBookMessage(msg)
	case "price_change":
		return m.handlePriceChangeMessage(msg)
	default:
		// Ignore other message types (last_trade_price, etc.)
		return nil
	}
}

// handleBookMessage handles full orderbook snapshot messages.
func (m *Manager) handleBookMessage(msg *types.OrderbookMessage) error {
	// Extract best bid and ask
	bestBidPrice, bestBidSize, err := extractBestLevel(msg.Bids)
	if err != nil {
		return fmt.Errorf("extract best bid: %w", err)
	}

	bestAskPrice, bestAskSize, err := extractBestLevel(msg.Asks)
	if err != nil {
		return fmt.Errorf("extract best ask: %w", err)
	}

	snapshot := &types.OrderbookSnapshot{
		MarketID:     msg.Market,
		TokenID:      msg.AssetID,
		BestBidPrice: bestBidPrice,
		BestBidSize:  bestBidSize,
		BestAskPrice: bestAskPrice,
		BestAskSize:  bestAskSize,
		LastUpdated:  time.Now(),
	}

	// Track lock contention
	lockStart := time.Now()
	m.mu.Lock()
	LockContentionDuration.Observe(time.Since(lockStart).Seconds())

	m.books[msg.AssetID] = snapshot
	SnapshotsTracked.Set(float64(len(m.books)))
	m.mu.Unlock()

	m.logger.Debug("orderbook-snapshot-updated",
		zap.String("token-id", msg.AssetID),
		zap.Float64("best-bid", bestBidPrice),
		zap.Float64("best-ask", bestAskPrice))

	// Notify subscribers of update (non-blocking)
	select {
	case m.updateChan <- snapshot:
	default:
		// Channel full, drop update
		m.logger.Error("CRITICAL-orderbook-update-channel-full-DROPPING-DATA",
			zap.String("token-id", msg.AssetID),
			zap.Int("buffer-size", cap(m.updateChan)),
			zap.String("action", "processing too slow or increase buffer"))
		UpdatesDroppedTotal.WithLabelValues("channel_full").Inc()
	}

	return nil
}

// handlePriceChangeMessage handles incremental orderbook updates.
func (m *Manager) handlePriceChangeMessage(msg *types.OrderbookMessage) error {
	// Parse price levels OUTSIDE the lock
	var bestBidPrice, bestBidSize, bestAskPrice, bestAskSize float64
	var hasBid, hasAsk bool

	if len(msg.Bids) > 0 {
		price, size, err := extractBestLevel(msg.Bids)
		if err == nil {
			bestBidPrice = price
			bestBidSize = size
			hasBid = true
		}
	}

	if len(msg.Asks) > 0 {
		price, size, err := extractBestLevel(msg.Asks)
		if err == nil {
			bestAskPrice = price
			bestAskSize = size
			hasAsk = true
		}
	}

	// Only hold lock for map access and update
	lockStart := time.Now()
	m.mu.Lock()
	LockContentionDuration.Observe(time.Since(lockStart).Seconds())

	snapshot, exists := m.books[msg.AssetID]
	if !exists {
		// No existing snapshot, unlock and treat as full book
		m.mu.Unlock()
		return m.handleBookMessage(msg)
	}

	// Update snapshot with pre-parsed values
	if hasBid {
		snapshot.BestBidPrice = bestBidPrice
		// Only update size if it's valid (> 0)
		// price_change messages have size="0", so we preserve existing size
		if bestBidSize > 0 {
			snapshot.BestBidSize = bestBidSize
		}
	}

	if hasAsk {
		snapshot.BestAskPrice = bestAskPrice
		// Only update size if it's valid (> 0)
		// price_change messages have size="0", so we preserve existing size
		if bestAskSize > 0 {
			snapshot.BestAskSize = bestAskSize
		}
	}

	snapshot.LastUpdated = time.Now()

	// Unlock before logging and channel sends
	m.mu.Unlock()

	m.logger.Debug("orderbook-price-updated",
		zap.String("token-id", msg.AssetID),
		zap.Float64("best-bid", snapshot.BestBidPrice),
		zap.Float64("best-ask", snapshot.BestAskPrice))

	// Notify subscribers of update (non-blocking)
	snapshotCopy := *snapshot
	select {
	case m.updateChan <- &snapshotCopy:
	default:
		// Channel full, drop update
		m.logger.Error("CRITICAL-orderbook-update-channel-full-DROPPING-DATA",
			zap.String("token-id", msg.AssetID),
			zap.Int("buffer-size", cap(m.updateChan)),
			zap.String("action", "processing too slow or increase buffer"))
		UpdatesDroppedTotal.WithLabelValues("channel_full").Inc()
	}

	return nil
}

// extractBestLevel extracts the best (first) price level.
func extractBestLevel(levels []types.PriceLevel) (float64, float64, error) {
	if len(levels) == 0 {
		return 0, 0, fmt.Errorf("no price levels")
	}

	price, err := strconv.ParseFloat(levels[0].Price, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("parse price: %w", err)
	}

	size, err := strconv.ParseFloat(levels[0].Size, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("parse size: %w", err)
	}

	return price, size, nil
}

// GetSnapshot returns the orderbook snapshot for a token.
func (m *Manager) GetSnapshot(tokenID string) (*types.OrderbookSnapshot, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	snapshot, exists := m.books[tokenID]
	if !exists {
		return nil, false
	}

	// Return a copy to avoid race conditions
	snapshotCopy := *snapshot
	return &snapshotCopy, true
}

// GetAllSnapshots returns all orderbook snapshots.
func (m *Manager) GetAllSnapshots() map[string]*types.OrderbookSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy of the map
	snapshots := make(map[string]*types.OrderbookSnapshot, len(m.books))
	for tokenID, snapshot := range m.books {
		snapshotCopy := *snapshot
		snapshots[tokenID] = &snapshotCopy
	}

	return snapshots
}

// UpdateChan returns the channel for receiving orderbook updates.
func (m *Manager) UpdateChan() <-chan *types.OrderbookSnapshot {
	return m.updateChan
}

// Close gracefully closes the orderbook manager.
func (m *Manager) Close() error {
	m.logger.Info("closing-orderbook-manager")
	m.wg.Wait()
	close(m.updateChan)
	m.logger.Info("orderbook-manager-closed")
	return nil
}
