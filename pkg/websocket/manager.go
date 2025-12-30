package websocket

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	json "github.com/goccy/go-json"
	"github.com/gorilla/websocket"
	"github.com/mselser95/polymarket-arb/pkg/types"
	"go.uber.org/zap"
)

// MetadataUpdater is an interface for updating market metadata cache
type MetadataUpdater interface {
	UpdateTickSize(tokenID string, newTickSize float64)
}

// Manager manages a single WebSocket connection to Polymarket.
type Manager struct {
	url             string
	conn            *websocket.Conn
	logger          *zap.Logger
	reconnectMgr    *ReconnectManager
	config          Config
	messageChan     chan *types.OrderbookMessage
	metadataUpdater MetadataUpdater // optional: for updating metadata cache
	ctx             context.Context
	cancel          context.CancelFunc
	wg              sync.WaitGroup
	mu              sync.RWMutex
	subscribed      map[string]bool // tracks subscribed token IDs
	connected       atomic.Bool
	lastPongTime    atomic.Int64
	connectionStart atomic.Int64 // Unix timestamp of connection start
}

// Config holds WebSocket manager configuration.
type Config struct {
	URL                   string
	DialTimeout           time.Duration
	PongTimeout           time.Duration
	PingInterval          time.Duration
	ReconnectInitialDelay time.Duration
	ReconnectMaxDelay     time.Duration
	ReconnectBackoffMult  float64
	MessageBufferSize     int
	Logger                *zap.Logger
	MetadataUpdater       MetadataUpdater // optional: for updating metadata cache on tick_size_change
}

// New creates a new WebSocket manager.
func New(cfg Config) *Manager {
	ctx, cancel := context.WithCancel(context.Background())

	reconnectCfg := ReconnectConfig{
		InitialDelay:      cfg.ReconnectInitialDelay,
		MaxDelay:          cfg.ReconnectMaxDelay,
		BackoffMultiplier: cfg.ReconnectBackoffMult,
		JitterPercent:     0.2,
	}

	return &Manager{
		url:             cfg.URL,
		logger:          cfg.Logger,
		reconnectMgr:    NewReconnectManager(reconnectCfg, cfg.Logger),
		config:          cfg,
		messageChan:     make(chan *types.OrderbookMessage, cfg.MessageBufferSize),
		metadataUpdater: cfg.MetadataUpdater,
		ctx:             ctx,
		cancel:          cancel,
		subscribed:      make(map[string]bool),
	}
}

// Start starts the WebSocket manager.
func (m *Manager) Start() error {
	m.logger.Info("websocket-manager-starting", zap.String("url", m.url))

	// Initial connection
	err := m.connect(m.ctx)
	if err != nil {
		return fmt.Errorf("initial connection: %w", err)
	}

	// Start goroutines
	m.wg.Add(3)
	go m.readLoop()
	go m.pingLoop()
	go m.reconnectLoop()

	return nil
}

// connect establishes a WebSocket connection.
func (m *Manager) connect(ctx context.Context) error {
	dialer := websocket.Dialer{
		HandshakeTimeout: m.config.DialTimeout,
		// Increase buffer sizes from default 4KB to 1MB to handle large orderbook messages
		// With 20 connections and high-volume markets, messages can be > 2KB
		ReadBufferSize:  1024 * 1024, // 1MB read buffer
		WriteBufferSize: 1024 * 1024, // 1MB write buffer
	}

	m.logger.Info("connecting-to-websocket", zap.String("url", m.url))

	conn, _, err := dialer.DialContext(ctx, m.url, nil)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}

	// Set up pong handler
	conn.SetPongHandler(func(string) error {
		m.lastPongTime.Store(time.Now().Unix())
		return nil
	})

	// Set read limit to 10MB to handle large messages with many price changes
	// Default is 512KB which can cause truncation with high-volume markets
	conn.SetReadLimit(10 * 1024 * 1024)

	m.mu.Lock()
	m.conn = conn
	m.mu.Unlock()

	now := time.Now()
	m.connected.Store(true)
	m.lastPongTime.Store(now.Unix())
	m.connectionStart.Store(now.Unix())
	ActiveConnections.Set(1)

	m.logger.Info("websocket-connected")

	return nil
}

