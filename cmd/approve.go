package cmd

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

var approveCmd = &cobra.Command{
	Use:   "approve",
	Short: "Approve CTF Exchange to spend your USDC",
	Long: `Approve the Polymarket CTF Exchange contract to spend your USDC.
This is a one-time on-chain transaction required before you can place orders.

The approval allows the exchange to transfer USDC from your wallet when orders are matched.
This command will approve unlimited spending (max uint256) by default.`,
	RunE: runApprove,
}

var (
	approvalAmount string
	polygonRPC     string
)

func init() {
	rootCmd.AddCommand(approveCmd)

	approveCmd.Flags().StringVarP(&approvalAmount, "amount", "a", "unlimited", "Approval amount (unlimited, or specific USDC amount)")
	approveCmd.Flags().StringVarP(&polygonRPC, "rpc", "r", "https://polygon-rpc.com", "Polygon RPC endpoint")
}

const (
	// Polygon mainnet addresses
	usdcAddress        = "0x2791Bca1f2de4661ED88A30C99A7a9449Aa84174"
	ctfExchangeAddress = "0x4bFb41d5B3570DeFd03C39a9A4D8dE6Bd8B8982E"
	ctfAddress         = "0x4d97dcd97ec945f40cf65f87097ace5ea0476045" // CTF ERC1155 contract
)

// ERC20 approve function ABI
const erc20ApproveABI = `[{"constant":false,"inputs":[{"name":"spender","type":"address"},{"name":"value","type":"uint256"}],"name":"approve","outputs":[{"name":"","type":"bool"}],"type":"function"}]`

// ERC1155 setApprovalForAll and isApprovedForAll ABIs
const erc1155ApprovalABI = `[
	{"constant":false,"inputs":[{"name":"operator","type":"address"},{"name":"approved","type":"bool"}],"name":"setApprovalForAll","outputs":[],"type":"function"},
	{"constant":true,"inputs":[{"name":"owner","type":"address"},{"name":"operator","type":"address"}],"name":"isApprovedForAll","outputs":[{"name":"","type":"bool"}],"type":"function"}
]`

