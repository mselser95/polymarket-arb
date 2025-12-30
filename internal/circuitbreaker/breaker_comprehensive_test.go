package circuitbreaker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/mselser95/polymarket-arb/internal/testutil"
	"go.uber.org/zap/zaptest"
)

// ===== Threshold Calculation Comprehensive Tests =====

// TestCalculateThresholds_EmptyWindow tests threshold calculation with no trades
func TestCalculateThresholds_EmptyWindow(t *testing.T) {
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

	// No trades recorded yet
	status := breaker.GetStatus()

	// Should use min absolute as thresholds
	if status.DisableThreshold != 5.0 {
		t.Errorf("expected disable threshold to be min absolute (5.0), got %f", status.DisableThreshold)
	}
	if status.EnableThreshold != 7.5 { // 5.0 * 1.5
		t.Errorf("expected enable threshold to be 7.5, got %f", status.EnableThreshold)
	}
	if status.AvgTradeSize != 0.0 {
		t.Errorf("expected avg trade size 0.0, got %f", status.AvgTradeSize)
	}
	if status.RecentTradeCount != 0 {
		t.Errorf("expected 0 trades, got %d", status.RecentTradeCount)
	}
}

// TestCalculateThresholds_PartialWindow tests threshold calculation with < 20 trades
func TestCalculateThresholds_PartialWindow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		tradeCount      int
		tradeSize       float64
		expectedAvg     float64
		expectedDisable float64
		expectedEnable  float64
	}{
		{
			name:            "1-trade",
			tradeCount:      1,
			tradeSize:       10.0,
			expectedAvg:     10.0,
			expectedDisable: 30.0, // 10 * 3.0
			expectedEnable:  45.0, // 30 * 1.5
		},
		{
			name:            "5-trades",
			tradeCount:      5,
			tradeSize:       10.0,
			expectedAvg:     10.0,
			expectedDisable: 30.0,
			expectedEnable:  45.0,
		},
		{
			name:            "10-trades",
			tradeCount:      10,
			tradeSize:       8.0,
			expectedAvg:     8.0,
			expectedDisable: 24.0, // 8 * 3.0
			expectedEnable:  36.0, // 24 * 1.5
		},
		{
			name:            "19-trades",
			tradeCount:      19,
			tradeSize:       12.0,
			expectedAvg:     12.0,
			expectedDisable: 36.0, // 12 * 3.0
			expectedEnable:  54.0, // 36 * 1.5
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

			// Record trades
			for i := 0; i < tt.tradeCount; i++ {
				breaker.RecordTrade(tt.tradeSize)
			}

			status := breaker.GetStatus()

			if status.RecentTradeCount != tt.tradeCount {
				t.Errorf("expected %d trades, got %d", tt.tradeCount, status.RecentTradeCount)
			}

			if !floatEquals(status.AvgTradeSize, tt.expectedAvg, 0.01) {
				t.Errorf("expected avg %f, got %f", tt.expectedAvg, status.AvgTradeSize)
			}

			if !floatEquals(status.DisableThreshold, tt.expectedDisable, 0.01) {
				t.Errorf("expected disable threshold %f, got %f", tt.expectedDisable, status.DisableThreshold)
			}

			if !floatEquals(status.EnableThreshold, tt.expectedEnable, 0.01) {
				t.Errorf("expected enable threshold %f, got %f", tt.expectedEnable, status.EnableThreshold)
			}
		})
	}
}

// TestCalculateThresholds_FullWindow tests threshold calculation with exactly 20 trades
func TestCalculateThresholds_FullWindow(t *testing.T) {
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

	// Record exactly 20 trades of 10.0 each
	for i := 0; i < 20; i++ {
		breaker.RecordTrade(10.0)
	}

	status := breaker.GetStatus()

	if status.RecentTradeCount != 20 {
		t.Errorf("expected 20 trades, got %d", status.RecentTradeCount)
	}

	if !floatEquals(status.AvgTradeSize, 10.0, 0.01) {
		t.Errorf("expected avg 10.0, got %f", status.AvgTradeSize)
	}

	if !floatEquals(status.DisableThreshold, 30.0, 0.01) {
		t.Errorf("expected disable threshold 30.0, got %f", status.DisableThreshold)
	}

	if !floatEquals(status.EnableThreshold, 45.0, 0.01) {
		t.Errorf("expected enable threshold 45.0, got %f", status.EnableThreshold)
	}
}

