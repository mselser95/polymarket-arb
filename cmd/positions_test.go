package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mselser95/polymarket-arb/pkg/wallet"
)

func TestDetermineWinLoss(t *testing.T) {
	tests := []struct {
		name           string
		position       wallet.Position
		expectedStatus string
		expectedEmoji  string
	}{
		{
			name: "clear-win-value-equals-size",
			position: wallet.Position{
				Size:  100.0,
				Value: 100.0, // $1 per token = WIN
			},
			expectedStatus: "SETTLED_WIN",
			expectedEmoji:  "🏆",
		},
		{
			name: "clear-loss-value-zero",
			position: wallet.Position{
				Size:  100.0,
				Value: 0.0,
			},
			expectedStatus: "SETTLED_LOSS",
			expectedEmoji:  "💀",
		},
		{
			name: "win-with-small-discrepancy",
			position: wallet.Position{
				Size:  100.0,
				Value: 97.0, // 97% of expected (within tolerance)
			},
			expectedStatus: "SETTLED_WIN",
			expectedEmoji:  "🏆",
		},
		{
			name: "loss-with-small-residual-value",
			position: wallet.Position{
				Size:  100.0,
				Value: 2.0, // 2% of expected (within loss threshold)
			},
			expectedStatus: "SETTLED_LOSS",
			expectedEmoji:  "💀",
		},
		{
			name: "ambiguous-value-50-percent",
			position: wallet.Position{
				Size:  100.0,
				Value: 50.0, // 50% - unclear outcome
			},
			expectedStatus: "SETTLED_UNKNOWN",
			expectedEmoji:  "❓",
		},
		{
			name: "exact-win-threshold-95-percent",
			position: wallet.Position{
				Size:  100.0,
				Value: 95.0, // Exactly at threshold
			},
			expectedStatus: "SETTLED_WIN",
			expectedEmoji:  "🏆",
		},
		{
			name: "exact-loss-threshold-5-percent",
			position: wallet.Position{
				Size:  100.0,
				Value: 5.0, // Exactly at threshold
			},
			expectedStatus: "SETTLED_LOSS",
			expectedEmoji:  "💀",
		},
		{
			name: "just-above-loss-threshold",
			position: wallet.Position{
				Size:  100.0,
				Value: 6.0, // Just above loss threshold (ambiguous)
			},
			expectedStatus: "SETTLED_UNKNOWN",
			expectedEmoji:  "❓",
		},
		{
			name: "just-below-win-threshold",
			position: wallet.Position{
				Size:  100.0,
				Value: 94.0, // Just below win threshold (ambiguous)
			},
			expectedStatus: "SETTLED_UNKNOWN",
			expectedEmoji:  "❓",
		},
		{
			name: "zero-size-position",
			position: wallet.Position{
				Size:  0.0,
				Value: 0.0,
			},
			expectedStatus: "SETTLED_UNKNOWN",
			expectedEmoji:  "❓",
		},
		{
			name: "large-position-win",
			position: wallet.Position{
				Size:  1000.0,
				Value: 999.0, // 99.9% of expected
			},
			expectedStatus: "SETTLED_WIN",
			expectedEmoji:  "🏆",
		},
		{
			name: "small-position-loss",
			position: wallet.Position{
				Size:  1.5,
				Value: 0.05, // ~3.3% of expected
			},
			expectedStatus: "SETTLED_LOSS",
			expectedEmoji:  "💀",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, emoji := determineWinLoss(tt.position)
			assert.Equal(t, tt.expectedStatus, status, "Status mismatch")
			assert.Equal(t, tt.expectedEmoji, emoji, "Emoji mismatch")
		})
	}
}