// Subscribe subscribes to a list of token IDs.
func (m *Manager) Subscribe(ctx context.Context, tokenIDs []string) error {
	if len(tokenIDs) == 0 {
		return nil
	}

	// Build subscription message and update state under lock
	m.mu.Lock()

	// Filter out already subscribed tokens
	newTokens := make([]string, 0, len(tokenIDs))
	for _, tokenID := range tokenIDs {
		if !m.subscribed[tokenID] {
			newTokens = append(newTokens, tokenID)
			m.subscribed[tokenID] = true
		}
	}

	if len(newTokens) == 0 {
		m.mu.Unlock()
		m.logger.Debug("all-tokens-already-subscribed")
		return nil
	}

	// Determine message type based on connection state
	var subscribeMsg map[string]interface{}
	isInitialSubscription := len(m.subscribed) == len(newTokens)

	if isInitialSubscription {
		// Initial subscription
		subscribeMsg = map[string]interface{}{
			"assets_ids": newTokens,
			"type":       "market",
		}
	} else {
		// Dynamic subscription (adding new markets)
		subscribeMsg = map[string]interface{}{
			"assets_ids": newTokens,
			"operation":  "subscribe",
		}
	}

	totalSubscribed := len(m.subscribed)
	m.mu.Unlock()

	// Check if connection exists before attempting network I/O
	if m.conn == nil {
		// Keep subscription tracked but don't attempt network I/O
		// This allows subscription state to be maintained across reconnects
		SubscriptionCount.Set(float64(totalSubscribed))
		m.logger.Debug("subscribe-no-connection-tracked-for-later",
			zap.Int("token-count", len(newTokens)))
		return fmt.Errorf("no active connection (tokens tracked for later)")
	}

	// Network I/O WITHOUT holding the lock
	err := m.conn.WriteJSON(subscribeMsg)
	if err != nil {
		// Rollback subscription state on failure
		m.mu.Lock()
		for _, tokenID := range newTokens {
			delete(m.subscribed, tokenID)
		}
		totalSubscribed = len(m.subscribed)
		m.mu.Unlock()

		SubscriptionCount.Set(float64(totalSubscribed))
		return fmt.Errorf("write subscribe message: %w", err)
	}

	SubscriptionCount.Set(float64(totalSubscribed))

	m.logger.Info("subscribed-to-tokens",
		zap.Int("new-count", len(newTokens)),
		zap.Int("total-count", totalSubscribed))

	return nil
}

// Unsubscribe unsubscribes from a list of token IDs.
func (m *Manager) Unsubscribe(ctx context.Context, tokenIDs []string) (err error) {
	if len(tokenIDs) == 0 {
		return nil
	}

	m.mu.Lock()

	// Filter to only tokens that are currently subscribed
	tokensToUnsubscribe := make([]string, 0, len(tokenIDs))
	for _, tokenID := range tokenIDs {
		if m.subscribed[tokenID] {
			tokensToUnsubscribe = append(tokensToUnsubscribe, tokenID)
			delete(m.subscribed, tokenID)
		}
	}

	if len(tokensToUnsubscribe) == 0 {
		m.mu.Unlock()
		m.logger.Debug("no-tokens-to-unsubscribe")
		return nil
	}

	// Build unsubscribe message
	unsubscribeMsg := map[string]interface{}{
		"assets_ids": tokensToUnsubscribe,
		"operation":  "unsubscribe",
	}

	totalSubscribed := len(m.subscribed)
	m.mu.Unlock()

	// Check if connection exists before attempting network I/O
	if m.conn == nil {
		// Keep unsubscription tracked but don't attempt network I/O
		SubscriptionCount.Set(float64(totalSubscribed))
		m.logger.Debug("unsubscribe-no-connection-state-updated",
			zap.Int("token-count", len(tokensToUnsubscribe)))
		return fmt.Errorf("no active connection (tokens untracked)")
	}

	// Send unsubscribe message (without holding lock)
	err = m.conn.WriteJSON(unsubscribeMsg)
	if err != nil {
		// Rollback: re-add tokens to subscribed map
		m.mu.Lock()
		for _, tokenID := range tokensToUnsubscribe {
			m.subscribed[tokenID] = true
		}
		totalSubscribed = len(m.subscribed)
		m.mu.Unlock()

		SubscriptionCount.Set(float64(totalSubscribed))
		return fmt.Errorf("write unsubscribe message: %w", err)
	}

	SubscriptionCount.Set(float64(totalSubscribed))
	UnsubscriptionsTotal.Inc()

	m.logger.Info("unsubscribed-from-tokens",
		zap.Int("count", len(tokensToUnsubscribe)),
		zap.Int("remaining-count", totalSubscribed))

	return nil
}

