package types

// OrderSubmissionResponse represents the response from POST /order or POST /orders.
// This is different from OrderQueryResponse (GET /order).
// Based on official Polymarket CLOB API documentation.
type OrderSubmissionResponse struct {
	Success      bool     `json:"success"`       // Server-side success indicator
	ErrorMsg     string   `json:"errorMsg"`      // Error message if success=false
	OrderID      string   `json:"orderId"`       // Note: lowercase 'd' per API spec
	OrderHashes  []string `json:"orderHashes"`   // Settlement transaction hashes
	Status       string   `json:"status"`        // matched, live, delayed, unmatched
	TakingAmount string   `json:"takingAmount"`  // Amount being taken (as string)
	MakingAmount string   `json:"makingAmount"`  // Amount being made (as string)
}

// SignedOrderJSON represents a signed order in the format expected by the CLOB API.
// Fields match the EIP-712 order structure after signing.
type SignedOrderJSON struct {
	Salt          int64  `json:"salt"`          // Integer per API spec (not string)
	Maker         string `json:"maker"`         // Funder address
	Signer        string `json:"signer"`        // Signing address (EOA)
	Taker         string `json:"taker"`         // Operator address (0x0000... for public)
	TokenID       string `json:"tokenId"`       // ERC1155 token ID
	MakerAmount   string `json:"makerAmount"`   // Raw amount (6 decimals for USDC)
	TakerAmount   string `json:"takerAmount"`   // Raw token amount
	Side          string `json:"side"`          // "BUY" or "SELL"
	Expiration    string `json:"expiration"`    // Unix timestamp (0 for no expiry)
	Nonce         string `json:"nonce"`         // Nonce value
	FeeRateBps    string `json:"feeRateBps"`    // Fee rate in basis points
	SignatureType int    `json:"signatureType"` // Integer: 0=EOA, 1=POLY_PROXY, 2=GNOSIS_SAFE
	Signature     string `json:"signature"`     // Hex-encoded signature with 0x prefix
}

// OrderSubmissionRequest represents a single order submission wrapped with metadata.
type OrderSubmissionRequest struct {
	Order     SignedOrderJSON `json:"order"`     // Signed order data
	Owner     string          `json:"owner"`     // API key (not maker address!)
	OrderType string          `json:"orderType"` // GTC, FOK, GTD, or FAK
}

// BatchOrderRequest represents a batch order submission to POST /orders.
// Maximum 15 orders per batch per Polymarket API limits.
type BatchOrderRequest []OrderSubmissionRequest

// BatchOrderResponse represents the response from POST /orders.
// Contains one OrderSubmissionResponse per submitted order.
type BatchOrderResponse []OrderSubmissionResponse

// OrderQueryResponse represents the response from GET /order.
// This is DIFFERENT from OrderSubmissionResponse (POST /order).
// Contains additional market and execution details not available in submission response.
type OrderQueryResponse struct {
	OrderID      string  `json:"orderID"`                  // Capital D in GET endpoint
	Status       string  `json:"status"`                   // Order status
	TokenID      string  `json:"asset_id"`                 // Token identifier
	Price        float64 `json:"price,string"`             // Order price (string to float64)
	Size         float64 `json:"original_size,string"`     // Original order size
	SizeFilled   float64 `json:"size_matched,string"`      // Filled size
	Side         string  `json:"side"`                     // "BUY" or "SELL"
	CreatedAt    string  `json:"created_at"`               // Creation timestamp
	UpdatedAt    string  `json:"updated_at"`               // Last update timestamp
	OrderType    string  `json:"type"`                     // GTC, FOK, GTD, FAK
	MarketID     string  `json:"market"`                   // Market slug
	Outcome      string  `json:"outcome"`                  // "Yes" or "No"
	Owner        string  `json:"owner"`                    // API key owner
	MakerAddress string  `json:"maker_address"`            // Maker wallet address
	Message      string  `json:"message,omitempty"`        // Optional message
	Error        string  `json:"error,omitempty"`          // Optional error
}

// OutcomeOrderParams holds parameters for a single outcome order.
// Used by OrderPlacer interface for multi-outcome arbitrage trades.
type OutcomeOrderParams struct {
	TokenID  string
	Price    float64
	TickSize float64
	MinSize  float64
}
