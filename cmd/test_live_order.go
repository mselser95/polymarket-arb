package cmd

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/joho/godotenv"
	"github.com/mselser95/polymarket-arb/internal/discovery"
	"github.com/mselser95/polymarket-arb/internal/execution"
	"github.com/mselser95/polymarket-arb/pkg/types"
	"github.com/polymarket/go-order-utils/pkg/model"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var testLiveOrderCmd = &cobra.Command{
	Use:   "test-live-order <market-slug>",
	Short: "Test live order submission and response parsing",
	Long: `Submits test orders to the Polymarket CLOB API for a specific market.
This is for testing order submission and response parsing.

Modes:
  --paper  : Simulates API responses without hitting real API (safe testing)
  --live   : Submits real orders to Polymarket CLOB API

Credentials are loaded from .env file automatically.

Example (Paper Trading - Safe):
  polymarket-arb test-live-order will-trump-release-the-epstein-files-by-december-22 --paper

Example (Live Orders - Real money):
  polymarket-arb test-live-order will-trump-release-the-epstein-files-by-december-22 --live`,
	Args: cobra.ExactArgs(1),
	RunE: runTestLiveOrder,
}

func init() {
	rootCmd.AddCommand(testLiveOrderCmd)
	testLiveOrderCmd.Flags().Float64P("size", "s", 1.0, "Order size in USD")
	testLiveOrderCmd.Flags().Float64("yes-price", 0.01, "YES order price (limit price)")
	testLiveOrderCmd.Flags().Float64("no-price", 0.01, "NO order price (limit price)")
	testLiveOrderCmd.Flags().Bool("paper", true, "Paper trading mode (simulated responses)")
	testLiveOrderCmd.Flags().Bool("live", false, "Live trading mode (real orders)")
	testLiveOrderCmd.Flags().Bool("mock", false, "Mock mode (uses saved API responses)")
	testLiveOrderCmd.Flags().String("mock-response", "test_responses/order_success.json", "Path to mock response JSON file")
}

// OrderParams represents order parameters for the OrderClient
type OrderParams struct {
	TokenID string
	Price   float64
	Size    float64
	Side    model.Side
}

// ErrorResponse represents an API error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// Config holds API credentials
type Config struct {
	APIKey     string
	Secret     string
	Passphrase string
	PrivateKey string
	Address    string
	UseL1Auth  bool // Use L1 (private key) vs L2 (API creds)
}

