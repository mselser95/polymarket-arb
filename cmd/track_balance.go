package cmd

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/joho/godotenv"
	"github.com/mselser95/polymarket-arb/pkg/healthprobe"
	"github.com/mselser95/polymarket-arb/pkg/httpserver"
	"github.com/mselser95/polymarket-arb/pkg/wallet"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

//nolint:gochecknoglobals // Cobra boilerplate
var trackBalanceCmd = &cobra.Command{
	Use:   "track-balance",
	Short: "Track wallet balance and positions with Prometheus metrics",
	Long: `Continuously monitors your wallet and exposes metrics via HTTP.

Metrics exposed at http://localhost:8080/metrics:
- polymarket_wallet_matic_balance - MATIC balance (for gas)
- polymarket_wallet_usdc_balance - USDC balance (for trading)
- polymarket_wallet_usdc_allowance - USDC approved to CTF Exchange
- polymarket_wallet_active_positions - Number of open positions
- polymarket_wallet_total_position_value - Sum of position values (USD)
- polymarket_wallet_total_position_cost - Sum of initial costs (USD)
- polymarket_wallet_unrealized_pnl - Total unrealized P&L (USD)
- polymarket_wallet_unrealized_pnl_percent - P&L percentage
- polymarket_wallet_portfolio_value - USDC + positions (USD)

Updates every minute by default. Use --interval to customize.

Example usage:
  track-balance                              # Update every 1 minute
  track-balance --interval 30s               # Update every 30 seconds
  track-balance --port 8081                  # Use custom port
  track-balance --rpc https://polygon.llamarpc.com  # Custom RPC`,
	RunE: runTrackBalance,
}

var (
	trackInterval string
	trackRPC      string
	trackPort     string
)

//nolint:gochecknoinits // Cobra boilerplate
func init() {
	rootCmd.AddCommand(trackBalanceCmd)

	trackBalanceCmd.Flags().StringVarP(&trackInterval, "interval", "i", "1m",
		"Polling interval (e.g., 30s, 1m, 5m)")
	trackBalanceCmd.Flags().StringVarP(&trackRPC, "rpc", "r",
		"https://polygon-rpc.com", "Polygon RPC endpoint")
	trackBalanceCmd.Flags().StringVarP(&trackPort, "port", "p",
		"8080", "HTTP server port for /metrics endpoint")
}

func runTrackBalance(cmd *cobra.Command, args []string) (err error) {
	// Load .env file
	envErr := godotenv.Load()
	if envErr != nil {
		fmt.Printf("Warning: .env file not found\n")
	}

	// Parse wallet address
	address, err := parseWalletAddress()
	if err != nil {
		return fmt.Errorf("parse wallet address: %w", err)
	}

	// Parse interval
	interval, err := time.ParseDuration(trackInterval)
	if err != nil {
		return fmt.Errorf("parse interval: %w", err)
	}

	// Create logger
	logger, err := createLogger()
	if err != nil {
		return fmt.Errorf("create logger: %w", err)
	}
	defer func() {
		_ = logger.Sync()
	}()

	logger.Info("wallet-tracker-starting",
		zap.String("address", address.Hex()),
		zap.Duration("interval", interval),
		zap.String("rpc", trackRPC),
		zap.String("port", trackPort))

	// Create components
	tracker, httpServer, err := createComponents(address, interval, logger)
	if err != nil {
		return fmt.Errorf("create components: %w", err)
	}

	// Run with graceful shutdown
	err = runWithGracefulShutdown(tracker, httpServer, logger)
	if err != nil {
		return fmt.Errorf("run failed: %w", err)
	}

	return nil
}

// parseWalletAddress loads and parses the wallet private key from environment.
func parseWalletAddress() (address common.Address, err error) {
	privateKeyHex := os.Getenv("POLYMARKET_PRIVATE_KEY")
	if privateKeyHex == "" {
		return common.Address{}, fmt.Errorf("POLYMARKET_PRIVATE_KEY not set in .env")
	}

	privateKey, err := crypto.HexToECDSA(strings.TrimPrefix(privateKeyHex, "0x"))
	if err != nil {
		return common.Address{}, fmt.Errorf("parse private key: %w", err)
	}

	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		return common.Address{}, fmt.Errorf("error casting public key to ECDSA")
	}

	address = crypto.PubkeyToAddress(*publicKeyECDSA)
	return address, nil
}

// createLogger creates a zap logger with appropriate configuration.
func createLogger() (logger *zap.Logger, err error) {
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}

	var zapConfig zap.Config
	if logLevel == "debug" {
		zapConfig = zap.NewDevelopmentConfig()
	} else {
		zapConfig = zap.NewProductionConfig()
	}

	logger, err = zapConfig.Build()
	if err != nil {
		return nil, fmt.Errorf("build logger: %w", err)
	}

	return logger, nil
}

// createComponents creates wallet tracker and HTTP server.
func createComponents(
	address common.Address,
	interval time.Duration,
	logger *zap.Logger,
) (tracker *wallet.Tracker, server *httpserver.Server, err error) {
	// Create wallet tracker
	tracker, err = wallet.New(&wallet.Config{
		RPCEndpoint:  trackRPC,
		Address:      address,
		PollInterval: interval,
		Logger:       logger,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("create tracker: %w", err)
	}

	// Create HTTP server for metrics
	healthChecker := healthprobe.New()
	server = httpserver.New(&httpserver.Config{
		Port:          trackPort,
		Logger:        logger,
		HealthChecker: healthChecker,
	})

	return tracker, server, nil
}

// runWithGracefulShutdown runs components with signal handling and graceful shutdown.
func runWithGracefulShutdown(
	tracker *wallet.Tracker,
	server *httpserver.Server,
	logger *zap.Logger,
) (err error) {
	// Setup context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Error channel for component failures
	errCh := make(chan error, 2)

	// Start HTTP server in goroutine
	go func() {
		startErr := server.Start()
		if startErr != nil {
			errCh <- fmt.Errorf("http server: %w", startErr)
		}
	}()

	// Start tracker in goroutine
	go func() {
		runErr := tracker.Run(ctx)
		if runErr != nil && runErr != context.Canceled {
			errCh <- fmt.Errorf("tracker: %w", runErr)
		}
	}()

	logger.Info("wallet-tracker-running",
		zap.String("metrics-url", fmt.Sprintf("http://localhost:%s/metrics", trackPort)),
		zap.String("health-url", fmt.Sprintf("http://localhost:%s/health", trackPort)))

	// Wait for shutdown signal or error
	select {
	case <-sigCh:
		logger.Info("shutdown-signal-received")
	case err = <-errCh:
		logger.Error("component-error", zap.Error(err))
	}

	// Graceful shutdown
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	shutdownErr := server.Shutdown(shutdownCtx)
	if shutdownErr != nil {
		logger.Error("http-server-shutdown-failed", zap.Error(shutdownErr))
	}

	logger.Info("wallet-tracker-stopped")
	return err
}
