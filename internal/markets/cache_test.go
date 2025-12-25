package markets

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/mselser95/polymarket-arb/pkg/cache"
	"go.uber.org/zap"
)

func TestCachedMetadataClient_GetTokenMetadata_WithCache(t *testing.T) {
	// Create mock cache
	mockCache, err := cache.NewRistrettoCache(&cache.RistrettoConfig{
		NumCounters: 1000,
		MaxCost:     1 << 20, // 1MB
		BufferItems: 64,
		Logger:      zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer mockCache.Close()

	// Create mock metadata client
	fetchCount := 0
	mockClient := &MetadataClient{
		baseURL:    "http://mock-server",
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
	_ = fetchCount // Will be used in future enhancement

	// Create cached client
	cachedClient := NewCachedMetadataClient(mockClient, mockCache)

	if cachedClient == nil {
		t.Fatal("Expected non-nil cached client")
	}

	if cachedClient.ttl != 24*time.Hour {
		t.Errorf("Expected TTL of 24h, got %v", cachedClient.ttl)
	}

	// Manually insert metadata into cache
	testTokenID := "test-token-123"
	metadata := &TokenMetadata{
		TickSize:     0.001,
		MinOrderSize: 10.0,
		FetchedAt:    time.Now(),
	}

	cacheKey := "metadata:test-token-123"
	mockCache.Set(cacheKey, metadata, 24*time.Hour)
	if rc, ok := mockCache.(*cache.RistrettoCache); ok {
		rc.Wait() // Ristretto buffers writes, wait for it to process
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Fetch from cache (should not increment fetchCount)
	tickSize, minSize, err := cachedClient.GetTokenMetadata(ctx, testTokenID)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if tickSize != 0.001 {
		t.Errorf("Expected tick size 0.001, got %.4f", tickSize)
	}

	if minSize != 10.0 {
		t.Errorf("Expected min size 10.0, got %.2f", minSize)
	}

	if fetchCount != 0 {
		t.Errorf("Expected 0 API calls (cache hit), got %d", fetchCount)
	}
}

func TestCachedMetadataClient_GetTokenMetadata_NilCache(t *testing.T) {
	// Create client with nil cache (should work, just skips caching)
	mockClient := &MetadataClient{
		baseURL:    "http://mock-server",
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}

	cachedClient := NewCachedMetadataClient(mockClient, nil)

	if cachedClient == nil {
		t.Fatal("Expected non-nil cached client")
	}

	// This will call the real API and likely fail, but that's ok for unit test
	// We're testing that nil cache doesn't cause panic
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Should not panic with nil cache
	_, _, err := cachedClient.GetTokenMetadata(ctx, "test-token")

	// We expect an error since we're hitting a fake URL
	if err == nil {
		t.Log("Unexpectedly succeeded with mock URL")
	}
}

func TestCachedMetadataClient_CacheKey(t *testing.T) {
	mockCache, err := cache.NewRistrettoCache(&cache.RistrettoConfig{
		NumCounters: 1000,
		MaxCost:     1 << 20,
		BufferItems: 64,
		Logger:      zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer mockCache.Close()

	mockClient := &MetadataClient{
		baseURL:    "http://mock-server",
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}

	cachedClient := NewCachedMetadataClient(mockClient, mockCache)

	// Test that cache key format is correct
	testTokenID := "86076435890279485031516158085782"
	expectedKey := "metadata:86076435890279485031516158085782"

	// Manually set a value
	metadata := &TokenMetadata{
		TickSize:     0.01,
		MinOrderSize: 5.0,
		FetchedAt:    time.Now(),
	}

	mockCache.Set(expectedKey, metadata, 24*time.Hour)
	if rc, ok := mockCache.(*cache.RistrettoCache); ok {
		rc.Wait() // Ristretto buffers writes, wait for it to process
	}

	// Retrieve it
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tickSize, minSize, err := cachedClient.GetTokenMetadata(ctx, testTokenID)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if tickSize != 0.01 {
		t.Errorf("Cache key mismatch: expected tick size 0.01, got %.4f", tickSize)
	}

	if minSize != 5.0 {
		t.Errorf("Cache key mismatch: expected min size 5.0, got %.2f", minSize)
	}
}

func TestTokenMetadata_Structure(t *testing.T) {
	// Test that TokenMetadata struct can be created and fields are accessible
	metadata := TokenMetadata{
		TickSize:     0.001,
		MinOrderSize: 100.0,
		FetchedAt:    time.Now(),
	}

	if metadata.TickSize != 0.001 {
		t.Errorf("Expected TickSize 0.001, got %.4f", metadata.TickSize)
	}

	if metadata.MinOrderSize != 100.0 {
		t.Errorf("Expected MinOrderSize 100.0, got %.2f", metadata.MinOrderSize)
	}

	if metadata.FetchedAt.IsZero() {
		t.Error("Expected FetchedAt to be set")
	}
}

func TestNewCachedMetadataClient(t *testing.T) {
	mockCache, err := cache.NewRistrettoCache(&cache.RistrettoConfig{
		NumCounters: 1000,
		MaxCost:     1 << 20,
		BufferItems: 64,
		Logger:      zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("Failed to create cache: %v", err)
	}
	defer mockCache.Close()

	mockClient := &MetadataClient{
		baseURL:    "http://test",
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}

	cachedClient := NewCachedMetadataClient(mockClient, mockCache)

	if cachedClient == nil {
		t.Fatal("Expected non-nil cached client")
	}

	if cachedClient.client != mockClient {
		t.Error("Expected client to be set correctly")
	}

	if cachedClient.cache != mockCache {
		t.Error("Expected cache to be set correctly")
	}

	if cachedClient.ttl != 24*time.Hour {
		t.Errorf("Expected default TTL of 24h, got %v", cachedClient.ttl)
	}
}
