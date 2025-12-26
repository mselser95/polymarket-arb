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
				MetadataCacheHitsTotal.Inc()
				return meta.TickSize, meta.MinOrderSize, nil
			}
		}
		// Cache miss
		MetadataCacheMissesTotal.Inc()
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

// UpdateTickSize updates the tick size for a token in the cache without refetching from API.
// This is called when a tick_size_change WebSocket event is received.
// If the token is not in cache, this is a no-op (will be fetched on next access).
func (c *CachedMetadataClient) UpdateTickSize(tokenID string, newTickSize float64) {
	if c.cache == nil {
		return
	}

	cacheKey := fmt.Sprintf("metadata:%s", tokenID)

	// Get existing cached metadata
	if cached, ok := c.cache.Get(cacheKey); ok {
		if meta, ok := cached.(*TokenMetadata); ok {
			// Update tick size while preserving other fields
			updatedMeta := &TokenMetadata{
				TickSize:     newTickSize,
				MinOrderSize: meta.MinOrderSize,
				FetchedAt:    time.Now(), // Update fetch time to indicate freshness
			}
			c.cache.Set(cacheKey, updatedMeta, c.ttl)
		}
	}
	// If not in cache, no-op - will be fetched with correct value on next access
}
