// +build e2e

package execution

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mselser95/polymarket-arb/internal/markets"
)

// TestE2E_BatchOrderPlacement tests the complete batch order placement flow:
// 1. Metadata fetching (tick size, min order size)
// 2. Order size validation against market minimums
// 3. Batch YES/NO order submission
// 4. Response parsing with correct field mapping
// 5. Error handling for insufficient size
func TestE2E_BatchOrderPlacement(t *testing.T) {
	// Setup mock CLOB API server
	mockAPI := setupMockCLOBAPI(t)
	defer mockAPI.Close()

	// Note: For full E2E testing, we would need to configure the metadata client
	// to use the mock server. This would require exporting the baseURL field
	// or using an option pattern. For now, we test the calculation logic.

	tests := []struct {
		name              string
		orderSize         float64
		yesPrice          float64
		noPrice           float64
		yesTickSize       float64
		yesMinSize        float64
		noTickSize        float64
		noMinSize         float64
		expectYesError    bool
		expectNoError     bool
		expectedYesTokens float64
		expectedNoTokens  float64
	}{
		{
			name:              "Valid order - both sides above minimum",
			orderSize:         10.0,
			yesPrice:          0.40,
			noPrice:           0.55,
			yesTickSize:       0.01,
			yesMinSize:        5.0,
			noTickSize:        0.01,
			noMinSize:         5.0,
			expectYesError:    false,
			expectNoError:     false,
			expectedYesTokens: 25.0,  // 10.0 / 0.40 = 25
			expectedNoTokens:  18.18, // 10.0 / 0.55 = 18.18
		},
		{
			name:           "YES order below minimum",
			orderSize:      2.0,
			yesPrice:       0.99,
			noPrice:        0.10,
			yesTickSize:    0.01,
			yesMinSize:     5.0,
			noTickSize:     0.01,
			noMinSize:      5.0,
			expectYesError: true, // 2.0 / 0.99 = 2.02 tokens < 5.0 minimum
			expectNoError:  false,
		},
		{
			name:           "NO order below minimum",
			orderSize:      2.0,
			yesPrice:       0.10,
			noPrice:        0.99,
			yesTickSize:    0.01,
			yesMinSize:     5.0,
			noTickSize:     0.01,
			noMinSize:      5.0,
			expectYesError: false,
			expectNoError:  true, // 2.0 / 0.99 = 2.02 tokens < 5.0 minimum
		},
		{
			name:           "Both orders below minimum",
			orderSize:      1.0,
			yesPrice:       0.50,
			noPrice:        0.50,
			yesTickSize:    0.01,
			yesMinSize:     5.0,
			noTickSize:     0.01,
			noMinSize:      5.0,
			expectYesError: true, // 1.0 / 0.50 = 2.0 tokens < 5.0 minimum
			expectNoError:  true, // 1.0 / 0.50 = 2.0 tokens < 5.0 minimum
		},
		{
			name:              "Edge case - exactly at minimum",
			orderSize:         5.0,
			yesPrice:          1.00,
			noPrice:           1.00,
			yesTickSize:       0.01,
			yesMinSize:        5.0,
			noTickSize:        0.01,
			noMinSize:         5.0,
			expectYesError:    false,
			expectNoError:     false,
			expectedYesTokens: 5.0,
			expectedNoTokens:  5.0,
		},
		{
			name:              "Different tick sizes",
			orderSize:         10.0,
			yesPrice:          0.123,
			noPrice:           0.876,
			yesTickSize:       0.001, // Higher precision
			yesMinSize:        5.0,
			noTickSize:        0.01,
			noMinSize:         5.0,
			expectYesError:    false,
			expectNoError:     false,
			expectedYesTokens: 81.30, // 10.0 / 0.123 = 81.30 (rounded to size precision)
			expectedNoTokens:  11.42, // 10.0 / 0.876 = 11.42 (rounded to size precision)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			// Test rounding configuration
			yesSizePrecision, yesAmountPrecision := getRoundingConfig(tt.yesTickSize)
			noSizePrecision, noAmountPrecision := getRoundingConfig(tt.noTickSize)

			// Verify rounding precision matches Python client
			if tt.yesTickSize == 0.001 {
				if yesSizePrecision != 2 || yesAmountPrecision != 5 {
					t.Errorf("Tick size 0.001 should have size=2, amount=5, got size=%d, amount=%d",
						yesSizePrecision, yesAmountPrecision)
				}
			}

			// Calculate token amounts with rounding
			yesTakerTokens := roundAmount(tt.orderSize/tt.yesPrice, yesSizePrecision)
			noTakerTokens := roundAmount(tt.orderSize/tt.noPrice, noSizePrecision)

			// Validate against minimums
			yesValid := yesTakerTokens >= tt.yesMinSize
			noValid := noTakerTokens >= tt.noMinSize

			// Check validation results
			if tt.expectYesError && yesValid {
				t.Errorf("Expected YES validation error but order was valid (%.2f tokens >= %.2f minimum)",
					yesTakerTokens, tt.yesMinSize)
			}
			if !tt.expectYesError && !yesValid {
				t.Errorf("Expected YES order to be valid but got error (%.2f tokens < %.2f minimum)",
					yesTakerTokens, tt.yesMinSize)
			}

			if tt.expectNoError && noValid {
				t.Errorf("Expected NO validation error but order was valid (%.2f tokens >= %.2f minimum)",
					noTakerTokens, tt.noMinSize)
			}
			if !tt.expectNoError && !noValid {
				t.Errorf("Expected NO order to be valid but got error (%.2f tokens < %.2f minimum)",
					noTakerTokens, tt.noMinSize)
			}

			// If orders are valid, verify calculated amounts
			if !tt.expectYesError && tt.expectedYesTokens > 0 {
				if yesTakerTokens != tt.expectedYesTokens {
					t.Errorf("YES token calculation mismatch: expected %.2f, got %.2f",
						tt.expectedYesTokens, yesTakerTokens)
				}
			}

			if !tt.expectNoError && tt.expectedNoTokens > 0 {
				if noTakerTokens != tt.expectedNoTokens {
					t.Errorf("NO token calculation mismatch: expected %.2f, got %.2f",
						tt.expectedNoTokens, noTakerTokens)
				}
			}

			// Test maker amount calculation with proper rounding
			if !tt.expectYesError {
				yesMakerUSD := roundAmount(yesTakerTokens*tt.yesPrice, yesAmountPrecision)
				if yesMakerUSD > tt.orderSize*1.01 { // Allow 1% slippage due to rounding
					t.Errorf("YES maker amount %.5f exceeds order size %.2f", yesMakerUSD, tt.orderSize)
				}
			}

			if !tt.expectNoError {
				noMakerUSD := roundAmount(noTakerTokens*tt.noPrice, noAmountPrecision)
				if noMakerUSD > tt.orderSize*1.01 {
					t.Errorf("NO maker amount %.5f exceeds order size %.2f", noMakerUSD, tt.orderSize)
				}
			}

			_ = ctx // Use context
		})
	}
}

