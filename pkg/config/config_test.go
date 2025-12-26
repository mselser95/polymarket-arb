package config

import (
	"os"
	"testing"
	"time"
)

func TestConfig_UnlimitedMarketLimit(t *testing.T) {
	t.Run("zero_market_limit_allowed", func(t *testing.T) {
		// Set environment variables
		os.Setenv("DISCOVERY_MARKET_LIMIT", "0")
		t.Cleanup(func() {
			os.Unsetenv("DISCOVERY_MARKET_LIMIT")
		})

		cfg, err := LoadFromEnv()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if cfg.DiscoveryMarketLimit != 0 {
			t.Errorf("expected DiscoveryMarketLimit to be 0, got %d", cfg.DiscoveryMarketLimit)
		}
	})

	t.Run("positive_market_limit_allowed", func(t *testing.T) {
		// Set environment variables
		os.Setenv("DISCOVERY_MARKET_LIMIT", "1000")
		t.Cleanup(func() {
			os.Unsetenv("DISCOVERY_MARKET_LIMIT")
		})

		cfg, err := LoadFromEnv()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if cfg.DiscoveryMarketLimit != 1000 {
			t.Errorf("expected DiscoveryMarketLimit to be 1000, got %d", cfg.DiscoveryMarketLimit)
		}
	})
}

func TestConfig_UnlimitedDuration(t *testing.T) {
	t.Run("zero_duration_allowed", func(t *testing.T) {
		// Set environment variables
		os.Setenv("ARB_MAX_MARKET_DURATION", "0")
		t.Cleanup(func() {
			os.Unsetenv("ARB_MAX_MARKET_DURATION")
		})

		cfg, err := LoadFromEnv()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if cfg.MaxMarketDuration != 0 {
			t.Errorf("expected MaxMarketDuration to be 0, got %v", cfg.MaxMarketDuration)
		}
	})

	t.Run("positive_duration_allowed", func(t *testing.T) {
		// Set environment variables
		os.Setenv("ARB_MAX_MARKET_DURATION", "24h")
		t.Cleanup(func() {
			os.Unsetenv("ARB_MAX_MARKET_DURATION")
		})

		cfg, err := LoadFromEnv()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if cfg.MaxMarketDuration != 24*time.Hour {
			t.Errorf("expected MaxMarketDuration to be 24h, got %v", cfg.MaxMarketDuration)
		}
	})
}

func TestConfig_NegativeValues(t *testing.T) {
	t.Run("negative_market_limit_rejected", func(t *testing.T) {
		// Create config directly with negative value
		cfg := &Config{
			HTTPPort:             "8080",
			PolymarketWSURL:      "wss://ws-subscriptions-clob.polymarket.com/ws/market",
			PolymarketGammaURL:   "https://gamma-api.polymarket.com",
			ArbThreshold:         0.995,
			ArbMinTradeSize:      1.0,
			ArbMaxTradeSize:      10.0,
			MaxMarketDuration:    1 * time.Hour,
			DiscoveryMarketLimit: -1, // Negative value
			CleanupInterval:      5 * time.Minute,
			WSPoolSize:           5,
			ExecutionMode:        "paper",
		}

		err := cfg.Validate()
		if err == nil {
			t.Fatal("expected error for negative market limit, got nil")
		}

		expectedMsg := "DISCOVERY_MARKET_LIMIT must be non-negative (0 = unlimited), got -1"
		if err.Error() != expectedMsg {
			t.Errorf("expected error %q, got %q", expectedMsg, err.Error())
		}
	})

	t.Run("negative_duration_rejected", func(t *testing.T) {
		// Create config directly with negative value
		cfg := &Config{
			HTTPPort:             "8080",
			PolymarketWSURL:      "wss://ws-subscriptions-clob.polymarket.com/ws/market",
			PolymarketGammaURL:   "https://gamma-api.polymarket.com",
			ArbThreshold:         0.995,
			ArbMinTradeSize:      1.0,
			ArbMaxTradeSize:      10.0,
			MaxMarketDuration:    -1 * time.Hour, // Negative value
			DiscoveryMarketLimit: 100,
			CleanupInterval:      5 * time.Minute,
			WSPoolSize:           5,
			ExecutionMode:        "paper",
		}

		err := cfg.Validate()
		if err == nil {
			t.Fatal("expected error for negative duration, got nil")
		}

		expectedMsg := "ARB_MAX_MARKET_DURATION must be non-negative (0 = unlimited), got -1h0m0s"
		if err.Error() != expectedMsg {
			t.Errorf("expected error %q, got %q", expectedMsg, err.Error())
		}
	})
}

