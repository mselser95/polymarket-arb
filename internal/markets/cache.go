package markets

import (
	"context"
	"fmt"
	"time"

	"github.com/mselser95/polymarket-arb/pkg/cache"
)

// CachedMetadataClient wraps MetadataClient with caching
type CachedMetadataClient struct {
	client *MetadataClient
	cache  cache.Cache
	ttl    time.Duration
}

// NewCachedMetadataClient creates a new cached metadata client
func NewCachedMetadataClient(client *MetadataClient, cache cache.Cache) *CachedMetadataClient {
	return &CachedMetadataClient{
		client: client,
		cache:  cache,
		ttl:    24 * time.Hour, // Cache for 24 hours
	}
}

// TokenMetadata holds cached metadata for a token
type TokenMetadata struct {
	TickSize     float64
	MinOrderSize float64
	FetchedAt    time.Time
}

// GetTokenMetadata fetches token metadata with caching
func (c *CachedMetadataClient) GetTokenMetadata(ctx context.Context, tokenID string) (tickSize, minOrderSize float64, err error) {
	// Check cache first
	if c.cache != nil {
		cacheKey := fmt.Sprintf("metadata:%s", tokenID)
		if cached, ok := c.cache.Get(cacheKey); ok {
			if meta, ok := cached.(*TokenMetadata); ok {
				return meta.TickSize, meta.MinOrderSize, nil
			}
		}
	}

	// Fetch from API
	tickSize, minOrderSize, err = c.client.FetchTokenMetadata(ctx, tokenID)
	if err != nil {
		return tickSize, minOrderSize, err
	}

	// Cache the result
	if c.cache != nil {
		meta := &TokenMetadata{
			TickSize:     tickSize,
			MinOrderSize: minOrderSize,
			FetchedAt:    time.Now(),
		}
		cacheKey := fmt.Sprintf("metadata:%s", tokenID)
		c.cache.Set(cacheKey, meta, c.ttl)
	}

	return tickSize, minOrderSize, nil
}
