package config

import (
	"os"
	"testing"
)

// BenchmarkConfig_Validate benchmarks configuration validation
func BenchmarkConfig_Validate(b *testing.B) {
	cfg := &Config{
		PolymarketGammaURL: "https://gamma-api.polymarket.com",
		ArbMaxPriceSum:     0.995,
		ArbMinTradeSize:    1.0,
		ArbMaxTradeSize:    100.0,
		ArbTakerFee:        0.01,
		WSPoolSize:         20,
		ExecutionMode:      "paper",
		StorageMode:        "console",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cfg.Validate()
	}
}

// BenchmarkConfig_LoadFromEnv benchmarks environment variable loading
func BenchmarkConfig_LoadFromEnv(b *testing.B) {
	// Set test environment variables
	os.Setenv("ARB_MAX_PRICE_SUM", "0.995")
	os.Setenv("ARB_MIN_TRADE_SIZE", "1.0")
	os.Setenv("ARB_MAX_TRADE_SIZE", "100.0")
	os.Setenv("EXECUTION_MODE", "paper")
	defer func() {
		os.Unsetenv("ARB_MAX_PRICE_SUM")
		os.Unsetenv("ARB_MIN_TRADE_SIZE")
		os.Unsetenv("ARB_MAX_TRADE_SIZE")
		os.Unsetenv("EXECUTION_MODE")
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = LoadFromEnv()
	}
}
