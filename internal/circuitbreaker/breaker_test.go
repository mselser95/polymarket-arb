package circuitbreaker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/mselser95/polymarket-arb/internal/testutil"
	"go.uber.org/zap/zaptest"
)

// Test New circuit breaker creation
func TestNew(t *testing.T) {
	t.Parallel()

	logger := zaptest.NewLogger(t)
	mockWallet := testutil.NewMockWalletClient()
	address := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")

	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid-config",
			config: &Config{
				CheckInterval:   5 * time.Minute,
				TradeMultiplier: 3.0,
				MinAbsolute:     5.0,
				HysteresisRatio: 1.5,
				WalletClient:    mockWallet,
				Address:         address,
				Logger:          logger,
			},
			wantErr: false,
		},
		{
			name:    "nil-config",
			config:  nil,
			wantErr: true,
			errMsg:  "config cannot be nil",
		},
		{
			name: "nil-wallet-client",
			config: &Config{
				CheckInterval:   5 * time.Minute,
				TradeMultiplier: 3.0,
				MinAbsolute:     5.0,
				HysteresisRatio: 1.5,
				WalletClient:    nil,
				Address:         address,
				Logger:          logger,
			},
			wantErr: true,
			errMsg:  "wallet client cannot be nil",
		},
		{
			name: "nil-logger",
			config: &Config{
				CheckInterval:   5 * time.Minute,
				TradeMultiplier: 3.0,
				MinAbsolute:     5.0,
				HysteresisRatio: 1.5,
				WalletClient:    mockWallet,
				Address:         address,
				Logger:          nil,
			},
			wantErr: true,
			errMsg:  "logger cannot be nil",
		},
		{
			name: "zero-check-interval",
			config: &Config{
				CheckInterval:   0,
				TradeMultiplier: 3.0,
				MinAbsolute:     5.0,
				HysteresisRatio: 1.5,
				WalletClient:    mockWallet,
				Address:         address,
				Logger:          logger,
			},
			wantErr: true,
			errMsg:  "check interval must be positive",
		},
		{
			name: "zero-trade-multiplier",
			config: &Config{
				CheckInterval:   5 * time.Minute,
				TradeMultiplier: 0,
				MinAbsolute:     5.0,
				HysteresisRatio: 1.5,
				WalletClient:    mockWallet,
				Address:         address,
				Logger:          logger,
			},
			wantErr: true,
			errMsg:  "trade multiplier must be positive",
		},
		{
			name: "zero-min-absolute",
			config: &Config{
				CheckInterval:   5 * time.Minute,
				TradeMultiplier: 3.0,
				MinAbsolute:     0,
				HysteresisRatio: 1.5,
				WalletClient:    mockWallet,
				Address:         address,
				Logger:          logger,
			},
			wantErr: true,
			errMsg:  "min absolute must be positive",
		},
		{
			name: "hysteresis-ratio-less-than-one",
			config: &Config{
				CheckInterval:   5 * time.Minute,
				TradeMultiplier: 3.0,
				MinAbsolute:     5.0,
				HysteresisRatio: 0.9,
				WalletClient:    mockWallet,
				Address:         address,
				Logger:          logger,
			},
			wantErr: true,
			errMsg:  "hysteresis ratio must be >= 1.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			breaker, err := New(tt.config)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errMsg)
					return
				}
				if err.Error() != tt.errMsg {
					t.Errorf("expected error %q, got %q", tt.errMsg, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if breaker == nil {
				t.Error("expected breaker, got nil")
				return
			}

			// Verify initial state
			if !breaker.IsEnabled() {
				t.Error("expected breaker to start enabled")
			}

			// Verify thresholds initialized to min absolute
			status := breaker.GetStatus()
			if status.DisableThreshold != tt.config.MinAbsolute {
				t.Errorf("expected disable threshold %f, got %f", tt.config.MinAbsolute, status.DisableThreshold)
			}
			expectedEnable := tt.config.MinAbsolute * tt.config.HysteresisRatio
			if status.EnableThreshold != expectedEnable {
				t.Errorf("expected enable threshold %f, got %f", expectedEnable, status.EnableThreshold)
			}
		})
	}
}

