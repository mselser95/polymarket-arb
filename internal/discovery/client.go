package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/mselser95/polymarket-arb/pkg/types"
	"go.uber.org/zap"
)

// Client is an HTTP client for the Polymarket Gamma API.
type Client struct {
	baseURL    string
	httpClient *http.Client
	logger     *zap.Logger
}

// NewClient creates a new Gamma API client.
func NewClient(baseURL string, logger *zap.Logger) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
	}
}

// FetchActiveMarkets fetches active markets from the Gamma API.
// orderBy specifies the field to sort by: "volume24hr", "createdAt", or "endDate".
func (c *Client) FetchActiveMarkets(ctx context.Context, limit int, offset int, orderBy string) (*types.MarketsResponse, error) {
	endpoint := fmt.Sprintf("%s/markets", c.baseURL)

	// Build query parameters
	params := url.Values{}
	params.Add("closed", "false")
	params.Add("active", "true")
	params.Add("limit", strconv.Itoa(limit))
	params.Add("offset", strconv.Itoa(offset))
	params.Add("order", orderBy)

	// For endDate, use ascending to get markets expiring soonest
	// For volume24hr and createdAt, use descending to get highest/newest
	if orderBy == "endDate" {
		params.Add("ascending", "true")
	} else {
		params.Add("ascending", "false")
	}

	requestURL := fmt.Sprintf("%s?%s", endpoint, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "polymarket-arb/1.0")

	c.logger.Debug("fetching-markets",
		zap.String("url", requestURL),
		zap.Int("limit", limit),
		zap.Int("offset", offset))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	// Gamma API returns a direct array, not wrapped in an object
	var markets []types.Market
	err = json.Unmarshal(body, &markets)
	if err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	// Wrap in response object for consistency
	marketsResp := &types.MarketsResponse{
		Data:   markets,
		Count:  len(markets),
		Limit:  limit,
		Offset: offset,
	}

	c.logger.Debug("fetched-markets",
		zap.Int("count", len(markets)))

	return marketsResp, nil
}

// FetchMarketBySlug fetches a single market by its slug.
// Note: The Gamma API doesn't support /markets/{slug}, only /markets/{id}.
// This function searches through the markets list to find the matching slug.
func (c *Client) FetchMarketBySlug(ctx context.Context, slug string) (*types.Market, error) {
	// Fetch markets with large limit to search through
	// We'll paginate if needed to find the market
	limit := 100
	offset := 0
	maxPages := 10 // Search up to 1000 markets

	for page := 0; page < maxPages; page++ {
		resp, err := c.FetchActiveMarkets(ctx, limit, offset, "volume24hr")
		if err != nil {
			return nil, fmt.Errorf("fetch markets: %w", err)
		}

		// Search for matching slug
		for i := range resp.Data {
			if resp.Data[i].Slug == slug {
				return &resp.Data[i], nil
			}
		}

		// If we got fewer markets than the limit, we've reached the end
		if len(resp.Data) < limit {
			break
		}

		offset += limit
	}

	return nil, fmt.Errorf("market not found: %s", slug)
}