// readLoop reads messages from the WebSocket.
func (m *Manager) readLoop() {
	defer m.wg.Done()

	for {
		select {
		case <-m.ctx.Done():
			return
		default:
		}

		m.mu.RLock()
		conn := m.conn
		m.mu.RUnlock()

		if conn == nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		_, message, err := conn.ReadMessage()
		if err != nil {
			m.logger.Warn("read-error", zap.Error(err))

			// Observe connection duration before marking as disconnected
			startTime := m.connectionStart.Load()
			if startTime > 0 {
				duration := time.Since(time.Unix(startTime, 0)).Seconds()
				ConnectionDuration.Observe(duration)
			}

			m.connected.Store(false)
			ActiveConnections.Set(0)
			return
		}

		// Parse message - Try different formats based on Polymarket CLOB API
		// The API sends messages in multiple formats:
		// - Array of book snapshots: [{...}, {...}] (initial subscription)
		// - Single book snapshot: {...} (individual updates)
		// - price_change messages (incremental updates)
		// - last_trade_price messages (trade notifications)

		// Try #1: Array of book messages (initial snapshots)
		var obMsgs []types.OrderbookMessage
		bookErr := json.Unmarshal(message, &obMsgs)
		if bookErr == nil && len(obMsgs) > 0 {
			// Successfully parsed as book message array
			for i := range obMsgs {
				start := time.Now()
				obMsg := &obMsgs[i]

				MessagesReceivedTotal.WithLabelValues(obMsg.EventType).Inc()

				// Send to channel (non-blocking)
				select {
				case m.messageChan <- obMsg:
					// Warn if channel is near capacity (90%)
					buffered := len(m.messageChan)
					capacity := cap(m.messageChan)
					if buffered > capacity*9/10 {
						m.logger.Warn("websocket-message-channel-near-full",
							zap.Int("buffered", buffered),
							zap.Int("capacity", capacity),
							zap.Float64("utilization", float64(buffered)/float64(capacity)*100))
					}
				default:
					m.logger.Error("CRITICAL-message-channel-full-DROPPING-DATA",
						zap.String("event-type", obMsg.EventType),
						zap.Int("buffer-size", cap(m.messageChan)),
						zap.String("action", "increase WS_MESSAGE_BUFFER_SIZE"))
					MessagesDroppedTotal.WithLabelValues("channel_full").Inc()
				}

				// Observe message processing latency
				MessageLatencySeconds.Observe(time.Since(start).Seconds())
			}
			continue
		}

		// Try #1b: Single book message (not in array)
		var singleObMsg types.OrderbookMessage
		singleBookErr := json.Unmarshal(message, &singleObMsg)
		if singleBookErr == nil && singleObMsg.EventType == "book" {
			// Successfully parsed as single book message
			start := time.Now()

			MessagesReceivedTotal.WithLabelValues(singleObMsg.EventType).Inc()

			// Send to channel (non-blocking)
			select {
			case m.messageChan <- &singleObMsg:
				// Warn if channel is near capacity (90%)
				buffered := len(m.messageChan)
				capacity := cap(m.messageChan)
				if buffered > capacity*9/10 {
					m.logger.Warn("websocket-message-channel-near-full",
						zap.Int("buffered", buffered),
						zap.Int("capacity", capacity),
						zap.Float64("utilization", float64(buffered)/float64(capacity)*100))
				}
			default:
				m.logger.Error("CRITICAL-message-channel-full-DROPPING-DATA",
					zap.String("event-type", singleObMsg.EventType),
					zap.Int("buffer-size", cap(m.messageChan)),
					zap.String("action", "increase WS_MESSAGE_BUFFER_SIZE"))
				MessagesDroppedTotal.WithLabelValues("channel_full").Inc()
			}

			// Observe message processing latency
			MessageLatencySeconds.Observe(time.Since(start).Seconds())
			continue
		}

		// Try #2: PriceChangeMessage (incremental updates)
		var priceChangeMsg types.PriceChangeMessage
		priceErr := json.Unmarshal(message, &priceChangeMsg)
		if priceErr == nil && priceChangeMsg.EventType == "price_change" {
			// Successfully parsed as price_change message
			// Convert each PriceChange to OrderbookMessage format
			for _, pc := range priceChangeMsg.PriceChanges {
				start := time.Now()

				// Convert to OrderbookMessage for compatibility with existing orderbook manager
				// NOTE: price_change messages from CLOB API only include best_bid/best_ask prices,
				// not sizes. We set size to "0" here, which will overwrite existing size in the snapshot.
				// This is acceptable since we prioritize price updates over size accuracy.
				// Initial book snapshots provide accurate sizes.
				obMsg := &types.OrderbookMessage{
					EventType: "price_change",
					AssetID:   pc.AssetID,
					Market:    priceChangeMsg.Market,
					Timestamp: priceChangeMsg.Timestamp,
					Bids:      []types.PriceLevel{{Price: pc.BestBid, Size: "0"}},
					Asks:      []types.PriceLevel{{Price: pc.BestAsk, Size: "0"}},
				}

				MessagesReceivedTotal.WithLabelValues("price_change").Inc()

				m.logger.Debug("price-change-message-converted",
					zap.String("asset-id", pc.AssetID),
					zap.String("best-bid", pc.BestBid),
					zap.String("best-ask", pc.BestAsk))

				// Send to channel (non-blocking)
				select {
				case m.messageChan <- obMsg:
					// Warn if channel is near capacity (90%)
					buffered := len(m.messageChan)
					capacity := cap(m.messageChan)
					if buffered > capacity*9/10 {
						m.logger.Warn("websocket-message-channel-near-full",
							zap.Int("buffered", buffered),
							zap.Int("capacity", capacity),
							zap.Float64("utilization", float64(buffered)/float64(capacity)*100))
					}
				default:
					m.logger.Error("CRITICAL-message-channel-full-DROPPING-DATA",
						zap.String("event-type", "price_change"),
						zap.Int("buffer-size", cap(m.messageChan)),
						zap.String("action", "increase WS_MESSAGE_BUFFER_SIZE"))
					MessagesDroppedTotal.WithLabelValues("channel_full").Inc()
				}

				// Observe message processing latency
				MessageLatencySeconds.Observe(time.Since(start).Seconds())
			}
			continue
		}

		// Try #3: LastTradePriceMessage (trade execution notifications)
		var tradeMsg types.LastTradePriceMessage
		tradeErr := json.Unmarshal(message, &tradeMsg)
		if tradeErr == nil && tradeMsg.EventType == "last_trade_price" {
			// Successfully parsed as last_trade_price message
			// These are informational only - we don't use them for arbitrage detection
			MessagesReceivedTotal.WithLabelValues("last_trade_price").Inc()

			m.logger.Debug("last-trade-price-received",
				zap.String("market", tradeMsg.Market),
				zap.String("asset-id", tradeMsg.AssetID),
				zap.String("price", tradeMsg.Price),
				zap.String("size", tradeMsg.Size),
				zap.String("side", tradeMsg.Side))
			continue
		}

		// Try #4: TickSizeChangeMessage (tick size updates)
		var tickSizeMsg types.TickSizeChangeMessage
		tickSizeErr := json.Unmarshal(message, &tickSizeMsg)
		if tickSizeErr == nil && tickSizeMsg.EventType == "tick_size_change" {
			// Successfully parsed as tick_size_change message
			MessagesReceivedTotal.WithLabelValues("tick_size_change").Inc()

			// Update metadata cache if updater is available
			if m.metadataUpdater != nil {
				// Parse new tick size
				var newTickSize float64
				_, scanErr := fmt.Sscanf(tickSizeMsg.NewTickSize, "%f", &newTickSize)
				if scanErr == nil {
					m.metadataUpdater.UpdateTickSize(tickSizeMsg.AssetID, newTickSize)
					m.logger.Info("tick-size-change-received-and-updated",
						zap.String("market", tickSizeMsg.Market),
						zap.String("asset-id", tickSizeMsg.AssetID),
						zap.String("old-tick-size", tickSizeMsg.OldTickSize),
						zap.String("new-tick-size", tickSizeMsg.NewTickSize),
						zap.String("action", "metadata cache updated"))
				} else {
					m.logger.Warn("tick-size-change-parse-error",
						zap.String("asset-id", tickSizeMsg.AssetID),
						zap.String("new-tick-size", tickSizeMsg.NewTickSize),
						zap.Error(scanErr))
				}
			} else {
				m.logger.Info("tick-size-change-received",
					zap.String("market", tickSizeMsg.Market),
					zap.String("asset-id", tickSizeMsg.AssetID),
					zap.String("old-tick-size", tickSizeMsg.OldTickSize),
					zap.String("new-tick-size", tickSizeMsg.NewTickSize),
					zap.String("action", "no metadata updater configured"))
			}
			continue
		}

		// Try #5: Identify other message types for better logging
		messageStr := string(message)

		// Check if it's a heartbeat/keepalive (empty array or minimal content)
		if messageStr == "[]" || messageStr == "" || len(message) < 10 {
			m.logger.Debug("websocket-heartbeat-received",
				zap.Int("bytes", len(message)))
			continue
		}

		// Check if it's a subscription confirmation or other control message
		var controlMsg map[string]interface{}
		if json.Unmarshal(message, &controlMsg) == nil {
			if msgType, ok := controlMsg["type"].(string); ok {
				m.logger.Debug("websocket-control-message",
					zap.String("type", msgType),
					zap.Int("bytes", len(message)))
				continue
			}
		}

		// Unknown message format - log FULL message for debugging
		m.logger.Warn("websocket-unparseable-message",
			zap.NamedError("book-array-parse-error", bookErr),
			zap.NamedError("book-single-parse-error", singleBookErr),
			zap.NamedError("price-change-parse-error", priceErr),
			zap.NamedError("trade-parse-error", tradeErr),
			zap.NamedError("tick-size-change-parse-error", tickSizeErr),
			zap.Int("bytes", len(message)),
			zap.String("full-message", messageStr))
	}
}

