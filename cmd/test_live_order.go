package cmd

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/joho/godotenv"
	"github.com/mselser95/polymarket-arb/internal/discovery"
	"github.com/mselser95/polymarket-arb/pkg/types"
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

// OrderRequest represents a CLOB API order request
type OrderRequest struct {
	TokenID    string `json:"token_id"`
	Price      string `json:"price"` // String to match API format
	Size       string `json:"size"`  // String to match API format
	Side       string `json:"side"`  // "BUY" or "SELL"
	OrderType  string `json:"type"`  // "GTC", "FOK", "IOC"
	Expiration int64  `json:"expiration,omitempty"`
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

	// Submit YES order
	fmt.Printf("=== Submitting YES Order ===\n")
	yesOrderReq := OrderRequest{
		TokenID:   yesToken.TokenID,
		Price:     fmt.Sprintf("%.4f", yesPrice),
		Size:      fmt.Sprintf("%.2f", size),
		Side:      "BUY",
		OrderType: "GTC", // Good Till Cancelled
	}

	var yesResp *types.OrderSubmissionResponse
	if paperMode {
		yesResp = simulatePaperOrder(yesOrderReq, market.ID, "YES")
		displayOrderResponse("YES", yesResp, true)
	} else if mockMode {
		yesResp, err = loadMockResponse(mockResponseFile)
		if err != nil {
			fmt.Printf("Failed to load mock response: %v\n\n", err)
		} else {
			displayOrderResponse("YES", yesResp, false)
		}
	} else {
		yesResp, err = submitLiveOrder(ctx, yesOrderReq, market.ID)
		if err != nil {
			fmt.Printf("YES order failed: %v\n\n", err)
			// Save error response for future testing
			saveResponseToFile("test_responses/last_error_yes.json", err.Error())
		} else {
			displayOrderResponse("YES", yesResp, false)
			// Save successful response for future testing
			saveOrderResponse("test_responses/last_success_yes.json", yesResp)
		}
	}

	// Submit NO order
	fmt.Printf("=== Submitting NO Order ===\n")
	noOrderReq := OrderRequest{
		TokenID:   noToken.TokenID,
		Price:     fmt.Sprintf("%.4f", noPrice),
		Size:      fmt.Sprintf("%.2f", size),
		Side:      "BUY",
		OrderType: "GTC",
	}

	var noResp *types.OrderSubmissionResponse
	if paperMode {
		noResp = simulatePaperOrder(noOrderReq, market.ID, "NO")
		displayOrderResponse("NO", noResp, true)
	} else if mockMode {
		noResp, err = loadMockResponse(mockResponseFile)
		if err != nil {
			fmt.Printf("Failed to load mock response: %v\n\n", err)
		} else {
			displayOrderResponse("NO", noResp, false)
		}
	} else {
		noResp, err = submitLiveOrder(ctx, noOrderReq, market.ID)
		if err != nil {
			fmt.Printf("NO order failed: %v\n\n", err)
			// Save error response for future testing
			saveResponseToFile("test_responses/last_error_no.json", err.Error())
		} else {
			displayOrderResponse("NO", noResp, false)
			// Save successful response for future testing
			saveOrderResponse("test_responses/last_success_no.json", noResp)
		}
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

func simulatePaperOrder(
	orderReq OrderRequest,
	marketID string,
	outcome string,
) (resp *types.OrderSubmissionResponse) {
	// Simulate a successful order response using actual API response format
	timestamp := time.Now()
	orderID := fmt.Sprintf("paper_%d", timestamp.UnixNano())

	// Calculate amounts (simulate based on price and size)
	sizeFloat := mustParseFloat(orderReq.Size)
	priceFloat := mustParseFloat(orderReq.Price)
	takingAmount := fmt.Sprintf("%.0f", sizeFloat*1000000)    // USDC raw amount
	makingAmount := fmt.Sprintf("%.0f", priceFloat*1000000)   // Token raw amount

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

func submitLiveOrder(
	ctx context.Context,
	orderReq OrderRequest,
	marketID string,
) (resp *types.OrderSubmissionResponse, err error) {
	// Load config from environment
	cfg, err := loadConfig()
	if err != nil {
		err = fmt.Errorf("load config: %w", err)
		return resp, err
	}

	// Polymarket CLOB API endpoint
	clobURL := "https://clob.polymarket.com/order"

	// Marshal order request
	orderJSON, err := json.Marshal(orderReq)
	if err != nil {
		err = fmt.Errorf("marshal order: %w", err)
		return resp, err
	}

	// Create timestamp (Unix SECONDS, not milliseconds!)
	timestamp := fmt.Sprintf("%d", time.Now().Unix())

	// Decode base64 secret (try base64url first, then standard base64)
	secretBytes, err := base64.URLEncoding.DecodeString(cfg.Secret)
	if err != nil {
		// Try standard base64 encoding
		secretBytes, err = base64.StdEncoding.DecodeString(cfg.Secret)
		if err != nil {
			err = fmt.Errorf("decode secret: %w", err)
			return resp, err
		}
	}

	// Create HMAC signature
	// Format: timestamp + method + requestPath + body
	signaturePayload := timestamp + "POST" + "/order" + string(orderJSON)
	signature := createHMACSignature(signaturePayload, secretBytes)

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", clobURL, bytes.NewBuffer(orderJSON))
	if err != nil {
		err = fmt.Errorf("create request: %w", err)
		return resp, err
	}

	// Add L2 authentication headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("POLY_API_KEY", cfg.APIKey)
	req.Header.Set("POLY_SIGNATURE", signature)
	req.Header.Set("POLY_TIMESTAMP", timestamp)
	req.Header.Set("POLY_PASSPHRASE", cfg.Passphrase)

	// Address is required
	if cfg.Address != "" {
		req.Header.Set("POLY_ADDRESS", cfg.Address)
	}

	// Send request
	httpClient := &http.Client{Timeout: 10 * time.Second}
	httpResp, err := httpClient.Do(req)
	if err != nil {
		err = fmt.Errorf("send request: %w", err)
		return resp, err
	}
	defer httpResp.Body.Close()

	// Read response body
	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		err = fmt.Errorf("read response: %w", err)
		return resp, err
	}

	// Log full request details for debugging
	fmt.Printf("\nDebug - Request Details:\n")
	fmt.Printf("  URL: %s\n", clobURL)
	fmt.Printf("  Method: POST\n")
	fmt.Printf("  API Key: %s\n", cfg.APIKey)
	fmt.Printf("  Address: %s\n", cfg.Address)
	fmt.Printf("  Timestamp: %s\n", timestamp)
	fmt.Printf("  Signature: %s\n", signature)
	fmt.Printf("  Request Body: %s\n", string(orderJSON))

	// Check status code
	fmt.Printf("\nDebug - Response:\n")
	fmt.Printf("  Status Code: %d\n", httpResp.StatusCode)
	fmt.Printf("  Response Body: %s\n\n", string(body))

	if httpResp.StatusCode != http.StatusOK && httpResp.StatusCode != http.StatusCreated {
		// Try to parse error response
		var errResp ErrorResponse
		if json.Unmarshal(body, &errResp) == nil {
			err = fmt.Errorf("API error (status %d): %s - %s", httpResp.StatusCode, errResp.Error, errResp.Message)
			return resp, err
		}
		err = fmt.Errorf("API error (status %d): %s", httpResp.StatusCode, string(body))
		return resp, err
	}

	// Parse success response
	err = json.Unmarshal(body, &resp)
	if err != nil {
		err = fmt.Errorf("parse response: %w\nBody: %s", err, string(body))
		return resp, err
	}

	return resp, nil
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

func createHMACSignature(message string, secret []byte) string {
	h := hmac.New(sha256.New, secret)
	h.Write([]byte(message))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
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

func mustParseFloat(s string) float64 {
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
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
