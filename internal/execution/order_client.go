package execution

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
	"math"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/polymarket/go-order-utils/pkg/builder"
	"github.com/polymarket/go-order-utils/pkg/model"
	"go.uber.org/zap"

	"github.com/mselser95/polymarket-arb/pkg/types"
)

// OrderClient handles order submission to Polymarket CLOB
type OrderClient struct {
	apiKey        string
	secret        string
	passphrase    string
	privateKey    *ecdsa.PrivateKey
	address       string // EOA address (signer)
	proxyAddress  string // Proxy address (maker/funder)
	signatureType model.SignatureType
	orderBuilder  builder.ExchangeOrderBuilder
	logger        *zap.Logger
}

// OrderClientConfig holds configuration for the order client
type OrderClientConfig struct {
	APIKey        string
	Secret        string
	Passphrase    string
	PrivateKey    string
	Address       string
	ProxyAddress  string
	SignatureType int
	Logger        *zap.Logger
}

// NewOrderClient creates a new order client
func NewOrderClient(cfg *OrderClientConfig) (*OrderClient, error) {
	// Parse private key
	privateKey, err := crypto.HexToECDSA(strings.TrimPrefix(cfg.PrivateKey, "0x"))
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}

	// Derive EOA address if not provided
	address := cfg.Address
	if address == "" {
		publicKey := privateKey.Public()
		publicKeyECDSA, _ := publicKey.(*ecdsa.PublicKey)
		address = crypto.PubkeyToAddress(*publicKeyECDSA).Hex()
	}

	chainID := big.NewInt(137) // Polygon mainnet
	orderBuilder := builder.NewExchangeOrderBuilderImpl(chainID, nil)

	return &OrderClient{
		apiKey:        cfg.APIKey,
		secret:        cfg.Secret,
		passphrase:    cfg.Passphrase,
		privateKey:    privateKey,
		address:       address,
		proxyAddress:  cfg.ProxyAddress,
		signatureType: model.SignatureType(cfg.SignatureType),
		orderBuilder:  orderBuilder,
		logger:        cfg.Logger,
	}, nil
}

// PlaceOrders places YES and NO orders for arbitrage (legacy sequential method).
// DEPRECATED: Use PlaceOrdersBatch for better atomicity and performance.
func (c *OrderClient) PlaceOrders(
	ctx context.Context,
	yesTokenID string,
	noTokenID string,
	size float64,
	yesPrice float64,
	noPrice float64,
	yesTickSize float64,
	yesMinSize float64,
	noTickSize float64,
	noMinSize float64,
) (yesResp *types.OrderSubmissionResponse, noResp *types.OrderSubmissionResponse, err error) {
	// Determine maker address
	makerAddress := c.address
	signerAddress := c.address
	if c.proxyAddress != "" {
		makerAddress = c.proxyAddress
	}

	// Get rounding precision for each token
	yesSizePrecision, yesAmountPrecision := getRoundingConfig(yesTickSize)
	noSizePrecision, noAmountPrecision := getRoundingConfig(noTickSize)

	// Calculate token sizes with rounding
	yesTakerTokens := roundAmount(size/yesPrice, yesSizePrecision)
	noTakerTokens := roundAmount(size/noPrice, noSizePrecision)

	// Validate against minimums
	if yesTakerTokens < yesMinSize {
		return nil, nil, fmt.Errorf("YES order size %.2f below minimum %.2f tokens", yesTakerTokens, yesMinSize)
	}
	if noTakerTokens < noMinSize {
		return nil, nil, fmt.Errorf("NO order size %.2f below minimum %.2f tokens", noTakerTokens, noMinSize)
	}

	// Build YES order with rounded amounts
	yesMakerUSD := roundAmount(yesTakerTokens*yesPrice, yesAmountPrecision)
	yesMakerAmount := usdToRawAmount(yesMakerUSD)
	yesTakerAmount := usdToRawAmount(yesTakerTokens)

	yesOrderData := &model.OrderData{
		Maker:         makerAddress,
		Taker:         "0x0000000000000000000000000000000000000000",
		TokenId:       yesTokenID,
		MakerAmount:   yesMakerAmount,
		TakerAmount:   yesTakerAmount,
		Side:          model.BUY, // BUY = 0, buying outcome tokens with USDC
		FeeRateBps:    "0",
		Nonce:         "0",
		Signer:        signerAddress,
		Expiration:    "0",
		SignatureType: c.signatureType,
	}

	yesSignedOrder, err := c.orderBuilder.BuildSignedOrder(c.privateKey, yesOrderData, model.CTFExchange)
	if err != nil {
		return nil, nil, fmt.Errorf("build YES order: %w", err)
	}

	// Build NO order with rounded amounts
	noMakerUSD := roundAmount(noTakerTokens*noPrice, noAmountPrecision)
	noMakerAmount := usdToRawAmount(noMakerUSD)
	noTakerAmount := usdToRawAmount(noTakerTokens)

	noOrderData := &model.OrderData{
		Maker:         makerAddress,
		Taker:         "0x0000000000000000000000000000000000000000",
		TokenId:       noTokenID,
		MakerAmount:   noMakerAmount,
		TakerAmount:   noTakerAmount,
		Side:          model.BUY, // BUY = 0, buying outcome tokens with USDC
		FeeRateBps:    "0",
		Nonce:         "0",
		Signer:        signerAddress,
		Expiration:    "0",
		SignatureType: c.signatureType,
	}

	noSignedOrder, err := c.orderBuilder.BuildSignedOrder(c.privateKey, noOrderData, model.CTFExchange)
	if err != nil {
		return nil, nil, fmt.Errorf("build NO order: %w", err)
	}

	c.logger.Info("orders-built",
		zap.String("maker", makerAddress),
		zap.String("signer", signerAddress),
		zap.Float64("size", size))

	// Submit orders
	yesResp, err = c.submitOrder(ctx, yesSignedOrder)
	if err != nil {
		err = fmt.Errorf("submit YES order: %w", err)
		return yesResp, noResp, err
	}

	noResp, err = c.submitOrder(ctx, noSignedOrder)
	if err != nil {
		err = fmt.Errorf("submit NO order: %w", err)
		return yesResp, noResp, err
	}

	return yesResp, noResp, nil
}