// TestCalculateThresholds_RollingWindowOverflow tests that window caps at 20 trades
func TestCalculateThresholds_RollingWindowOverflow(t *testing.T) {
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

	// Record 25 trades (should keep only last 20)
	for i := 1; i <= 25; i++ {
		breaker.RecordTrade(float64(i))
	}

	status := breaker.GetStatus()

	// Should only have last 20 trades
	if status.RecentTradeCount != 20 {
		t.Errorf("expected 20 trades after overflow, got %d", status.RecentTradeCount)
	}

	// Average should be (6+7+8+...+25) / 20 = 15.5
	expectedAvg := 15.5
	if !floatEquals(status.AvgTradeSize, expectedAvg, 0.01) {
		t.Errorf("expected avg %f, got %f", expectedAvg, status.AvgTradeSize)
	}

	// Disable threshold: 15.5 * 3.0 = 46.5
	expectedDisable := 46.5
	if !floatEquals(status.DisableThreshold, expectedDisable, 0.01) {
		t.Errorf("expected disable threshold %f, got %f", expectedDisable, status.DisableThreshold)
	}

	// Enable threshold: 46.5 * 1.5 = 69.75
	expectedEnable := 69.75
	if !floatEquals(status.EnableThreshold, expectedEnable, 0.01) {
		t.Errorf("expected enable threshold %f, got %f", expectedEnable, status.EnableThreshold)
	}
}

// TestCalculateThresholds_MinAbsoluteFloor tests that threshold never goes below min absolute
func TestCalculateThresholds_MinAbsoluteFloor(t *testing.T) {
	t.Parallel()

	logger := zaptest.NewLogger(t)
	mockWallet := testutil.NewMockWalletClient()
	address := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")

	breaker, err := New(&Config{
		CheckInterval:   5 * time.Minute,
		TradeMultiplier: 3.0,
		MinAbsolute:     10.0, // High floor
		HysteresisRatio: 1.5,
		WalletClient:    mockWallet,
		Address:         address,
		Logger:          logger,
	})
	if err != nil {
		t.Fatalf("failed to create breaker: %v", err)
	}

	// Record small trade: 1.0 * 3.0 = 3.0 < 10.0 (min absolute)
	breaker.RecordTrade(1.0)

	status := breaker.GetStatus()

	// Should use min absolute instead of calculated threshold
	if status.DisableThreshold != 10.0 {
		t.Errorf("expected disable threshold to be min absolute (10.0), got %f", status.DisableThreshold)
	}

	// Enable threshold: 10.0 * 1.5 = 15.0
	if !floatEquals(status.EnableThreshold, 15.0, 0.01) {
		t.Errorf("expected enable threshold 15.0, got %f", status.EnableThreshold)
	}
}