func TestApplyFilters(t *testing.T) {
	// Setup test data
	positions := []EnrichedPosition{
		{Status: "ACTIVE", Position: wallet.Position{MarketSlug: "market-1"}},
		{Status: "SETTLED_WIN", Position: wallet.Position{MarketSlug: "market-2"}},
		{Status: "SETTLED_LOSS", Position: wallet.Position{MarketSlug: "market-3"}},
		{Status: "ACTIVE", Position: wallet.Position{MarketSlug: "market-4"}},
		{Status: "SETTLED_UNKNOWN", Position: wallet.Position{MarketSlug: "market-5"}},
	}

	t.Run("no-filters", func(t *testing.T) {
		// Reset flags
		settledOnly = false
		activeOnly = false

		filtered := applyFilters(positions)
		assert.Len(t, filtered, 5, "Should return all positions")
	})

	t.Run("settled-only-filter", func(t *testing.T) {
		// Set flags
		settledOnly = true
		activeOnly = false

		filtered := applyFilters(positions)
		assert.Len(t, filtered, 3, "Should return only settled positions")

		// Verify no active positions
		for _, pos := range filtered {
			assert.NotEqual(t, "ACTIVE", pos.Status, "Should not contain active positions")
		}

		// Verify expected slugs
		slugs := make([]string, len(filtered))
		for i, pos := range filtered {
			slugs[i] = pos.Position.MarketSlug
		}
		assert.Contains(t, slugs, "market-2")
		assert.Contains(t, slugs, "market-3")
		assert.Contains(t, slugs, "market-5")
	})

	t.Run("active-only-filter", func(t *testing.T) {
		// Set flags
		settledOnly = false
		activeOnly = true

		filtered := applyFilters(positions)
		assert.Len(t, filtered, 2, "Should return only active positions")

		// Verify all are active
		for _, pos := range filtered {
			assert.Equal(t, "ACTIVE", pos.Status, "All should be active")
		}

		// Verify expected slugs
		slugs := make([]string, len(filtered))
		for i, pos := range filtered {
			slugs[i] = pos.Position.MarketSlug
		}
		assert.Contains(t, slugs, "market-1")
		assert.Contains(t, slugs, "market-4")
	})

	// Reset flags for other tests
	settledOnly = false
	activeOnly = false
}

func TestSortPositions(t *testing.T) {
	t.Run("default-sort-active-first-then-pnl", func(t *testing.T) {
		positions := []EnrichedPosition{
			{Status: "SETTLED_WIN", Position: wallet.Position{MarketSlug: "market-1", CashPnL: 10.0}},
			{Status: "ACTIVE", Position: wallet.Position{MarketSlug: "market-2", CashPnL: 5.0}},
			{Status: "SETTLED_LOSS", Position: wallet.Position{MarketSlug: "market-3", CashPnL: -20.0}},
			{Status: "ACTIVE", Position: wallet.Position{MarketSlug: "market-4", CashPnL: 15.0}},
		}

		// Set flags
		sortByPnL = false

		sortPositions(positions)

		// Active positions should come first
		assert.Equal(t, "ACTIVE", positions[0].Status, "First position should be active")
		assert.Equal(t, "ACTIVE", positions[1].Status, "Second position should be active")

		// Within active, highest P&L first
		assert.Equal(t, "market-4", positions[0].Position.MarketSlug, "Highest P&L active first")
		assert.Equal(t, "market-2", positions[1].Position.MarketSlug, "Lower P&L active second")

		// Settled positions after, sorted by P&L
		assert.NotEqual(t, "ACTIVE", positions[2].Status, "Third position should be settled")
		assert.NotEqual(t, "ACTIVE", positions[3].Status, "Fourth position should be settled")
		assert.True(t, positions[2].Position.CashPnL > positions[3].Position.CashPnL, "Settled sorted by P&L")
	})

	t.Run("sort-by-pnl-flag", func(t *testing.T) {
		positions := []EnrichedPosition{
			{Status: "SETTLED_WIN", Position: wallet.Position{MarketSlug: "market-1", CashPnL: 10.0}},
			{Status: "ACTIVE", Position: wallet.Position{MarketSlug: "market-2", CashPnL: 5.0}},
			{Status: "SETTLED_LOSS", Position: wallet.Position{MarketSlug: "market-3", CashPnL: -20.0}},
			{Status: "ACTIVE", Position: wallet.Position{MarketSlug: "market-4", CashPnL: 15.0}},
		}

		// Set flags
		sortByPnL = true

		sortPositions(positions)

		// Sorted purely by P&L (highest first)
		assert.Equal(t, 15.0, positions[0].Position.CashPnL, "Highest P&L first")
		assert.Equal(t, 10.0, positions[1].Position.CashPnL, "Second highest P&L")
		assert.Equal(t, 5.0, positions[2].Position.CashPnL, "Third highest P&L")
		assert.Equal(t, -20.0, positions[3].Position.CashPnL, "Lowest P&L last")

		// Verify market slugs
		assert.Equal(t, "market-4", positions[0].Position.MarketSlug)
		assert.Equal(t, "market-1", positions[1].Position.MarketSlug)
		assert.Equal(t, "market-2", positions[2].Position.MarketSlug)
		assert.Equal(t, "market-3", positions[3].Position.MarketSlug)

		// Reset flag
		sortByPnL = false
	})
}