// pingLoop sends periodic PING messages.
func (m *Manager) pingLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.config.PingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			if !m.connected.Load() {
				continue
			}

			m.mu.RLock()
			conn := m.conn
			m.mu.RUnlock()

			if conn == nil {
				continue
			}

			err := conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(time.Second))
			if err != nil {
				m.logger.Warn("ping-error", zap.Error(err))
			}
		}
	}
}

// reconnectLoop handles reconnection when connection drops.
func (m *Manager) reconnectLoop() {
	defer m.wg.Done()

	for {
		select {
		case <-m.ctx.Done():
			return
		default:
		}

		// Wait for disconnection
		if m.connected.Load() {
			time.Sleep(time.Second)
			continue
		}

		m.logger.Warn("connection-lost-initiating-reconnect")

		// Attempt reconnection
		err := m.reconnectMgr.Reconnect(m.ctx, m.connect)
		if err != nil {
			if err == context.Canceled {
				return
			}
			m.logger.Error("reconnection-failed", zap.Error(err))
			continue
		}

		// Resubscribe to all markets
		err = m.resubscribeAll(m.ctx)
		if err != nil {
			m.logger.Error("resubscribe-failed", zap.Error(err))
			m.connected.Store(false)
			continue
		}

		m.logger.Info("reconnection-complete-restarting-read-loop")

		// Restart read loop
		m.wg.Add(1)
		go m.readLoop()
	}
}

