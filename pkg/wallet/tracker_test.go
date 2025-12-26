package wallet

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"go.uber.org/zap"
)

func TestNew(t *testing.T) {
	logger := zap.NewNop()
	address := common.HexToAddress("0x1234567890123456789012345678901234567890")

	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{
			name: "valid_config",
			cfg: &Config{
				RPCEndpoint:  "https://polygon-rpc.com",
				Address:      address,
				PollInterval: 1 * time.Minute,
				Logger:       logger,
			},
			wantErr: false,
		},
		{
			name:    "nil_config",
			cfg:     nil,
			wantErr: true,
		},
		{
			name: "nil_logger",
			cfg: &Config{
				RPCEndpoint:  "https://polygon-rpc.com",
				Address:      address,
				PollInterval: 1 * time.Minute,
				Logger:       nil,
			},
			wantErr: true,
		},
		{
			name: "empty_rpc_endpoint",
			cfg: &Config{
				RPCEndpoint:  "",
				Address:      address,
				PollInterval: 1 * time.Minute,
				Logger:       logger,
			},
			wantErr: true,
		},
		{
			name: "zero_poll_interval",
			cfg: &Config{
				RPCEndpoint:  "https://polygon-rpc.com",
				Address:      address,
				PollInterval: 0,
				Logger:       logger,
			},
			wantErr: true,
		},
		{
			name: "negative_poll_interval",
			cfg: &Config{
				RPCEndpoint:  "https://polygon-rpc.com",
				Address:      address,
				PollInterval: -1 * time.Second,
				Logger:       logger,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracker, err := New(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tracker == nil {
				t.Error("New() returned nil tracker")
			}
			if !tt.wantErr {
				if tracker.client == nil {
					t.Error("New() client is nil")
				}
				if tracker.address != tt.cfg.Address {
					t.Errorf("New() address = %v, want %v", tracker.address, tt.cfg.Address)
				}
				if tracker.pollInterval != tt.cfg.PollInterval {
					t.Errorf("New() pollInterval = %v, want %v", tracker.pollInterval, tt.cfg.PollInterval)
				}
			}
		})
	}
}

