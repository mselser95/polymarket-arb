package cmd

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

var balanceCmd = &cobra.Command{
	Use:   "balance",
	Short: "Check your wallet balances and positions",
	Long: `Display your current holdings including:
- MATIC balance (for gas)
- USDC balance (for trading)
- USDC allowance (approved to CTF Exchange)
- Active positions (outcome tokens you hold)`,
	RunE: runBalance,
}

var (
	showPositions bool
	balanceRPC    string
)

func init() {
	rootCmd.AddCommand(balanceCmd)

	balanceCmd.Flags().BoolVarP(&showPositions, "positions", "p", true, "Show active positions")
	balanceCmd.Flags().StringVarP(&balanceRPC, "rpc", "r", "https://polygon-rpc.com", "Polygon RPC endpoint")
}

const (
	polygonUSDC        = "0x2791Bca1f2de4661ED88A30C99A7a9449Aa84174"
	polygonCTFExchange = "0x4bFb41d5B3570DeFd03C39a9A4D8dE6Bd8B8982E"
)

func runBalance(cmd *cobra.Command, args []string) error {
	// Load .env
	if err := godotenv.Load(); err != nil {
		fmt.Printf("Warning: .env file not found\n")
	}

	privateKeyHex := os.Getenv("POLYMARKET_PRIVATE_KEY")
	if privateKeyHex == "" {
		return fmt.Errorf("POLYMARKET_PRIVATE_KEY not set in .env")
	}

	apiKey := os.Getenv("POLYMARKET_API_KEY")

	// Parse private key
	privateKey, err := crypto.HexToECDSA(strings.TrimPrefix(privateKeyHex, "0x"))
	if err != nil {
		return fmt.Errorf("parse private key: %w", err)
	}

	// Derive address
	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		return fmt.Errorf("error casting public key to ECDSA")
	}
	address := crypto.PubkeyToAddress(*publicKeyECDSA)

	fmt.Printf("=== Wallet Balance Sheet ===\n\n")
	fmt.Printf("Address: %s\n\n", address.Hex())

	// Connect to Polygon
	client, err := ethclient.Dial(balanceRPC)
	if err != nil {
		return fmt.Errorf("connect to Polygon: %w", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get MATIC balance
	maticBalance, err := client.BalanceAt(ctx, address, nil)
	if err != nil {
		return fmt.Errorf("get MATIC balance: %w", err)
	}

	maticFloat := new(big.Float).Quo(new(big.Float).SetInt(maticBalance), big.NewFloat(1e18))
	fmt.Printf("MATIC Balance: %s MATIC\n", maticFloat.Text('f', 6))

	// Get USDC balance
	usdcBalance, err := getTokenBalance(client, address, polygonUSDC)
	if err != nil {
		return fmt.Errorf("get USDC balance: %w", err)
	}

	usdcFloat := new(big.Float).Quo(new(big.Float).SetInt(usdcBalance), big.NewFloat(1e6))
	fmt.Printf("USDC Balance: %s USDC\n", usdcFloat.Text('f', 2))

	// Get USDC allowance
	allowance, err := getUSDCAllowance(client, address)
	if err != nil {
		return fmt.Errorf("get allowance: %w", err)
	}

	allowanceFloat := new(big.Float).Quo(new(big.Float).SetInt(allowance), big.NewFloat(1e6))
	if allowance.Cmp(big.NewInt(0).SetUint64(1e18)) > 0 {
		fmt.Printf("USDC Allowance: Unlimited ✅\n")
	} else {
		fmt.Printf("USDC Allowance: %s USDC\n", allowanceFloat.Text('f', 2))
	}

	// Show positions if requested
	if showPositions && apiKey != "" {
		fmt.Printf("\n=== Active Positions ===\n\n")
		positions, err := getPositions(apiKey, address.Hex())
		if err != nil {
			fmt.Printf("Error fetching positions: %v\n", err)
		} else if len(positions) == 0 {
			fmt.Printf("No active positions\n")
		} else {
			totalValue := 0.0
			for _, pos := range positions {
				fmt.Printf("Market: %s\n", pos.MarketSlug)
				fmt.Printf("  Outcome: %s\n", pos.Outcome)
				fmt.Printf("  Size: %.2f tokens\n", pos.Size)
				fmt.Printf("  Value: $%.2f\n\n", pos.Value)
				totalValue += pos.Value
			}
			fmt.Printf("Total Position Value: $%.2f\n", totalValue)
		}
	}

	fmt.Printf("\n=== Summary ===\n")
	fmt.Printf("Ready to trade: ")
	if usdcBalance.Cmp(big.NewInt(1000000)) >= 0 && allowance.Cmp(big.NewInt(0)) > 0 {
		fmt.Printf("✅ YES\n")
		fmt.Printf("\nYou can place orders:\n")
		fmt.Printf("  go run . place-orders <market> --size 1.0 --yes-price 0.50 --no-price 0.50\n")
	} else {
		fmt.Printf("❌ NO\n")
		if usdcBalance.Cmp(big.NewInt(1000000)) < 0 {
			fmt.Printf("  - Need more USDC (minimum $1.00)\n")
		}
		if allowance.Cmp(big.NewInt(0)) == 0 {
			fmt.Printf("  - Need to approve USDC spending: go run . approve\n")
		}
	}

	return nil
}

func getTokenBalance(client *ethclient.Client, owner common.Address, tokenAddr string) (*big.Int, error) {
	// balanceOf(address owner) returns (uint256)
	balanceOfABI := `[{"constant":true,"inputs":[{"name":"owner","type":"address"}],"name":"balanceOf","outputs":[{"name":"","type":"uint256"}],"type":"function"}]`

	parsedABI, err := abi.JSON(strings.NewReader(balanceOfABI))
	if err != nil {
		return nil, err
	}

	data, err := parsedABI.Pack("balanceOf", owner)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tokenAddress := common.HexToAddress(tokenAddr)
	msg := ethereum.CallMsg{
		To:   &tokenAddress,
		Data: data,
	}

	result, err := client.CallContract(ctx, msg, nil)
	if err != nil {
		return nil, err
	}

	balance := new(big.Int).SetBytes(result)
	return balance, nil
}

func getUSDCAllowance(client *ethclient.Client, owner common.Address) (*big.Int, error) {
	// allowance(address owner, address spender) returns (uint256)
	allowanceABI := `[{"constant":true,"inputs":[{"name":"owner","type":"address"},{"name":"spender","type":"address"}],"name":"allowance","outputs":[{"name":"","type":"uint256"}],"type":"function"}]`

	parsedABI, err := abi.JSON(strings.NewReader(allowanceABI))
	if err != nil {
		return nil, err
	}

	data, err := parsedABI.Pack("allowance", owner, common.HexToAddress(polygonCTFExchange))
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	usdcAddr := common.HexToAddress(polygonUSDC)
	msg := ethereum.CallMsg{
		To:   &usdcAddr,
		Data: data,
	}

	result, err := client.CallContract(ctx, msg, nil)
	if err != nil {
		return nil, err
	}

	allowance := new(big.Int).SetBytes(result)
	return allowance, nil
}

type Position struct {
	MarketSlug string
	Outcome    string
	Size       float64
	Value      float64
}

func getPositions(apiKey, address string) ([]Position, error) {
	url := fmt.Sprintf("https://clob.polymarket.com/positions?user=%s", address)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: status %d", resp.StatusCode)
	}

	var rawPositions []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&rawPositions); err != nil {
		return nil, err
	}

	positions := make([]Position, 0)
	for _, pos := range rawPositions {
		if size, ok := pos["size"].(float64); ok && size > 0 {
			position := Position{
				Size: size,
			}

			if market, ok := pos["market"].(string); ok {
				position.MarketSlug = market
			}
			if outcome, ok := pos["outcome"].(string); ok {
				position.Outcome = outcome
			}
			if value, ok := pos["value"].(float64); ok {
				position.Value = value
			}

			positions = append(positions, position)
		}
	}

	return positions, nil
}
