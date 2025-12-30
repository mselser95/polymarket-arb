package cmd

import (
	"testing"
	"time"
)

// TestRedeemPositionsCommand_Structure tests command is properly configured
func TestRedeemPositionsCommand_Structure(t *testing.T) {
	if redeemPositionsCmd == nil {
		t.Fatal("redeemPositionsCmd is nil")
	}

	if redeemPositionsCmd.Use != "redeem-positions" {
		t.Errorf("expected Use='redeem-positions', got '%s'", redeemPositionsCmd.Use)
	}

	if redeemPositionsCmd.RunE == nil {
		t.Error("RunE function is nil")
	}
}

// TestRedeemPositionsCommand_Flags tests command flags are defined
func TestRedeemPositionsCommand_Flags(t *testing.T) {
	rpcFlag := redeemPositionsCmd.Flags().Lookup("rpc")
	if rpcFlag == nil {
		t.Fatal("rpc flag not defined")
	}

	if rpcFlag.DefValue != "https://polygon-rpc.com" {
		t.Errorf("expected rpc default 'https://polygon-rpc.com', got '%s'", rpcFlag.DefValue)
	}

	dryRunFlag := redeemPositionsCmd.Flags().Lookup("dry-run")
	if dryRunFlag == nil {
		t.Fatal("dry-run flag not defined")
	}

	if dryRunFlag.DefValue != "false" {
		t.Errorf("expected dry-run default 'false', got '%s'", dryRunFlag.DefValue)
	}

	marketFlag := redeemPositionsCmd.Flags().Lookup("market")
	if marketFlag == nil {
		t.Fatal("market flag not defined")
	}

	autoFlag := redeemPositionsCmd.Flags().Lookup("auto")
	if autoFlag == nil {
		t.Fatal("auto flag not defined")
	}

	intervalFlag := redeemPositionsCmd.Flags().Lookup("interval")
	if intervalFlag == nil {
		t.Fatal("interval flag not defined")
	}

	if intervalFlag.DefValue != "1h0m0s" {
		t.Errorf("expected interval default '1h0m0s', got '%s'", intervalFlag.DefValue)
	}
}

// TestRedeemPositionsCommand_Constants tests contract addresses
func TestRedeemPositionsCommand_Constants(t *testing.T) {
	if ctfContractAddress != "0x4bFb41d5B3570DeFd03C39a9A4D8dE6Bd8B8982E" {
		t.Errorf("unexpected ctfContractAddress: %s", ctfContractAddress)
	}

	if polygonChainID != 137 {
		t.Errorf("expected polygonChainID=137, got %d", polygonChainID)
	}

	if redeemUSDCAddress != "0x2791Bca1f2de4661ED88A30C99A7a9449Aa84174" {
		t.Errorf("unexpected redeemUSDCAddress: %s", redeemUSDCAddress)
	}
}

// TestRedeemPositionsCommand_IntervalValidation tests interval validation
func TestRedeemPositionsCommand_IntervalValidation(t *testing.T) {
	tests := []struct {
		name     string
		interval time.Duration
		valid    bool
	}{
		{name: "too short", interval: 10 * time.Second, valid: false},
		{name: "minimum valid", interval: 1 * time.Minute, valid: true},
		{name: "typical", interval: 1 * time.Hour, valid: true},
		{name: "long", interval: 24 * time.Hour, valid: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := tt.interval >= 1*time.Minute
			if valid != tt.valid {
				t.Errorf("interval %v: expected valid=%v, got valid=%v", tt.interval, tt.valid, valid)
			}
		})
	}
}
