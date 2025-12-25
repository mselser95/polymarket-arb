package execution

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
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/polymarket/go-order-utils/pkg/builder"
	"github.com/polymarket/go-order-utils/pkg/model"
	"go.uber.org/zap"
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

// SignedOrderJSON represents a signed order in JSON format
type SignedOrderJSON struct {
	Salt          int64  `json:"salt"`          // Integer, not string
	Maker         string `json:"maker"`
	Signer        string `json:"signer"`
	Taker         string `json:"taker"`
	TokenID       string `json:"tokenId"`
	MakerAmount   string `json:"makerAmount"`
	TakerAmount   string `json:"takerAmount"`
	Side          string `json:"side"`
	Expiration    string `json:"expiration"`
	Nonce         string `json:"nonce"`
	FeeRateBps    string `json:"feeRateBps"`
	SignatureType int    `json:"signatureType"` // Integer, not string
	Signature     string `json:"signature"`
}

// OrderResponse represents the API response for an order
type OrderResponse struct {
	OrderID string  `json:"orderID"`
	Status  string  `json:"status"`
	Price   float64 `json:"price,string"`
	Size    float64 `json:"size,string"`
}

// PlaceOrders places YES and NO orders for arbitrage
func (c *OrderClient) PlaceOrders(ctx context.Context, yesTokenID, noTokenID string, size, yesPrice, noPrice float64) (*OrderResponse, *OrderResponse, error) {
	// Determine maker address
	makerAddress := c.address
	signerAddress := c.address
	if c.proxyAddress != "" {
		makerAddress = c.proxyAddress
	}

	// Build YES order
	yesMakerAmount := usdToRawAmount(size)
	yesTakerAmount := usdToRawAmount(size / yesPrice)

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

	// Build NO order
	noMakerAmount := usdToRawAmount(size)
	noTakerAmount := usdToRawAmount(size / noPrice)

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
	yesResp, err := c.submitOrder(ctx, yesSignedOrder)
	if err != nil {
		return nil, nil, fmt.Errorf("submit YES order: %w", err)
	}

	noResp, err := c.submitOrder(ctx, noSignedOrder)
	if err != nil {
		return yesResp, nil, fmt.Errorf("submit NO order: %w", err)
	}

	return yesResp, noResp, nil
}

func (c *OrderClient) submitOrder(ctx context.Context, order *model.SignedOrder) (*OrderResponse, error) {
	// Convert Side to string ("BUY" or "SELL")
	sideStr := "BUY"
	if order.Side.Uint64() == uint64(model.SELL) {
		sideStr = "SELL"
	}

	// Convert to JSON format
	jsonOrder := SignedOrderJSON{
		Salt:          order.Salt.Int64(),                      // Integer
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
		SignatureType: int(order.SignatureType.Int64()),        // Integer
		Signature:     "0x" + common.Bytes2Hex(order.Signature),
	}

	// Wrap order in the required structure
	// Note: "owner" is the API key, not the maker address (per Python client)
	orderRequest := map[string]interface{}{
		"order":     jsonOrder,
		"owner":     c.apiKey,
		"orderType": "GTC",
	}

	reqBody, err := json.Marshal(orderRequest)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Create HMAC signature
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	method := "POST"
	requestPath := "/order"

	signaturePayload := timestamp + method + requestPath + string(reqBody)

	// Decode secret using URL-safe base64 (Python client uses urlsafe_b64decode)
	secretBytes, err := base64.URLEncoding.DecodeString(c.secret)
	if err != nil {
		return nil, fmt.Errorf("decode secret: %w", err)
	}

	h := hmac.New(sha256.New, secretBytes)
	h.Write([]byte(signaturePayload))
	// Encode signature using URL-safe base64 (Python client uses urlsafe_b64encode)
	signature := base64.URLEncoding.EncodeToString(h.Sum(nil))

	// Make request
	url := "https://clob.polymarket.com" + requestPath
	req, err := http.NewRequestWithContext(ctx, method, url, strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// POLY_ADDRESS header should be the EOA address (per Python client: signer.address())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("POLY_API_KEY", c.apiKey)
	req.Header.Set("POLY_SIGNATURE", signature)
	req.Header.Set("POLY_TIMESTAMP", timestamp)
	req.Header.Set("POLY_PASSPHRASE", c.passphrase)
	req.Header.Set("POLY_ADDRESS", c.address) // EOA address from private key

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var orderResp OrderResponse
	if err := json.Unmarshal(body, &orderResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &orderResp, nil
}

func usdToRawAmount(usd float64) string {
	rawAmount := int64(usd * 1000000)
	return fmt.Sprintf("%d", rawAmount)
}
