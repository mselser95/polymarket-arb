package cmd

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/joho/godotenv"
	"github.com/mselser95/polymarket-arb/internal/discovery"
	"github.com/mselser95/polymarket-arb/internal/execution"
	"github.com/mselser95/polymarket-arb/internal/markets"
	"github.com/mselser95/polymarket-arb/pkg/config"
	"github.com/mselser95/polymarket-arb/pkg/types"
	"github.com/mselser95/polymarket-arb/pkg/wallet"
	"github.com/polymarket/go-order-utils/pkg/model"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

//nolint:gochecknoglobals // Cobra boilerplate
var closePositionsCmd = &cobra.Command{
	Use:   "close-positions",
	Short: "Close all open positions by selling at market prices",
	Long: `Fetches all open positions and places market sell orders to close them.

This command will:
1. Fetch all your open positions from Polymarket
2. Get current market bid prices for each position
3. Show a summary and ask for confirmation
4. Place SELL orders at market prices
5. Report results with execution details

Example:
  close-positions              # Close all positions with confirmation
  close-positions --yes        # Skip confirmation (use with caution!)
`,
	RunE: runClosePositions,
}

var (
	skipConfirmation bool
)

//nolint:gochecknoinits // Cobra boilerplate
func init() {
	rootCmd.AddCommand(closePositionsCmd)
	closePositionsCmd.Flags().BoolVar(&skipConfirmation, "yes", false, "Skip confirmation prompt")
}

// PositionToClose holds position data with market info for closing.
type PositionToClose struct {
	Position  wallet.Position
	Market    *types.Market
	TokenID   string
	BidPrice  float64
	TickSize  float64
	MinSize   float64
}

// CloseResult holds the result of closing a single position.
type CloseResult struct {
	Position    wallet.Position
	Success     bool
	OrderID     string
	USDReceived float64
	Error       error
}

func runClosePositions(cmd *cobra.Command, args []string) (err error) {
	// Load .env
	envErr := godotenv.Load()
	if envErr != nil {
		fmt.Printf("Warning: .env file not found\n")
	}

	// Parse wallet address
	address, privateKey, err := parseWalletCredentials()
	if err != nil {
		return fmt.Errorf("parse credentials: %w", err)
	}

	// Create logger
	logger, err := createCloseLogger()
	if err != nil {
		return fmt.Errorf("create logger: %w", err)
	}
	defer func() {
		_ = logger.Sync()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	fmt.Printf("\n=== Close All Positions ===\n\n")

	// Step 1: Fetch positions
	fmt.Printf("Fetching open positions...\n")
	positionsToClose, err := fetchPositionsToClose(ctx, address, logger)
	if err != nil {
		return fmt.Errorf("fetch positions: %w", err)
	}

	if len(positionsToClose) == 0 {
		fmt.Printf("✅ No open positions to close.\n")
		return nil
	}

	// Step 2: Show confirmation
	if !skipConfirmation {
		confirmed, err := showConfirmationPrompt(positionsToClose)
		if err != nil {
			return fmt.Errorf("confirmation prompt: %w", err)
		}
		if !confirmed {
			fmt.Printf("\n❌ Operation cancelled by user.\n")
			return nil
		}
	}

	// Step 3: Submit orders
	fmt.Printf("\n=== Submitting Orders ===\n\n")
	results, err := submitCloseOrders(ctx, positionsToClose, address, privateKey, logger)
	if err != nil {
		return fmt.Errorf("submit orders: %w", err)
	}

	// Step 4: Report results
	reportResults(results)

	return nil
}

// parseWalletCredentials loads and parses wallet credentials from environment.
func parseWalletCredentials() (address common.Address, privateKey *ecdsa.PrivateKey, err error) {
	privateKeyHex := os.Getenv("POLYMARKET_PRIVATE_KEY")
	if privateKeyHex == "" {
		return common.Address{}, nil, errors.New("POLYMARKET_PRIVATE_KEY not set in .env")
	}

	privateKey, err = crypto.HexToECDSA(strings.TrimPrefix(privateKeyHex, "0x"))
	if err != nil {
		return common.Address{}, nil, fmt.Errorf("parse private key: %w", err)
	}

	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		return common.Address{}, nil, errors.New("error casting public key to ECDSA")
	}

	address = crypto.PubkeyToAddress(*publicKeyECDSA)
	return address, privateKey, nil
}

// createCloseLogger creates a logger for the close command.
func createCloseLogger() (logger *zap.Logger, err error) {
	cfg := zap.NewProductionConfig()
	cfg.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)

	logger, err = cfg.Build()
	if err != nil {
		return nil, fmt.Errorf("build logger: %w", err)
	}

	return logger, nil
}

