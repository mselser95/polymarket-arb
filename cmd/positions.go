package cmd

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/mselser95/polymarket-arb/internal/discovery"
	"github.com/mselser95/polymarket-arb/pkg/config"
	"github.com/mselser95/polymarket-arb/pkg/types"
	"github.com/mselser95/polymarket-arb/pkg/wallet"
)

//nolint:gochecknoglobals // Cobra boilerplate
var positionsCmd = &cobra.Command{
	Use:   "positions",
	Short: "Display all positions with active/settled status and win/loss",
	Long: `Fetches positions from your wallet and enriches them with market metadata.

For each position, displays:
- Market name and outcome
- Position size and value
- P&L (profit/loss)
- Status: ACTIVE or SETTLED (with win/loss indication)

Active positions are markets that haven't settled yet.
Settled positions show whether you won or lost based on position value.

Examples:
  # Show all positions (default table format)
  go run . positions

  # Show only settled positions
  go run . positions --settled-only

  # Show only active positions
  go run . positions --active-only

  # Export to JSON
  go run . positions --format json > positions.json

  # Export to CSV
  go run . positions --format csv > positions.csv

  # Sort by P&L (most profitable first)
  go run . positions --sort-by-pnl`,
	RunE: runPositions,
}

var (
	settledOnly  bool
	activeOnly   bool
	outputFormat string
	sortByPnL    bool
)

//nolint:gochecknoinits // Cobra boilerplate
func init() {
	rootCmd.AddCommand(positionsCmd)

	positionsCmd.Flags().BoolVar(&settledOnly, "settled-only", false, "Show only settled positions")
	positionsCmd.Flags().BoolVar(&activeOnly, "active-only", false, "Show only active positions")
	positionsCmd.Flags().StringVar(&outputFormat, "format", "table", "Output format: table, json, csv")
	positionsCmd.Flags().BoolVar(&sortByPnL, "sort-by-pnl", false, "Sort positions by P&L (highest first)")
}

// EnrichedPosition extends wallet.Position with market metadata and status.
type EnrichedPosition struct {
	Position wallet.Position

	// Market metadata
	MarketQuestion string
	MarketEndDate  time.Time
	MarketClosed   bool
	MarketActive   bool

	// Calculated status
	Status      string // "ACTIVE", "SETTLED_WIN", "SETTLED_LOSS", "SETTLED_UNKNOWN"
	StatusEmoji string // "🟢", "🏆", "💀", "❓"

	// Error handling
	MetadataError error
}

// PositionSummary holds aggregate statistics.
type PositionSummary struct {
	TotalPositions   int
	ActiveCount      int
	SettledCount     int
	WinCount         int
	LossCount        int
	UnknownCount     int
	TotalValueUSD    float64
	TotalCostUSD     float64
	TotalPnLUSD      float64
	TotalPnLPercent  float64
	SettledPnLUSD    float64
	UnrealizedPnLUSD float64
}

func runPositions(cmd *cobra.Command, args []string) (err error) {
	// Validate flags
	err = validateFlags()
	if err != nil {
		return err
	}

	// Load environment
	err = godotenv.Load()
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("load .env: %w", err)
	}

	// Get private key
	privateKeyHex := os.Getenv("POLYMARKET_PRIVATE_KEY")
	if privateKeyHex == "" {
		return fmt.Errorf("POLYMARKET_PRIVATE_KEY not set in environment")
	}

	// Parse private key
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return fmt.Errorf("invalid private key: %w", err)
	}

	// Get address
	address := crypto.PubkeyToAddress(privateKey.PublicKey)

	// Create logger
	logger, err := zap.NewProduction()
	if err != nil {
		return fmt.Errorf("create logger: %w", err)
	}
	defer logger.Sync()

	// Create wallet client (use default RPC)
	walletClient, err := wallet.NewClient("https://polygon-rpc.com", logger)
	if err != nil {
		return fmt.Errorf("create wallet client: %w", err)
	}

	// Load config for discovery URL
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Create discovery client
	discoveryClient := discovery.NewClient(cfg.PolymarketGammaURL, logger)

	// Fetch positions
	ctx := context.Background()
	positions, err := walletClient.GetPositions(ctx, address.Hex())
	if err != nil {
		return fmt.Errorf("get positions: %w", err)
	}

	if len(positions) == 0 {
		fmt.Println("No positions found")
		return nil
	}

	// Enrich positions with market metadata
	enriched := enrichPositions(ctx, positions, discoveryClient, logger)

	// Apply filters
	enriched = applyFilters(enriched)

	// Sort positions
	sortPositions(enriched)

	// Display results
	err = displayPositions(enriched)
	if err != nil {
		return fmt.Errorf("display positions: %w", err)
	}

	return nil
}