// Test IsEnabled (lock-free read)
func TestIsEnabled(t *testing.T) {
	t.Parallel()

	logger := zaptest.NewLogger(t)
	mockWallet := testutil.NewMockWalletClient()
	address := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")

	breaker, err := New(&Config{
		CheckInterval:   5 * time.Minute,
		TradeMultiplier: 3.0,
		MinAbsolute:     5.0,
		HysteresisRatio: 1.5,
		WalletClient:    mockWallet,
		Address:         address,
		Logger:          logger,
	})
	if err != nil {
		t.Fatalf("failed to create breaker: %v", err)
	}

	// Should start enabled
	if !breaker.IsEnabled() {
		t.Error("expected breaker to be enabled initially")
	}

	// Manually disable
	breaker.enabled.Store(false)
	if breaker.IsEnabled() {
		t.Error("expected breaker to be disabled after Store(false)")
	}

	// Re-enable
	breaker.enabled.Store(true)
	if !breaker.IsEnabled() {
		t.Error("expected breaker to be enabled after Store(true)")
	}
}

// Test RecordTrade and threshold calculation
func TestRecordTrade(t *testing.T) {
	t.Parallel()

	logger := zaptest.NewLogger(t)
	mockWallet := testutil.NewMockWalletClient()
	address := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")

	breaker, err := New(&Config{
		CheckInterval:   5 * time.Minute,
		TradeMultiplier: 3.0,
		MinAbsolute:     5.0,
		HysteresisRatio: 1.5,
		WalletClient:    mockWallet,
		Address:         address,
		Logger:          logger,
	})
	if err != nil {
		t.Fatalf("failed to create breaker: %v", err)
	}

	tests := []struct {
		name               string
		trades             []float64
		expectedAvg        float64
		expectedDisable    float64
		expectedEnable     float64
		expectedTradeCount int
	}{
		{
			name:               "single-trade",
			trades:             []float64{10.0},
			expectedAvg:        10.0,
			expectedDisable:    30.0, // 10 * 3.0
			expectedEnable:     45.0, // 30 * 1.5
			expectedTradeCount: 1,
		},
		{
			name:               "multiple-trades",
			trades:             []float64{10.0, 20.0, 30.0},
			expectedAvg:        20.0,
			expectedDisable:    60.0, // 20 * 3.0
			expectedEnable:     90.0, // 60 * 1.5
			expectedTradeCount: 3,
		},
		{
			name:               "below-min-absolute",
			trades:             []float64{1.0}, // avg = 1.0, 1.0 * 3.0 = 3.0 < 5.0 (min absolute)
			expectedAvg:        1.0,
			expectedDisable:    5.0,  // max(3.0, 5.0) = 5.0
			expectedEnable:     7.5,  // 5.0 * 1.5
			expectedTradeCount: 1,
		},
		{
			name:               "rolling-window-20-trades",
			trades:             []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20},
			expectedAvg:        10.5,
			expectedDisable:    31.5, // 10.5 * 3.0
			expectedEnable:     47.25, // 31.5 * 1.5
			expectedTradeCount: 20,
		},
		{
			name: "rolling-window-overflow",
			trades: []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20,
				21}, // 21st trade should drop the 1st
			expectedAvg:        11.5, // avg(2..21)
			expectedDisable:    34.5, // 11.5 * 3.0
			expectedEnable:     51.75, // 34.5 * 1.5
			expectedTradeCount: 20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset breaker for each test
			breaker.mu.Lock()
			breaker.recentTrades = make([]float64, 0, 20)
			breaker.disableThreshold = 5.0
			breaker.enableThreshold = 7.5
			breaker.mu.Unlock()

			// Record trades
			for _, tradeSize := range tt.trades {
				breaker.RecordTrade(tradeSize)
			}

			// Verify status
			status := breaker.GetStatus()

			if status.AvgTradeSize < tt.expectedAvg-0.01 || status.AvgTradeSize > tt.expectedAvg+0.01 {
				t.Errorf("expected avg trade size %f, got %f", tt.expectedAvg, status.AvgTradeSize)
			}

			if status.DisableThreshold < tt.expectedDisable-0.01 || status.DisableThreshold > tt.expectedDisable+0.01 {
				t.Errorf("expected disable threshold %f, got %f", tt.expectedDisable, status.DisableThreshold)
			}

			if status.EnableThreshold < tt.expectedEnable-0.01 || status.EnableThreshold > tt.expectedEnable+0.01 {
				t.Errorf("expected enable threshold %f, got %f", tt.expectedEnable, status.EnableThreshold)
			}

			if status.RecentTradeCount != tt.expectedTradeCount {
				t.Errorf("expected %d trades, got %d", tt.expectedTradeCount, status.RecentTradeCount)
			}
		})
	}
}

