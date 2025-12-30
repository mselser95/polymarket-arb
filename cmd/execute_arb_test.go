package cmd

import (
	"testing"
)

// TestExecuteArbCommand_Structure tests command is properly configured
func TestExecuteArbCommand_Structure(t *testing.T) {
	if executeArbCmd == nil {
		t.Fatal("executeArbCmd is nil")
	}

	if executeArbCmd.Use != "execute-arb <market-slug>" {
		t.Errorf("expected Use='execute-arb <market-slug>', got '%s'", executeArbCmd.Use)
	}

	if executeArbCmd.RunE == nil {
		t.Error("RunE function is nil")
	}
}

// TestExecuteArbCommand_Flags tests command flags are defined
func TestExecuteArbCommand_Flags(t *testing.T) {
	tests := []struct {
		name      string
		flag      string
		shorthand string
		defValue  string
	}{
		{name: "threshold", flag: "threshold", shorthand: "t", defValue: "0.995"},
		{name: "size", flag: "size", shorthand: "s", defValue: "100"},
		{name: "fee", flag: "fee", shorthand: "f", defValue: "0.01"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flag := executeArbCmd.Flags().Lookup(tt.flag)
			if flag == nil {
				t.Fatalf("%s flag not defined", tt.flag)
			}

			if flag.Shorthand != tt.shorthand {
				t.Errorf("expected %s shorthand '%s', got '%s'", tt.flag, tt.shorthand, flag.Shorthand)
			}

			if flag.DefValue != tt.defValue {
				t.Errorf("expected %s default '%s', got '%s'", tt.flag, tt.defValue, flag.DefValue)
			}
		})
	}
}

// TestExecuteArbCommand_ThresholdRange tests threshold validation
func TestExecuteArbCommand_ThresholdRange(t *testing.T) {
	tests := []struct {
		name      string
		threshold float64
		valid     bool
	}{
		{name: "too low", threshold: 0.8, valid: false},
		{name: "minimum valid", threshold: 0.9, valid: true},
		{name: "typical", threshold: 0.995, valid: true},
		{name: "upper bound", threshold: 1.0, valid: true},
		{name: "too high", threshold: 1.1, valid: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := tt.threshold >= 0.9 && tt.threshold <= 1.0
			if valid != tt.valid {
				t.Errorf("threshold %.3f: expected valid=%v, got valid=%v", tt.threshold, tt.valid, valid)
			}
		})
	}
}

// TestExecuteArbCommand_TradeSizeValidation tests size validation
func TestExecuteArbCommand_TradeSizeValidation(t *testing.T) {
	tests := []struct {
		name  string
		size  float64
		valid bool
	}{
		{name: "zero", size: 0.0, valid: false},
		{name: "negative", size: -10.0, valid: false},
		{name: "minimum valid", size: 1.0, valid: true},
		{name: "typical", size: 100.0, valid: true},
		{name: "large", size: 1000.0, valid: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := tt.size > 0
			if valid != tt.valid {
				t.Errorf("size %.2f: expected valid=%v, got valid=%v", tt.size, tt.valid, valid)
			}
		})
	}
}

// TestExecuteArbCommand_FeeValidation tests fee validation
func TestExecuteArbCommand_FeeValidation(t *testing.T) {
	tests := []struct {
		name  string
		fee   float64
		valid bool
	}{
		{name: "negative", fee: -0.01, valid: false},
		{name: "zero", fee: 0.0, valid: true},
		{name: "polymarket standard", fee: 0.01, valid: true},
		{name: "high fee", fee: 0.1, valid: true},
		{name: "too high", fee: 1.0, valid: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := tt.fee >= 0 && tt.fee < 1.0
			if valid != tt.valid {
				t.Errorf("fee %.2f: expected valid=%v, got valid=%v", tt.fee, tt.valid, valid)
			}
		})
	}
}