// GetMakerAddress returns the maker address (proxy if set, otherwise EOA).
func (c *OrderClient) GetMakerAddress() (makerAddress string) {
	if c.proxyAddress != "" {
		return c.proxyAddress
	}
	return c.address
}

// GetSignerAddress returns the signer address (always the EOA).
func (c *OrderClient) GetSignerAddress() (signerAddress string) {
	return c.address
}

// GetSignatureType returns the signature type.
func (c *OrderClient) GetSignatureType() (signatureType model.SignatureType) {
	return c.signatureType
}

// PlaceSingleOrder places a single order with the given OrderData.
// This method is useful for closing positions or placing standalone orders.
func (c *OrderClient) PlaceSingleOrder(
	ctx context.Context,
	orderData *model.OrderData,
) (resp *types.OrderSubmissionResponse, err error) {
	// Build and sign the order
	signedOrder, err := c.orderBuilder.BuildSignedOrder(c.privateKey, orderData, model.CTFExchange)
	if err != nil {
		return nil, fmt.Errorf("build order: %w", err)
	}

	// Convert Side to string for logging
	sideStr := "BUY"
	if orderData.Side == model.SELL {
		sideStr = "SELL"
	}

	c.logger.Info("single-order-built",
		zap.String("maker", orderData.Maker),
		zap.String("signer", orderData.Signer),
		zap.String("token_id", orderData.TokenId),
		zap.String("side", sideStr))

	// Submit the order
	resp, err = c.submitOrder(ctx, signedOrder)
	if err != nil {
		return nil, fmt.Errorf("submit order: %w", err)
	}

	return resp, nil
}