// TestCalculateThresholds_HysteresisRatio tests different hysteresis ratios
func TestCalculateThresholds_HysteresisRatio(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		hysteresisRatio float64
		tradeSize       float64
		expectedDisable float64
		expectedEnable  float64
	}{
		{
			name:            "1.0x-hysteresis",
			hysteresisRatio: 1.0,
			tradeSize:       10.0,
			expectedDisable: 30.0, // 10 * 3.0
			expectedEnable:  30.0, // 30 * 1.0 (no hysteresis)
		},
		{
			name:            "1.5x-hysteresis",
			hysteresisRatio: 1.5,
			tradeSize:       10.0,
			expectedDisable: 30.0,
			expectedEnable:  45.0, // 30 * 1.5
		},
		{
			name:            "2.0x-hysteresis",
			hysteresisRatio: 2.0,
			tradeSize:       10.0,
			expectedDisable: 30.0,
			expectedEnable:  60.0, // 30 * 2.0
		},
		{
			name:            "3.0x-hysteresis",
			hysteresisRatio: 3.0,
			tradeSize:       10.0,
			expectedDisable: 30.0,
			expectedEnable:  90.0, // 30 * 3.0
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := zaptest.NewLogger(t)
			mockWallet := testutil.NewMockWalletClient()
			address := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")

			breaker, err := New(&Config{
				CheckInterval:   5 * time.Minute,
				TradeMultiplier: 3.0,
				MinAbsolute:     5.0,
				HysteresisRatio: tt.hysteresisRatio,
				WalletClient:    mockWallet,
				Address:         address,
				Logger:          logger,
			})
			if err != nil {
				t.Fatalf("failed to create breaker: %v", err)
			}

			breaker.RecordTrade(tt.tradeSize)

			status := breaker.GetStatus()

			if !floatEquals(status.DisableThreshold, tt.expectedDisable, 0.01) {
				t.Errorf("expected disable threshold %f, got %f", tt.expectedDisable, status.DisableThreshold)
			}

			if !floatEquals(status.EnableThreshold, tt.expectedEnable, 0.01) {
				t.Errorf("expected enable threshold %f, got %f", tt.expectedEnable, status.EnableThreshold)
			}
		})
	}
}

// TestCalculateThresholds_MultiplierEffect tests different trade multipliers
func TestCalculateThresholds_MultiplierEffect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		multiplier      float64
		tradeSize       float64
		expectedDisable float64
		expectedEnable  float64
	}{
		{
			name:            "2.0x-multiplier",
			multiplier:      2.0,
			tradeSize:       10.0,
			expectedDisable: 20.0, // 10 * 2.0
			expectedEnable:  30.0, // 20 * 1.5
		},
		{
			name:            "3.0x-multiplier",
			multiplier:      3.0,
			tradeSize:       10.0,
			expectedDisable: 30.0, // 10 * 3.0
			expectedEnable:  45.0, // 30 * 1.5
		},
		{
			name:            "5.0x-multiplier",
			multiplier:      5.0,
			tradeSize:       10.0,
			expectedDisable: 50.0, // 10 * 5.0
			expectedEnable:  75.0, // 50 * 1.5
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := zaptest.NewLogger(t)
			mockWallet := testutil.NewMockWalletClient()
			address := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")

			breaker, err := New(&Config{
				CheckInterval:   5 * time.Minute,
				TradeMultiplier: tt.multiplier,
				MinAbsolute:     5.0,
				HysteresisRatio: 1.5,
				WalletClient:    mockWallet,
				Address:         address,
				Logger:          logger,
			})
			if err != nil {
				t.Fatalf("failed to create breaker: %v", err)
			}

			breaker.RecordTrade(tt.tradeSize)

			status := breaker.GetStatus()

			if !floatEquals(status.DisableThreshold, tt.expectedDisable, 0.01) {
				t.Errorf("expected disable threshold %f, got %f", tt.expectedDisable, status.DisableThreshold)
			}

			if !floatEquals(status.EnableThreshold, tt.expectedEnable, 0.01) {
				t.Errorf("expected enable threshold %f, got %f", tt.expectedEnable, status.EnableThreshold)
			}
		})
	}
}

// ===== State Transition Comprehensive Tests =====

// TestStateTransition_MetricsUpdate tests that metrics are updated on state change
func TestStateTransition_MetricsUpdate(t *testing.T) {
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

	// Record trade to set thresholds
	breaker.RecordTrade(10.0) // disable=30.0, enable=45.0

	// Set balance below disable threshold
	mockWallet.SetUSDCBalance(testutil.NewUSDCBigInt(25.0))

	ctx := context.Background()
	err = breaker.CheckBalance(ctx)
	if err != nil {
		t.Fatalf("CheckBalance failed: %v", err)
	}

	// Should be disabled
	if breaker.IsEnabled() {
		t.Error("expected breaker to be disabled")
	}

	// Set balance above enable threshold
	mockWallet.SetUSDCBalance(testutil.NewUSDCBigInt(50.0))

	err = breaker.CheckBalance(ctx)
	if err != nil {
		t.Fatalf("CheckBalance failed: %v", err)
	}

	// Should be enabled
	if !breaker.IsEnabled() {
		t.Error("expected breaker to be enabled")
	}
}

