package config

import (
	"fmt"
	"os"
	"testing"
	"time"
)

// ===== Comprehensive Validation Tests =====

// TestValidate_MinTradeSize_Positive tests that min trade size must be > 0
func TestValidate_MinTradeSize_Positive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		minSize  float64
		wantErr  bool
		errMsg   string
	}{
		{
			name:    "positive-min-size",
			minSize: 1.0,
			wantErr: false,
		},
		{
			name:    "zero-min-size",
			minSize: 0,
			wantErr: true,
			errMsg:  "ARB_MIN_TRADE_SIZE must be positive, got 0.000000",
		},
		{
			name:    "negative-min-size",
			minSize: -1.0,
			wantErr: true,
			errMsg:  "ARB_MIN_TRADE_SIZE must be positive, got -1.000000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				HTTPPort:            "8080",
				PolymarketWSURL:     "wss://test.com",
				PolymarketGammaURL:  "https://test.com",
				ArbMaxPriceSum:      0.995,
				ArbMinTradeSize:     tt.minSize,
				ArbMaxTradeSize:     10.0,
				MaxMarketDuration:   1 * time.Hour,
				DiscoveryMarketLimit: 100,
				CleanupInterval:     5 * time.Minute,
				WSPoolSize:          20,
				ExecutionMode:       "paper",
			}

			err := cfg.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				} else if err.Error() != tt.errMsg {
					t.Errorf("expected error %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			}
		})
	}
}

// TestValidate_MaxTradeSize_Positive tests that max trade size must be > 0
func TestValidate_MaxTradeSize_Positive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		maxSize float64
		wantErr bool
		errMsg  string
	}{
		{
			name:    "positive-max-size",
			maxSize: 10.0,
			wantErr: false,
		},
		{
			name:    "zero-max-size",
			maxSize: 0,
			wantErr: true,
			errMsg:  "ARB_MAX_TRADE_SIZE must be positive, got 0.000000",
		},
		{
			name:    "negative-max-size",
			maxSize: -5.0,
			wantErr: true,
			errMsg:  "ARB_MAX_TRADE_SIZE must be positive, got -5.000000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				HTTPPort:             "8080",
				PolymarketWSURL:      "wss://test.com",
				PolymarketGammaURL:   "https://test.com",
				ArbMaxPriceSum:       0.995,
				ArbMinTradeSize:      1.0,
				ArbMaxTradeSize:      tt.maxSize,
				MaxMarketDuration:    1 * time.Hour,
				DiscoveryMarketLimit: 100,
				CleanupInterval:      5 * time.Minute,
				WSPoolSize:           20,
				ExecutionMode:        "paper",
			}

			err := cfg.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				} else if err.Error() != tt.errMsg {
					t.Errorf("expected error %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			}
		})
	}
}

// TestValidate_MaxGTE_Min tests that MAX >= MIN
func TestValidate_MaxGTE_Min(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		minSize float64
		maxSize float64
		wantErr bool
		errMsg  string
	}{
		{
			name:    "max-equals-min",
			minSize: 5.0,
			maxSize: 5.0,
			wantErr: false,
		},
		{
			name:    "max-greater-than-min",
			minSize: 1.0,
			maxSize: 10.0,
			wantErr: false,
		},
		{
			name:    "max-less-than-min",
			minSize: 10.0,
			maxSize: 5.0,
			wantErr: true,
			errMsg:  "ARB_MAX_TRADE_SIZE (5.000000) must be >= ARB_MIN_TRADE_SIZE (10.000000)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				HTTPPort:             "8080",
				PolymarketWSURL:      "wss://test.com",
				PolymarketGammaURL:   "https://test.com",
				ArbMaxPriceSum:       0.995,
				ArbMinTradeSize:      tt.minSize,
				ArbMaxTradeSize:      tt.maxSize,
				MaxMarketDuration:    1 * time.Hour,
				DiscoveryMarketLimit: 100,
				CleanupInterval:      5 * time.Minute,
				WSPoolSize:           20,
				ExecutionMode:        "paper",
			}

			err := cfg.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				} else if err.Error() != tt.errMsg {
					t.Errorf("expected error %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			}
		})
	}
}

