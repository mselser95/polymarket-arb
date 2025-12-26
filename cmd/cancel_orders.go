package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
	"github.com/mselser95/polymarket-arb/internal/execution"
	"github.com/mselser95/polymarket-arb/pkg/config"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

//nolint:gochecknoglobals // Cobra boilerplate
var cancelOrdersCmd = &cobra.Command{
	Use:   "cancel-orders",
	Short: "Cancel all open orders on Polymarket",
	Long: `Cancel all open orders atomically using the /cancel-all endpoint.

Use --dry-run to preview orders without canceling.

Examples:
  # Preview orders without canceling
  go run . cancel-orders --dry-run

  # Cancel all orders immediately
  go run . cancel-orders`,
	Args: cobra.NoArgs,
	RunE: runCancelOrders,
}

//nolint:gochecknoglobals // Cobra boilerplate
var dryRunFlag bool

//nolint:gochecknoinits // Cobra boilerplate
func init() {
	rootCmd.AddCommand(cancelOrdersCmd)
	cancelOrdersCmd.Flags().BoolVar(&dryRunFlag, "dry-run", false, "Preview orders without canceling")
}

func runCancelOrders(cmd *cobra.Command, args []string) (err error) {
	// Load configuration
	cfg, err := loadCancelOrdersConfig()
	if err != nil {
		return err
	}

	// Initialize logger
	logger, err := initCancelOrdersLogger(cfg)
	if err != nil {
		return err
	}
	defer func() {
		_ = logger.Sync()
	}()

	// Create OrderClient
	client, err := createCancelOrdersClient(logger)
	if err != nil {
		return err
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Fetch open orders
	orders, err := client.GetOpenOrders(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch open orders: %w", err)
	}

	// Handle empty case
	if len(orders) == 0 {
		fmt.Println("No open orders found.")
		return nil
	}

	// Display orders table
	displayCancelOrdersTable(orders)
	displayCancelOrdersSummary(orders)

	// Exit if dry-run
	if dryRunFlag {
		fmt.Println("\n[DRY RUN] No orders were canceled.")
		return nil
	}

	// Execute cancellation
	fmt.Println("\nCanceling all orders...")
	result, err := client.CancelAllOrders(ctx)
	if err != nil {
		return fmt.Errorf("failed to cancel orders: %w", err)
	}

	// Display results
	displayCancelResults(result)

	return nil
}

func loadCancelOrdersConfig() (cfg *config.Config, err error) {
	// Load .env file if exists
	err = godotenv.Load()
	if err != nil && !os.IsNotExist(err) {
		err = fmt.Errorf("failed to load .env: %w", err)
		return cfg, err
	}

	cfg, err = config.LoadFromEnv()
	if err != nil {
		err = fmt.Errorf("failed to load config: %w", err)
		return cfg, err
	}

	return cfg, nil
}

func initCancelOrdersLogger(cfg *config.Config) (logger *zap.Logger, err error) {
	logLevel := zapcore.InfoLevel
	err = logLevel.UnmarshalText([]byte(cfg.LogLevel))
	if err != nil {
		err = fmt.Errorf("invalid log level: %w", err)
		return logger, err
	}

	zapConfig := zap.NewProductionConfig()
	zapConfig.Level = zap.NewAtomicLevelAt(logLevel)
	logger, err = zapConfig.Build()
	if err != nil {
		err = fmt.Errorf("failed to create logger: %w", err)
		return logger, err
	}

	return logger, nil
}

func createCancelOrdersClient(
	logger *zap.Logger,
) (client *execution.OrderClient, err error) {
	// Load credentials from environment
	apiKey := os.Getenv("POLYMARKET_API_KEY")
	secret := os.Getenv("POLYMARKET_SECRET")
	passphrase := os.Getenv("POLYMARKET_PASSPHRASE")
	privateKey := os.Getenv("POLYMARKET_PRIVATE_KEY")

	// Validate required credentials
	if apiKey == "" {
		err = errors.New("POLYMARKET_API_KEY not set")
		return client, err
	}
	if secret == "" {
		err = errors.New("POLYMARKET_SECRET not set")
		return client, err
	}
	if passphrase == "" {
		err = errors.New("POLYMARKET_PASSPHRASE not set")
		return client, err
	}
	if privateKey == "" {
		err = errors.New("POLYMARKET_PRIVATE_KEY not set")
		return client, err
	}

	// Load optional fields
	address := os.Getenv("POLYMARKET_ADDRESS")
	sigTypeStr := os.Getenv("POLYMARKET_SIGNATURE_TYPE")
	if sigTypeStr == "" {
		sigTypeStr = "0"
	}

	sigType, err := strconv.Atoi(sigTypeStr)
	if err != nil {
		err = fmt.Errorf("invalid POLYMARKET_SIGNATURE_TYPE: %w", err)
		return client, err
	}

	clientCfg := &execution.OrderClientConfig{
		APIKey:        apiKey,
		Secret:        secret,
		Passphrase:    passphrase,
		PrivateKey:    privateKey,
		Address:       address,
		ProxyAddress:  "", // Empty for EOA signatures (maker == signer)
		SignatureType: sigType,
		Logger:        logger,
	}

	client, err = execution.NewOrderClient(clientCfg)
	if err != nil {
		err = fmt.Errorf("failed to create order client: %w", err)
		return client, err
	}

	return client, nil
}

func displayCancelOrdersTable(orders []execution.OrderInfo) {
	fmt.Println("\n========================================")
	fmt.Println("Open Orders")
	fmt.Println("========================================")
	fmt.Printf("%-12s %-30s %-10s %-8s %-10s\n",
		"Order ID", "Market", "Side", "Price", "Size")
	fmt.Println("----------------------------------------")

	for _, order := range orders {
		// Truncate order ID to first 8 chars
		shortID := order.OrderID
		if len(shortID) > 8 {
			shortID = shortID[:8] + "..."
		}

		// Truncate market slug if too long
		market := order.Market
		if len(market) > 30 {
			market = market[:27] + "..."
		}

		// Format side with outcome if available
		side := order.Side
		if order.Outcome != "" && order.Outcome != "null" {
			side = order.Outcome
		}

		fmt.Printf("%-12s %-30s %-10s $%-7s %-10s\n",
			shortID, market, side, order.Price, order.OriginalSize)
	}
}

func displayCancelOrdersSummary(orders []execution.OrderInfo) {
	totalValue := calculateCancelOrdersValue(orders)
	fmt.Printf("\nTotal: %d orders, $%.2f locked\n", len(orders), totalValue)
}

func calculateCancelOrdersValue(orders []execution.OrderInfo) (total float64) {
	for _, order := range orders {
		price, err := strconv.ParseFloat(order.Price, 64)
		if err != nil {
			continue
		}
		size, err := strconv.ParseFloat(order.OriginalSize, 64)
		if err != nil {
			continue
		}
		total += price * size
	}
	return total
}

func displayCancelResults(result execution.CancelAllResult) {
	fmt.Println("\n========================================")
	fmt.Println("Cancellation Results")
	fmt.Println("========================================")

	fmt.Printf("✅ Canceled: %d orders\n", len(result.Canceled))

	if len(result.NotCanceled) > 0 {
		fmt.Printf("❌ Not canceled: %d orders\n", len(result.NotCanceled))
		fmt.Println("\nFailed cancellations:")
		for orderID, reason := range result.NotCanceled {
			shortID := orderID
			if len(shortID) > 12 {
				shortID = shortID[:12] + "..."
			}
			fmt.Printf("  - %s: %s\n", shortID, reason)
		}
	}
}