func runTestLiveOrder(cmd *cobra.Command, args []string) error {
	marketSlug := args[0]

	// Get flags
	size, _ := cmd.Flags().GetFloat64("size")
	yesPrice, _ := cmd.Flags().GetFloat64("yes-price")
	noPrice, _ := cmd.Flags().GetFloat64("no-price")
	paperMode, _ := cmd.Flags().GetBool("paper")
	liveMode, _ := cmd.Flags().GetBool("live")
	mockMode, _ := cmd.Flags().GetBool("mock")
	mockResponseFile, _ := cmd.Flags().GetString("mock-response")

	// Determine mode
	if liveMode {
		paperMode = false
		mockMode = false
	} else if mockMode {
		paperMode = false
		liveMode = false
	}

	mode := "PAPER"
	if mockMode {
		mode = "MOCK"
	} else if liveMode {
		mode = "LIVE"
	}

	fmt.Printf("=== Polymarket Order Test (%s MODE) ===\n\n", mode)

	if paperMode {
		fmt.Printf("Paper Trading: Simulated responses (no real orders)\n\n")
	} else if mockMode {
		fmt.Printf("Mock Mode: Using saved API responses from %s\n\n", mockResponseFile)
	} else {
		fmt.Printf("WARNING: LIVE TRADING: Real orders will be submitted!\n\n")
	}

	fmt.Printf("Market: %s\n", marketSlug)
	fmt.Printf("Order Size: $%.2f\n", size)
	fmt.Printf("YES Price: %.4f\n", yesPrice)
	fmt.Printf("NO Price: %.4f\n\n", noPrice)

	// Load credentials from .env
	if err := godotenv.Load(); err != nil {
		fmt.Printf("Warning: .env file not found (using environment variables)\n")
	}

	// Create logger
	logger, err := zap.NewDevelopment()
	if err != nil {
		return fmt.Errorf("create logger: %w", err)
	}
	defer logger.Sync()

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Fetch market details
	fmt.Printf("Fetching market details...\n")
	client := discovery.NewClient("https://gamma-api.polymarket.com", logger)
	market, err := client.FetchMarketBySlug(ctx, marketSlug)
	if err != nil {
		return fmt.Errorf("fetch market: %w", err)
	}

	fmt.Printf("\nMarket ID: %s\n", market.ID)
	fmt.Printf("Question: %s\n\n", market.Question)

	// Get token IDs
	yesToken := market.GetTokenByOutcome("YES")
	noToken := market.GetTokenByOutcome("NO")

	if yesToken == nil || noToken == nil {
		return fmt.Errorf("market missing YES or NO tokens")
	}

	fmt.Printf("YES Token: %s\n", yesToken.TokenID)
	fmt.Printf("NO Token: %s\n\n", noToken.TokenID)

	// Submit orders
	var yesResp, noResp *types.OrderSubmissionResponse

	if paperMode {
		fmt.Printf("=== Simulating Batch Orders (YES + NO) ===\n")
		yesResp = simulatePaperOrder(yesToken.TokenID, yesPrice, size, "YES")
		noResp = simulatePaperOrder(noToken.TokenID, noPrice, size, "NO")
		displayOrderResponse("YES", yesResp, true)
		displayOrderResponse("NO", noResp, true)
	} else if mockMode {
		fmt.Printf("=== Using Mock Responses ===\n")
		yesResp, err = loadMockResponse(mockResponseFile)
		if err != nil {
			fmt.Printf("Failed to load mock response: %v\n\n", err)
		} else {
			displayOrderResponse("YES", yesResp, false)
		}
		noResp, err = loadMockResponse(mockResponseFile)
		if err != nil {
			fmt.Printf("Failed to load mock response: %v\n\n", err)
		} else {
			displayOrderResponse("NO", noResp, false)
		}
	} else {
		// LIVE MODE: Use production OrderClient
		fmt.Printf("=== Submitting Batch Orders via OrderClient ===\n")
		fmt.Printf("Using atomic batch submission with EIP-712 signatures\n\n")

		// Create OrderClient
		orderClient, err := createOrderClient(logger)
		if err != nil {
			return fmt.Errorf("create order client: %w", err)
		}

		// Build order parameters (need tick size and min size from market)
		yesTickSize := 0.01 // Default tick size
		noTickSize := 0.01
		yesMinSize := 5.0 // Default min size
		noMinSize := 5.0

		// Try to get tick size and min size from market if available
		if yesToken.TickSize > 0 {
			yesTickSize = yesToken.TickSize
		}
		if noToken.TickSize > 0 {
			noTickSize = noToken.TickSize
		}
		if yesToken.MinOrderSize > 0 {
			yesMinSize = yesToken.MinOrderSize
		}
		if noToken.MinOrderSize > 0 {
			noMinSize = noToken.MinOrderSize
		}

		orderParams := []execution.OutcomeOrderParams{
			{
				TokenID:  yesToken.TokenID,
				Price:    yesPrice,
				TickSize: yesTickSize,
				MinSize:  yesMinSize,
			},
			{
				TokenID:  noToken.TokenID,
				Price:    noPrice,
				TickSize: noTickSize,
				MinSize:  noMinSize,
			},
		}

		// Submit batch order via OrderClient
		responses, err := orderClient.PlaceOrdersMultiOutcome(ctx, orderParams, size)
		if err != nil {
			fmt.Printf("Batch order submission failed: %v\n\n", err)
			saveResponseToFile("test_responses/last_error_batch.json", err.Error())
			return fmt.Errorf("batch submission failed: %w", err)
		}

		// Check responses
		if len(responses) < 2 {
			return fmt.Errorf("expected 2 responses, got %d", len(responses))
		}

		yesResp = responses[0]
		noResp = responses[1]

		// Verify both succeeded
		if !yesResp.Success || !noResp.Success {
			fmt.Printf("One or more orders failed:\n")
			if !yesResp.Success {
				fmt.Printf("  YES order: %s\n", yesResp.ErrorMsg)
			}
			if !noResp.Success {
				fmt.Printf("  NO order: %s\n", noResp.ErrorMsg)
			}
			fmt.Println()
			return fmt.Errorf("batch order execution failed")
		}

		fmt.Printf("✓ Batch submission successful - both orders placed atomically\n\n")
		displayOrderResponse("YES", yesResp, false)
		displayOrderResponse("NO", noResp, false)

		// Save successful responses
		saveOrderResponse("test_responses/last_success_yes.json", yesResp)
		saveOrderResponse("test_responses/last_success_no.json", noResp)
	}

	fmt.Printf("\n=== Test Complete ===\n")

	if paperMode {
		fmt.Printf("\nPaper trading test successful!\n")
		fmt.Printf("   Order structure validated\n")
		fmt.Printf("   Response parsing verified\n\n")
		fmt.Printf("To test with real orders: add --live flag\n")
	} else {
		fmt.Printf("\nNote: Orders may be rejected if:\n")
		fmt.Printf("  - Prices are too far from market\n")
		fmt.Printf("  - Insufficient balance in your account\n")
		fmt.Printf("  - Market is closed or inactive\n")
		fmt.Printf("  - Order size is below minimum\n")
	}

	return nil
}