func TestCalculateSummary(t *testing.T) {
	positions := []EnrichedPosition{
		{
			Status: "ACTIVE",
			Position: wallet.Position{
				Value:        100.0,
				InitialValue: 90.0,
				CashPnL:      10.0,
			},
		},
		{
			Status: "ACTIVE",
			Position: wallet.Position{
				Value:        50.0,
				InitialValue: 55.0,
				CashPnL:      -5.0,
			},
		},
		{
			Status: "SETTLED_WIN",
			Position: wallet.Position{
				Value:        200.0,
				InitialValue: 150.0,
				CashPnL:      50.0,
			},
		},
		{
			Status: "SETTLED_LOSS",
			Position: wallet.Position{
				Value:        0.0,
				InitialValue: 50.0,
				CashPnL:      -50.0,
			},
		},
		{
			Status: "SETTLED_UNKNOWN",
			Position: wallet.Position{
				Value:        25.0,
				InitialValue: 30.0,
				CashPnL:      -5.0,
			},
		},
	}

	summary := calculateSummary(positions)

	// Test counts
	assert.Equal(t, 5, summary.TotalPositions, "Total positions")
	assert.Equal(t, 2, summary.ActiveCount, "Active count")
	assert.Equal(t, 3, summary.SettledCount, "Settled count")
	assert.Equal(t, 1, summary.WinCount, "Win count")
	assert.Equal(t, 1, summary.LossCount, "Loss count")
	assert.Equal(t, 1, summary.UnknownCount, "Unknown count")

	// Test values
	assert.Equal(t, 375.0, summary.TotalValueUSD, "Total value")
	assert.Equal(t, 375.0, summary.TotalCostUSD, "Total cost")
	assert.Equal(t, 0.0, summary.TotalPnLUSD, "Total P&L")
	assert.Equal(t, 0.0, summary.TotalPnLPercent, "Total P&L percent")

	// Test P&L breakdown
	assert.Equal(t, 5.0, summary.UnrealizedPnLUSD, "Unrealized P&L (10 - 5)")
	assert.Equal(t, -5.0, summary.SettledPnLUSD, "Settled P&L (50 - 50 - 5)")
}

func TestCalculateSummary_AllWins(t *testing.T) {
	positions := []EnrichedPosition{
		{
			Status: "SETTLED_WIN",
			Position: wallet.Position{
				Value:        100.0,
				InitialValue: 50.0,
				CashPnL:      50.0,
			},
		},
		{
			Status: "SETTLED_WIN",
			Position: wallet.Position{
				Value:        100.0,
				InitialValue: 60.0,
				CashPnL:      40.0,
			},
		},
	}

	summary := calculateSummary(positions)

	assert.Equal(t, 2, summary.WinCount, "Win count")
	assert.Equal(t, 0, summary.LossCount, "Loss count")
	assert.Equal(t, 90.0, summary.SettledPnLUSD, "Settled P&L")
	assert.InDelta(t, 81.82, summary.TotalPnLPercent, 0.01, "Total P&L percent")
}

func TestCalculateSummary_EmptyPositions(t *testing.T) {
	positions := []EnrichedPosition{}

	summary := calculateSummary(positions)

	assert.Equal(t, 0, summary.TotalPositions, "Total positions")
	assert.Equal(t, 0, summary.ActiveCount, "Active count")
	assert.Equal(t, 0, summary.SettledCount, "Settled count")
	assert.Equal(t, 0.0, summary.TotalValueUSD, "Total value")
	assert.Equal(t, 0.0, summary.TotalPnLUSD, "Total P&L")
}

func TestCalculateSummary_UnknownStatus(t *testing.T) {
	// Test that positions with "UNKNOWN" status are counted as settled
	positions := []EnrichedPosition{
		{
			Status: "UNKNOWN", // Market metadata fetch failed
			Position: wallet.Position{
				Value:        0.0,
				InitialValue: 10.0,
				CashPnL:      -10.0,
			},
		},
		{
			Status: "ACTIVE",
			Position: wallet.Position{
				Value:        20.0,
				InitialValue: 15.0,
				CashPnL:      5.0,
			},
		},
	}

	summary := calculateSummary(positions)

	assert.Equal(t, 2, summary.TotalPositions, "Total positions")
	assert.Equal(t, 1, summary.ActiveCount, "Active count")
	assert.Equal(t, 1, summary.SettledCount, "Settled count (UNKNOWN should count as settled)")
	assert.Equal(t, 1, summary.UnknownCount, "Unknown count")
	assert.Equal(t, -10.0, summary.SettledPnLUSD, "Settled P&L")
	assert.Equal(t, 5.0, summary.UnrealizedPnLUSD, "Unrealized P&L")
}