func validateFlags() (err error) {
	if settledOnly && activeOnly {
		return fmt.Errorf("cannot use both --settled-only and --active-only")
	}

	validFormats := map[string]bool{"table": true, "json": true, "csv": true}
	if !validFormats[outputFormat] {
		return fmt.Errorf("invalid format: %s (valid: table, json, csv)", outputFormat)
	}

	return nil
}

func enrichPositions(
	ctx context.Context,
	positions []wallet.Position,
	discoveryClient *discovery.Client,
	logger *zap.Logger,
) (enriched []EnrichedPosition) {
	enriched = make([]EnrichedPosition, len(positions))

	// Use worker pool pattern for parallel market fetching
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 10) // Max 10 concurrent requests

	for i, pos := range positions {
		wg.Add(1)
		go func(idx int, p wallet.Position) {
			defer wg.Done()
			semaphore <- struct{}{}        // Acquire
			defer func() { <-semaphore }() // Release

			enriched[idx] = enrichPositionWithStatus(ctx, p, discoveryClient, logger)
		}(i, pos)
	}

	wg.Wait()
	return enriched
}

func enrichPositionWithStatus(
	ctx context.Context,
	pos wallet.Position,
	discoveryClient *discovery.Client,
	logger *zap.Logger,
) (enriched EnrichedPosition) {
	enriched.Position = pos

	// Fetch market metadata with retry (now searches both active AND closed markets)
	market, err := fetchMarketWithRetry(ctx, discoveryClient, pos.MarketSlug, 3)
	if err != nil {
		// Market not found in API (expired micro-markets are often removed)
		// Fall back to value-based status determination
		logger.Debug("market not found in API, using value-based status determination",
			zap.String("market_slug", pos.MarketSlug),
			zap.Float64("value", pos.Value),
			zap.Float64("size", pos.Size),
			zap.Error(err))

		enriched.MetadataError = err
		enriched.MarketQuestion = pos.MarketSlug // Use slug as fallback

		// Determine status from position value/size ratio
		enriched.Status, enriched.StatusEmoji = determineStatus(pos, nil)

		return enriched
	}

	// Enrich with market data
	enriched.MarketQuestion = market.Question
	enriched.MarketEndDate = market.EndDate
	enriched.MarketClosed = market.Closed
	enriched.MarketActive = market.Active

	// Determine status (now has access to market.Closed from both active/closed markets)
	status, emoji := determineStatus(pos, market)
	enriched.Status = status
	enriched.StatusEmoji = emoji

	return enriched
}

func fetchMarketWithRetry(
	ctx context.Context,
	client *discovery.Client,
	slug string,
	maxRetries int,
) (market *types.Market, err error) {
	backoff := 500 * time.Millisecond

	for attempt := 0; attempt < maxRetries; attempt++ {
		market, err = client.FetchMarketBySlug(ctx, slug)
		if err == nil {
			return market, nil
		}

		// Don't retry on context cancellation
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		// Exponential backoff
		if attempt < maxRetries-1 {
			time.Sleep(backoff)
			backoff *= 2
		}
	}

	return nil, err
}

func determineStatus(pos wallet.Position, market *types.Market) (status string, emoji string) {
	// If market data unavailable, determine status from position value
	// This handles cases where expired markets are removed from the API
	if market == nil {
		return determineStatusFromValue(pos)
	}

	// Active position (market not settled)
	if !market.Closed {
		return "ACTIVE", "🟢"
	}

	// Settled position - determine win/loss
	status, emoji = determineWinLoss(pos)
	return status, emoji
}