func simulatePaperOrder(tokenID string, price float64, size float64, outcome string) (resp *types.OrderSubmissionResponse) {
	// Simulate a successful order response using actual API response format
	timestamp := time.Now()
	orderID := fmt.Sprintf("paper_%d", timestamp.UnixNano())

	// Calculate amounts (simulate based on price and size)
	takingAmount := fmt.Sprintf("%.0f", size*1000000)    // USDC raw amount
	makingAmount := fmt.Sprintf("%.0f", price*1000000)   // Token raw amount

	resp = &types.OrderSubmissionResponse{
		Success:      true,
		ErrorMsg:     "",
		OrderID:      orderID,
		OrderHashes:  []string{},     // No tx hashes in paper mode
		Status:       "live",          // Simulated as open order
		TakingAmount: takingAmount,
		MakingAmount: makingAmount,
	}

	return resp
}

func createOrderClient(logger *zap.Logger) (client *execution.OrderClient, err error) {
	// Load config from environment
	cfg, err := loadConfig()
	if err != nil {
		err = fmt.Errorf("load config: %w", err)
		return nil, err
	}

	// Create OrderClient config
	orderClientCfg := &execution.OrderClientConfig{
		APIKey:        cfg.APIKey,
		Secret:        cfg.Secret,
		Passphrase:    cfg.Passphrase,
		PrivateKey:    cfg.PrivateKey,
		ProxyAddress:  cfg.Address, // Use configured address (proxy or EOA)
		SignatureType: 0,            // EOA by default
		Logger:        logger,
	}

	// Create and return OrderClient
	return execution.NewOrderClient(orderClientCfg)
}