// Test RecordTrade with invalid values
func TestRecordTrade_InvalidValues(t *testing.T) {
	t.Parallel()

	logger := zaptest.NewLogger(t)
	mockWallet := testutil.NewMockWalletClient()
	address := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")

	breaker, err := New(&Config{
		CheckInterval:   5 * time.Minute,
		TradeMultiplier: 3.0,
		MinAbsolute:     5.0,
		HysteresisRatio: 1.5,
		WalletClient:    mockWallet,
		Address:         address,
		Logger:          logger,
	})
	if err != nil {
		t.Fatalf("failed to create breaker: %v", err)
	}

	// Record invalid trades (should be ignored)
	breaker.RecordTrade(0)
	breaker.RecordTrade(-10.0)

	status := breaker.GetStatus()
	if status.RecentTradeCount != 0 {
		t.Errorf("expected 0 trades after invalid inputs, got %d", status.RecentTradeCount)
	}
}

// Test CheckBalance with various scenarios
func TestCheckBalance(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		initialEnabled   bool
		usdcBalance      float64 // dollars
		tradeHistory     []float64
		expectEnabled    bool
		expectStateChange bool
	}{
		{
			name:              "sufficient-balance-stay-enabled",
			initialEnabled:    true,
			usdcBalance:       100.0,
			tradeHistory:      []float64{10.0},
			expectEnabled:     true,
			expectStateChange: false,
		},
		{
			name:              "low-balance-disable",
			initialEnabled:    true,
			usdcBalance:       20.0,
			tradeHistory:      []float64{10.0}, // disable threshold = 30.0
			expectEnabled:     false,
			expectStateChange: true,
		},
		{
			name:              "at-disable-threshold-stay-enabled",
			initialEnabled:    true,
			usdcBalance:       30.0,
			tradeHistory:      []float64{10.0}, // disable threshold = 30.0 (NOT <, so stays enabled)
			expectEnabled:     true,
			expectStateChange: false,
		},
		{
			name:              "below-disable-threshold",
			initialEnabled:    true,
			usdcBalance:       29.99,
			tradeHistory:      []float64{10.0}, // disable threshold = 30.0
			expectEnabled:     false,
			expectStateChange: true,
		},
		{
			name:              "disabled-stays-disabled",
			initialEnabled:    false,
			usdcBalance:       40.0,
			tradeHistory:      []float64{10.0}, // enable threshold = 45.0
			expectEnabled:     false,
			expectStateChange: false,
		},
		{
			name:              "re-enable-above-threshold",
			initialEnabled:    false,
			usdcBalance:       50.0,
			tradeHistory:      []float64{10.0}, // enable threshold = 45.0
			expectEnabled:     true,
			expectStateChange: true,
		},
		{
			name:              "at-enable-threshold-re-enables",
			initialEnabled:    false,
			usdcBalance:       45.0,
			tradeHistory:      []float64{10.0}, // enable threshold = 45.0 (>=, so enables)
			expectEnabled:     true,
			expectStateChange: true,
		},
		{
			name:              "hysteresis-prevents-flapping",
			initialEnabled:    false,
			usdcBalance:       35.0,
			tradeHistory:      []float64{10.0}, // disable=30.0, enable=45.0 (35 is in between)
			expectEnabled:     false,
			expectStateChange: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := zaptest.NewLogger(t)
			mockWallet := testutil.NewMockWalletClient()
			address := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")

			// Set mock balance
			mockWallet.SetUSDCBalance(testutil.NewUSDCBigInt(tt.usdcBalance))

			breaker, err := New(&Config{
				CheckInterval:   5 * time.Minute,
				TradeMultiplier: 3.0,
				MinAbsolute:     5.0,
				HysteresisRatio: 1.5,
				WalletClient:    mockWallet,
				Address:         address,
				Logger:          logger,
			})
			if err != nil {
				t.Fatalf("failed to create breaker: %v", err)
			}

			// Record trade history
			for _, tradeSize := range tt.tradeHistory {
				breaker.RecordTrade(tradeSize)
			}

			// Set initial state
			breaker.enabled.Store(tt.initialEnabled)

			// Check balance
			ctx := context.Background()
			err = breaker.CheckBalance(ctx)
			if err != nil {
				t.Fatalf("CheckBalance failed: %v", err)
			}

			// Verify final state
			if breaker.IsEnabled() != tt.expectEnabled {
				t.Errorf("expected enabled=%v, got %v", tt.expectEnabled, breaker.IsEnabled())
			}

			// Verify balance was recorded
			status := breaker.GetStatus()
			if status.LastBalance < tt.usdcBalance-0.01 || status.LastBalance > tt.usdcBalance+0.01 {
				t.Errorf("expected last balance %f, got %f", tt.usdcBalance, status.LastBalance)
			}
		})
	}
}