// determineStatusFromValue determines status from position value/size ratio.
// Used as fallback when market metadata is unavailable.
func determineStatusFromValue(pos wallet.Position) (status string, emoji string) {
	const (
		winThreshold  = 0.95 // >= 95% of size = WIN
		lossThreshold = 0.05 // <= 5% of size = LOSS
		activeMin     = 0.10 // > 10% might be active
		activeMax     = 0.90 // < 90% might be active
	)

	// Handle zero size
	if pos.Size == 0 {
		return "SETTLED_UNKNOWN", "❓"
	}

	valueRatio := pos.Value / pos.Size

	// Clear win/loss
	if valueRatio >= winThreshold {
		return "SETTLED_WIN", "🏆"
	}
	if valueRatio <= lossThreshold {
		return "SETTLED_LOSS", "💀"
	}

	// Could be active (price in normal trading range)
	if valueRatio > activeMin && valueRatio < activeMax {
		return "ACTIVE", "🟢"
	}

	// Ambiguous
	return "SETTLED_UNKNOWN", "❓"
}

func determineWinLoss(pos wallet.Position) (status string, emoji string) {
	const (
		winThreshold  = 0.95 // Value should be >= 95% of size for win
		lossThreshold = 0.05 // Value should be <= 5% of size for loss
	)

	// Expected value if we won: size × $1.00
	expectedWinValue := pos.Size

	// Handle zero size (shouldn't happen, but be defensive)
	if expectedWinValue == 0 {
		return "SETTLED_UNKNOWN", "❓"
	}

	// Calculate ratio: actual value / expected win value
	valueRatio := pos.Value / expectedWinValue

	// WIN: Position value is close to size (tokens worth $1 each)
	if valueRatio >= winThreshold {
		return "SETTLED_WIN", "🏆"
	}

	// LOSS: Position value is near zero (tokens worthless)
	if valueRatio <= lossThreshold {
		return "SETTLED_LOSS", "💀"
	}

	// UNKNOWN: Value in ambiguous range (shouldn't happen often)
	return "SETTLED_UNKNOWN", "❓"
}

func applyFilters(positions []EnrichedPosition) (filtered []EnrichedPosition) {
	filtered = make([]EnrichedPosition, 0, len(positions))

	for _, pos := range positions {
		// Apply active-only filter
		if activeOnly && pos.Status != "ACTIVE" {
			continue
		}

		// Apply settled-only filter
		if settledOnly && pos.Status == "ACTIVE" {
			continue
		}

		filtered = append(filtered, pos)
	}

	return filtered
}

func sortPositions(positions []EnrichedPosition) {
	if sortByPnL {
		// Sort by P&L only (highest first)
		sort.Slice(positions, func(i, j int) bool {
			return positions[i].Position.CashPnL > positions[j].Position.CashPnL
		})
	} else {
		// Default: sort by status (active first), then by P&L
		sort.Slice(positions, func(i, j int) bool {
			// Active positions first
			if positions[i].Status == "ACTIVE" && positions[j].Status != "ACTIVE" {
				return true
			}
			if positions[i].Status != "ACTIVE" && positions[j].Status == "ACTIVE" {
				return false
			}
			// Within same status, sort by P&L
			return positions[i].Position.CashPnL > positions[j].Position.CashPnL
		})
	}
}

func displayPositions(positions []EnrichedPosition) (err error) {
	switch outputFormat {
	case "table":
		displayTableFormat(positions)
		return nil
	case "json":
		return displayJSONFormat(positions)
	case "csv":
		return displayCSVFormat(positions)
	default:
		return fmt.Errorf("unknown format: %s", outputFormat)
	}
}

