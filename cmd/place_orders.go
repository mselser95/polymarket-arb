package cmd

import (
	"context"
	"crypto/ecdsa"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/joho/godotenv"
	"github.com/mselser95/polymarket-arb/internal/discovery"
	"github.com/mselser95/polymarket-arb/internal/markets"
	"github.com/mselser95/polymarket-arb/pkg/types"
	"github.com/polymarket/go-order-utils/pkg/builder"
	"github.com/polymarket/go-order-utils/pkg/model"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var placeOrdersCmd = &cobra.Command{
	Use:   "place-orders <market-slug>",
	Short: "Place batch orders (YES + NO) for arbitrage",
	Long: `Place both YES and NO orders simultaneously using Polymarket's batch order endpoint.
This uses EIP-712 signed orders via the official go-order-utils library.`,
	Args: cobra.ExactArgs(1),
	RunE: runPlaceOrders,
}

var (
	yesPrice  float64
	noPrice   float64
	orderSize float64
	dryRun    bool
)

func init() {
	rootCmd.AddCommand(placeOrdersCmd)

	placeOrdersCmd.Flags().Float64VarP(&yesPrice, "yes-price", "y", 0.01, "YES order price")
	placeOrdersCmd.Flags().Float64VarP(&noPrice, "no-price", "n", 0.01, "NO order price")
	placeOrdersCmd.Flags().Float64VarP(&orderSize, "size", "s", 1.0, "Order size in USD")
	placeOrdersCmd.Flags().BoolVarP(&dryRun, "dry-run", "d", false, "Build orders but don't submit")
}

func runPlaceOrders(cmd *cobra.Command, args []string) error {
	marketSlug := args[0]

	// Load env
	if err := godotenv.Load(); err != nil {
		fmt.Printf("Warning: .env file not found\n")
	}

	// Load credentials
	cfg, err := loadPlaceOrdersConfig()
	if err != nil {
		return err
	}

	fmt.Printf("=== Polymarket Batch Order Placement ===\n\n")
	fmt.Printf("Market: %s\n", marketSlug)
	fmt.Printf("Order Size: $%.2f\n", orderSize)
	fmt.Printf("YES Price: %.4f\n", yesPrice)
	fmt.Printf("NO Price: %.4f\n", noPrice)
	fmt.Printf("Mode: %s\n\n", map[bool]string{true: "DRY RUN", false: "LIVE"}[dryRun])

	// Fetch market details
	fmt.Printf("Fetching market details...\n")
	logger, _ := zap.NewDevelopment()
	ctx2, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel2()

	client := discovery.NewClient("https://gamma-api.polymarket.com", logger)
	market, err := client.FetchMarketBySlug(ctx2, marketSlug)
	if err != nil {
		return fmt.Errorf("fetch market: %w", err)
	}

	fmt.Printf("\nMarket ID: %s\n", market.ID)
	fmt.Printf("Question: %s\n\n", market.Question)

	yesToken := market.GetTokenByOutcome("YES")
	noToken := market.GetTokenByOutcome("NO")

	if yesToken == nil || noToken == nil {
		return fmt.Errorf("market missing YES or NO tokens")
	}

	fmt.Printf("YES Token: %s\n", yesToken.TokenID)
	fmt.Printf("NO Token: %s\n\n", noToken.TokenID)

	// Create metadata client and fetch tick size + min order size
	fmt.Printf("Fetching market metadata...\n")
	metadataClient := markets.NewMetadataClient()
	cachedMetadataClient := markets.NewCachedMetadataClient(metadataClient, nil) // No cache for CLI

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	yesTickSize, yesMinSize, err := cachedMetadataClient.GetTokenMetadata(ctx, yesToken.TokenID)
	if err != nil {
		return fmt.Errorf("fetch YES token metadata: %w", err)
	}

	noTickSize, noMinSize, err := cachedMetadataClient.GetTokenMetadata(ctx, noToken.TokenID)
	if err != nil {
		return fmt.Errorf("fetch NO token metadata: %w", err)
	}

	fmt.Printf("YES Token - Tick Size: %.4f, Min Order Size: %.2f tokens\n", yesTickSize, yesMinSize)
	fmt.Printf("NO Token - Tick Size: %.4f, Min Order Size: %.2f tokens\n\n", noTickSize, noMinSize)

	// Build and sign orders
	chainID := big.NewInt(137) // Polygon mainnet
	orderBuilder := builder.NewExchangeOrderBuilderImpl(chainID, nil)

	// Parse private key
	privateKey, err := crypto.HexToECDSA(strings.TrimPrefix(cfg.PrivateKey, "0x"))
	if err != nil {
		return fmt.Errorf("parse private key: %w", err)
	}

	// Convert prices to maker/taker amounts with tick-size-based rounding
	// For BUY orders: taker = tokens to receive, maker = USDC to spend
	// Python client applies market-specific rounding based on tick size
	yesSizePrecision, yesAmountPrecision := getRoundingConfig(yesTickSize)
	noSizePrecision, noAmountPrecision := getRoundingConfig(noTickSize)

	// YES order: calculate taker amount (tokens) first, then maker amount (USDC)
	yesTakerTokens := roundAmount(orderSize/yesPrice, yesSizePrecision)              // tokens: size precision (2 decimals)
	yesMakerUSD := roundAmount(yesTakerTokens*yesPrice, yesAmountPrecision)          // USDC: amount precision (5 decimals)
	yesMakerAmount := usdToRawAmount(yesMakerUSD)
	yesTakerAmount := usdToRawAmount(yesTakerTokens)

	// Validate YES order size against minimum
	if yesTakerTokens < yesMinSize {
		return fmt.Errorf("YES order size %.2f tokens below minimum %.2f tokens (need $%.2f USDC at price %.4f)",
			yesTakerTokens, yesMinSize, yesMinSize*yesPrice, yesPrice)
	}

	// NO order: calculate taker amount (tokens) first, then maker amount (USDC)
	noTakerTokens := roundAmount(orderSize/noPrice, noSizePrecision)              // tokens: size precision (2 decimals)
	noMakerUSD := roundAmount(noTakerTokens*noPrice, noAmountPrecision)          // USDC: amount precision (5 decimals)
	noMakerAmount := usdToRawAmount(noMakerUSD)
	noTakerAmount := usdToRawAmount(noTakerTokens)

	// Validate NO order size against minimum
	if noTakerTokens < noMinSize {
		return fmt.Errorf("NO order size %.2f tokens below minimum %.2f tokens (need $%.2f USDC at price %.4f)",
			noTakerTokens, noMinSize, noMinSize*noPrice, noPrice)
	}

	// For signatureType=0 (EOA), maker and signer should be the same
	// For signatureType=1/2 (Proxy/Gnosis), maker can be different
	makerAddress := cfg.Address
	signerAddress := cfg.Address
	if cfg.SignatureType > 0 && cfg.ProxyAddress != "" {
		makerAddress = cfg.ProxyAddress
	}

	fmt.Printf("=== Building YES Order ===\n")
	fmt.Printf("Maker (Funder): %s\n", makerAddress)
	fmt.Printf("Signer: %s\n", signerAddress)
	fmt.Printf("Signature Type: %d\n", cfg.SignatureType)
	fmt.Printf("Maker Amount (USDC): %s (raw: $%.2f)\n", yesMakerAmount, orderSize)
	fmt.Printf("Taker Amount (tokens): %s (%.4f tokens)\n\n", yesTakerAmount, orderSize/yesPrice)

	yesOrderData := &model.OrderData{
		Maker:         makerAddress,
		Taker:         "0x0000000000000000000000000000000000000000", // Public order
		TokenId:       yesToken.TokenID,
		MakerAmount:   yesMakerAmount,
		TakerAmount:   yesTakerAmount,
		Side:          model.BUY, // BUY = 0, buying outcome tokens with USDC
		FeeRateBps:    "0",      // Fee rate in basis points
		Nonce:         "0",
		Signer:        signerAddress,
		Expiration:    "0", // No expiration
		SignatureType: cfg.SignatureType,
	}

	yesSignedOrder, err := orderBuilder.BuildSignedOrder(privateKey, yesOrderData, model.CTFExchange)
	if err != nil {
		return fmt.Errorf("build YES order: %w", err)
	}

	fmt.Printf("=== Building NO Order ===\n")
	fmt.Printf("Maker (Funder): %s\n", makerAddress)
	fmt.Printf("Signer: %s\n", signerAddress)
	fmt.Printf("Signature Type: %d\n", cfg.SignatureType)
	fmt.Printf("Maker Amount (USDC): %s (raw: $%.2f)\n", noMakerAmount, orderSize)
	fmt.Printf("Taker Amount (tokens): %s (%.4f tokens)\n\n", noTakerAmount, orderSize/noPrice)

	noOrderData := &model.OrderData{
		Maker:         makerAddress,
		Taker:         "0x0000000000000000000000000000000000000000",
		TokenId:       noToken.TokenID,
		MakerAmount:   noMakerAmount,
		TakerAmount:   noTakerAmount,
		Side:          model.BUY, // BUY = 0, buying outcome tokens with USDC
		FeeRateBps:    "0",
		Nonce:         "0",
		Signer:        signerAddress,
		Expiration:    "0",
		SignatureType: cfg.SignatureType,
	}

	noSignedOrder, err := orderBuilder.BuildSignedOrder(privateKey, noOrderData, model.CTFExchange)
	if err != nil {
		return fmt.Errorf("build NO order: %w", err)
	}

	fmt.Printf("✅ Orders built and signed successfully\n\n")

	if dryRun {
		fmt.Printf("=== DRY RUN - Orders Built Successfully ===\n\n")

		fmt.Printf("YES Order:\n")
		fmt.Printf("  Salt: %s\n", yesSignedOrder.Salt.String())
		fmt.Printf("  Maker: %s\n", yesSignedOrder.Maker.Hex())
		fmt.Printf("  Signer: %s\n", yesSignedOrder.Signer.Hex())
		fmt.Printf("  Token ID: %s\n", yesSignedOrder.TokenId.String())
		fmt.Printf("  Maker Amount: %s\n", yesSignedOrder.MakerAmount.String())
		fmt.Printf("  Taker Amount: %s\n", yesSignedOrder.TakerAmount.String())
		fmt.Printf("  Side: %s\n", yesSignedOrder.Side.String())
		fmt.Printf("  Signature: 0x%s\n\n", common.Bytes2Hex(yesSignedOrder.Signature))

		fmt.Printf("NO Order:\n")
		fmt.Printf("  Salt: %s\n", noSignedOrder.Salt.String())
		fmt.Printf("  Maker: %s\n", noSignedOrder.Maker.Hex())
		fmt.Printf("  Signer: %s\n", noSignedOrder.Signer.Hex())
		fmt.Printf("  Token ID: %s\n", noSignedOrder.TokenId.String())
		fmt.Printf("  Maker Amount: %s\n", noSignedOrder.MakerAmount.String())
		fmt.Printf("  Taker Amount: %s\n", noSignedOrder.TakerAmount.String())
		fmt.Printf("  Side: %s\n", noSignedOrder.Side.String())
		fmt.Printf("  Signature: 0x%s\n\n", common.Bytes2Hex(noSignedOrder.Signature))

		fmt.Printf("✅ Orders are valid EIP-712 signed messages\n")
		fmt.Printf("   Ready to submit when --dry-run flag is removed\n")
		return nil
	}

	// Submit orders individually
	fmt.Printf("=== Submitting YES Order ===\n\n")

	ctx3, cancel3 := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel3()

	yesResp, err := submitSingleOrder(ctx3, cfg, yesSignedOrder)
	if err != nil {
		fmt.Printf("❌ YES order failed: %v\n\n", err)
	} else {
		fmt.Printf("✅ YES order submitted!\n")
		fmt.Printf("  Success: %v\n", yesResp.Success)
		fmt.Printf("  Order ID: %s\n", yesResp.OrderID)
		fmt.Printf("  Status: %s\n", yesResp.Status)
		fmt.Printf("  Taking Amount: %s\n", yesResp.TakingAmount)
		fmt.Printf("  Making Amount: %s\n\n", yesResp.MakingAmount)
	}

	fmt.Printf("=== Submitting NO Order ===\n\n")

	noResp, err := submitSingleOrder(ctx3, cfg, noSignedOrder)
	if err != nil {
		fmt.Printf("❌ NO order failed: %v\n\n", err)
	} else {
		fmt.Printf("✅ NO order submitted!\n")
		fmt.Printf("  Success: %v\n", noResp.Success)
		fmt.Printf("  Order ID: %s\n", noResp.OrderID)
		fmt.Printf("  Status: %s\n", noResp.Status)
		fmt.Printf("  Taking Amount: %s\n", noResp.TakingAmount)
		fmt.Printf("  Making Amount: %s\n\n", noResp.MakingAmount)
	}

	return nil
}

type PlaceOrdersConfig struct {
	APIKey        string
	Secret        string
	Passphrase    string
	PrivateKey    string
	Address       string // EOA address (signer)
	ProxyAddress  string // Proxy address (maker/funder)
	SignatureType model.SignatureType
}

func loadPlaceOrdersConfig() (*PlaceOrdersConfig, error) {
	cfg := &PlaceOrdersConfig{}

	cfg.APIKey = getEnv("POLYMARKET_API_KEY", "POLY_API_KEY")
	cfg.Secret = getEnv("POLYMARKET_SECRET", "POLY_SECRET")
	cfg.Passphrase = getEnv("POLYMARKET_PASSPHRASE", "POLY_PASSPHRASE")
	cfg.PrivateKey = getEnv("POLYMARKET_PRIVATE_KEY", "POLY_PRIVATE_KEY")

	// EOA address (signer) - derived from private key
	cfg.Address = getEnv("POLYMARKET_ADDRESS")
	if cfg.Address == "" {
		privateKey, err := crypto.HexToECDSA(strings.TrimPrefix(cfg.PrivateKey, "0x"))
		if err != nil {
			return nil, fmt.Errorf("derive address: %w", err)
		}
		publicKey := privateKey.Public()
		publicKeyECDSA, _ := publicKey.(*ecdsa.PublicKey)
		cfg.Address = crypto.PubkeyToAddress(*publicKeyECDSA).Hex()
	}

	// Proxy address (maker/funder) - where funds actually are
	cfg.ProxyAddress = getEnv("POLYMARKET_PROXY_ADDRESS")

	// Signature type: 0=EOA, 1=POLY_PROXY, 2=GNOSIS_SAFE
	sigTypeStr := getEnv("POLYMARKET_SIGNATURE_TYPE")
	if sigTypeStr == "" {
		cfg.SignatureType = model.EOA
	} else {
		sigType, err := strconv.Atoi(sigTypeStr)
		if err != nil {
			return nil, fmt.Errorf("invalid signature type: %w", err)
		}
		cfg.SignatureType = model.SignatureType(sigType)
	}

	if cfg.APIKey == "" {
		return nil, fmt.Errorf("missing POLYMARKET_API_KEY")
	}
	if cfg.Secret == "" {
		return nil, fmt.Errorf("missing POLYMARKET_SECRET")
	}
	if cfg.Passphrase == "" {
		return nil, fmt.Errorf("missing POLYMARKET_PASSPHRASE")
	}
	if cfg.PrivateKey == "" {
		return nil, fmt.Errorf("missing POLYMARKET_PRIVATE_KEY")
	}

	return cfg, nil
}

// getRoundingConfig returns the precision for size and amount based on tick size
// Matches Python client's ROUNDING_CONFIG
func getRoundingConfig(tickSize float64) (sizePrecision int, amountPrecision int) {
	switch tickSize {
	case 0.1:
		return 2, 3  // size=2, amount=3
	case 0.01:
		return 2, 4  // size=2, amount=4
	case 0.001:
		return 2, 5  // size=2, amount=5
	case 0.0001:
		return 2, 6  // size=2, amount=6
	default:
		return 2, 4  // Default to 0.01 tick size
	}
}

// roundAmount rounds an amount to the specified number of decimal places
// Uses "round half to even" (banker's rounding) like Python
func roundAmount(value float64, decimals int) float64 {
	multiplier := float64(1)
	for i := 0; i < decimals; i++ {
		multiplier *= 10
	}
	return float64(int64(value*multiplier+0.5)) / multiplier
}

// Convert USD amount to raw amount with 6 decimals (USDC format)
func usdToRawAmount(usd float64) string {
	rawAmount := int64(usd * 1000000)
	return fmt.Sprintf("%d", rawAmount)
}

func submitSingleOrder(
	ctx context.Context,
	cfg *PlaceOrdersConfig,
	order *model.SignedOrder,
) (resp *types.OrderSubmissionResponse, err error) {
	// Convert Side to string ("BUY" or "SELL")
	sideStr := "BUY"
	if order.Side.Uint64() == uint64(model.SELL) {
		sideStr = "SELL"
	}

	// Convert to JSON format
	jsonOrder := types.SignedOrderJSON{
		Salt:          order.Salt.Int64(),
		Maker:         order.Maker.Hex(),
		Signer:        order.Signer.Hex(),
		Taker:         order.Taker.Hex(),
		TokenID:       order.TokenId.String(),
		MakerAmount:   order.MakerAmount.String(),
		TakerAmount:   order.TakerAmount.String(),
		Side:          sideStr,
		Expiration:    order.Expiration.String(),
		Nonce:         order.Nonce.String(),
		FeeRateBps:    order.FeeRateBps.String(),
		SignatureType: int(order.SignatureType.Int64()),
		Signature:     "0x" + common.Bytes2Hex(order.Signature),
	}

	// Wrap order in the required structure
	// Note: "owner" is the API key, not the maker address (per Python client)
	orderRequest := types.OrderSubmissionRequest{
		Order:     jsonOrder,
		Owner:     cfg.APIKey,
		OrderType: "GTC",
	}

	reqBody, err := json.Marshal(orderRequest)
	if err != nil {
		err = fmt.Errorf("marshal request: %w", err)
		return resp, err
	}

	// Create HMAC signature
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	method := "POST"
	requestPath := "/order" // Single order endpoint

	signaturePayload := timestamp + method + requestPath + string(reqBody)

	// Decode secret using URL-safe base64 (Python client uses urlsafe_b64decode)
	secretBytes, err := base64.URLEncoding.DecodeString(cfg.Secret)
	if err != nil {
		err = fmt.Errorf("decode secret: %w", err)
		return resp, err
	}

	h := hmac.New(sha256.New, secretBytes)
	h.Write([]byte(signaturePayload))
	// Encode signature using URL-safe base64 (Python client uses urlsafe_b64encode)
	signature := base64.URLEncoding.EncodeToString(h.Sum(nil))

	// Make request
	url := "https://clob.polymarket.com" + requestPath
	req, err := http.NewRequestWithContext(ctx, method, url, strings.NewReader(string(reqBody)))
	if err != nil {
		err = fmt.Errorf("create request: %w", err)
		return resp, err
	}

	// POLY_ADDRESS header should be the EOA address (per Python client: signer.address())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("POLY_API_KEY", cfg.APIKey)
	req.Header.Set("POLY_SIGNATURE", signature)
	req.Header.Set("POLY_TIMESTAMP", timestamp)
	req.Header.Set("POLY_PASSPHRASE", cfg.Passphrase)
	req.Header.Set("POLY_ADDRESS", cfg.Address) // EOA address from private key

	client := &http.Client{Timeout: 30 * time.Second}
	httpResp, err := client.Do(req)
	if err != nil {
		err = fmt.Errorf("send request: %w", err)
		return resp, err
	}
	defer httpResp.Body.Close()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		err = fmt.Errorf("read response: %w", err)
		return resp, err
	}

	if httpResp.StatusCode != http.StatusOK && httpResp.StatusCode != http.StatusCreated {
		err = fmt.Errorf("API error (status %d): %s", httpResp.StatusCode, string(body))
		return resp, err
	}

	err = json.Unmarshal(body, &resp)
	if err != nil {
		err = fmt.Errorf("parse response: %w", err)
		return resp, err
	}

	return resp, nil
}