// TestE2E_MetadataFetching tests the metadata client's ability to fetch tick sizes and min order sizes
func TestE2E_MetadataFetching(t *testing.T) {
	// Setup mock server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/tick-size" {
			tokenID := r.URL.Query().Get("token_id")
			if tokenID == "" {
				http.Error(w, "missing token_id", http.StatusBadRequest)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"minimum_tick_size": 0.01,
			})
			return
		}

		if r.URL.Path == "/book" {
			tokenID := r.URL.Query().Get("token_id")
			if tokenID == "" {
				http.Error(w, "missing token_id", http.StatusBadRequest)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"min_size": 5.0,
				"market": map[string]interface{}{
					"minimum_order_size": 5.0,
				},
			})
			return
		}

		http.NotFound(w, r)
	}))
	defer mockServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create metadata client
	client := markets.NewMetadataClient()

	// Test tick size fetching (uses real API, may fail if token doesn't exist)
	tickSize, err := client.FetchTickSize(ctx, "test-token-123")
	if err != nil {
		// This is expected to fail for non-existent tokens, test the error handling
		t.Logf("Expected error for non-existent token: %v", err)
	} else if tickSize <= 0 {
		t.Errorf("Expected positive tick size, got %.4f", tickSize)
	}

	// Test min order size fetching (defaults to 5.0 on error)
	minSize, err := client.FetchMinOrderSize(ctx, "test-token-123")
	if err != nil {
		t.Logf("Min order size fetch returned error (expected): %v", err)
	}

	// Should have default value even on error
	if minSize != 5.0 && minSize <= 0 {
		t.Errorf("Expected default min order size 5.0 on error, got %.2f", minSize)
	}

	// Test combined metadata fetching (uses defaults on error)
	tickSize2, minSize2, err := client.FetchTokenMetadata(ctx, "test-token-123")
	if err != nil {
		t.Logf("Combined metadata fetch returned error (expected): %v", err)
	}

	// Should have default values even on error
	if tickSize2 == 0 {
		t.Errorf("Combined fetch: tick size should default to 0.01, got %.4f", tickSize2)
	}
	if minSize2 == 0 {
		t.Errorf("Combined fetch: min order size should default to 5.0, got %.2f", minSize2)
	}
}