func displayTableFormat(positions []EnrichedPosition) {
	// Calculate summary
	summary := calculateSummary(positions)

	// Header
	fmt.Printf("Polymarket Positions (%d active, %d settled)\n", summary.ActiveCount, summary.SettledCount)
	fmt.Println("================================================================================")
	fmt.Println()

	// Separate active and settled positions
	activePositions := make([]EnrichedPosition, 0)
	settledPositions := make([]EnrichedPosition, 0)

	for _, pos := range positions {
		if pos.Status == "ACTIVE" {
			activePositions = append(activePositions, pos)
		} else {
			settledPositions = append(settledPositions, pos)
		}
	}

	// Display active positions
	if len(activePositions) > 0 {
		fmt.Printf("ACTIVE POSITIONS (%d)\n", len(activePositions))
		fmt.Println("--------------------------------------------------------------------------------")
		for _, pos := range activePositions {
			displayPosition(pos)
		}
	}

	// Display settled positions
	if len(settledPositions) > 0 {
		if len(activePositions) > 0 {
			fmt.Println()
		}
		fmt.Printf("SETTLED POSITIONS (%d)\n", len(settledPositions))
		fmt.Println("--------------------------------------------------------------------------------")
		for _, pos := range settledPositions {
			displayPosition(pos)
		}
	}

	// Display summary
	fmt.Println()
	fmt.Println("SUMMARY")
	fmt.Println("--------------------------------------------------------------------------------")
	fmt.Printf("Total Positions: %d (%d active, %d settled)\n",
		summary.TotalPositions, summary.ActiveCount, summary.SettledCount)
	if summary.SettledCount > 0 {
		fmt.Printf("  Settled: %d wins 🏆, %d losses 💀", summary.WinCount, summary.LossCount)
		if summary.UnknownCount > 0 {
			fmt.Printf(", %d unknown ❓", summary.UnknownCount)
		}
		fmt.Println()
	}
	fmt.Println()
	fmt.Printf("Total Value: $%.2f | Total Cost: $%.2f\n", summary.TotalValueUSD, summary.TotalCostUSD)

	pnlSign := ""
	if summary.TotalPnLUSD > 0 {
		pnlSign = "+"
	}
	fmt.Printf("Total P&L: %s$%.2f (%.1f%%)\n", pnlSign, summary.TotalPnLUSD, summary.TotalPnLPercent)

	if summary.SettledCount > 0 && summary.ActiveCount > 0 {
		settledSign := ""
		if summary.SettledPnLUSD > 0 {
			settledSign = "+"
		}
		unrealizedSign := ""
		if summary.UnrealizedPnLUSD > 0 {
			unrealizedSign = "+"
		}
		fmt.Printf("  Settled: %s$%.2f\n", settledSign, summary.SettledPnLUSD)
		fmt.Printf("  Unrealized: %s$%.2f (active positions)\n", unrealizedSign, summary.UnrealizedPnLUSD)
	}
}

func displayPosition(pos EnrichedPosition) {
	p := pos.Position

	fmt.Printf("%s Market: %s\n", pos.StatusEmoji, pos.MarketQuestion)
	fmt.Printf("   Outcome: %s\n", p.Outcome)
	fmt.Printf("   Size: %.2f tokens @ $%.4f avg price\n", p.Size, p.AvgPrice)

	if pos.Status == "ACTIVE" {
		fmt.Printf("   Current Value: $%.2f (cost: $%.2f)\n", p.Value, p.InitialValue)
	} else {
		fmt.Printf("   Final Value: $%.2f (cost: $%.2f)\n", p.Value, p.InitialValue)
	}

	pnlSign := ""
	if p.CashPnL > 0 {
		pnlSign = "+"
	}
	fmt.Printf("   P&L: %s$%.2f (%.1f%%)", pnlSign, p.CashPnL, p.PercentPnL)

	// Add win/loss indicator for settled positions
	if pos.Status == "SETTLED_WIN" {
		fmt.Print(" ✅ WIN")
	} else if pos.Status == "SETTLED_LOSS" {
		fmt.Print(" ❌ LOSS")
	}
	fmt.Println()

	// Display date
	if pos.Status == "ACTIVE" && !pos.MarketEndDate.IsZero() {
		fmt.Printf("   End Date: %s\n", pos.MarketEndDate.Format("2006-01-02 15:04:05 MST"))
	} else if pos.Status != "ACTIVE" && !pos.MarketEndDate.IsZero() {
		fmt.Printf("   Settled: %s\n", pos.MarketEndDate.Format("2006-01-02 15:04:05 MST"))
	}

	fmt.Println()
}