func TestConfig_PoolSizeValidation(t *testing.T) {
	t.Run("pool_size_zero_rejected", func(t *testing.T) {
		// Create config directly with zero pool size
		cfg := &Config{
			HTTPPort:             "8080",
			PolymarketWSURL:      "wss://ws-subscriptions-clob.polymarket.com/ws/market",
			PolymarketGammaURL:   "https://gamma-api.polymarket.com",
			ArbThreshold:         0.995,
			ArbMinTradeSize:      1.0,
			ArbMaxTradeSize:      10.0,
			MaxMarketDuration:    1 * time.Hour,
			DiscoveryMarketLimit: 100,
			CleanupInterval:      5 * time.Minute,
			WSPoolSize:           0, // Invalid: must be >= 1
			ExecutionMode:        "paper",
		}

		err := cfg.Validate()
		if err == nil {
			t.Fatal("expected error for pool size 0, got nil")
		}

		expectedMsg := "WS_POOL_SIZE must be at least 1, got 0"
		if err.Error() != expectedMsg {
			t.Errorf("expected error %q, got %q", expectedMsg, err.Error())
		}
	})

	t.Run("pool_size_too_large_rejected", func(t *testing.T) {
		// Create config directly with pool size > 20
		cfg := &Config{
			HTTPPort:             "8080",
			PolymarketWSURL:      "wss://ws-subscriptions-clob.polymarket.com/ws/market",
			PolymarketGammaURL:   "https://gamma-api.polymarket.com",
			ArbThreshold:         0.995,
			ArbMinTradeSize:      1.0,
			ArbMaxTradeSize:      10.0,
			MaxMarketDuration:    1 * time.Hour,
			DiscoveryMarketLimit: 100,
			CleanupInterval:      5 * time.Minute,
			WSPoolSize:           25, // Invalid: must be <= 20
			ExecutionMode:        "paper",
		}

		err := cfg.Validate()
		if err == nil {
			t.Fatal("expected error for pool size > 20, got nil")
		}

		expectedMsg := "WS_POOL_SIZE must not exceed 20, got 25"
		if err.Error() != expectedMsg {
			t.Errorf("expected error %q, got %q", expectedMsg, err.Error())
		}
	})

	t.Run("pool_size_1_allowed", func(t *testing.T) {
		// Set environment variables
		os.Setenv("WS_POOL_SIZE", "1")
		t.Cleanup(func() {
			os.Unsetenv("WS_POOL_SIZE")
		})

		cfg, err := LoadFromEnv()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if cfg.WSPoolSize != 1 {
			t.Errorf("expected WSPoolSize to be 1, got %d", cfg.WSPoolSize)
		}
	})

	t.Run("pool_size_20_allowed", func(t *testing.T) {
		// Set environment variables
		os.Setenv("WS_POOL_SIZE", "20")
		t.Cleanup(func() {
			os.Unsetenv("WS_POOL_SIZE")
		})

		cfg, err := LoadFromEnv()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if cfg.WSPoolSize != 20 {
			t.Errorf("expected WSPoolSize to be 20, got %d", cfg.WSPoolSize)
		}
	})

	t.Run("pool_size_default_is_5", func(t *testing.T) {
		// Don't set WS_POOL_SIZE, check default
		cfg, err := LoadFromEnv()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if cfg.WSPoolSize != 5 {
			t.Errorf("expected default WSPoolSize to be 5, got %d", cfg.WSPoolSize)
		}
	})
}

func TestConfig_DefaultMarketLimit(t *testing.T) {
	t.Run("default_market_limit_is_1000", func(t *testing.T) {
		// Don't set DISCOVERY_MARKET_LIMIT, check default
		cfg, err := LoadFromEnv()
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if cfg.DiscoveryMarketLimit != 1000 {
			t.Errorf("expected default DiscoveryMarketLimit to be 1000, got %d", cfg.DiscoveryMarketLimit)
		}
	})
}