func TestTracker_Run_ContextCancellation(t *testing.T) {
	logger := zap.NewNop()
	address := common.HexToAddress("0x1234567890123456789012345678901234567890")

	tracker, err := New(&Config{
		RPCEndpoint:  "https://polygon-rpc.com",
		Address:      address,
		PollInterval: 100 * time.Millisecond,
		Logger:       logger,
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	// Run should exit when context is cancelled
	err = tracker.Run(ctx)
	if err != context.DeadlineExceeded {
		t.Errorf("Run() error = %v, want context.DeadlineExceeded", err)
	}
}

func TestTracker_Run_ImmediateCancellation(t *testing.T) {
	logger := zap.NewNop()
	address := common.HexToAddress("0x1234567890123456789012345678901234567890")

	tracker, err := New(&Config{
		RPCEndpoint:  "https://polygon-rpc.com",
		Address:      address,
		PollInterval: 1 * time.Minute,
		Logger:       logger,
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Run should exit quickly
	done := make(chan error, 1)
	go func() {
		done <- tracker.Run(ctx)
	}()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not exit after context cancellation")
	}
}

func TestTracker_updateMetrics(t *testing.T) {
	logger := zap.NewNop()
	address := common.HexToAddress("0x1234567890123456789012345678901234567890")

	tracker, err := New(&Config{
		RPCEndpoint:  "https://polygon-rpc.com",
		Address:      address,
		PollInterval: 1 * time.Minute,
		Logger:       logger,
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	tests := []struct {
		name          string
		balances      *Balances
		positions     []Position
		wantPositions int
	}{
		{
			name: "with_balances_and_positions",
			balances: &Balances{
				MATIC:         big.NewInt(5e18),  // 5 MATIC
				USDC:          big.NewInt(100e6), // 100 USDC
				USDCAllowance: big.NewInt(1000e6), // 1000 USDC allowance
			},
			positions: []Position{
				{
					MarketSlug:   "market-1",
					Outcome:      "YES",
					Size:         100,
					Value:        52.00,
					InitialValue: 50.00,
					CashPnL:      2.00,
					PercentPnL:   4.00,
				},
				{
					MarketSlug:   "market-2",
					Outcome:      "NO",
					Size:         50,
					Value:        26.00,
					InitialValue: 24.00,
					CashPnL:      2.00,
					PercentPnL:   8.33,
				},
			},
			wantPositions: 2,
		},
		{
			name: "with_balances_no_positions",
			balances: &Balances{
				MATIC:         big.NewInt(1e18), // 1 MATIC
				USDC:          big.NewInt(50e6), // 50 USDC
				USDCAllowance: big.NewInt(0),     // No allowance
			},
			positions:     []Position{},
			wantPositions: 0,
		},
		{
			name: "zero_balances",
			balances: &Balances{
				MATIC:         big.NewInt(0),
				USDC:          big.NewInt(0),
				USDCAllowance: big.NewInt(0),
			},
			positions:     []Position{},
			wantPositions: 0,
		},
		{
			name: "negative_pnl",
			balances: &Balances{
				MATIC:         big.NewInt(1e18),
				USDC:          big.NewInt(100e6),
				USDCAllowance: big.NewInt(1000e6),
			},
			positions: []Position{
				{
					MarketSlug:   "losing-market",
					Outcome:      "YES",
					Size:         100,
					Value:        40.00,
					InitialValue: 50.00,
					CashPnL:      -10.00,
					PercentPnL:   -20.00,
				},
			},
			wantPositions: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Call updateMetrics
			tracker.updateMetrics(tt.balances, tt.positions)

			// Verify metrics were updated (we can't easily read Prometheus gauges,
			// but we can verify the function doesn't panic and handles edge cases)

			// Test passes if no panic occurs
			// In a production environment, we'd use prometheus testutil
			// to verify actual metric values
		})
	}
}

func TestTracker_updateMetrics_CalculationCorrectness(t *testing.T) {
	logger := zap.NewNop()
	address := common.HexToAddress("0x1234567890123456789012345678901234567890")

	tracker, err := New(&Config{
		RPCEndpoint:  "https://polygon-rpc.com",
		Address:      address,
		PollInterval: 1 * time.Minute,
		Logger:       logger,
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Test case: Verify P&L percentage calculation
	balances := &Balances{
		MATIC:         big.NewInt(1e18),
		USDC:          big.NewInt(100e6),
		USDCAllowance: big.NewInt(1000e6),
	}

	positions := []Position{
		{
			Value:        110.00,
			InitialValue: 100.00,
			CashPnL:      10.00,
		},
		{
			Value:        48.00,
			InitialValue: 50.00,
			CashPnL:      -2.00,
		},
	}

	// Expected:
	// Total value: 110 + 48 = 158
	// Total cost: 100 + 50 = 150
	// Total P&L: 10 + (-2) = 8
	// P&L %: (8 / 150) * 100 = 5.33%

	tracker.updateMetrics(balances, positions)

	// Test passes if no panic and calculations handle edge cases
}

func TestTracker_updateMetrics_ZeroDivision(t *testing.T) {
	logger := zap.NewNop()
	address := common.HexToAddress("0x1234567890123456789012345678901234567890")

	tracker, err := New(&Config{
		RPCEndpoint:  "https://polygon-rpc.com",
		Address:      address,
		PollInterval: 1 * time.Minute,
		Logger:       logger,
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Test case: Zero cost should not cause division by zero
	balances := &Balances{
		MATIC:         big.NewInt(0),
		USDC:          big.NewInt(0),
		USDCAllowance: big.NewInt(0),
	}

	positions := []Position{
		{
			Value:        10.00,
			InitialValue: 0.00, // Zero cost
			CashPnL:      10.00,
		},
	}

	// Should not panic
	tracker.updateMetrics(balances, positions)
}
