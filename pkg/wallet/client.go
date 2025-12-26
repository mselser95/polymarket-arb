package wallet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"go.uber.org/zap"
)

const (
	polygonUSDC        = "0x2791Bca1f2de4661ED88A30C99A7a9449Aa84174"
	polygonCTFExchange = "0x4bFb41d5B3570DeFd03C39a9A4D8dE6Bd8B8982E"
	dataAPIBaseURL     = "https://data-api.polymarket.com"
)

// Client handles wallet data fetching from blockchain and APIs.
type Client struct {
	rpcURL     string
	httpClient *http.Client
	logger     *zap.Logger
}

// Balances holds on-chain token balances.
type Balances struct {
	MATIC         *big.Int // in wei
	USDC          *big.Int // in 6-decimal units
	USDCAllowance *big.Int // in 6-decimal units
}

// Position represents a market position from the Data API.
type Position struct {
	MarketSlug   string
	Outcome      string
	Size         float64
	Value        float64 // Current USD value
	InitialValue float64 // Cost basis USD
	CashPnL      float64 // USD P&L
	PercentPnL   float64 // Percentage P&L
}

// dataAPIPosition represents the response from Polymarket Data API.
type dataAPIPosition struct {
	Asset        string  `json:"asset"`
	ConditionID  string  `json:"conditionId"`
	Size         float64 `json:"size"`
	AvgPrice     float64 `json:"avgPrice"`
	InitialValue float64 `json:"initialValue"`
	CurrentValue float64 `json:"currentValue"`
	CashPnL      float64 `json:"cashPnl"`
	PercentPnL   float64 `json:"percentPnl"`
	CurPrice     float64 `json:"curPrice"`
	Title        string  `json:"title"`
	Slug         string  `json:"slug"`
	Outcome      string  `json:"outcome"`
}

// NewClient creates a new wallet client.
func NewClient(rpcURL string, logger *zap.Logger) (c *Client, err error) {
	if rpcURL == "" {
		return nil, errors.New("rpcURL cannot be empty")
	}

	if logger == nil {
		return nil, errors.New("logger cannot be nil")
	}

	client := &Client{
		rpcURL: rpcURL,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		logger: logger,
	}

	return client, nil
}

// GetBalances fetches on-chain token balances.
func (c *Client) GetBalances(ctx context.Context, address common.Address) (balances *Balances, err error) {
	client, err := ethclient.DialContext(ctx, c.rpcURL)
	if err != nil {
		return nil, fmt.Errorf("dial RPC: %w", err)
	}
	defer client.Close()

	// Get MATIC balance
	maticBalance, err := client.BalanceAt(ctx, address, nil)
	if err != nil {
		return nil, fmt.Errorf("get MATIC balance: %w", err)
	}

	// Get USDC balance
	usdcBalance, err := c.getERC20Balance(ctx, client, address, polygonUSDC)
	if err != nil {
		return nil, fmt.Errorf("get USDC balance: %w", err)
	}

	// Get USDC allowance
	allowance, err := c.getERC20Allowance(ctx, client, address, polygonUSDC, polygonCTFExchange)
	if err != nil {
		return nil, fmt.Errorf("get USDC allowance: %w", err)
	}

	balances = &Balances{
		MATIC:         maticBalance,
		USDC:          usdcBalance,
		USDCAllowance: allowance,
	}

	return balances, nil
}

// getERC20Balance fetches ERC20 token balance for an address.
func (c *Client) getERC20Balance(
	ctx context.Context,
	client *ethclient.Client,
	owner common.Address,
	tokenAddr string,
) (balance *big.Int, err error) {
	balanceOfABI := `[{"constant":true,"inputs":[{"name":"owner","type":"address"}],"name":"balanceOf","outputs":[{"name":"","type":"uint256"}],"type":"function"}]`

	parsedABI, err := abi.JSON(strings.NewReader(balanceOfABI))
	if err != nil {
		return nil, fmt.Errorf("parse ABI: %w", err)
	}

	data, err := parsedABI.Pack("balanceOf", owner)
	if err != nil {
		return nil, fmt.Errorf("pack ABI: %w", err)
	}

	tokenAddress := common.HexToAddress(tokenAddr)
	msg := ethereum.CallMsg{
		To:   &tokenAddress,
		Data: data,
	}

	result, err := client.CallContract(ctx, msg, nil)
	if err != nil {
		return nil, fmt.Errorf("call contract: %w", err)
	}

	balance = new(big.Int).SetBytes(result)
	return balance, nil
}

// getERC20Allowance fetches ERC20 token allowance.
func (c *Client) getERC20Allowance(
	ctx context.Context,
	client *ethclient.Client,
	owner common.Address,
	tokenAddr string,
	spender string,
) (allowance *big.Int, err error) {
	allowanceABI := `[{"constant":true,"inputs":[{"name":"owner","type":"address"},{"name":"spender","type":"address"}],"name":"allowance","outputs":[{"name":"","type":"uint256"}],"type":"function"}]`

	parsedABI, err := abi.JSON(strings.NewReader(allowanceABI))
	if err != nil {
		return nil, fmt.Errorf("parse ABI: %w", err)
	}

	data, err := parsedABI.Pack("allowance", owner, common.HexToAddress(spender))
	if err != nil {
		return nil, fmt.Errorf("pack ABI: %w", err)
	}

	tokenAddress := common.HexToAddress(tokenAddr)
	msg := ethereum.CallMsg{
		To:   &tokenAddress,
		Data: data,
	}

	result, err := client.CallContract(ctx, msg, nil)
	if err != nil {
		return nil, fmt.Errorf("call contract: %w", err)
	}

	allowance = new(big.Int).SetBytes(result)
	return allowance, nil
}

// GetPositions fetches positions from Polymarket Data API.
func (c *Client) GetPositions(ctx context.Context, address string) (positions []Position, err error) {
	url := fmt.Sprintf("%s/positions?user=%s&sizeThreshold=0.01", dataAPIBaseURL, address)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: status %d", resp.StatusCode)
	}

	var apiPositions []dataAPIPosition
	err = json.NewDecoder(resp.Body).Decode(&apiPositions)
	if err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	positions = make([]Position, 0, len(apiPositions))
	for _, pos := range apiPositions {
		if pos.Size > 0 {
			position := Position{
				MarketSlug:   pos.Slug,
				Outcome:      pos.Outcome,
				Size:         pos.Size,
				Value:        pos.CurrentValue,
				InitialValue: pos.InitialValue,
				CashPnL:      pos.CashPnL,
				PercentPnL:   pos.PercentPnL,
			}
			positions = append(positions, position)
		}
	}

	return positions, nil
}