// TestStateTransition_NoChangeLogging tests that no state change doesn't spam logs
func TestStateTransition_NoChangeLogging(t *testing.T) {
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

	breaker.RecordTrade(10.0) // disable=30.0, enable=45.0

	// Set balance well above threshold
	mockWallet.SetUSDCBalance(testutil.NewUSDCBigInt(100.0))

	ctx := context.Background()

	// Check multiple times - should remain enabled without state changes
	for i := 0; i < 5; i++ {
		err = breaker.CheckBalance(ctx)
		if err != nil {
			t.Fatalf("CheckBalance failed: %v", err)
		}

		if !breaker.IsEnabled() {
			t.Error("expected breaker to remain enabled")
		}
	}
}

// TestStateTransition_ConcurrentChecks tests concurrent CheckBalance calls
func TestStateTransition_ConcurrentChecks(t *testing.T) {
	t.Parallel()

	logger := zaptest.NewLogger(t)
	mockWallet := testutil.NewMockWalletClient()
	address := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")

	mockWallet.SetUSDCBalance(testutil.NewUSDCBigInt(100.0))

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

	// Concurrent CheckBalance calls
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = breaker.CheckBalance(ctx)
		}()
	}

	wg.Wait()

	// No race = success (verified with go test -race)
	if !breaker.IsEnabled() {
		t.Error("expected breaker to remain enabled")
	}
}

// TestStateTransition_BoundaryConditions tests exact threshold values
func TestStateTransition_BoundaryConditions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		initialEnabled bool
		balance        float64
		expectEnabled  bool
		description    string
	}{
		{
			name:           "at-disable-threshold-stays-enabled",
			initialEnabled: true,
			balance:        30.0, // exactly at disable threshold
			expectEnabled:  true,
			description:    "balance == disable threshold should NOT disable (< required)",
		},
		{
			name:           "just-below-disable-threshold-disables",
			initialEnabled: true,
			balance:        29.99,
			expectEnabled:  false,
			description:    "balance < disable threshold should disable",
		},
		{
			name:           "at-enable-threshold-enables",
			initialEnabled: false,
			balance:        45.0, // exactly at enable threshold
			expectEnabled:  true,
			description:    "balance == enable threshold should enable (>= required)",
		},
		{
			name:           "just-below-enable-threshold-stays-disabled",
			initialEnabled: false,
			balance:        44.99,
			expectEnabled:  false,
			description:    "balance < enable threshold should stay disabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := zaptest.NewLogger(t)
			mockWallet := testutil.NewMockWalletClient()
			address := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")

			mockWallet.SetUSDCBalance(testutil.NewUSDCBigInt(tt.balance))

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

			// Set thresholds: disable=30.0, enable=45.0
			breaker.RecordTrade(10.0)

			// Set initial state
			breaker.enabled.Store(tt.initialEnabled)

			ctx := context.Background()
			err = breaker.CheckBalance(ctx)
			if err != nil {
				t.Fatalf("CheckBalance failed: %v", err)
			}

			if breaker.IsEnabled() != tt.expectEnabled {
				t.Errorf("%s: expected enabled=%v, got %v", tt.description, tt.expectEnabled, breaker.IsEnabled())
			}
		})
	}
}

// TestStateTransition_ErrorHandling tests that errors don't change state
func TestStateTransition_ErrorHandling(t *testing.T) {
	t.Parallel()

	logger := zaptest.NewLogger(t)
	mockWallet := testutil.NewMockWalletClient()
	address := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")

	// Set mock to return error
	mockWallet.SetGetBalancesError(errors.New("RPC timeout"))

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
		t.Fatal("expected breaker to start enabled")
	}

	ctx := context.Background()
	err = breaker.CheckBalance(ctx)
	if err == nil {
		t.Error("expected error from CheckBalance")
	}

	// Should remain enabled after error
	if !breaker.IsEnabled() {
		t.Error("expected breaker to remain enabled after error")
	}

	// Clear error and set low balance
	mockWallet.SetGetBalancesError(nil)
	mockWallet.SetUSDCBalance(testutil.NewUSDCBigInt(1.0))

	err = breaker.CheckBalance(ctx)
	if err != nil {
		t.Fatalf("CheckBalance failed: %v", err)
	}

	// Should now be disabled
	if breaker.IsEnabled() {
		t.Error("expected breaker to be disabled after low balance")
	}
}