func runApprove(cmd *cobra.Command, args []string) error {
	// Load .env
	if err := godotenv.Load(); err != nil {
		fmt.Printf("Warning: .env file not found\n")
	}

	privateKeyHex := os.Getenv("POLYMARKET_PRIVATE_KEY")
	if privateKeyHex == "" {
		return fmt.Errorf("POLYMARKET_PRIVATE_KEY not set in .env")
	}

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
	fromAddress := crypto.PubkeyToAddress(*publicKeyECDSA)

	fmt.Printf("=== Approve Polymarket Trading ===\n\n")
	fmt.Printf("Your Address: %s\n", fromAddress.Hex())
	fmt.Printf("USDC Token: %s\n", usdcAddress)
	fmt.Printf("CTF Contract: %s\n", ctfAddress)
	fmt.Printf("CTF Exchange: %s\n", ctfExchangeAddress)
	fmt.Printf("RPC: %s\n\n", polygonRPC)

	// Connect to Polygon
	client, err := ethclient.Dial(polygonRPC)
	if err != nil {
		return fmt.Errorf("connect to Polygon: %w", err)
	}
	defer client.Close()

	// Check current allowance
	fmt.Printf("Checking current allowance...\n")
	currentAllowance, err := checkAllowance(client, fromAddress)
	if err != nil {
		return fmt.Errorf("check allowance: %w", err)
	}

	allowanceUSDC := new(big.Float).Quo(new(big.Float).SetInt(currentAllowance), big.NewFloat(1e6))
	fmt.Printf("Current Allowance: %s USDC\n\n", allowanceUSDC.Text('f', 2))

	// Determine approval amount
	var approveAmount *big.Int
	if approvalAmount == "unlimited" {
		// Max uint256
		approveAmount = new(big.Int)
		approveAmount.SetString("115792089237316195423570985008687907853269984665640564039457584007913129639935", 10)
		fmt.Printf("Approving: Unlimited (max uint256)\n\n")
	} else {
		// Parse specific amount
		amountFloat := new(big.Float)
		if _, ok := amountFloat.SetString(approvalAmount); !ok {
			return fmt.Errorf("invalid amount: %s", approvalAmount)
		}
		// Convert to USDC (6 decimals)
		amountFloat.Mul(amountFloat, big.NewFloat(1e6))
		approveAmount, _ = amountFloat.Int(nil)
		fmt.Printf("Approving: %s USDC\n\n", approvalAmount)
	}

	// Build approval transaction
	parsedABI, err := abi.JSON(strings.NewReader(erc20ApproveABI))
	if err != nil {
		return fmt.Errorf("parse ABI: %w", err)
	}

	data, err := parsedABI.Pack("approve", common.HexToAddress(ctfExchangeAddress), approveAmount)
	if err != nil {
		return fmt.Errorf("pack approve call: %w", err)
	}

	// Get nonce
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	nonce, err := client.PendingNonceAt(ctx, fromAddress)
	if err != nil {
		return fmt.Errorf("get nonce: %w", err)
	}

	// Get gas price
	gasPrice, err := client.SuggestGasPrice(ctx)
	if err != nil {
		return fmt.Errorf("get gas price: %w", err)
	}

	// Estimate gas
	gasLimit := uint64(100000) // Standard ERC20 approve

	// Create transaction
	tx := types.NewTransaction(
		nonce,
		common.HexToAddress(usdcAddress),
		big.NewInt(0),
		gasLimit,
		gasPrice,
		data,
	)

	// Get chain ID
	chainID, err := client.ChainID(ctx)
	if err != nil {
		return fmt.Errorf("get chain ID: %w", err)
	}

	// Sign transaction
	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), privateKey)
	if err != nil {
		return fmt.Errorf("sign transaction: %w", err)
	}

	// Calculate cost
	gasCostWei := new(big.Int).Mul(gasPrice, big.NewInt(int64(gasLimit)))
	gasCostEth := new(big.Float).Quo(new(big.Float).SetInt(gasCostWei), big.NewFloat(1e18))

	fmt.Printf("Transaction Details:\n")
	fmt.Printf("  Nonce: %d\n", nonce)
	fmt.Printf("  Gas Limit: %d\n", gasLimit)
	fmt.Printf("  Gas Price: %s Gwei\n", new(big.Float).Quo(new(big.Float).SetInt(gasPrice), big.NewFloat(1e9)).Text('f', 2))
	fmt.Printf("  Estimated Cost: %s MATIC\n\n", gasCostEth.Text('f', 6))

	// Send transaction
	fmt.Printf("Sending transaction...\n")
	if err := client.SendTransaction(ctx, signedTx); err != nil {
		return fmt.Errorf("send transaction: %w", err)
	}

	txHash := signedTx.Hash().Hex()
	fmt.Printf("Transaction sent!\n")
	fmt.Printf("   TX Hash: %s\n", txHash)
	fmt.Printf("   View: https://polygonscan.com/tx/%s\n\n", txHash)

	// Wait for confirmation
	fmt.Printf("Waiting for confirmation")
	receipt, err := waitForReceipt(client, signedTx.Hash())
	if err != nil {
		return fmt.Errorf("\nTransaction failed: %w", err)
	}

	if receipt.Status == 1 {
		fmt.Printf("\n\nUSDC Approval successful!\n")
		fmt.Printf("   Gas Used: %d\n", receipt.GasUsed)
		fmt.Printf("   Block: %d\n\n", receipt.BlockNumber.Uint64())
	} else {
		fmt.Printf("\n\nUSDC Transaction reverted\n")
		return fmt.Errorf("USDC approval failed")
	}

	// Step 2: Check and approve CTF tokens for selling positions
	fmt.Printf("=== Step 2: Approve CTF Outcome Tokens ===\n\n")
	fmt.Printf("Checking CTF approval status...\n")

	ctfApproved, err := checkCTFApproval(client, fromAddress)
	if err != nil {
		return fmt.Errorf("check CTF approval: %w", err)
	}

	if ctfApproved {
		fmt.Printf("CTF tokens already approved ✓\n\n")
	} else {
		fmt.Printf("CTF tokens NOT approved. Approving now...\n\n")

		_, err = approveCTF(client, privateKey, fromAddress)
		if err != nil {
			return fmt.Errorf("approve CTF: %w", err)
		}
	}

	fmt.Printf("=== Setup Complete ===\n\n")
	fmt.Printf("✓ USDC approved (for buying positions)\n")
	fmt.Printf("✓ CTF tokens approved (for selling positions)\n\n")
	fmt.Printf("You can now:\n")
	fmt.Printf("  • Buy positions:  go run . place-orders <market> --size 1.0 --yes-price 0.50 --no-price 0.50\n")
	fmt.Printf("  • Sell positions: go run . close-positions\n")

	return nil
}

