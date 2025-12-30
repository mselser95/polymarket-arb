package cmd

import (
	"testing"
)

// TestApproveCommand_Structure tests command is properly configured
func TestApproveCommand_Structure(t *testing.T) {
	if approveCmd == nil {
		t.Fatal("approveCmd is nil")
	}

	if approveCmd.Use != "approve" {
		t.Errorf("expected Use='approve', got '%s'", approveCmd.Use)
	}

	if approveCmd.RunE == nil {
		t.Error("RunE function is nil")
	}
}

// TestApproveCommand_Flags tests command flags are defined
func TestApproveCommand_Flags(t *testing.T) {
	amountFlag := approveCmd.Flags().Lookup("amount")
	if amountFlag == nil {
		t.Error("amount flag not defined")
	}

	if amountFlag.Shorthand != "a" {
		t.Errorf("expected amount shorthand 'a', got '%s'", amountFlag.Shorthand)
	}

	if amountFlag.DefValue != "unlimited" {
		t.Errorf("expected amount default 'unlimited', got '%s'", amountFlag.DefValue)
	}

	rpcFlag := approveCmd.Flags().Lookup("rpc")
	if rpcFlag == nil {
		t.Error("rpc flag not defined")
	}

	if rpcFlag.Shorthand != "r" {
		t.Errorf("expected rpc shorthand 'r', got '%s'", rpcFlag.Shorthand)
	}
}

// TestApproveCommand_Constants tests contract addresses are valid
func TestApproveCommand_Constants(t *testing.T) {
	tests := []struct {
		name    string
		address string
	}{
		{name: "USDC address", address: usdcAddress},
		{name: "CTF Exchange address", address: ctfExchangeAddress},
		{name: "CTF address", address: ctfAddress},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.address) != 42 {
				t.Errorf("%s length = %d, want 42 (0x + 40 hex chars)", tt.name, len(tt.address))
			}

			if tt.address[:2] != "0x" {
				t.Errorf("%s does not start with 0x: %s", tt.name, tt.address)
			}
		})
	}
}

// TestApproveCommand_ABIs tests ABI constants are non-empty
func TestApproveCommand_ABIs(t *testing.T) {
	if erc20ApproveABI == "" {
		t.Error("erc20ApproveABI is empty")
	}

	if erc1155ApprovalABI == "" {
		t.Error("erc1155ApprovalABI is empty")
	}
}
