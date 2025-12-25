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
