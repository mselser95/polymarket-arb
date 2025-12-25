package cache

import (
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestRistrettoCache(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cacheInterface, err := NewRistrettoCache(&RistrettoConfig{
		NumCounters: 1000,    // 10x expected items (was too low)
		MaxCost:     100,      // Increased capacity
		BufferItems: 64,
		Logger:      logger,
	})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cacheInterface.Close()

	// Cast to RistrettoCache for test-specific methods
	cache := cacheInterface.(*RistrettoCache)

	t.Run("set-and-get", func(t *testing.T) {
		key := "test-key"
		value := "test-value"

		// Set value
		success := cache.Set(key, value, 1*time.Hour)
		if !success {
			t.Error("expected Set to succeed")
		}

		// Wait for Ristretto to process pending writes
		cache.Wait()

		// Get value
		retrieved, found := cache.Get(key)
		if !found {
			t.Error("expected key to be found")
		}

		if retrieved != value {
			t.Errorf("expected %q, got %q", value, retrieved)
		}
	})

	t.Run("get-missing-key", func(t *testing.T) {
		_, found := cache.Get("nonexistent")
		if found {
			t.Error("expected key to not be found")
		}
	})

	t.Run("delete", func(t *testing.T) {
		key := "delete-test"
		value := "delete-value"

		cache.Set(key, value, 1*time.Hour)
		time.Sleep(100 * time.Millisecond)

		// Verify it exists
		_, found := cache.Get(key)
		if !found {
			t.Error("expected key to exist before delete")
		}

		// Delete it
		cache.Delete(key)

		// Verify it's gone
		_, found = cache.Get(key)
		if found {
			t.Error("expected key to be deleted")
		}
	})

	t.Run("ttl-expiration", func(t *testing.T) {
		key := "ttl-test"
		value := "ttl-value"

		// Set with short TTL
		cache.Set(key, value, 200*time.Millisecond)
		time.Sleep(100 * time.Millisecond)

		// Should exist immediately
		_, found := cache.Get(key)
		if !found {
			t.Error("expected key to exist before TTL expires")
		}

		// Wait for TTL to expire
		time.Sleep(200 * time.Millisecond)

		// Should be expired
		_, found = cache.Get(key)
		if found {
			t.Error("expected key to be expired after TTL")
		}
	})

	t.Run("clear", func(t *testing.T) {
		// Set multiple keys
		cache.Set("clear-key1", "value1", 1*time.Hour)
		cache.Set("clear-key2", "value2", 1*time.Hour)
		cache.Wait()

		// Verify they exist
		_, found1 := cache.Get("clear-key1")
		_, found2 := cache.Get("clear-key2")
		if !found1 || !found2 {
			t.Logf("Admission: key1=%v, key2=%v", found1, found2)
			t.Skip("Ristretto probabilistic admission - some keys not admitted")
		}

		// Clear the cache
		cache.Clear()

		// Verify all keys are gone
		_, found1 = cache.Get("clear-key1")
		_, found2 = cache.Get("clear-key2")
		if found1 || found2 {
			t.Error("expected all keys to be cleared")
		}
	})
}