func calculateSummary(positions []EnrichedPosition) (summary PositionSummary) {
	summary.TotalPositions = len(positions)

	for _, pos := range positions {
		p := pos.Position

		// Count by status
		switch pos.Status {
		case "ACTIVE":
			summary.ActiveCount++
			summary.UnrealizedPnLUSD += p.CashPnL
		case "SETTLED_WIN":
			summary.SettledCount++
			summary.WinCount++
			summary.SettledPnLUSD += p.CashPnL
		case "SETTLED_LOSS":
			summary.SettledCount++
			summary.LossCount++
			summary.SettledPnLUSD += p.CashPnL
		case "SETTLED_UNKNOWN", "UNKNOWN":
			summary.SettledCount++
			summary.UnknownCount++
			summary.SettledPnLUSD += p.CashPnL
		}

		// Accumulate totals
		summary.TotalValueUSD += p.Value
		summary.TotalCostUSD += p.InitialValue
		summary.TotalPnLUSD += p.CashPnL
	}

	// Calculate total P&L percentage
	if summary.TotalCostUSD > 0 {
		summary.TotalPnLPercent = (summary.TotalPnLUSD / summary.TotalCostUSD) * 100
	}

	return summary
}

func displayJSONFormat(positions []EnrichedPosition) (err error) {
	// Build JSON structure
	type jsonPosition struct {
		MarketSlug    string  `json:"market_slug"`
		MarketQuestion string `json:"market_question"`
		Outcome       string  `json:"outcome"`
		Status        string  `json:"status"`
		Size          float64 `json:"size"`
		AvgPrice      float64 `json:"avg_price"`
		CurrentPrice  float64 `json:"current_price"`
		Value         float64 `json:"value"`
		InitialValue  float64 `json:"initial_value"`
		PnL           float64 `json:"pnl"`
		PnLPercent    float64 `json:"pnl_percent"`
		EndDate       string  `json:"end_date,omitempty"`
	}

	type jsonOutput struct {
		Positions []jsonPosition  `json:"positions"`
		Summary   PositionSummary `json:"summary"`
	}

	output := jsonOutput{
		Positions: make([]jsonPosition, len(positions)),
		Summary:   calculateSummary(positions),
	}

	for i, pos := range positions {
		p := pos.Position
		output.Positions[i] = jsonPosition{
			MarketSlug:    p.MarketSlug,
			MarketQuestion: pos.MarketQuestion,
			Outcome:       p.Outcome,
			Status:        pos.Status,
			Size:          p.Size,
			AvgPrice:      p.AvgPrice,
			CurrentPrice:  p.CurrentPrice,
			Value:         p.Value,
			InitialValue:  p.InitialValue,
			PnL:           p.CashPnL,
			PnLPercent:    p.PercentPnL,
		}
		if !pos.MarketEndDate.IsZero() {
			output.Positions[i].EndDate = pos.MarketEndDate.Format(time.RFC3339)
		}
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	err = encoder.Encode(output)
	if err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}

	return nil
}

func displayCSVFormat(positions []EnrichedPosition) (err error) {
	writer := csv.NewWriter(os.Stdout)
	defer writer.Flush()

	// Write header
	err = writer.Write([]string{
		"Status",
		"Market",
		"Outcome",
		"Size",
		"AvgPrice",
		"CurrentPrice",
		"Value",
		"Cost",
		"PnL",
		"PnL%",
		"EndDate",
	})
	if err != nil {
		return fmt.Errorf("write CSV header: %w", err)
	}

	// Write rows
	for _, pos := range positions {
		p := pos.Position
		endDate := ""
		if !pos.MarketEndDate.IsZero() {
			endDate = pos.MarketEndDate.Format("2006-01-02")
		}

		err = writer.Write([]string{
			pos.Status,
			pos.MarketQuestion,
			p.Outcome,
			fmt.Sprintf("%.2f", p.Size),
			fmt.Sprintf("%.4f", p.AvgPrice),
			fmt.Sprintf("%.4f", p.CurrentPrice),
			fmt.Sprintf("%.2f", p.Value),
			fmt.Sprintf("%.2f", p.InitialValue),
			fmt.Sprintf("%.2f", p.CashPnL),
			fmt.Sprintf("%.2f", p.PercentPnL),
			endDate,
		})
		if err != nil {
			return fmt.Errorf("write CSV row: %w", err)
		}
	}

	return nil
}