// TestValidate_PriceSumRange tests 0 to 1.10 range for research mode
func TestValidate_PriceSumRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		priceSum   float64
		wantErr    bool
		errMsg     string
	}{
		{
			name:     "normal-threshold-0.995",
			priceSum: 0.995,
			wantErr:  false,
		},
		{
			name:     "research-mode-1.05",
			priceSum: 1.05,
			wantErr:  false,
		},
		{
			name:     "max-allowed-1.10",
			priceSum: 1.10,
			wantErr:  false,
		},
		{
			name:     "zero-threshold",
			priceSum: 0.0,
			wantErr:  true,
			errMsg:   "ARB_MAX_PRICE_SUM must be between 0 and 1.10 (values > 1.0 for research mode), got 0.000000",
		},
		{
			name:     "exceeds-max",
			priceSum: 1.15,
			wantErr:  true,
			errMsg:   "ARB_MAX_PRICE_SUM must be between 0 and 1.10 (values > 1.0 for research mode), got 1.150000",
		},
		{
			name:     "negative-threshold",
			priceSum: -0.5,
			wantErr:  true,
			errMsg:   "ARB_MAX_PRICE_SUM must be between 0 and 1.10 (values > 1.0 for research mode), got -0.500000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				HTTPPort:             "8080",
				PolymarketWSURL:      "wss://test.com",
				PolymarketGammaURL:   "https://test.com",
				ArbMaxPriceSum:       tt.priceSum,
				ArbMinTradeSize:      1.0,
				ArbMaxTradeSize:      10.0,
				MaxMarketDuration:    1 * time.Hour,
				DiscoveryMarketLimit: 100,
				CleanupInterval:      5 * time.Minute,
				WSPoolSize:           20,
				ExecutionMode:        "paper",
			}

			err := cfg.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				} else if err.Error() != tt.errMsg {
					t.Errorf("expected error %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			}
		})
	}
}

// TestValidate_ExecutionMode tests enum validation
func TestValidate_ExecutionMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mode    string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "dry-run-mode",
			mode:    "dry-run",
			wantErr: false,
		},
		{
			name:    "paper-mode",
			mode:    "paper",
			wantErr: false,
		},
		{
			name:    "live-mode",
			mode:    "live",
			wantErr: false,
		},
		{
			name:    "invalid-mode",
			mode:    "invalid",
			wantErr: true,
			errMsg:  "EXECUTION_MODE must be 'paper', 'live', or 'dry-run', got \"invalid\"",
		},
		{
			name:    "empty-mode",
			mode:    "",
			wantErr: true,
			errMsg:  "EXECUTION_MODE must be 'paper', 'live', or 'dry-run', got \"\"",
		},
		{
			name:    "uppercase-mode",
			mode:    "PAPER",
			wantErr: true,
			errMsg:  "EXECUTION_MODE must be 'paper', 'live', or 'dry-run', got \"PAPER\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				HTTPPort:             "8080",
				PolymarketWSURL:      "wss://test.com",
				PolymarketGammaURL:   "https://test.com",
				ArbMaxPriceSum:       0.995,
				ArbMinTradeSize:      1.0,
				ArbMaxTradeSize:      10.0,
				MaxMarketDuration:    1 * time.Hour,
				DiscoveryMarketLimit: 100,
				CleanupInterval:      5 * time.Minute,
				WSPoolSize:           20,
				ExecutionMode:        tt.mode,
			}

			err := cfg.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				} else if err.Error() != tt.errMsg {
					t.Errorf("expected error %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			}
		})
	}
}

// TestValidate_WSPoolSize_Range tests 1-20 range
func TestValidate_WSPoolSize_Range(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		poolSize int
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "min-pool-size-1",
			poolSize: 1,
			wantErr:  false,
		},
		{
			name:     "mid-pool-size-10",
			poolSize: 10,
			wantErr:  false,
		},
		{
			name:     "max-pool-size-20",
			poolSize: 20,
			wantErr:  false,
		},
		{
			name:     "zero-pool-size",
			poolSize: 0,
			wantErr:  true,
			errMsg:   "WS_POOL_SIZE must be at least 1, got 0",
		},
		{
			name:     "exceeds-max-21",
			poolSize: 21,
			wantErr:  true,
			errMsg:   "WS_POOL_SIZE must not exceed 20, got 21",
		},
		{
			name:     "negative-pool-size",
			poolSize: -5,
			wantErr:  true,
			errMsg:   "WS_POOL_SIZE must be at least 1, got -5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				HTTPPort:             "8080",
				PolymarketWSURL:      "wss://test.com",
				PolymarketGammaURL:   "https://test.com",
				ArbMaxPriceSum:       0.995,
				ArbMinTradeSize:      1.0,
				ArbMaxTradeSize:      10.0,
				MaxMarketDuration:    1 * time.Hour,
				DiscoveryMarketLimit: 100,
				CleanupInterval:      5 * time.Minute,
				WSPoolSize:           tt.poolSize,
				ExecutionMode:        "paper",
			}

			err := cfg.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				} else if err.Error() != tt.errMsg {
					t.Errorf("expected error %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			}
		})
	}
}

