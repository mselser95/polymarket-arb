package markets

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// MetadataClient fetches market metadata from the Polymarket CLOB API
type MetadataClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewMetadataClient creates a new metadata client
func NewMetadataClient() *MetadataClient {
	return &MetadataClient{
		baseURL: "https://clob.polymarket.com",
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// FetchTickSize fetches tick size for a token from the CLOB API
func (c *MetadataClient) FetchTickSize(ctx context.Context, tokenID string) (tickSize float64, err error) {
	url := fmt.Sprintf("%s/tick-size?token_id=%s", c.baseURL, tokenID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return tickSize, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return tickSize, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return tickSize, fmt.Errorf("API error: status %d", resp.StatusCode)
	}

	var data struct {
		MinimumTickSize float64 `json:"minimum_tick_size"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return tickSize, err
	}

	tickSize = data.MinimumTickSize
	return tickSize, err
}

// FetchMinOrderSize fetches minimum order size for a token
// Tries the orderbook endpoint to find this value
func (c *MetadataClient) FetchMinOrderSize(ctx context.Context, tokenID string) (minOrderSize float64, err error) {
	// Try orderbook endpoint
	url := fmt.Sprintf("%s/book?token_id=%s", c.baseURL, tokenID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return minOrderSize, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return minOrderSize, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Default to 5.0 if API doesn't provide it
		minOrderSize = 5.0
		return minOrderSize, nil
	}

	var data struct {
		MinSize float64 `json:"min_size"`
		Market  struct {
			MinSize float64 `json:"minimum_order_size"`
		} `json:"market"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		// Default to 5.0 on parse error
		minOrderSize = 5.0
		return minOrderSize, nil
	}

	if data.MinSize > 0 {
		minOrderSize = data.MinSize
		return minOrderSize, nil
	}
	if data.Market.MinSize > 0 {
		minOrderSize = data.Market.MinSize
		return minOrderSize, nil
	}

	// Default minimum
	minOrderSize = 5.0
	return minOrderSize, nil
}

// FetchTokenMetadata fetches both tick size and min order size for a token
func (c *MetadataClient) FetchTokenMetadata(ctx context.Context, tokenID string) (tickSize, minOrderSize float64, err error) {
	tickSize, err = c.FetchTickSize(ctx, tokenID)
	if err != nil {
		tickSize = 0.01 // Default
	}

	minOrderSize, err = c.FetchMinOrderSize(ctx, tokenID)
	if err != nil {
		minOrderSize = 5.0 // Default
	}

	return tickSize, minOrderSize, nil
}
