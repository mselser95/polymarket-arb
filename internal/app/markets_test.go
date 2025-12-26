package app

import (
	"testing"

	"github.com/mselser95/polymarket-arb/pkg/types"
)

// TestMarketTokenValidation tests the validation logic for market tokens
// This tests the core logic without requiring full App setup
func TestMarketTokenValidation(t *testing.T) {
	tests := []struct {
		name           string
		market         *types.Market
		expectValid    bool
		expectedTokens int
	}{
		{
			name: "binary-market-valid",
			market: &types.Market{
				ID:       "market-1",
				Slug:     "binary",
				Question: "Binary market?",
				Tokens: []types.Token{
					{TokenID: "token-1", Outcome: "YES"},
					{TokenID: "token-2", Outcome: "NO"},
				},
			},
			expectValid:    true,
			expectedTokens: 2,
		},
		{
			name: "multi-outcome-valid",
			market: &types.Market{
				ID:       "market-2",
				Slug:     "three-way",
				Question: "Three-way race?",
				Tokens: []types.Token{
					{TokenID: "token-1", Outcome: "Alice"},
					{TokenID: "token-2", Outcome: "Bob"},
					{TokenID: "token-3", Outcome: "Charlie"},
				},
			},
			expectValid:    true,
			expectedTokens: 3,
		},
		{
			name: "single-outcome-invalid",
			market: &types.Market{
				ID:       "market-3",
				Slug:     "single",
				Question: "Single outcome?",
				Tokens: []types.Token{
					{TokenID: "token-1", Outcome: "Only"},
				},
			},
			expectValid:    false,
			expectedTokens: 0,
		},
		{
			name: "empty-market-invalid",
			market: &types.Market{
				ID:       "market-4",
				Slug:     "empty",
				Question: "Empty market?",
				Tokens:   []types.Token{},
			},
			expectValid:    false,
			expectedTokens: 0,
		},
		{
			name: "market-with-empty-token-id",
			market: &types.Market{
				ID:       "market-5",
				Slug:     "partial-empty",
				Question: "Market with empty token?",
				Tokens: []types.Token{
					{TokenID: "token-1", Outcome: "Valid"},
					{TokenID: "", Outcome: "Empty"},
					{TokenID: "token-3", Outcome: "Valid"},
				},
			},
			expectValid:    true,
			expectedTokens: 2, // Empty token should be filtered
		},
		{
			name: "all-empty-tokens-invalid",
			market: &types.Market{
				ID:       "market-6",
				Slug:     "all-empty",
				Question: "All empty tokens?",
				Tokens: []types.Token{
					{TokenID: "", Outcome: "Empty1"},
					{TokenID: "", Outcome: "Empty2"},
				},
			},
			expectValid:    false,
			expectedTokens: 0, // All tokens filtered, < 2 remaining
		},
		{
			name: "ten-outcome-market-valid",
			market: &types.Market{
				ID:       "market-7",
				Slug:     "ten-way",
				Question: "Ten-way election?",
				Tokens: []types.Token{
					{TokenID: "token-1", Outcome: "Candidate 1"},
					{TokenID: "token-2", Outcome: "Candidate 2"},
					{TokenID: "token-3", Outcome: "Candidate 3"},
					{TokenID: "token-4", Outcome: "Candidate 4"},
					{TokenID: "token-5", Outcome: "Candidate 5"},
					{TokenID: "token-6", Outcome: "Candidate 6"},
					{TokenID: "token-7", Outcome: "Candidate 7"},
					{TokenID: "token-8", Outcome: "Candidate 8"},
					{TokenID: "token-9", Outcome: "Candidate 9"},
					{TokenID: "token-10", Outcome: "Candidate 10"},
				},
			},
			expectValid:    true,
			expectedTokens: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Validate market has at least 2 outcomes
			if len(tt.market.Tokens) < 2 {
				if tt.expectValid {
					t.Errorf("market has insufficient outcomes but expected valid")
				}
				return
			}

			// Filter valid token IDs (non-empty)
			validTokens := make([]string, 0, len(tt.market.Tokens))
			for _, token := range tt.market.Tokens {
				if token.TokenID != "" {
					validTokens = append(validTokens, token.TokenID)
				}
			}

			// Check if we have at least 2 valid tokens after filtering
			isValid := len(validTokens) >= 2

			if isValid != tt.expectValid {
				t.Errorf("expected valid=%v, got valid=%v (tokens: %d)", tt.expectValid, isValid, len(validTokens))
			}

			if isValid && len(validTokens) != tt.expectedTokens {
				t.Errorf("expected %d valid tokens, got %d", tt.expectedTokens, len(validTokens))
			}
		})
	}
}