// PlaceOrdersBatch places YES and NO orders atomically using the batch endpoint.
// This is the preferred method as it submits both orders in a single API call.
func (c *OrderClient) PlaceOrdersBatch(
	ctx context.Context,
	yesTokenID string,
	noTokenID string,
	size float64,
	yesPrice float64,
	noPrice float64,
	yesTickSize float64,
	yesMinSize float64,
	noTickSize float64,
	noMinSize float64,
) (yesResp *types.OrderSubmissionResponse, noResp *types.OrderSubmissionResponse, err error) {
	// Determine maker address
	makerAddress := c.address
	signerAddress := c.address
	if c.proxyAddress != "" {
		makerAddress = c.proxyAddress
	}

	// Get rounding precision for each token
	yesSizePrecision, yesAmountPrecision := getRoundingConfig(yesTickSize)
	noSizePrecision, noAmountPrecision := getRoundingConfig(noTickSize)

	// Calculate token sizes with rounding
	yesTakerTokens := roundAmount(size/yesPrice, yesSizePrecision)
	noTakerTokens := roundAmount(size/noPrice, noSizePrecision)

	// Validate against minimums
	if yesTakerTokens < yesMinSize {
		err = fmt.Errorf("YES order size %.2f below minimum %.2f tokens", yesTakerTokens, yesMinSize)
		return yesResp, noResp, err
	}
	if noTakerTokens < noMinSize {
		err = fmt.Errorf("NO order size %.2f below minimum %.2f tokens", noTakerTokens, noMinSize)
		return yesResp, noResp, err
	}

	// Build YES order with rounded amounts
	yesMakerUSD := roundAmount(yesTakerTokens*yesPrice, yesAmountPrecision)
	yesMakerAmount := usdToRawAmount(yesMakerUSD)
	yesTakerAmount := usdToRawAmount(yesTakerTokens)

	yesOrderData := &model.OrderData{
		Maker:         makerAddress,
		Taker:         "0x0000000000000000000000000000000000000000",
		TokenId:       yesTokenID,
		MakerAmount:   yesMakerAmount,
		TakerAmount:   yesTakerAmount,
		Side:          model.BUY,
		FeeRateBps:    "0",
		Nonce:         "0",
		Signer:        signerAddress,
		Expiration:    "0",
		SignatureType: c.signatureType,
	}

	yesSignedOrder, err := c.orderBuilder.BuildSignedOrder(c.privateKey, yesOrderData, model.CTFExchange)
	if err != nil {
		err = fmt.Errorf("build YES order: %w", err)
		return yesResp, noResp, err
	}

	// Build NO order with rounded amounts
	noMakerUSD := roundAmount(noTakerTokens*noPrice, noAmountPrecision)
	noMakerAmount := usdToRawAmount(noMakerUSD)
	noTakerAmount := usdToRawAmount(noTakerTokens)

	noOrderData := &model.OrderData{
		Maker:         makerAddress,
		Taker:         "0x0000000000000000000000000000000000000000",
		TokenId:       noTokenID,
		MakerAmount:   noMakerAmount,
		TakerAmount:   noTakerAmount,
		Side:          model.BUY,
		FeeRateBps:    "0",
		Nonce:         "0",
		Signer:        signerAddress,
		Expiration:    "0",
		SignatureType: c.signatureType,
	}

	noSignedOrder, err := c.orderBuilder.BuildSignedOrder(c.privateKey, noOrderData, model.CTFExchange)
	if err != nil {
		err = fmt.Errorf("build NO order: %w", err)
		return yesResp, noResp, err
	}

	c.logger.Info("batch-orders-built",
		zap.String("maker", makerAddress),
		zap.String("signer", signerAddress),
		zap.Float64("size", size))

	// Convert signed orders to JSON format
	yesOrderJSON := c.convertToOrderJSON(yesSignedOrder)
	noOrderJSON := c.convertToOrderJSON(noSignedOrder)

	// Create batch request
	batchReq := types.BatchOrderRequest{
		{Order: yesOrderJSON, Owner: c.apiKey, OrderType: "GTC"},
		{Order: noOrderJSON, Owner: c.apiKey, OrderType: "GTC"},
	}

	// Submit batch
	batchResp, err := c.submitBatchOrder(ctx, batchReq)
	if err != nil {
		return yesResp, noResp, err
	}

	// Validate we got 2 responses
	if len(batchResp) != 2 {
		err = fmt.Errorf("expected 2 responses, got %d", len(batchResp))
		return yesResp, noResp, err
	}

	yesResp = &batchResp[0]
	noResp = &batchResp[1]

	// Check for errors
	if !yesResp.Success {
		err = &types.OrderError{
			Code:    yesResp.ErrorMsg,
			Message: yesResp.ErrorMsg,
			OrderID: yesResp.OrderID,
			Side:    "YES",
		}
		return yesResp, noResp, err
	}
	if !noResp.Success {
		err = &types.OrderError{
			Code:    noResp.ErrorMsg,
			Message: noResp.ErrorMsg,
			OrderID: noResp.OrderID,
			Side:    "NO",
		}
		return yesResp, noResp, err
	}

	return yesResp, noResp, nil
}