// TestValidate_MarketDuration_NonNegative tests >= 0 requirement
func TestValidate_MarketDuration_NonNegative(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		duration time.Duration
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "zero-duration-unlimited",
			duration: 0,
			wantErr:  false,
		},
		{
			name:     "positive-duration-1h",
			duration: 1 * time.Hour,
			wantErr:  false,
		},
		{
			name:     "positive-duration-24h",
			duration: 24 * time.Hour,
			wantErr:  false,
		},
		{
			name:     "negative-duration",
			duration: -1 * time.Hour,
			wantErr:  true,
			errMsg:   "ARB_MAX_MARKET_DURATION must be non-negative (0 = unlimited), got -1h0m0s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				HTTPPort:             "8080",
				PolymarketWSURL:      "wss://test.com",
				PolymarketGammaURL:   "https://test.com",
				ArbMaxPriceSum:       0.995,
				ArbMinTradeSize:      1.0,
				ArbMaxTradeSize:      10.0,
				MaxMarketDuration:    tt.duration,
				DiscoveryMarketLimit: 100,
				CleanupInterval:      5 * time.Minute,
				WSPoolSize:           20,
				ExecutionMode:        "paper",
			}

			err := cfg.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				} else if err.Error() != tt.errMsg {
					t.Errorf("expected error %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			}
		})
	}
}

// TestValidate_AllValid tests complete valid configuration
func TestValidate_AllValid(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		HTTPPort:             "8080",
		PolymarketWSURL:      "wss://ws-subscriptions-clob.polymarket.com/ws/market",
		PolymarketGammaURL:   "https://gamma-api.polymarket.com",
		ArbMaxPriceSum:       0.995,
		ArbMinTradeSize:      1.0,
		ArbMaxTradeSize:      100.0,
		MaxMarketDuration:    6 * time.Hour,
		DiscoveryMarketLimit: 1000,
		CleanupInterval:      10 * time.Minute,
		WSPoolSize:           15,
		ExecutionMode:        "paper",
	}

	err := cfg.Validate()
	if err != nil {
		t.Errorf("expected no error for valid config, got %v", err)
	}
}

// ===== Type Conversion Tests =====

// TestGetIntOrDefault_Valid tests successful int parsing
func TestGetIntOrDefault_Valid(t *testing.T) {

	tests := []struct {
		name          string
		envValue      string
		defaultValue  int
		expectedValue int
	}{
		{name: "parse-100", envValue: "100", defaultValue: 50, expectedValue: 100},
		{name: "parse-0", envValue: "0", defaultValue: 50, expectedValue: 0},
		{name: "parse-negative", envValue: "-10", defaultValue: 50, expectedValue: -10},
		{name: "parse-large", envValue: "999999", defaultValue: 50, expectedValue: 999999},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("TEST_INT_VAR", tt.envValue)
			t.Cleanup(func() { os.Unsetenv("TEST_INT_VAR") })

			result := getIntOrDefault("TEST_INT_VAR", tt.defaultValue)
			if result != tt.expectedValue {
				t.Errorf("expected %d, got %d", tt.expectedValue, result)
			}
		})
	}
}

// TestGetIntOrDefault_Invalid tests fallback on parse failure
func TestGetIntOrDefault_Invalid(t *testing.T) {

	tests := []struct {
		name         string
		envValue     string
		defaultValue int
	}{
		{name: "non-numeric", envValue: "abc", defaultValue: 42},
		{name: "empty-string", envValue: "", defaultValue: 42},
		{name: "float", envValue: "3.14", defaultValue: 42},
		{name: "mixed", envValue: "12abc", defaultValue: 42},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("TEST_INT_VAR", tt.envValue)
			t.Cleanup(func() { os.Unsetenv("TEST_INT_VAR") })

			result := getIntOrDefault("TEST_INT_VAR", tt.defaultValue)
			if result != tt.defaultValue {
				t.Errorf("expected default %d, got %d", tt.defaultValue, result)
			}
		})
	}
}

