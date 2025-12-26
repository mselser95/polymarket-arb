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
var listOrdersCmd = &cobra.Command{
	Use:   "list-orders",
	Short: "List all open orders on Polymarket",
	Long: `List all open orders for the authenticated account.

Shows order details including market, side, price, size, and status.

Examples:
  # List all open orders
  go run . list-orders`,
	Args: cobra.NoArgs,
	RunE: runListOrders,
}

//nolint:gochecknoinits // Cobra boilerplate
func init() {
	rootCmd.AddCommand(listOrdersCmd)
}

func runListOrders(cmd *cobra.Command, args []string) (err error) {
	// Load configuration
	cfg, err := loadListOrdersConfig()
	if err != nil {
		return err
	}

	// Initialize logger
	logger, err := initListOrdersLogger(cfg)
	if err != nil {
		return err
	}
	defer func() {
		_ = logger.Sync()
	}()

	// Create OrderClient
	client, err := createListOrdersClient(logger)
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
	displayListOrdersTable(orders)
	displayListOrdersSummary(orders)

	return nil
}

func loadListOrdersConfig() (cfg *config.Config, err error) {
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

func initListOrdersLogger(cfg *config.Config) (logger *zap.Logger, err error) {
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

func createListOrdersClient(
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

func displayListOrdersTable(orders []execution.OrderInfo) {
	fmt.Println("\n========================================")
	fmt.Println("Open Orders")
	fmt.Println("========================================")
	fmt.Printf("%-14s %-32s %-10s %-10s %-10s %-8s\n",
		"Order ID", "Market", "Side", "Outcome", "Price", "Size")
	fmt.Println("--------------------------------------------------------------------------------")

	for _, order := range orders {
		// Truncate order ID to first 10 chars
		shortID := order.OrderID
		if len(shortID) > 10 {
			shortID = shortID[:10] + "..."
		}

		// Truncate market ID to first 30 chars
		market := order.Market
		if len(market) > 30 {
			market = market[:27] + "..."
		}

		// Format side and outcome
		side := order.Side
		outcome := order.Outcome
		if outcome == "" || outcome == "null" {
			outcome = "-"
		}

		fmt.Printf("%-14s %-32s %-10s %-10s $%-9s %-8s\n",
			shortID, market, side, outcome, order.Price, order.OriginalSize)
	}
}

func displayListOrdersSummary(orders []execution.OrderInfo) {
	totalValue := calculateListOrdersValue(orders)

	// Count by side
	buyCount := 0
	sellCount := 0
	for _, order := range orders {
		if order.Side == "BUY" {
			buyCount++
		} else {
			sellCount++
		}
	}

	fmt.Println("\n========================================")
	fmt.Println("Summary")
	fmt.Println("========================================")
	fmt.Printf("Total Orders:   %d\n", len(orders))
	fmt.Printf("  BUY:          %d\n", buyCount)
	fmt.Printf("  SELL:         %d\n", sellCount)
	fmt.Printf("Total Locked:   $%.2f\n", totalValue)
}

func calculateListOrdersValue(orders []execution.OrderInfo) (total float64) {
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