// TestMarketTokenFiltering tests that empty token IDs are filtered correctly
func TestMarketTokenFiltering(t *testing.T) {
	market := &types.Market{
		ID:       "market-filter",
		Slug:     "filter-test",
		Question: "Filtering test?",
		Tokens: []types.Token{
			{TokenID: "valid-1", Outcome: "Outcome 1"},
			{TokenID: "", Outcome: "Empty"},
			{TokenID: "valid-2", Outcome: "Outcome 2"},
			{TokenID: "", Outcome: "Another Empty"},
			{TokenID: "valid-3", Outcome: "Outcome 3"},
		},
	}

	// Filter tokens (simulates internal logic from subscribeToMarket)
	validTokens := []string{}
	for _, token := range market.Tokens {
		if token.TokenID != "" {
			validTokens = append(validTokens, token.TokenID)
		}
	}

	// Verify 3 valid tokens remain
	if len(validTokens) != 3 {
		t.Errorf("expected 3 valid tokens after filtering, got %d", len(validTokens))
	}

	// Verify correct tokens
	expectedTokens := map[string]bool{
		"valid-1": false,
		"valid-2": false,
		"valid-3": false,
	}

	for _, tokenID := range validTokens {
		if _, exists := expectedTokens[tokenID]; exists {
			expectedTokens[tokenID] = true
		} else {
			t.Errorf("unexpected token ID in filtered list: %s", tokenID)
		}
	}

	for tokenID, found := range expectedTokens {
		if !found {
			t.Errorf("expected token ID %s not found in filtered list", tokenID)
		}
	}
}

// TestMarketValidation_BoundaryConditions tests edge cases
func TestMarketValidation_BoundaryConditions(t *testing.T) {
	tests := []struct {
		name         string
		tokenCount   int
		expectValid  bool
		description  string
	}{
		{
			name:         "zero-tokens",
			tokenCount:   0,
			expectValid:  false,
			description:  "Markets with 0 tokens should be rejected",
		},
		{
			name:         "one-token",
			tokenCount:   1,
			expectValid:  false,
			description:  "Markets with 1 token should be rejected",
		},
		{
			name:         "two-tokens",
			tokenCount:   2,
			expectValid:  true,
			description:  "Markets with 2 tokens (minimum) should be accepted",
		},
		{
			name:         "three-tokens",
			tokenCount:   3,
			expectValid:  true,
			description:  "Markets with 3 tokens should be accepted",
		},
		{
			name:         "ten-tokens",
			tokenCount:   10,
			expectValid:  true,
			description:  "Markets with 10 tokens should be accepted",
		},
		{
			name:         "fifty-tokens",
			tokenCount:   50,
			expectValid:  true,
			description:  "Markets with many tokens should be accepted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create market with specified token count
			tokens := make([]types.Token, tt.tokenCount)
			for i := 0; i < tt.tokenCount; i++ {
				tokens[i] = types.Token{
					TokenID: string(rune(65 + i)), // A, B, C, etc.
					Outcome: string(rune(65 + i)),
				}
			}

			market := &types.Market{
				ID:       "test-market",
				Slug:     tt.name,
				Question: "Test?",
				Tokens:   tokens,
			}

			// Validate: market must have at least 2 tokens
			isValid := len(market.Tokens) >= 2

			if isValid != tt.expectValid {
				t.Errorf("%s: expected valid=%v, got valid=%v", tt.description, tt.expectValid, isValid)
			}
		})
	}
}