// fetchPositionsToClose fetches positions and enriches with market data.
func fetchPositionsToClose(
	ctx context.Context,
	address common.Address,
	logger *zap.Logger,
) (positionsToClose []PositionToClose, err error) {
	// Create wallet client
	walletClient, err := wallet.NewClient("https://polygon-rpc.com", logger)
	if err != nil {
		return nil, fmt.Errorf("create wallet client: %w", err)
	}

	// Fetch positions
	positions, err := walletClient.GetPositions(ctx, address.Hex())
	if err != nil {
		return nil, fmt.Errorf("get positions: %w", err)
	}

	if len(positions) == 0 {
		return nil, nil
	}

	// Create discovery client
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	discoveryClient := discovery.NewClient(cfg.PolymarketGammaURL, logger)

	// Create metadata client
	metadataClient := markets.NewMetadataClient(cfg.PolymarketGammaURL, logger)

	// Enrich each position with market data
	positionsToClose = make([]PositionToClose, 0, len(positions))

	for _, pos := range positions {
		ptc, err := enrichPosition(ctx, pos, discoveryClient, metadataClient)
		if err != nil {
			fmt.Printf("⚠️  Warning: Skipping %s (%s): %v\n", pos.MarketSlug, pos.Outcome, err)
			continue
		}
		positionsToClose = append(positionsToClose, ptc)
	}

	return positionsToClose, nil
}

// enrichPosition fetches market data for a position.
func enrichPosition(
	ctx context.Context,
	pos wallet.Position,
	discoveryClient *discovery.Client,
	metadataClient *markets.MetadataClient,
) (ptc PositionToClose, err error) {
	// Fetch market by slug
	market, err := discoveryClient.FetchMarketBySlug(ctx, pos.MarketSlug)
	if err != nil {
		return PositionToClose{}, fmt.Errorf("fetch market: %w", err)
	}

	// Find token ID for outcome
	tokenID, err := findTokenIDForOutcome(market, pos.Outcome)
	if err != nil {
		return PositionToClose{}, fmt.Errorf("find token ID: %w", err)
	}

	// Fetch token metadata
	metadata, err := metadataClient.GetTokenMetadata(ctx, tokenID)
	if err != nil {
		return PositionToClose{}, fmt.Errorf("get metadata: %w", err)
	}

	// Fetch current bid price
	bidPrice, err := discoveryClient.FetchMarketBidPrice(ctx, market.ConditionID, pos.Outcome)
	if err != nil {
		return PositionToClose{}, fmt.Errorf("fetch bid price: %w", err)
	}

	if bidPrice <= 0 {
		return PositionToClose{}, fmt.Errorf("no bids available (price: %.4f)", bidPrice)
	}

	ptc = PositionToClose{
		Position:  pos,
		Market:    market,
		TokenID:   tokenID,
		BidPrice:  bidPrice,
		TickSize:  metadata.MinimumTickSize,
		MinSize:   metadata.MinimumOrderSize,
	}

	return ptc, nil
}

// findTokenIDForOutcome finds the token ID for a given outcome.
func findTokenIDForOutcome(market *types.Market, outcome string) (tokenID string, err error) {
	outcomeNormalized := strings.ToLower(strings.TrimSpace(outcome))

	for _, token := range market.Tokens {
		if strings.ToLower(token.Outcome) == outcomeNormalized {
			return token.TokenID, nil
		}
	}

	return "", fmt.Errorf("outcome %q not found in market tokens", outcome)
}

// showConfirmationPrompt displays positions and asks for confirmation.
func showConfirmationPrompt(positions []PositionToClose) (confirmed bool, err error) {
	fmt.Printf("Positions to close:\n\n")

	totalProceeds := 0.0
	for i, ptc := range positions {
		proceeds := ptc.Position.Size * ptc.BidPrice
		totalProceeds += proceeds

		fmt.Printf("[%d] %s (%s)\n", i+1, ptc.Position.MarketSlug, ptc.Position.Outcome)
		fmt.Printf("    %.2f tokens @ $%.4f = $%.2f\n",
			ptc.Position.Size, ptc.BidPrice, proceeds)
	}

	fmt.Printf("\nTotal positions: %d\n", len(positions))
	fmt.Printf("Total estimated proceeds: $%.2f USDC\n", totalProceeds)
	fmt.Printf("\n⚠️  This will place market sell orders. Proceed? [y/N]: ")

	var response string
	_, err = fmt.Scanln(&response)
	if err != nil && err.Error() != "unexpected newline" {
		return false, fmt.Errorf("read input: %w", err)
	}

	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes", nil
}