// Test CheckBalance with wallet client error
func TestCheckBalance_WalletError(t *testing.T) {
	t.Parallel()

	logger := zaptest.NewLogger(t)
	mockWallet := testutil.NewMockWalletClient()
	address := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")

	// Set mock to return error
	mockWallet.SetGetBalancesError(errors.New("RPC connection failed"))

	breaker, err := New(&Config{
		CheckInterval:   5 * time.Minute,
		TradeMultiplier: 3.0,
		MinAbsolute:     5.0,
		HysteresisRatio: 1.5,
		WalletClient:    mockWallet,
		Address:         address,
		Logger:          logger,
	})
	if err != nil {
		t.Fatalf("failed to create breaker: %v", err)
	}

	ctx := context.Background()
	err = breaker.CheckBalance(ctx)
	if err == nil {
		t.Error("expected error from CheckBalance, got nil")
	}

	// Breaker should remain in current state (enabled) on error
	if !breaker.IsEnabled() {
		t.Error("expected breaker to remain enabled after error")
	}
}

// Test Start and monitorLoop
func TestStart_MonitorLoop(t *testing.T) {
	t.Parallel()

	logger := zaptest.NewLogger(t)
	mockWallet := testutil.NewMockWalletClient()
	address := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")

	// Set initial balance
	mockWallet.SetUSDCBalance(testutil.NewUSDCBigInt(100.0))

	breaker, err := New(&Config{
		CheckInterval:   100 * time.Millisecond, // Short interval for testing
		TradeMultiplier: 3.0,
		MinAbsolute:     5.0,
		HysteresisRatio: 1.5,
		WalletClient:    mockWallet,
		Address:         address,
		Logger:          logger,
	})
	if err != nil {
		t.Fatalf("failed to create breaker: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Start monitoring
	breaker.Start(ctx)

	// Wait for a few checks
	time.Sleep(350 * time.Millisecond)

	// Verify balance was checked (lastCheck should be recent)
	status := breaker.GetStatus()
	if status.LastCheck.IsZero() {
		t.Error("expected lastCheck to be set")
	}
	if status.LastBalance != 100.0 {
		t.Errorf("expected balance 100.0, got %f", status.LastBalance)
	}

	// Should still be enabled
	if !breaker.IsEnabled() {
		t.Error("expected breaker to be enabled")
	}

	// Wait for context cancellation
	<-ctx.Done()

	// Give goroutine time to stop
	time.Sleep(50 * time.Millisecond)
}

// Test context cancellation stops monitoring
func TestStart_ContextCancellation(t *testing.T) {
	t.Parallel()

	logger := zaptest.NewLogger(t)
	mockWallet := testutil.NewMockWalletClient()
	address := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")

	mockWallet.SetUSDCBalance(testutil.NewUSDCBigInt(100.0))

	breaker, err := New(&Config{
		CheckInterval:   50 * time.Millisecond,
		TradeMultiplier: 3.0,
		MinAbsolute:     5.0,
		HysteresisRatio: 1.5,
		WalletClient:    mockWallet,
		Address:         address,
		Logger:          logger,
	})
	if err != nil {
		t.Fatalf("failed to create breaker: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Start monitoring
	breaker.Start(ctx)

	// Let it run for a bit
	time.Sleep(150 * time.Millisecond)

	// Cancel context
	cancel()

	// Give goroutine time to stop
	time.Sleep(100 * time.Millisecond)

	// Monitoring should have stopped (no way to verify directly, but no panic = success)
}

// Test GetStatus
func TestGetStatus(t *testing.T) {
	t.Parallel()

	logger := zaptest.NewLogger(t)
	mockWallet := testutil.NewMockWalletClient()
	address := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")

	breaker, err := New(&Config{
		CheckInterval:   5 * time.Minute,
		TradeMultiplier: 3.0,
		MinAbsolute:     5.0,
		HysteresisRatio: 1.5,
		WalletClient:    mockWallet,
		Address:         address,
		Logger:          logger,
	})
	if err != nil {
		t.Fatalf("failed to create breaker: %v", err)
	}

	// Initial status
	status := breaker.GetStatus()
	if !status.Enabled {
		t.Error("expected initial status to be enabled")
	}
	if status.RecentTradeCount != 0 {
		t.Errorf("expected 0 trades, got %d", status.RecentTradeCount)
	}
	if status.AvgTradeSize != 0.0 {
		t.Errorf("expected 0 avg trade size, got %f", status.AvgTradeSize)
	}

	// Record trades
	breaker.RecordTrade(10.0)
	breaker.RecordTrade(20.0)

	status = breaker.GetStatus()
	if status.RecentTradeCount != 2 {
		t.Errorf("expected 2 trades, got %d", status.RecentTradeCount)
	}
	if status.AvgTradeSize != 15.0 {
		t.Errorf("expected avg 15.0, got %f", status.AvgTradeSize)
	}
}

// Test full lifecycle integration
func TestIntegration_FullLifecycle(t *testing.T) {
	t.Parallel()

	logger := zaptest.NewLogger(t)
	mockWallet := testutil.NewMockWalletClient()
	address := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")

	// Start with high balance
	mockWallet.SetUSDCBalance(testutil.NewUSDCBigInt(100.0))

	breaker, err := New(&Config{
		CheckInterval:   100 * time.Millisecond,
		TradeMultiplier: 3.0,
		MinAbsolute:     5.0,
		HysteresisRatio: 1.5,
		WalletClient:    mockWallet,
		Address:         address,
		Logger:          logger,
	})
	if err != nil {
		t.Fatalf("failed to create breaker: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start monitoring
	breaker.Start(ctx)

	// Should start enabled
	if !breaker.IsEnabled() {
		t.Error("expected breaker to start enabled")
	}

	// Record trades
	breaker.RecordTrade(10.0)
	// Disable threshold is now 30.0, enable threshold is 45.0

	// Reduce balance below disable threshold
	mockWallet.SetUSDCBalance(testutil.NewUSDCBigInt(25.0))

	// Wait for monitor to detect low balance
	time.Sleep(250 * time.Millisecond)

	// Should now be disabled
	if breaker.IsEnabled() {
		t.Error("expected breaker to be disabled after low balance")
	}

	// Increase balance above enable threshold
	mockWallet.SetUSDCBalance(testutil.NewUSDCBigInt(50.0))

	// Wait for monitor to detect recovery
	time.Sleep(250 * time.Millisecond)

	// Should be re-enabled
	if !breaker.IsEnabled() {
		t.Error("expected breaker to be re-enabled after balance recovery")
	}

	// Verify final status
	status := breaker.GetStatus()
	if status.RecentTradeCount != 1 {
		t.Errorf("expected 1 trade, got %d", status.RecentTradeCount)
	}
	if status.AvgTradeSize != 10.0 {
		t.Errorf("expected avg 10.0, got %f", status.AvgTradeSize)
	}
	if status.LastBalance != 50.0 {
		t.Errorf("expected last balance 50.0, got %f", status.LastBalance)
	}
}

// Test concurrent access (race detector)
func TestConcurrentAccess(t *testing.T) {
	t.Parallel()

	logger := zaptest.NewLogger(t)
	mockWallet := testutil.NewMockWalletClient()
	address := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")

	mockWallet.SetUSDCBalance(testutil.NewUSDCBigInt(100.0))

	breaker, err := New(&Config{
		CheckInterval:   50 * time.Millisecond,
		TradeMultiplier: 3.0,
		MinAbsolute:     5.0,
		HysteresisRatio: 1.5,
		WalletClient:    mockWallet,
		Address:         address,
		Logger:          logger,
	})
	if err != nil {
		t.Fatalf("failed to create breaker: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Start monitoring (reads/writes state)
	breaker.Start(ctx)

	// Concurrently record trades (writes state)
	go func() {
		for i := 0; i < 10; i++ {
			breaker.RecordTrade(float64(i + 1))
			time.Sleep(10 * time.Millisecond)
		}
	}()

	// Concurrently check status (reads state)
	go func() {
		for i := 0; i < 10; i++ {
			_ = breaker.GetStatus()
			time.Sleep(15 * time.Millisecond)
		}
	}()

	// Concurrently check enabled state (lock-free read)
	go func() {
		for i := 0; i < 20; i++ {
			_ = breaker.IsEnabled()
			time.Sleep(5 * time.Millisecond)
		}
	}()

	// Wait for timeout
	<-ctx.Done()

	// Give goroutines time to finish
	time.Sleep(100 * time.Millisecond)

	// No race conditions = success (checked by go test -race)
}

// Benchmark IsEnabled (hot path)
func BenchmarkIsEnabled(b *testing.B) {
	logger := zaptest.NewLogger(b)
	mockWallet := testutil.NewMockWalletClient()
	address := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")

	breaker, err := New(&Config{
		CheckInterval:   5 * time.Minute,
		TradeMultiplier: 3.0,
		MinAbsolute:     5.0,
		HysteresisRatio: 1.5,
		WalletClient:    mockWallet,
		Address:         address,
		Logger:          logger,
	})
	if err != nil {
		b.Fatalf("failed to create breaker: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = breaker.IsEnabled()
	}
}

// Benchmark RecordTrade
func BenchmarkRecordTrade(b *testing.B) {
	logger := zaptest.NewLogger(b)
	mockWallet := testutil.NewMockWalletClient()
	address := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")

	breaker, err := New(&Config{
		CheckInterval:   5 * time.Minute,
		TradeMultiplier: 3.0,
		MinAbsolute:     5.0,
		HysteresisRatio: 1.5,
		WalletClient:    mockWallet,
		Address:         address,
		Logger:          logger,
	})
	if err != nil {
		b.Fatalf("failed to create breaker: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		breaker.RecordTrade(10.0)
	}
}