// resubscribeAll resubscribes to all previously subscribed tokens.
func (m *Manager) resubscribeAll(ctx context.Context) error {
	m.mu.RLock()
	tokenIDs := make([]string, 0, len(m.subscribed))
	for tokenID := range m.subscribed {
		tokenIDs = append(tokenIDs, tokenID)
	}
	m.mu.RUnlock()

	if len(tokenIDs) == 0 {
		return nil
	}

	// Initial subscribe message after reconnect
	subscribeMsg := map[string]interface{}{
		"assets_ids": tokenIDs,
		"type":       "market",
	}

	m.mu.RLock()
	if m.conn == nil {
		m.mu.RUnlock()
		return fmt.Errorf("no active connection for resubscribe")
	}
	err := m.conn.WriteJSON(subscribeMsg)
	m.mu.RUnlock()

	if err != nil {
		return fmt.Errorf("write resubscribe message: %w", err)
	}

	m.logger.Info("resubscribed-to-all-markets", zap.Int("count", len(tokenIDs)))

	return nil
}

// MessageChan returns the channel for receiving orderbook messages.
func (m *Manager) MessageChan() <-chan *types.OrderbookMessage {
	return m.messageChan
}

// Close gracefully closes the WebSocket manager.
func (m *Manager) Close() error {
	m.logger.Info("closing-websocket-manager")

	m.cancel()

	m.mu.RLock()
	if m.conn != nil {
		m.conn.Close()
	}
	m.mu.RUnlock()

	m.wg.Wait()

	close(m.messageChan)

	ActiveConnections.Set(0)

	m.logger.Info("websocket-manager-closed")

	return nil
}