// TestGetFloat64OrDefault_Valid tests successful float parsing
func TestGetFloat64OrDefault_Valid(t *testing.T) {

	tests := []struct {
		name          string
		envValue      string
		defaultValue  float64
		expectedValue float64
	}{
		{name: "parse-1.5", envValue: "1.5", defaultValue: 0.5, expectedValue: 1.5},
		{name: "parse-0.995", envValue: "0.995", defaultValue: 0.5, expectedValue: 0.995},
		{name: "parse-integer", envValue: "10", defaultValue: 0.5, expectedValue: 10.0},
		{name: "parse-negative", envValue: "-2.5", defaultValue: 0.5, expectedValue: -2.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("TEST_FLOAT_VAR", tt.envValue)
			t.Cleanup(func() { os.Unsetenv("TEST_FLOAT_VAR") })

			result := getFloat64OrDefault("TEST_FLOAT_VAR", tt.defaultValue)
			if result != tt.expectedValue {
				t.Errorf("expected %f, got %f", tt.expectedValue, result)
			}
		})
	}
}

// TestGetFloat64OrDefault_Invalid tests fallback on parse failure
func TestGetFloat64OrDefault_Invalid(t *testing.T) {

	tests := []struct {
		name         string
		envValue     string
		defaultValue float64
	}{
		{name: "non-numeric", envValue: "abc", defaultValue: 0.995},
		{name: "empty-string", envValue: "", defaultValue: 0.995},
		{name: "invalid-format", envValue: "1.2.3", defaultValue: 0.995},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("TEST_FLOAT_VAR", tt.envValue)
			t.Cleanup(func() { os.Unsetenv("TEST_FLOAT_VAR") })

			result := getFloat64OrDefault("TEST_FLOAT_VAR", tt.defaultValue)
			if result != tt.defaultValue {
				t.Errorf("expected default %f, got %f", tt.defaultValue, result)
			}
		})
	}
}

// TestGetDurationOrDefault_Valid tests successful duration parsing
func TestGetDurationOrDefault_Valid(t *testing.T) {

	tests := []struct {
		name          string
		envValue      string
		defaultValue  time.Duration
		expectedValue time.Duration
	}{
		{name: "parse-1h", envValue: "1h", defaultValue: 5 * time.Minute, expectedValue: 1 * time.Hour},
		{name: "parse-30m", envValue: "30m", defaultValue: 5 * time.Minute, expectedValue: 30 * time.Minute},
		{name: "parse-5s", envValue: "5s", defaultValue: 5 * time.Minute, expectedValue: 5 * time.Second},
		{name: "parse-0", envValue: "0", defaultValue: 5 * time.Minute, expectedValue: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("TEST_DUR_VAR", tt.envValue)
			t.Cleanup(func() { os.Unsetenv("TEST_DUR_VAR") })

			result := getDurationOrDefault("TEST_DUR_VAR", tt.defaultValue)
			if result != tt.expectedValue {
				t.Errorf("expected %v, got %v", tt.expectedValue, result)
			}
		})
	}
}

// TestGetDurationOrDefault_Invalid tests fallback on parse failure
func TestGetDurationOrDefault_Invalid(t *testing.T) {

	tests := []struct {
		name         string
		envValue     string
		defaultValue time.Duration
	}{
		{name: "invalid-format", envValue: "abc", defaultValue: 5 * time.Minute},
		{name: "missing-unit", envValue: "30", defaultValue: 5 * time.Minute},
		{name: "empty-string", envValue: "", defaultValue: 5 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("TEST_DUR_VAR", tt.envValue)
			t.Cleanup(func() { os.Unsetenv("TEST_DUR_VAR") })

			result := getDurationOrDefault("TEST_DUR_VAR", tt.defaultValue)
			if result != tt.defaultValue {
				t.Errorf("expected default %v, got %v", tt.defaultValue, result)
			}
		})
	}
}

// TestGetBoolOrDefault_Valid tests successful bool parsing
func TestGetBoolOrDefault_Valid(t *testing.T) {

	tests := []struct {
		name          string
		envValue      string
		defaultValue  bool
		expectedValue bool
	}{
		{name: "parse-true", envValue: "true", defaultValue: false, expectedValue: true},
		{name: "parse-false", envValue: "false", defaultValue: true, expectedValue: false},
		{name: "parse-1", envValue: "1", defaultValue: false, expectedValue: true},
		{name: "parse-0", envValue: "0", defaultValue: true, expectedValue: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("TEST_BOOL_VAR", tt.envValue)
			t.Cleanup(func() { os.Unsetenv("TEST_BOOL_VAR") })

			result := getBoolOrDefault("TEST_BOOL_VAR", tt.defaultValue)
			if result != tt.expectedValue {
				t.Errorf("expected %v, got %v", tt.expectedValue, result)
			}
		})
	}
}