func checkAllowance(client *ethclient.Client, owner common.Address) (*big.Int, error) {
	// allowance(address owner, address spender) returns (uint256)
	allowanceABI := `[{"constant":true,"inputs":[{"name":"owner","type":"address"},{"name":"spender","type":"address"}],"name":"allowance","outputs":[{"name":"","type":"uint256"}],"type":"function"}]`

	parsedABI, err := abi.JSON(strings.NewReader(allowanceABI))
	if err != nil {
		return nil, err
	}

	data, err := parsedABI.Pack("allowance", owner, common.HexToAddress(ctfExchangeAddress))
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	usdcAddr := common.HexToAddress(usdcAddress)
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

func waitForReceipt(client *ethclient.Client, txHash common.Hash) (*types.Receipt, error) {
	ctx := context.Background()

	for i := 0; i < 60; i++ {
		receipt, err := client.TransactionReceipt(ctx, txHash)
		if err == nil {
			return receipt, nil
		}

		fmt.Printf(".")
		time.Sleep(2 * time.Second)
	}

	return nil, fmt.Errorf("timeout waiting for receipt")
}

func checkCTFApproval(client *ethclient.Client, owner common.Address) (approved bool, err error) {
	// isApprovedForAll(address owner, address operator) returns (bool)
	parsedABI, err := abi.JSON(strings.NewReader(erc1155ApprovalABI))
	if err != nil {
		return false, fmt.Errorf("parse ABI: %w", err)
	}

	data, err := parsedABI.Pack("isApprovedForAll", owner, common.HexToAddress(ctfExchangeAddress))
	if err != nil {
		return false, fmt.Errorf("pack call: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ctfAddr := common.HexToAddress(ctfAddress)
	msg := ethereum.CallMsg{
		To:   &ctfAddr,
		Data: data,
	}

	result, err := client.CallContract(ctx, msg, nil)
	if err != nil {
		return false, fmt.Errorf("call contract: %w", err)
	}

	// Parse boolean result
	var isApproved bool
	err = parsedABI.UnpackIntoInterface(&isApproved, "isApprovedForAll", result)
	if err != nil {
		return false, fmt.Errorf("unpack result: %w", err)
	}

	return isApproved, nil
}

func approveCTF(
	client *ethclient.Client,
	privateKey *ecdsa.PrivateKey,
	fromAddress common.Address,
) (txHash string, err error) {
	// setApprovalForAll(address operator, bool approved)
	parsedABI, err := abi.JSON(strings.NewReader(erc1155ApprovalABI))
	if err != nil {
		return "", fmt.Errorf("parse ABI: %w", err)
	}

	data, err := parsedABI.Pack("setApprovalForAll", common.HexToAddress(ctfExchangeAddress), true)
	if err != nil {
		return "", fmt.Errorf("pack setApprovalForAll: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get nonce
	nonce, err := client.PendingNonceAt(ctx, fromAddress)
	if err != nil {
		return "", fmt.Errorf("get nonce: %w", err)
	}

	// Get gas price
	gasPrice, err := client.SuggestGasPrice(ctx)
	if err != nil {
		return "", fmt.Errorf("get gas price: %w", err)
	}

	// Estimate gas
	gasLimit := uint64(100000) // Standard ERC1155 setApprovalForAll

	// Create transaction
	tx := types.NewTransaction(
		nonce,
		common.HexToAddress(ctfAddress),
		big.NewInt(0),
		gasLimit,
		gasPrice,
		data,
	)

	// Get chain ID
	chainID, err := client.ChainID(ctx)
	if err != nil {
		return "", fmt.Errorf("get chain ID: %w", err)
	}

	// Sign transaction
	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), privateKey)
	if err != nil {
		return "", fmt.Errorf("sign transaction: %w", err)
	}

	// Calculate cost
	gasCostWei := new(big.Int).Mul(gasPrice, big.NewInt(int64(gasLimit)))
	gasCostEth := new(big.Float).Quo(new(big.Float).SetInt(gasCostWei), big.NewFloat(1e18))

	fmt.Printf("Transaction Details:\n")
	fmt.Printf("  Nonce: %d\n", nonce)
	fmt.Printf("  Gas Limit: %d\n", gasLimit)
	fmt.Printf("  Gas Price: %s Gwei\n", new(big.Float).Quo(new(big.Float).SetInt(gasPrice), big.NewFloat(1e9)).Text('f', 2))
	fmt.Printf("  Estimated Cost: %s MATIC\n\n", gasCostEth.Text('f', 6))

	// Send transaction
	fmt.Printf("Sending transaction...\n")
	err = client.SendTransaction(ctx, signedTx)
	if err != nil {
		return "", fmt.Errorf("send transaction: %w", err)
	}

	txHash = signedTx.Hash().Hex()
	fmt.Printf("Transaction sent!\n")
	fmt.Printf("   TX Hash: %s\n", txHash)
	fmt.Printf("   View: https://polygonscan.com/tx/%s\n\n", txHash)

	// Wait for confirmation
	fmt.Printf("Waiting for confirmation")
	receipt, err := waitForReceipt(client, signedTx.Hash())
	if err != nil {
		return "", fmt.Errorf("\nTransaction failed: %w", err)
	}

	if receipt.Status == 1 {
		fmt.Printf("\n\nApproval successful!\n")
		fmt.Printf("   Gas Used: %d\n", receipt.GasUsed)
		fmt.Printf("   Block: %d\n\n", receipt.BlockNumber.Uint64())
	} else {
		return "", fmt.Errorf("\nTransaction reverted")
	}

	return txHash, nil
}
