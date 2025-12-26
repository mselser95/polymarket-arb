package cmd

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/joho/godotenv"
	"github.com/mselser95/polymarket-arb/internal/discovery"
	"github.com/mselser95/polymarket-arb/pkg/config"
	"github.com/mselser95/polymarket-arb/pkg/wallet"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

const (
	ctfContractAddress = "0x4bFb41d5B3570DeFd03C39a9A4D8dE6Bd8B8982E"
	polygonChainID     = 137
	redeemUSDCAddress  = "0x2791Bca1f2de4661ED88A30C99A7a9449Aa84174"
)

//nolint:gochecknoglobals // Cobra boilerplate
var redeemPositionsCmd = &cobra.Command{
	Use:   "redeem-positions",
	Short: "Redeem settled positions for USDC",
	Long: `Claims winning positions from settled markets by calling the CTF contract's
redeemPositions function. Converts winning outcome tokens to USDC at 1:1 ratio.

Requires:
- POLYMARKET_PRIVATE_KEY in .env
- MATIC balance for gas (~$0.01 per market)
- Positions in settled markets (closed=true)

Example:
  # Preview redeemable positions
  polymarket-arb redeem-positions --dry-run

  # Redeem all settled positions
  polymarket-arb redeem-positions

  # Redeem specific market
  polymarket-arb redeem-positions --market will-trump-win-2024`,
	RunE: runRedeemPositions,
}

var (
	redeemRPCURL       string
	redeemDryRun       bool
	redeemMarketSlug   string
	redeemAutoMode     bool
	redeemCheckInterval time.Duration
)

//nolint:gochecknoinits // Cobra boilerplate
func init() {
	rootCmd.AddCommand(redeemPositionsCmd)
	redeemPositionsCmd.Flags().StringVar(&redeemRPCURL, "rpc",
		"https://polygon-rpc.com", "Polygon RPC URL")
	redeemPositionsCmd.Flags().BoolVar(&redeemDryRun, "dry-run", false,
		"Show redeemable positions without executing transactions")
	redeemPositionsCmd.Flags().StringVar(&redeemMarketSlug, "market", "",
		"Redeem specific market only (optional)")
	redeemPositionsCmd.Flags().BoolVar(&redeemAutoMode, "auto", false,
		"Run continuously, checking for settled positions periodically")
	redeemPositionsCmd.Flags().DurationVar(&redeemCheckInterval, "interval", 1*time.Hour,
		"Check interval in auto mode (default: 1h)")
}

func runRedeemPositions(cmd *cobra.Command, args []string) (err error) {
	if redeemAutoMode {
		return runAutoMode(cmd)
	}
	return runOnceMode(cmd)
}

func runAutoMode(cmd *cobra.Command) (err error) {
	// Load .env
	err = godotenv.Load()
	if err != nil {
		fmt.Printf("Warning: .env file not found\n")
	}

	// Create logger
	logger, err := config.NewLogger()
	if err != nil {
		return fmt.Errorf("create logger: %w", err)
	}
	defer func() {
		_ = logger.Sync()
	}()

	logger.Info("position-redeemer-starting-auto-mode",
		zap.Duration("interval", redeemCheckInterval),
		zap.Bool("dry-run", redeemDryRun))

	fmt.Printf("=== Polymarket Position Redeemer (Auto Mode) ===\n\n")
	fmt.Printf("Check interval: %s\n", redeemCheckInterval)
	fmt.Printf("Mode: %s\n\n", map[bool]string{true: "DRY RUN", false: "LIVE"}[redeemDryRun])

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// Start background monitor
	go func() {
		<-sigCh
		logger.Info("shutdown-signal-received")
		fmt.Printf("\nShutdown signal received, stopping...\n")
		cancel()
	}()

	// Run initial check immediately
	err = executeRedemption(ctx, cmd, logger)
	if err != nil {
		logger.Error("initial-check-failed", zap.Error(err))
	}

	// Start periodic checking
	ticker := time.NewTicker(redeemCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("position-redeemer-stopped")
			fmt.Printf("Position redeemer stopped.\n")
			return nil
		case <-ticker.C:
			err = executeRedemption(ctx, cmd, logger)
			if err != nil {
				logger.Error("redemption-check-failed", zap.Error(err))
			}
		}
	}
}

func runOnceMode(cmd *cobra.Command) (err error) {
	ctx := cmd.Context()

	// Load .env
	err = godotenv.Load()
	if err != nil {
		fmt.Printf("Warning: .env file not found\n")
	}

	// Create logger
	logger, err := config.NewLogger()
	if err != nil {
		return fmt.Errorf("create logger: %w", err)
	}
	defer func() {
		_ = logger.Sync()
	}()

	return executeRedemption(ctx, cmd, logger)
}