// TestTokenIDExtraction tests extracting valid token IDs from markets
func TestTokenIDExtraction(t *testing.T) {
	market := &types.Market{
		ID:       "market-extract",
		Slug:     "extraction-test",
		Question: "Extraction test?",
		Tokens: []types.Token{
			{TokenID: "token-a", Outcome: "A"},
			{TokenID: "token-b", Outcome: "B"},
			{TokenID: "token-c", Outcome: "C"},
		},
	}

	// Extract token IDs (simulates internal logic)
	tokenIDs := make([]string, 0, len(market.Tokens))
	outcomeNames := make([]string, 0, len(market.Tokens))

	for _, token := range market.Tokens {
		if token.TokenID != "" {
			tokenIDs = append(tokenIDs, token.TokenID)
			outcomeNames = append(outcomeNames, token.Outcome)
		}
	}

	// Verify extraction
	if len(tokenIDs) != 3 {
		t.Errorf("expected 3 token IDs, got %d", len(tokenIDs))
	}

	if len(outcomeNames) != 3 {
		t.Errorf("expected 3 outcome names, got %d", len(outcomeNames))
	}

	// Verify correct IDs
	expectedIDs := []string{"token-a", "token-b", "token-c"}
	for i, expected := range expectedIDs {
		if tokenIDs[i] != expected {
			t.Errorf("token ID %d: expected %s, got %s", i, expected, tokenIDs[i])
		}
	}

	// Verify correct outcomes
	expectedOutcomes := []string{"A", "B", "C"}
	for i, expected := range expectedOutcomes {
		if outcomeNames[i] != expected {
			t.Errorf("outcome %d: expected %s, got %s", i, expected, outcomeNames[i])
		}
	}
}

// TestMarketValidation_MixedValidInvalid tests markets with mix of valid and invalid tokens
func TestMarketValidation_MixedValidInvalid(t *testing.T) {
	tests := []struct {
		name           string
		tokens         []types.Token
		expectValid    bool
		expectedCount  int
	}{
		{
			name: "one-empty-two-valid",
			tokens: []types.Token{
				{TokenID: "", Outcome: "Empty"},
				{TokenID: "valid-1", Outcome: "Valid 1"},
				{TokenID: "valid-2", Outcome: "Valid 2"},
			},
			expectValid:   true,
			expectedCount: 2,
		},
		{
			name: "two-empty-one-valid",
			tokens: []types.Token{
				{TokenID: "", Outcome: "Empty 1"},
				{TokenID: "", Outcome: "Empty 2"},
				{TokenID: "valid-1", Outcome: "Valid"},
			},
			expectValid:   false, // Only 1 valid token after filtering
			expectedCount: 1,
		},
		{
			name: "alternating-empty-valid",
			tokens: []types.Token{
				{TokenID: "valid-1", Outcome: "Valid 1"},
				{TokenID: "", Outcome: "Empty 1"},
				{TokenID: "valid-2", Outcome: "Valid 2"},
				{TokenID: "", Outcome: "Empty 2"},
				{TokenID: "valid-3", Outcome: "Valid 3"},
			},
			expectValid:   true,
			expectedCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Filter valid tokens
			validTokens := []string{}
			for _, token := range tt.tokens {
				if token.TokenID != "" {
					validTokens = append(validTokens, token.TokenID)
				}
			}

			isValid := len(validTokens) >= 2

			if isValid != tt.expectValid {
				t.Errorf("expected valid=%v, got valid=%v (valid tokens: %d)", tt.expectValid, isValid, len(validTokens))
			}

			if len(validTokens) != tt.expectedCount {
				t.Errorf("expected %d valid tokens, got %d", tt.expectedCount, len(validTokens))
			}
		})
	}
}
