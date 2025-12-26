package types

import (
	"encoding/json"
	"strconv"
	"time"
)

// OrderbookMessage represents a message from the Polymarket WebSocket.
type OrderbookMessage struct {
	EventType string       `json:"event_type"` // "book", "price_change", "last_trade_price"
	AssetID   string       `json:"asset_id"`
	Market    string       `json:"market"`
	Timestamp int64        `json:"-"` // Parsed from string via UnmarshalJSON
	Hash      string       `json:"hash,omitempty"`
	Bids      []PriceLevel `json:"bids,omitempty"`
	Asks      []PriceLevel `json:"asks,omitempty"`
}

// UnmarshalJSON custom unmarshaler to handle string timestamp.
func (o *OrderbookMessage) UnmarshalJSON(data []byte) error {
	type Alias OrderbookMessage
	aux := &struct {
		TimestampStr string `json:"timestamp"`
		*Alias
	}{
		Alias: (*Alias)(o),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// Parse timestamp from string to int64
	if aux.TimestampStr != "" {
		timestamp, err := strconv.ParseInt(aux.TimestampStr, 10, 64)
		if err != nil {
			return err
		}
		o.Timestamp = timestamp
	}

	return nil
}

// PriceLevel represents a single price level in the orderbook.
type PriceLevel struct {
	Price string `json:"price"`
	Size  string `json:"size"`
}

// OrderbookSnapshot represents the current state of an orderbook for a token.
type OrderbookSnapshot struct {
	MarketID     string
	TokenID      string
	Outcome      string // "YES" or "NO"
	BestBidPrice float64
	BestBidSize  float64
	BestAskPrice float64
	BestAskSize  float64
	LastUpdated  time.Time
}

// PriceChangeMessage represents incremental price update messages from the Polymarket CLOB API.
// The API sends price_change messages with a different structure than book messages.
type PriceChangeMessage struct {
	EventType    string        `json:"event_type"` // "price_change"
	Market       string        `json:"market"`
	Timestamp    int64         `json:"-"` // Parsed from string via UnmarshalJSON
	PriceChanges []PriceChange `json:"price_changes"`
}

// UnmarshalJSON custom unmarshaler to handle string timestamp.
func (p *PriceChangeMessage) UnmarshalJSON(data []byte) error {
	type Alias PriceChangeMessage
	aux := &struct {
		TimestampStr string `json:"timestamp"`
		*Alias
	}{
		Alias: (*Alias)(p),
	}

	err := json.Unmarshal(data, &aux)
	if err != nil {
		return err
	}

	// Parse timestamp from string to int64
	if aux.TimestampStr != "" {
		timestamp, parseErr := strconv.ParseInt(aux.TimestampStr, 10, 64)
		if parseErr != nil {
			return parseErr
		}
		p.Timestamp = timestamp
	}

	return nil
}

// PriceChange represents a single asset's price update within a price_change message.
// This matches the Polymarket CLOB API structure for price_change events.
type PriceChange struct {
	AssetID string `json:"asset_id"`
	Price   string `json:"price"`    // Price level affected
	Size    string `json:"size"`     // New aggregate size for this price level
	Side    string `json:"side"`     // "BUY" or "SELL"
	Hash    string `json:"hash"`     // Hash of the order
	BestBid string `json:"best_bid"` // Current best bid price
	BestAsk string `json:"best_ask"` // Current best ask price
}

// LastTradePriceMessage represents trade execution notifications from the CLOB API.
// These are informational messages about completed trades and don't affect orderbook state.
type LastTradePriceMessage struct {
	EventType  string `json:"event_type"` // "last_trade_price"
	Market     string `json:"market"`
	AssetID    string `json:"asset_id"`
	Price      string `json:"price"`
	Size       string `json:"size"`
	Side       string `json:"side"` // "BUY" or "SELL"
	FeeRateBps string `json:"fee_rate_bps"`
	Timestamp  string `json:"timestamp"` // Unix timestamp as string
}