// submitCloseOrders builds and submits sell orders.
func submitCloseOrders(
	ctx context.Context,
	positions []PositionToClose,
	address common.Address,
	privateKey *ecdsa.PrivateKey,
	logger *zap.Logger,
) (results []CloseResult, err error) {
	// Create order client
	apiKey := os.Getenv("POLYMARKET_API_KEY")
	secret := os.Getenv("POLYMARKET_SECRET")
	passphrase := os.Getenv("POLYMARKET_API_PASSPHRASE")

	if apiKey == "" || secret == "" || passphrase == "" {
		return nil, errors.New("POLYMARKET_API_KEY, SECRET, and PASSPHRASE must be set")
	}

	sigType := os.Getenv("POLYMARKET_SIGNATURE_TYPE")
	if sigType == "" {
		sigType = "0"
	}

	orderClient, err := execution.NewOrderClient(&execution.OrderClientConfig{
		APIKey:        apiKey,
		Secret:        secret,
		Passphrase:    passphrase,
		PrivateKey:    privateKey,
		Address:       address,
		SignatureType: model.SignatureType(mustParseInt(sigType)),
		Logger:        logger,
	})
	if err != nil {
		return nil, fmt.Errorf("create order client: %w", err)
	}

	results = make([]CloseResult, 0, len(positions))

	// Submit orders individually with progress tracking
	for i, ptc := range positions {
		fmt.Printf("[%d/%d] Closing %s (%s)...\n",
			i+1, len(positions), ptc.Position.MarketSlug, ptc.Position.Outcome)

		result := submitSingleCloseOrder(ctx, orderClient, ptc)
		results = append(results, result)

		if result.Success {
			fmt.Printf("  ✅ Order placed: %s\n", result.OrderID)
		} else {
			fmt.Printf("  ❌ Failed: %v\n", result.Error)
		}
	}

	return results, nil
}

// submitSingleCloseOrder submits a single sell order.
func submitSingleCloseOrder(
	ctx context.Context,
	orderClient *execution.OrderClient,
	ptc PositionToClose,
) (result CloseResult) {
	// Build sell order
	usdcAmount := ptc.Position.Size * ptc.BidPrice * 1e6 // 6 decimals
	tokenAmount := ptc.Position.Size

	// TODO: Round amounts using tick size and min size
	// For now, using raw amounts

	orderData := model.OrderData{
		Maker:         orderClient.GetMakerAddress(),
		Taker:         "0x0000000000000000000000000000000000000000",
		TokenID:       ptc.TokenID,
		MakerAmount:   fmt.Sprintf("%.0f", usdcAmount),
		TakerAmount:   fmt.Sprintf("%.0f", tokenAmount*1e6), // Assuming 6 decimals
		Side:          model.SELL,
		FeeRateBps:    "0",
		Nonce:         "0",
		Signer:        orderClient.GetSignerAddress().Hex(),
		Expiration:    "0",
		SignatureType: orderClient.GetSignatureType(),
	}

	// Place order
	resp, err := orderClient.PlaceOrder(ctx, orderData)
	if err != nil {
		return CloseResult{
			Position: ptc.Position,
			Success:  false,
			Error:    err,
		}
	}

	return CloseResult{
		Position:    ptc.Position,
		Success:     resp.Success,
		OrderID:     resp.OrderID,
		USDReceived: ptc.Position.Size * ptc.BidPrice,
		Error:       nil,
	}
}

// reportResults displays execution summary.
func reportResults(results []CloseResult) {
	fmt.Printf("\n=== Execution Summary ===\n\n")

	successCount := 0
	totalUSD := 0.0

	fmt.Printf("Successfully closed:\n")
	for _, r := range results {
		if r.Success {
			successCount++
			totalUSD += r.USDReceived
			fmt.Printf("✅ %s (%s) - %.2f tokens sold ≈ $%.2f received\n",
				r.Position.MarketSlug, r.Position.Outcome, r.Position.Size, r.USDReceived)
		}
	}

	if successCount < len(results) {
		fmt.Printf("\nFailed:\n")
		for _, r := range results {
			if !r.Success {
				fmt.Printf("❌ %s (%s) - Error: %v\n",
					r.Position.MarketSlug, r.Position.Outcome, r.Error)
			}
		}
	}

	fmt.Printf("\nSummary:\n")
	fmt.Printf("- Closed: %d/%d positions\n", successCount, len(results))
	fmt.Printf("- Total USDC received (estimated): $%.2f\n", totalUSD)

	if successCount < len(results) {
		fmt.Printf("- Errors: %d\n", len(results)-successCount)
	}
}

func mustParseInt(s string) int {
	var i int
	fmt.Sscanf(s, "%d", &i)
	return i
}