// TestGetBoolOrDefault_Invalid tests fallback on parse failure
func TestGetBoolOrDefault_Invalid(t *testing.T) {

	tests := []struct {
		name         string
		envValue     string
		defaultValue bool
	}{
		{name: "invalid-value", envValue: "yes", defaultValue: false},
		{name: "empty-string", envValue: "", defaultValue: true},
		{name: "numeric-2", envValue: "2", defaultValue: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("TEST_BOOL_VAR", tt.envValue)
			t.Cleanup(func() { os.Unsetenv("TEST_BOOL_VAR") })

			result := getBoolOrDefault("TEST_BOOL_VAR", tt.defaultValue)
			if result != tt.defaultValue {
				t.Errorf("expected default %v, got %v", tt.defaultValue, result)
			}
		})
	}
}

// ===== Edge Cases Tests =====

// TestConfig_MaxMarketDuration_Zero tests 0 = unlimited
func TestConfig_MaxMarketDuration_Zero(t *testing.T) {
	t.Parallel()

	os.Setenv("ARB_MAX_MARKET_DURATION", "0")
	t.Cleanup(func() { os.Unsetenv("ARB_MAX_MARKET_DURATION") })

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.MaxMarketDuration != 0 {
		t.Errorf("expected duration 0 (unlimited), got %v", cfg.MaxMarketDuration)
	}

	// Should pass validation (0 = unlimited)
	err = cfg.Validate()
	if err != nil {
		t.Errorf("expected validation to pass for 0 duration, got %v", err)
	}
}

// TestConfig_MaxPriceSum_ResearchMode tests > 1.0 allowed for research
func TestConfig_MaxPriceSum_ResearchMode(t *testing.T) {
	t.Parallel()

	tests := []float64{1.01, 1.05, 1.10}

	for _, priceSum := range tests {
		t.Run(fmt.Sprintf("price-sum-%.2f", priceSum), func(t *testing.T) {
			cfg := &Config{
				HTTPPort:             "8080",
				PolymarketWSURL:      "wss://test.com",
				PolymarketGammaURL:   "https://test.com",
				ArbMaxPriceSum:       priceSum,
				ArbMinTradeSize:      1.0,
				ArbMaxTradeSize:      10.0,
				MaxMarketDuration:    1 * time.Hour,
				DiscoveryMarketLimit: 100,
				CleanupInterval:      5 * time.Minute,
				WSPoolSize:           20,
				ExecutionMode:        "dry-run",
			}

			err := cfg.Validate()
			if err != nil {
				t.Errorf("expected validation to pass for research mode price sum %.2f, got %v", priceSum, err)
			}
		})
	}
}

// TestConfig_NegativeInput_Rejected tests negative values are caught by validation
func TestConfig_NegativeInput_Rejected(t *testing.T) {
	t.Parallel()

	// Set negative values in env (should fail validation)
	os.Setenv("ARB_MIN_TRADE_SIZE", "-1.0")
	t.Cleanup(func() {
		os.Unsetenv("ARB_MIN_TRADE_SIZE")
	})

	cfg, err := LoadFromEnv()
	// LoadFromEnv calls Validate(), which should reject negative values
	if err == nil {
		t.Fatal("expected validation error for negative min trade size, got nil")
	}

	// Error should mention ARB_MIN_TRADE_SIZE
	if !contains(err.Error(), "ARB_MIN_TRADE_SIZE") {
		t.Errorf("expected error about ARB_MIN_TRADE_SIZE, got %v", err)
	}

	_ = cfg // Keep linter happy
}

// Helper function for string containment check
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && hasSubstring(s, substr)))
}

func hasSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestConfig_EmptyString_Default tests empty string → default conversion
func TestConfig_EmptyString_Default(t *testing.T) {
	t.Parallel()

	// Set empty strings in env (should use defaults)
	os.Setenv("ARB_MIN_TRADE_SIZE", "")
	os.Setenv("ARB_MAX_TRADE_SIZE", "")
	os.Setenv("WS_POOL_SIZE", "")
	t.Cleanup(func() {
		os.Unsetenv("ARB_MIN_TRADE_SIZE")
		os.Unsetenv("ARB_MAX_TRADE_SIZE")
		os.Unsetenv("WS_POOL_SIZE")
	})

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Should use defaults
	if cfg.ArbMinTradeSize != 1.0 {
		t.Errorf("expected default min trade size 1.0, got %f", cfg.ArbMinTradeSize)
	}
	if cfg.ArbMaxTradeSize != 2.0 {
		t.Errorf("expected default max trade size 2.0, got %f", cfg.ArbMaxTradeSize)
	}
	if cfg.WSPoolSize != 20 {
		t.Errorf("expected default pool size 20, got %d", cfg.WSPoolSize)
	}
}
