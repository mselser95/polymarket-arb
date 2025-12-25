package cmd

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

var deriveAPICredsCmd = &cobra.Command{
	Use:   "derive-api-creds",
	Short: "Derive API credentials using L1 authentication (private key)",
	Long: `Uses your private key to derive Polymarket API credentials via L1 authentication.
This creates or retrieves the API KEY, SECRET, and PASSPHRASE needed for trading.

The credentials will be printed - save them to your .env file:
  POLYMARKET_API_KEY=...
  POLYMARKET_SECRET=...
  POLYMARKET_PASSPHRASE=...`,
	RunE: runDeriveAPICreds,
}

func init() {
	rootCmd.AddCommand(deriveAPICredsCmd)
}

func runDeriveAPICreds(cmd *cobra.Command, args []string) error {
	// Load .env
	if err := godotenv.Load(); err != nil {
		fmt.Printf("Warning: .env file not found\n")
	}

	// Get private key from env
	privateKeyHex := getEnv("POLYMARKET_PRIVATE_KEY", "POLY_PRIVATE_KEY")
	if privateKeyHex == "" {
		return fmt.Errorf("missing POLYMARKET_PRIVATE_KEY in .env file")
	}

	// Clean up private key
	privateKeyHex = strings.TrimSpace(privateKeyHex)
	privateKeyHex = strings.TrimPrefix(privateKeyHex, "0x")

	// Parse private key
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return fmt.Errorf("invalid private key: %w", err)
	}

	// Get address
	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		return fmt.Errorf("error casting public key to ECDSA")
	}
	address := crypto.PubkeyToAddress(*publicKeyECDSA)

	fmt.Printf("=== Deriving Polymarket API Credentials ===\n\n")
	fmt.Printf("Private Key: %s...\n", privateKeyHex[:10])
	fmt.Printf("EOA Address: %s\n", address.Hex())

	proxyAddr := getEnv("POLYMARKET_PROXY_ADDRESS")
	if proxyAddr != "" {
		fmt.Printf("Proxy Address: %s\n\n", proxyAddr)
	} else {
		fmt.Printf("\n")
	}

	// Create EIP-712 signature for /auth/derive-api-key
	timestamp := time.Now().Unix()
	nonce := 0

	// EIP-712 domain
	chainID := math.NewHexOrDecimal256(137)
	domain := apitypes.TypedDataDomain{
		Name:    "ClobAuthDomain",
		Version: "1",
		ChainId: chainID, // Polygon
	}

	// Message types
	message := map[string]interface{}{
		"address":   address.Hex(),
		"timestamp": fmt.Sprintf("%d", timestamp),
		"nonce":     fmt.Sprintf("%d", nonce),
		"message":   "This message attests that I control the given wallet",
	}

	types := apitypes.Types{
		"EIP712Domain": []apitypes.Type{
			{Name: "name", Type: "string"},
			{Name: "version", Type: "string"},
			{Name: "chainId", Type: "uint256"},
		},
		"ClobAuth": []apitypes.Type{
			{Name: "address", Type: "address"},
			{Name: "timestamp", Type: "string"},
			{Name: "nonce", Type: "uint256"},
			{Name: "message", Type: "string"},
		},
	}

	typedData := apitypes.TypedData{
		Types:       types,
		PrimaryType: "ClobAuth",
		Domain:      domain,
		Message:     message,
	}

	// Sign the message
	domainSeparator, err := typedData.HashStruct("EIP712Domain", typedData.Domain.Map())
	if err != nil {
		return fmt.Errorf("hash domain: %w", err)
	}

	typedDataHash, err := typedData.HashStruct(typedData.PrimaryType, typedData.Message)
	if err != nil {
		return fmt.Errorf("hash message: %w", err)
	}

	rawData := []byte(fmt.Sprintf("\x19\x01%s%s", string(domainSeparator), string(typedDataHash)))
	hash := crypto.Keccak256Hash(rawData)

	signature, err := crypto.Sign(hash.Bytes(), privateKey)
	if err != nil {
		return fmt.Errorf("sign message: %w", err)
	}

	// Adjust V value for Ethereum (27 or 28)
	if signature[64] < 27 {
		signature[64] += 27
	}

	signatureHex := hexutil.Encode(signature)

	fmt.Printf("Timestamp: %d\n", timestamp)
	fmt.Printf("Signature: %s\n\n", signatureHex)

	// Call the API
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	url := "https://clob.polymarket.com/auth/derive-api-key"
	fmt.Printf("Calling: GET %s\n\n", url)

	// Make request
	req, err := newGetRequest(ctx, url, map[string]string{
		"POLY_ADDRESS":   address.Hex(),
		"POLY_SIGNATURE": signatureHex,
		"POLY_TIMESTAMP": fmt.Sprintf("%d", timestamp),
		"POLY_NONCE":     fmt.Sprintf("%d", nonce),
	})
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	fmt.Printf("Status: %d\n", resp.StatusCode)
	fmt.Printf("Response: %s\n\n", string(body))

	if resp.StatusCode != 200 {
		return fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	// Parse credentials
	var creds struct {
		APIKey     string `json:"apiKey"`
		Secret     string `json:"secret"`
		Passphrase string `json:"passphrase"`
	}

	if err := json.Unmarshal(body, &creds); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	// Display credentials
	fmt.Printf("=== ✅ API Credentials Derived ===\n\n")
	fmt.Printf("POLYMARKET_API_KEY=%s\n", creds.APIKey)
	fmt.Printf("POLYMARKET_SECRET=%s\n", creds.Secret)
	fmt.Printf("POLYMARKET_PASSPHRASE=%s\n\n", creds.Passphrase)
	fmt.Printf("⚠️  Save these to your .env file immediately!\n")
	fmt.Printf("⚠️  They are cryptographically linked to your private key.\n")

	return nil
}

func newGetRequest(ctx context.Context, url string, headers map[string]string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	return req, nil
}