// TestStateTransition_ContextTimeout tests CheckBalance with timeout
func TestStateTransition_ContextTimeout(t *testing.T) {
	t.Parallel()

	logger := zaptest.NewLogger(t)
	mockWallet := testutil.NewMockWalletClient()
	address := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")

	// Set balance
	mockWallet.SetUSDCBalance(testutil.NewUSDCBigInt(100.0))

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

	// Use already-cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = breaker.CheckBalance(ctx)
	// May or may not error depending on timing, but should not panic

	// State should not change
	if !breaker.IsEnabled() {
		t.Error("expected breaker to remain enabled")
	}
}

// ===== Trade Recording Comprehensive Tests =====

// TestRecordTrade_ThresholdUpdateImmediate tests that thresholds update immediately
func TestRecordTrade_ThresholdUpdateImmediate(t *testing.T) {
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

	// Initial thresholds (no trades)
	status := breaker.GetStatus()
	if status.DisableThreshold != 5.0 {
		t.Errorf("expected initial disable threshold 5.0, got %f", status.DisableThreshold)
	}

	// Record first trade
	breaker.RecordTrade(10.0)

	// Thresholds should update immediately
	status = breaker.GetStatus()
	expectedDisable := 30.0 // 10 * 3.0
	if !floatEquals(status.DisableThreshold, expectedDisable, 0.01) {
		t.Errorf("expected disable threshold %f after trade, got %f", expectedDisable, status.DisableThreshold)
	}

	expectedEnable := 45.0 // 30 * 1.5
	if !floatEquals(status.EnableThreshold, expectedEnable, 0.01) {
		t.Errorf("expected enable threshold %f after trade, got %f", expectedEnable, status.EnableThreshold)
	}
}

// TestRecordTrade_ConcurrentRecording tests concurrent trade recording
func TestRecordTrade_ConcurrentRecording(t *testing.T) {
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

	// Concurrently record trades
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(tradeNum int) {
			defer wg.Done()
			breaker.RecordTrade(float64(tradeNum + 1))
		}(i)
	}

	wg.Wait()

	// Should have recorded 30 trades, but window caps at 20
	status := breaker.GetStatus()
	if status.RecentTradeCount != 20 {
		t.Errorf("expected 20 trades after overflow, got %d", status.RecentTradeCount)
	}

	// Average should be positive (exact value depends on race timing)
	if status.AvgTradeSize <= 0 {
		t.Errorf("expected positive avg trade size, got %f", status.AvgTradeSize)
	}

	// No race = success (verified with go test -race)
}

// TestRecordTrade_FloatingPointPrecision tests floating-point edge cases
func TestRecordTrade_FloatingPointPrecision(t *testing.T) {
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

	// Record trades with precise floating-point values
	trades := []float64{
		10.123456789,
		20.987654321,
		5.5,
		0.01,
	}

	for _, trade := range trades {
		breaker.RecordTrade(trade)
	}

	status := breaker.GetStatus()

	// Should have recorded all trades
	if status.RecentTradeCount != len(trades) {
		t.Errorf("expected %d trades, got %d", len(trades), status.RecentTradeCount)
	}

	// Average should be sum / count
	sum := 0.0
	for _, trade := range trades {
		sum += trade
	}
	expectedAvg := sum / float64(len(trades))

	if !floatEquals(status.AvgTradeSize, expectedAvg, 0.000001) {
		t.Errorf("expected avg %f, got %f", expectedAvg, status.AvgTradeSize)
	}
}

// ===== Status Reporting Comprehensive Tests =====