// convertToOrderJSON converts a signed order to JSON format
func (c *OrderClient) convertToOrderJSON(order *model.SignedOrder) types.SignedOrderJSON {
	sideStr := "BUY"
	if order.Side.Uint64() == uint64(model.SELL) {
		sideStr = "SELL"
	}

	return types.SignedOrderJSON{
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
}

// submitBatchOrder submits a batch of orders to POST /orders endpoint
func (c *OrderClient) submitBatchOrder(
	ctx context.Context,
	req types.BatchOrderRequest,
) (resp types.BatchOrderResponse, err error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		err = fmt.Errorf("marshal batch request: %w", err)
		return resp, err
	}

	// Create HMAC signature
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	method := "POST"
	requestPath := "/orders" // Note: plural for batch endpoint

	signaturePayload := timestamp + method + requestPath + string(reqBody)

	// Decode secret using URL-safe base64
	secretBytes, err := base64.URLEncoding.DecodeString(c.secret)
	if err != nil {
		err = fmt.Errorf("decode secret: %w", err)
		return resp, err
	}

	h := hmac.New(sha256.New, secretBytes)
	h.Write([]byte(signaturePayload))
	signature := base64.URLEncoding.EncodeToString(h.Sum(nil))

	// Make request
	url := "https://clob.polymarket.com" + requestPath
	httpReq, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(reqBody))
	if err != nil {
		err = fmt.Errorf("create request: %w", err)
		return resp, err
	}

	// Set headers (same as single order)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("POLY_API_KEY", c.apiKey)
	httpReq.Header.Set("POLY_SIGNATURE", signature)
	httpReq.Header.Set("POLY_TIMESTAMP", timestamp)
	httpReq.Header.Set("POLY_PASSPHRASE", c.passphrase)
	httpReq.Header.Set("POLY_ADDRESS", c.address)

	client := &http.Client{Timeout: 30 * time.Second}
	httpResp, err := client.Do(httpReq)
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
		err = fmt.Errorf("parse batch response: %w\nBody: %s", err, string(body))
		return resp, err
	}

	return resp, nil
}

func (c *OrderClient) submitOrder(
	ctx context.Context,
	order *model.SignedOrder,
) (resp *types.OrderSubmissionResponse, err error) {
	// Convert to JSON format using helper method
	jsonOrder := c.convertToOrderJSON(order)

	// Wrap order in the required structure
	// Note: "owner" is the API key, not the maker address (per Python client)
	orderRequest := types.OrderSubmissionRequest{
		Order:     jsonOrder,
		Owner:     c.apiKey,
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
	requestPath := "/order"

	signaturePayload := timestamp + method + requestPath + string(reqBody)

	// Decode secret using URL-safe base64 (Python client uses urlsafe_b64decode)
	secretBytes, err := base64.URLEncoding.DecodeString(c.secret)
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
	req.Header.Set("POLY_API_KEY", c.apiKey)
	req.Header.Set("POLY_SIGNATURE", signature)
	req.Header.Set("POLY_TIMESTAMP", timestamp)
	req.Header.Set("POLY_PASSPHRASE", c.passphrase)
	req.Header.Set("POLY_ADDRESS", c.address) // EOA address from private key

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

func usdToRawAmount(usd float64) string {
	rawAmount := int64(usd * 1000000)
	return fmt.Sprintf("%d", rawAmount)
}

// getRoundingConfig returns the precision for size and amount based on tick size
// Matches Python client's ROUNDING_CONFIG
func getRoundingConfig(tickSize float64) (sizePrecision int, amountPrecision int) {
	switch tickSize {
	case 0.1:
		return 2, 3 // size=2, amount=3
	case 0.01:
		return 2, 4 // size=2, amount=4
	case 0.001:
		return 2, 5 // size=2, amount=5
	case 0.0001:
		return 2, 6 // size=2, amount=6
	default:
		return 2, 4 // Default to 0.01 tick size
	}
}

// roundAmount rounds an amount to the specified number of decimal places
func roundAmount(value float64, decimals int) float64 {
	multiplier := math.Pow(10, float64(decimals))
	return math.Round(value*multiplier) / multiplier
}