func executeRedemption(ctx context.Context, cmd *cobra.Command, logger *zap.Logger) (err error) {
	// Load config
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Load private key
	privateKeyHex := os.Getenv("POLYMARKET_PRIVATE_KEY")
	if privateKeyHex == "" {
		return errors.New("POLYMARKET_PRIVATE_KEY not set")
	}

	privateKey, err := crypto.HexToECDSA(strings.TrimPrefix(privateKeyHex, "0x"))
	if err != nil {
		return fmt.Errorf("parse private key: %w", err)
	}

	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		return errors.New("error casting public key to ECDSA")
	}

	address := crypto.PubkeyToAddress(*publicKeyECDSA)

	logger.Info("redeem-positions-check",
		zap.String("address", address.Hex()),
		zap.Bool("dry-run", redeemDryRun),
		zap.Bool("auto-mode", redeemAutoMode))

	if !redeemAutoMode {
		fmt.Printf("=== Polymarket Position Redemption ===\n\n")
		fmt.Printf("Address: %s\n", address.Hex())
		fmt.Printf("Mode: %s\n\n", map[bool]string{true: "DRY RUN", false: "LIVE"}[redeemDryRun])
	} else {
		logger.Info("checking-for-settled-positions",
			zap.String("address", address.Hex()))
	}

	// Connect to RPC
	client, err := ethclient.DialContext(ctx, redeemRPCURL)
	if err != nil {
		return fmt.Errorf("dial RPC: %w", err)
	}
	defer client.Close()

	// Fetch positions from Data API
	walletClient, err := wallet.NewClient(redeemRPCURL, logger)
	if err != nil {
		return fmt.Errorf("create wallet client: %w", err)
	}

	positions, err := walletClient.GetPositions(ctx, address.Hex())
	if err != nil {
		return fmt.Errorf("get positions: %w", err)
	}

	if len(positions) == 0 {
		fmt.Printf("No positions found.\n")
		logger.Info("no-positions-found")
		return nil
	}

	fmt.Printf("Found %d total position(s)\n\n", len(positions))
	logger.Info("positions-fetched", zap.Int("count", len(positions)))

	// Filter for settled markets and redeem each
	var redeemed int
	var totalUSDC float64
	var skipped int

	for i := range positions {
		position := &positions[i]

		// Filter by market slug if specified
		if redeemMarketSlug != "" && position.MarketSlug != redeemMarketSlug {
			continue
		}

		// Check if market is settled (closed)
		isSettled, settleErr := isMarketSettled(ctx, position.MarketSlug, cfg)
		if settleErr != nil {
			logger.Error("failed-to-check-market-state",
				zap.String("slug", position.MarketSlug),
				zap.Error(settleErr))
			fmt.Printf("⚠️  %s: Failed to check market state - %v\n", position.MarketSlug, settleErr)
			skipped++
			continue
		}

		if !isSettled {
			logger.Debug("skipping-unsettled-market",
				zap.String("slug", position.MarketSlug))
			skipped++
			continue
		}

		// Redeem position
		usdcAmount, err := redeemPosition(ctx, client, privateKey, address, position, logger, redeemDryRun)
		if err != nil {
			logger.Error("redeem-failed",
				zap.String("slug", position.MarketSlug),
				zap.Error(err))
			fmt.Printf("❌ %s: Redemption failed - %v\n", position.MarketSlug, err)
			continue
		}

		redeemed++
		totalUSDC += usdcAmount

		if redeemDryRun {
			fmt.Printf("✓  %s (%s): Would redeem %.2f USDC\n",
				position.MarketSlug, position.Outcome, usdcAmount)
		} else {
			fmt.Printf("✓  %s (%s): Redeemed %.2f USDC\n",
				position.MarketSlug, position.Outcome, usdcAmount)
		}

		logger.Info("position-redeemed",
			zap.String("slug", position.MarketSlug),
			zap.String("outcome", position.Outcome),
			zap.Float64("usdc", usdcAmount),
			zap.Bool("dry-run", redeemDryRun))
	}

	fmt.Printf("\n=== Summary ===\n")
	fmt.Printf("Total positions: %d\n", len(positions))
	fmt.Printf("Redeemed: %d\n", redeemed)
	fmt.Printf("Skipped (unsettled): %d\n", skipped)
	fmt.Printf("Total USDC: %.2f\n", totalUSDC)

	logger.Info("redemption-complete",
		zap.Int("positions-redeemed", redeemed),
		zap.Int("positions-skipped", skipped),
		zap.Float64("total-usdc", totalUSDC))

	return nil
}