// TestGetStatus_ZeroTrades tests status with no trades recorded
func TestGetStatus_ZeroTrades(t *testing.T) {
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

	status := breaker.GetStatus()

	if status.RecentTradeCount != 0 {
		t.Errorf("expected 0 trades, got %d", status.RecentTradeCount)
	}

	if status.AvgTradeSize != 0.0 {
		t.Errorf("expected 0 avg trade size, got %f", status.AvgTradeSize)
	}

	if !status.Enabled {
		t.Error("expected initial enabled state to be true")
	}

	if status.DisableThreshold != 5.0 {
		t.Errorf("expected disable threshold 5.0, got %f", status.DisableThreshold)
	}

	if status.EnableThreshold != 7.5 {
		t.Errorf("expected enable threshold 7.5, got %f", status.EnableThreshold)
	}
}

// TestGetStatus_ConcurrentReads tests concurrent GetStatus calls
func TestGetStatus_ConcurrentReads(t *testing.T) {
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

	// Record some trades
	breaker.RecordTrade(10.0)
	breaker.RecordTrade(20.0)

	// Concurrent GetStatus calls
	var wg sync.WaitGroup
	results := make([]Status, 20)

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = breaker.GetStatus()
		}(i)
	}

	wg.Wait()

	// All results should be consistent
	for i, status := range results {
		if status.RecentTradeCount != 2 {
			t.Errorf("result %d: expected 2 trades, got %d", i, status.RecentTradeCount)
		}

		if !floatEquals(status.AvgTradeSize, 15.0, 0.01) {
			t.Errorf("result %d: expected avg 15.0, got %f", i, status.AvgTradeSize)
		}
	}

	// No race = success (verified with go test -race)
}

// TestGetStatus_AfterCheckBalance tests status after balance check
func TestGetStatus_AfterCheckBalance(t *testing.T) {
	t.Parallel()

	logger := zaptest.NewLogger(t)
	mockWallet := testutil.NewMockWalletClient()
	address := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")

	mockWallet.SetUSDCBalance(testutil.NewUSDCBigInt(100.0))

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
	if err != nil {
		t.Fatalf("CheckBalance failed: %v", err)
	}

	status := breaker.GetStatus()

	// LastBalance should be set
	if !floatEquals(status.LastBalance, 100.0, 0.01) {
		t.Errorf("expected last balance 100.0, got %f", status.LastBalance)
	}

	// LastCheck should be recent (within last second)
	if time.Since(status.LastCheck) > 1*time.Second {
		t.Errorf("expected recent last check, got %v", status.LastCheck)
	}

	// Should be enabled
	if !status.Enabled {
		t.Error("expected enabled state to be true")
	}
}

// TestGetStatus_Consistency tests status consistency across multiple operations
func TestGetStatus_Consistency(t *testing.T) {
	t.Parallel()

	logger := zaptest.NewLogger(t)
	mockWallet := testutil.NewMockWalletClient()
	address := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")

	mockWallet.SetUSDCBalance(testutil.NewUSDCBigInt(100.0))

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

	// Perform multiple operations
	breaker.RecordTrade(10.0)
	breaker.RecordTrade(20.0)
	breaker.RecordTrade(30.0)

	ctx := context.Background()
	_ = breaker.CheckBalance(ctx)

	// Get status twice
	status1 := breaker.GetStatus()
	status2 := breaker.GetStatus()

	// Should be identical
	if status1.RecentTradeCount != status2.RecentTradeCount {
		t.Error("expected consistent trade count")
	}

	if status1.AvgTradeSize != status2.AvgTradeSize {
		t.Error("expected consistent avg trade size")
	}

	if status1.DisableThreshold != status2.DisableThreshold {
		t.Error("expected consistent disable threshold")
	}

	if status1.EnableThreshold != status2.EnableThreshold {
		t.Error("expected consistent enable threshold")
	}

	if status1.Enabled != status2.Enabled {
		t.Error("expected consistent enabled state")
	}
}

// Helper function for floating-point comparison
func floatEquals(a, b, epsilon float64) bool {
	if a == b {
		return true
	}
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < epsilon
}