func TestValidateFlags(t *testing.T) {
	t.Run("valid-default-flags", func(t *testing.T) {
		settledOnly = false
		activeOnly = false
		outputFormat = "table"

		err := validateFlags()
		assert.NoError(t, err, "Default flags should be valid")
	})

	t.Run("valid-json-format", func(t *testing.T) {
		settledOnly = false
		activeOnly = false
		outputFormat = "json"

		err := validateFlags()
		assert.NoError(t, err, "JSON format should be valid")
	})

	t.Run("valid-csv-format", func(t *testing.T) {
		settledOnly = false
		activeOnly = false
		outputFormat = "csv"

		err := validateFlags()
		assert.NoError(t, err, "CSV format should be valid")
	})

	t.Run("valid-settled-only", func(t *testing.T) {
		settledOnly = true
		activeOnly = false
		outputFormat = "table"

		err := validateFlags()
		assert.NoError(t, err, "Settled-only flag should be valid")
	})

	t.Run("valid-active-only", func(t *testing.T) {
		settledOnly = false
		activeOnly = true
		outputFormat = "table"

		err := validateFlags()
		assert.NoError(t, err, "Active-only flag should be valid")
	})

	t.Run("invalid-conflicting-filters", func(t *testing.T) {
		settledOnly = true
		activeOnly = true
		outputFormat = "table"

		err := validateFlags()
		require.Error(t, err, "Conflicting filters should return error")
		assert.Contains(t, err.Error(), "cannot use both", "Error message should mention conflict")
	})

	t.Run("invalid-format", func(t *testing.T) {
		settledOnly = false
		activeOnly = false
		outputFormat = "xml"

		err := validateFlags()
		require.Error(t, err, "Invalid format should return error")
		assert.Contains(t, err.Error(), "invalid format", "Error message should mention invalid format")
	})

	// Reset flags
	settledOnly = false
	activeOnly = false
	outputFormat = "table"
}

func TestDetermineWinLoss_EdgeCases(t *testing.T) {
	t.Run("value-slightly-above-size", func(t *testing.T) {
		pos := wallet.Position{
			Size:  100.0,
			Value: 101.0, // Slightly over (shouldn't happen but test it)
		}
		status, emoji := determineWinLoss(pos)
		assert.Equal(t, "SETTLED_WIN", status, "Over 100% should still be win")
		assert.Equal(t, "🏆", emoji)
	})

	t.Run("very-small-position", func(t *testing.T) {
		pos := wallet.Position{
			Size:  0.01,
			Value: 0.0095, // 95% of size
		}
		status, emoji := determineWinLoss(pos)
		assert.Equal(t, "SETTLED_WIN", status, "Should work with tiny positions")
		assert.Equal(t, "🏆", emoji)
	})

	t.Run("negative-pnl-but-still-win", func(t *testing.T) {
		pos := wallet.Position{
			Size:         100.0,
			Value:        100.0, // WIN: tokens worth $1 each
			InitialValue: 120.0, // But bought at high price
			CashPnL:      -20.0, // Negative P&L
		}
		status, emoji := determineWinLoss(pos)
		assert.Equal(t, "SETTLED_WIN", status, "Win status independent of P&L")
		assert.Equal(t, "🏆", emoji)
	})

	t.Run("positive-pnl-but-actually-loss", func(t *testing.T) {
		// This scenario shouldn't happen in practice, but testing defensive logic
		pos := wallet.Position{
			Size:         100.0,
			Value:        0.0,    // LOSS: tokens worthless
			InitialValue: -10.0,  // Hypothetical negative cost
			CashPnL:      10.0,   // Positive P&L
		}
		status, emoji := determineWinLoss(pos)
		assert.Equal(t, "SETTLED_LOSS", status, "Loss status based on value, not P&L")
		assert.Equal(t, "💀", emoji)
	})
}