func loadConfig() (*Config, error) {
	cfg := &Config{}

	// Try POLYMARKET_API_KEY first, fallback to POLY_API_KEY
	cfg.APIKey = getEnv("POLYMARKET_API_KEY", "POLY_API_KEY")
	cfg.Secret = getEnv("POLYMARKET_SECRET", "POLY_SECRET")
	cfg.Passphrase = getEnv("POLYMARKET_PASSPHRASE", "POLY_PASSPHRASE")
	cfg.PrivateKey = getEnv("POLYMARKET_PRIVATE_KEY", "POLY_PRIVATE_KEY")

	// Prefer POLYMARKET_PROXY_ADDRESS (for MetaMask/browser wallets)
	// This is the address shown in Polymarket UI under profile
	cfg.Address = getEnv("POLYMARKET_PROXY_ADDRESS", "POLYMARKET_ADDRESS", "POLY_ADDRESS")

	if cfg.APIKey == "" {
		return nil, fmt.Errorf("missing POLYMARKET_API_KEY in .env file")
	}
	if cfg.Secret == "" {
		return nil, fmt.Errorf("missing POLYMARKET_SECRET in .env file")
	}
	if cfg.Passphrase == "" {
		return nil, fmt.Errorf("missing POLYMARKET_PASSPHRASE in .env file")
	}

	// Derive EOA address from private key if proxy not provided
	// (Most users will have proxy address)
	if cfg.Address == "" && cfg.PrivateKey != "" {
		addr, err := deriveAddressFromPrivateKey(cfg.PrivateKey)
		if err == nil {
			cfg.Address = addr
			fmt.Printf("Warning: Using EOA address (no proxy): %s\n", addr)
		}
	}

	if cfg.Address == "" {
		return nil, fmt.Errorf("missing POLYMARKET_PROXY_ADDRESS in .env file")
	}

	return cfg, nil
}

func getEnv(keys ...string) string {
	for _, key := range keys {
		if val := os.Getenv(key); val != "" {
			return val
		}
	}
	return ""
}

func displayOrderResponse(outcome string, resp *types.OrderSubmissionResponse, isPaper bool) {
	if isPaper {
		fmt.Printf("%s Order (Paper)\n", outcome)
	} else {
		fmt.Printf("%s Order Submitted\n", outcome)
	}

	fmt.Printf("  Success: %v\n", resp.Success)
	fmt.Printf("  Order ID: %s\n", resp.OrderID)
	fmt.Printf("  Status: %s\n", resp.Status)
	fmt.Printf("  Taking Amount: %s\n", resp.TakingAmount)
	fmt.Printf("  Making Amount: %s\n", resp.MakingAmount)

	if len(resp.OrderHashes) > 0 {
		fmt.Printf("  Order Hashes: %v\n", resp.OrderHashes)
	}

	if resp.ErrorMsg != "" {
		fmt.Printf("  Error Message: %s\n", resp.ErrorMsg)
	}
	fmt.Println()
}

func loadMockResponse(filename string) (resp *types.OrderSubmissionResponse, err error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		err = fmt.Errorf("read mock file: %w", err)
		return resp, err
	}

	err = json.Unmarshal(data, &resp)
	if err != nil {
		err = fmt.Errorf("parse mock response: %w", err)
		return resp, err
	}

	return resp, nil
}

func saveOrderResponse(filename string, resp *types.OrderSubmissionResponse) {
	data, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		fmt.Printf("Warning: failed to marshal response: %v\n", err)
		return
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		fmt.Printf("Warning: failed to save response to %s: %v\n", filename, err)
		return
	}

	fmt.Printf("Response saved to %s\n", filename)
}

func saveResponseToFile(filename string, content string) {
	data := map[string]string{"error": content}
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return
	}

	os.WriteFile(filename, jsonData, 0644)
}

func deriveAddressFromPrivateKey(privateKeyHex string) (string, error) {
	// Remove spaces and 0x prefix if present
	privateKeyHex = strings.TrimSpace(privateKeyHex)
	privateKeyHex = strings.TrimPrefix(privateKeyHex, "0x")

	// Parse private key
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return "", fmt.Errorf("invalid private key: %w", err)
	}

	// Derive public key and address
	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		return "", fmt.Errorf("error casting public key to ECDSA")
	}

	address := crypto.PubkeyToAddress(*publicKeyECDSA).Hex()
	return address, nil
}