func redeemPosition(
	ctx context.Context,
	client *ethclient.Client,
	privateKey *ecdsa.PrivateKey,
	address common.Address,
	position *wallet.Position,
	logger *zap.Logger,
	dryRun bool,
) (usdcAmount float64, err error) {
	// Parse condition ID from position
	conditionIDBytes := common.HexToHash(position.ConditionID)

	// Determine index set based on outcome
	var indexSet *big.Int
	if strings.EqualFold(position.Outcome, "Yes") {
		indexSet = big.NewInt(1) // Bit 0 set
	} else if strings.EqualFold(position.Outcome, "No") {
		indexSet = big.NewInt(2) // Bit 1 set
	} else {
		return 0, fmt.Errorf("unknown outcome: %s", position.Outcome)
	}

	// Build redeemPositions call data
	redeemABI := `[{
		"inputs": [
			{"name": "collateralToken", "type": "address"},
			{"name": "parentCollectionId", "type": "bytes32"},
			{"name": "conditionId", "type": "bytes32"},
			{"name": "indexSets", "type": "uint256[]"}
		],
		"name": "redeemPositions",
		"outputs": [],
		"stateMutability": "nonpayable",
		"type": "function"
	}]`

	parsedABI, err := abi.JSON(strings.NewReader(redeemABI))
	if err != nil {
		return 0, fmt.Errorf("parse ABI: %w", err)
	}

	usdcAddr := common.HexToAddress(redeemUSDCAddress)
	parentCollectionID := common.Hash{} // Zero/null for Polymarket
	indexSets := []*big.Int{indexSet}

	data, err := parsedABI.Pack("redeemPositions",
		usdcAddr,
		parentCollectionID,
		conditionIDBytes,
		indexSets)
	if err != nil {
		return 0, fmt.Errorf("pack call data: %w", err)
	}

	if dryRun {
		logger.Info("dry-run-would-redeem",
			zap.String("condition-id", position.ConditionID),
			zap.String("outcome", position.Outcome),
			zap.Float64("size", position.Size))
		return position.Size, nil
	}

	// Build transaction
	nonce, err := client.PendingNonceAt(ctx, address)
	if err != nil {
		return 0, fmt.Errorf("get nonce: %w", err)
	}

	gasPrice, err := client.SuggestGasPrice(ctx)
	if err != nil {
		return 0, fmt.Errorf("suggest gas price: %w", err)
	}

	ctfAddress := common.HexToAddress(ctfContractAddress)
	tx := types.NewTransaction(
		nonce,
		ctfAddress,
		big.NewInt(0),  // value: 0
		uint64(200000), // gas limit (estimate)
		gasPrice,
		data,
	)

	// Sign transaction
	chainID := big.NewInt(polygonChainID)
	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), privateKey)
	if err != nil {
		return 0, fmt.Errorf("sign tx: %w", err)
	}

	// Send transaction
	err = client.SendTransaction(ctx, signedTx)
	if err != nil {
		return 0, fmt.Errorf("send tx: %w", err)
	}

	logger.Info("redemption-tx-sent",
		zap.String("tx-hash", signedTx.Hash().Hex()))

	// Wait for confirmation
	receipt, err := bind.WaitMined(ctx, client, signedTx)
	if err != nil {
		return 0, fmt.Errorf("wait for tx: %w", err)
	}

	if receipt.Status != types.ReceiptStatusSuccessful {
		return 0, errors.New("transaction failed")
	}

	logger.Info("redemption-confirmed",
		zap.String("tx-hash", receipt.TxHash.Hex()),
		zap.Uint64("gas-used", receipt.GasUsed))

	return position.Size, nil
}

func isMarketSettled(ctx context.Context, marketSlug string, cfg *config.Config) (settled bool, err error) {
	// Create logger for discovery client
	logger, err := config.NewLogger()
	if err != nil {
		return false, fmt.Errorf("create logger: %w", err)
	}
	defer func() {
		_ = logger.Sync()
	}()

	// Create discovery client to query market state
	client := discovery.NewClient(cfg.PolymarketGammaURL, logger)

	market, err := client.FetchMarketBySlug(ctx, marketSlug)
	if err != nil {
		return false, fmt.Errorf("fetch market: %w", err)
	}

	return market.Closed, nil
}
