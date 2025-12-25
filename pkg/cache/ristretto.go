package cache

import (
	"time"

	"github.com/dgraph-io/ristretto"
	"go.uber.org/zap"
)

// RistrettoCache is a cache implementation using Ristretto.
type RistrettoCache struct {
	cache  *ristretto.Cache
	logger *zap.Logger
}

// RistrettoConfig holds configuration for Ristretto cache.
type RistrettoConfig struct {
	NumCounters int64         // Number of keys to track frequency (10x max items)
	MaxCost     int64         // Maximum cost of cache (in bytes or items)
	BufferItems int64         // Number of keys per Get buffer
	Logger      *zap.Logger
}

// NewRistrettoCache creates a new Ristretto-backed cache.
func NewRistrettoCache(cfg *RistrettoConfig) (Cache, error) {
	cache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: cfg.NumCounters,
		MaxCost:     cfg.MaxCost,
		BufferItems: cfg.BufferItems,
		Metrics:     true, // Enable metrics
	})
	if err != nil {
		return nil, err
	}

	return &RistrettoCache{
		cache:  cache,
		logger: cfg.Logger,
	}, nil
}

// Get retrieves a value from the cache.
func (r *RistrettoCache) Get(key string) (interface{}, bool) {
	value, found := r.cache.Get(key)
	if found {
		CacheHitsTotal.Inc()
		r.logger.Debug("cache-hit", zap.String("key", key))
	} else {
		CacheMissesTotal.Inc()
		r.logger.Debug("cache-miss", zap.String("key", key))
	}
	return value, found
}

// Set stores a value in the cache with a TTL.
func (r *RistrettoCache) Set(key string, value interface{}, ttl time.Duration) bool {
	// Cost = 1 (we're counting items, not bytes)
	success := r.cache.SetWithTTL(key, value, 1, ttl)
	if success {
		CacheSetsTotal.Inc()
		r.logger.Debug("cache-set",
			zap.String("key", key),
			zap.Duration("ttl", ttl))
	}
	return success
}

// Delete removes a value from the cache.
func (r *RistrettoCache) Delete(key string) {
	r.cache.Del(key)
	CacheDeletesTotal.Inc()
	r.logger.Debug("cache-delete", zap.String("key", key))
}

// Clear removes all values from the cache.
func (r *RistrettoCache) Clear() {
	r.cache.Clear()
	r.logger.Info("cache-cleared")
}

// Close closes the cache and releases resources.
func (r *RistrettoCache) Close() {
	r.cache.Close()
	r.logger.Info("cache-closed")
}

// Metrics returns Ristretto's internal metrics.
func (r *RistrettoCache) Metrics() *ristretto.Metrics {
	return r.cache.Metrics
}

// Wait blocks until all pending writes have been applied.
// This is useful for testing or when you need to ensure a value is cached.
func (r *RistrettoCache) Wait() {
	r.cache.Wait()
}