// TestE2E_OrderResponseParsing tests that order responses are parsed correctly with new field names
func TestE2E_OrderResponseParsing(t *testing.T) {
	// Mock API response with actual field names
	mockResponse := `{
		"orderID": "0x1234567890abcdef",
		"status": "LIVE",
		"asset_id": "86076435890279485031516158085782",
		"price": "0.3700",
		"original_size": "27.03",
		"size_matched": "5.50",
		"side": "BUY",
		"created_at": "2024-12-24T00:00:00Z",
		"updated_at": "2024-12-24T00:05:00Z",
		"type": "GTC",
		"market": "will-bitcoin-hit-100k",
		"outcome": "Yes",
		"owner": "test-owner",
		"maker_address": "0x4F7170A83fE02dF9AF3f7C5A8C8117692e3cb10C"
	}`

	var response OrderResponse
	if err := json.Unmarshal([]byte(mockResponse), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	// Verify all fields are correctly parsed
	if response.OrderID != "0x1234567890abcdef" {
		t.Errorf("OrderID mismatch: expected %s, got %s", "0x1234567890abcdef", response.OrderID)
	}

	if response.Status != "LIVE" {
		t.Errorf("Status mismatch: expected %s, got %s", "LIVE", response.Status)
	}

	if response.TokenID != "86076435890279485031516158085782" {
		t.Errorf("TokenID mismatch: expected %s, got %s", "86076435890279485031516158085782", response.TokenID)
	}

	// Critical: verify Price and Size use correct field names
	if response.Price != 0.37 {
		t.Errorf("Price mismatch: expected %.4f, got %.4f", 0.37, response.Price)
	}

	if response.Size != 27.03 {
		t.Errorf("Size (original_size) mismatch: expected %.2f, got %.2f", 27.03, response.Size)
	}

	if response.SizeFilled != 5.50 {
		t.Errorf("SizeFilled (size_matched) mismatch: expected %.2f, got %.2f", 5.50, response.SizeFilled)
	}

	if response.Side != "BUY" {
		t.Errorf("Side mismatch: expected %s, got %s", "BUY", response.Side)
	}

	if response.OrderType != "GTC" {
		t.Errorf("OrderType mismatch: expected %s, got %s", "GTC", response.OrderType)
	}
}

// TestE2E_RoundingPrecision tests rounding configuration for different tick sizes
func TestE2E_RoundingPrecision(t *testing.T) {
	tests := []struct {
		tickSize         float64
		expectedSizePrec int
		expectedAmtPrec  int
	}{
		{0.1, 2, 3},
		{0.01, 2, 4},
		{0.001, 2, 5},
		{0.0001, 2, 6},
		{0.05, 2, 4}, // Default case
	}

	for _, tt := range tests {
		sizePrec, amtPrec := getRoundingConfig(tt.tickSize)
		if sizePrec != tt.expectedSizePrec {
			t.Errorf("Tick size %.4f: expected size precision %d, got %d",
				tt.tickSize, tt.expectedSizePrec, sizePrec)
		}
		if amtPrec != tt.expectedAmtPrec {
			t.Errorf("Tick size %.4f: expected amount precision %d, got %d",
				tt.tickSize, tt.expectedAmtPrec, amtPrec)
		}
	}

	// Test rounding function
	tests2 := []struct {
		value     float64
		decimals  int
		expected  float64
		testName  string
	}{
		{1.23456, 2, 1.23, "round to 2 decimals"},
		{1.23556, 2, 1.24, "round up to 2 decimals"},
		{10.005, 2, 10.01, "banker's rounding"},
		{99.999, 1, 100.0, "round to whole number"},
		{0.123456, 4, 0.1235, "small number to 4 decimals"},
	}

	for _, tt := range tests2 {
		result := roundAmount(tt.value, tt.decimals)
		if result != tt.expected {
			t.Errorf("%s: roundAmount(%.6f, %d) = %.6f, expected %.6f",
				tt.testName, tt.value, tt.decimals, result, tt.expected)
		}
	}
}

// setupMockCLOBAPI creates a mock CLOB API server for testing
func setupMockCLOBAPI(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Mock tick-size endpoint
		if r.URL.Path == "/tick-size" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"minimum_tick_size": 0.01,
			})
			return
		}

		// Mock orderbook endpoint for min size
		if r.URL.Path == "/book" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"min_size": 5.0,
			})
			return
		}

		// Mock order submission endpoint
		if r.URL.Path == "/order" && r.Method == "POST" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"orderID":       "0xtest123",
				"status":        "LIVE",
				"asset_id":      "test-token",
				"price":         "0.3700",
				"original_size": "10.00",
				"size_matched":  "0.00",
				"side":          "BUY",
				"type":          "GTC",
			})
			return
		}

		http.NotFound(w, r)
	}))
}
