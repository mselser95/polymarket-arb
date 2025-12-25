package cache

import "time"

// Cache is the interface for caching market metadata.
type Cache interface {
	// Get retrieves a value from the cache.
	// Returns (value, true) if found, (nil, false) if not found.
	Get(key string) (interface{}, bool)

	// Set stores a value in the cache with a TTL.
	Set(key string, value interface{}, ttl time.Duration) bool

	// Delete removes a value from the cache.
	Delete(key string)

	// Clear removes all values from the cache.
	Clear()

	// Close closes the cache and releases resources.
	Close()
}
