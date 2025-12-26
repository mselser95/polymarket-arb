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

// Manager manages a single WebSocket connection to Polymarket.
type Manager struct {
	url             string
	conn            *websocket.Conn
	logger          *zap.Logger
	reconnectMgr    *ReconnectManager
	config          Config
	messageChan     chan *types.OrderbookMessage
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
		url:          cfg.URL,
		logger:       cfg.Logger,
		reconnectMgr: NewReconnectManager(reconnectCfg, cfg.Logger),
		config:       cfg,
		messageChan:  make(chan *types.OrderbookMessage, cfg.MessageBufferSize),
		ctx:          ctx,
		cancel:       cancel,
		subscribed:   make(map[string]bool),
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

		// Parse message - Polymarket sends an array of messages
		var obMsgs []types.OrderbookMessage
		err = json.Unmarshal(message, &obMsgs)
		if err != nil {
			// Try to identify message type for better logging
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

			// Unknown message format
			previewLen := len(messageStr)
			if previewLen > 100 {
				previewLen = 100
			}
			m.logger.Debug("websocket-unparseable-message",
				zap.Error(err),
				zap.Int("bytes", len(message)),
				zap.String("preview", messageStr[:previewLen]))
			continue
		}

		// Process each message in the array
		for i := range obMsgs {
			start := time.Now()
			obMsg := &obMsgs[i]

			MessagesReceivedTotal.WithLabelValues(obMsg.EventType).Inc()

			// Send to channel (non-blocking)
			select {
			case m.messageChan <- obMsg:
			default:
				m.logger.Warn("message-channel-full", zap.String("event-type", obMsg.EventType))
				MessagesDroppedTotal.WithLabelValues("channel_full").Inc()
			}

			// Observe message processing latency
			MessageLatencySeconds.Observe(time.Since(start).Seconds())
		}
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
