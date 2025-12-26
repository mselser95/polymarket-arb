package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/mselser95/polymarket-arb/pkg/types"
	"go.uber.org/zap"
)

// TestClient_Pagination_SmallLimit tests that small limits (<= MaxBatchSize) use a single request.
func TestClient_Pagination_SmallLimit(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	requestCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		limit := r.URL.Query().Get("limit")
		offset := r.URL.Query().Get("offset")

		// Should be a single request
		if requestCount > 1 {
			t.Errorf("expected single request for small limit, got %d requests", requestCount)
		}

		// Should request exact limit
		if limit != "50" {
			t.Errorf("expected limit=50, got %s", limit)
		}

		if offset != "0" {
			t.Errorf("expected offset=0, got %s", offset)
		}

		// Return 50 markets
		markets := make([]types.Market, 50)
		for i := 0; i < 50; i++ {
			markets[i] = types.Market{
				ID:       fmt.Sprintf("market%d", i+1),
				Slug:     fmt.Sprintf("market-%d", i+1),
				Question: fmt.Sprintf("Question %d", i+1),
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(markets)
	}))
	defer server.Close()

	client := NewClient(server.URL, logger)
	ctx := context.Background()

	// Request 50 markets (< MaxBatchSize of 100)
	resp, err := client.FetchActiveMarkets(ctx, 50, 0, "volume24hr")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Count != 50 {
		t.Errorf("expected count=50, got %d", resp.Count)
	}

	if requestCount != 1 {
		t.Errorf("expected 1 request, got %d", requestCount)
	}
}

// TestClient_Pagination_LargeLimit tests that large limits (> MaxBatchSize) use pagination.
func TestClient_Pagination_LargeLimit(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	requestCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		limitStr := r.URL.Query().Get("limit")
		offsetStr := r.URL.Query().Get("offset")

		limit, _ := strconv.Atoi(limitStr)
		offset, _ := strconv.Atoi(offsetStr)

		// Expected requests:
		// Request 1: limit=100, offset=0
		// Request 2: limit=100, offset=100
		// Request 3: limit=50, offset=200

		var expectedLimit, expectedOffset int
		switch requestCount {
		case 1:
			expectedLimit = 100
			expectedOffset = 0
		case 2:
			expectedLimit = 100
			expectedOffset = 100
		case 3:
			expectedLimit = 50
			expectedOffset = 200
		default:
			t.Errorf("unexpected request %d", requestCount)
		}

		if limit != expectedLimit {
			t.Errorf("request %d: expected limit=%d, got %d", requestCount, expectedLimit, limit)
		}

		if offset != expectedOffset {
			t.Errorf("request %d: expected offset=%d, got %d", requestCount, expectedOffset, offset)
		}

		// Return appropriate number of markets
		var numMarkets int
		switch requestCount {
		case 1, 2:
			numMarkets = 100 // Full batch
		case 3:
			numMarkets = 50 // Partial last batch
		}

		markets := make([]types.Market, numMarkets)
		for i := 0; i < numMarkets; i++ {
			marketNum := offset + i + 1
			markets[i] = types.Market{
				ID:       fmt.Sprintf("market%d", marketNum),
				Slug:     fmt.Sprintf("market-%d", marketNum),
				Question: fmt.Sprintf("Question %d", marketNum),
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(markets)
	}))
	defer server.Close()

	client := NewClient(server.URL, logger)
	ctx := context.Background()

	// Request 250 markets (> MaxBatchSize of 100)
	resp, err := client.FetchActiveMarkets(ctx, 250, 0, "volume24hr")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Count != 250 {
		t.Errorf("expected count=250, got %d", resp.Count)
	}

	if requestCount != 3 {
		t.Errorf("expected 3 requests, got %d", requestCount)
	}

	// Verify we got unique markets in order
	if resp.Data[0].ID != "market1" {
		t.Errorf("expected first market to be market1, got %s", resp.Data[0].ID)
	}

	if resp.Data[249].ID != "market250" {
		t.Errorf("expected last market to be market250, got %s", resp.Data[249].ID)
	}
}

// TestClient_Pagination_FetchAll tests that limit=0 fetches all available markets.
func TestClient_Pagination_FetchAll(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	requestCount := 0
	totalMarkets := 350 // Total markets available

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		limitStr := r.URL.Query().Get("limit")
		offsetStr := r.URL.Query().Get("offset")

		limit, _ := strconv.Atoi(limitStr)
		offset, _ := strconv.Atoi(offsetStr)

		// All requests should use MaxBatchSize
		if limit != MaxBatchSize {
			t.Errorf("request %d: expected limit=%d, got %d", requestCount, MaxBatchSize, limit)
		}

		// Calculate how many markets to return
		remaining := totalMarkets - offset
		var numMarkets int
		if remaining >= MaxBatchSize {
			numMarkets = MaxBatchSize
		} else {
			numMarkets = remaining
		}

		markets := make([]types.Market, numMarkets)
		for i := 0; i < numMarkets; i++ {
			marketNum := offset + i + 1
			markets[i] = types.Market{
				ID:       fmt.Sprintf("market%d", marketNum),
				Slug:     fmt.Sprintf("market-%d", marketNum),
				Question: fmt.Sprintf("Question %d", marketNum),
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(markets)
	}))
	defer server.Close()

	client := NewClient(server.URL, logger)
	ctx := context.Background()

	// Request all markets (limit=0)
	resp, err := client.FetchActiveMarkets(ctx, 0, 0, "volume24hr")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Count != totalMarkets {
		t.Errorf("expected count=%d, got %d", totalMarkets, resp.Count)
	}

	// Should make 4 requests: 100, 100, 100, 50
	expectedRequests := 4
	if requestCount != expectedRequests {
		t.Errorf("expected %d requests, got %d", expectedRequests, requestCount)
	}

	// Verify all markets are present
	if len(resp.Data) != totalMarkets {
		t.Errorf("expected %d markets, got %d", totalMarkets, len(resp.Data))
	}
}

// TestClient_Pagination_WithOffset tests pagination with non-zero offset.
func TestClient_Pagination_WithOffset(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	requestCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		limitStr := r.URL.Query().Get("limit")
		offsetStr := r.URL.Query().Get("offset")

		limit, _ := strconv.Atoi(limitStr)
		offset, _ := strconv.Atoi(offsetStr)

		// Expected requests:
		// Request 1: limit=100, offset=50
		// Request 2: limit=50, offset=150

		var expectedLimit, expectedOffset int
		switch requestCount {
		case 1:
			expectedLimit = 100
			expectedOffset = 50
		case 2:
			expectedLimit = 50
			expectedOffset = 150
		default:
			t.Errorf("unexpected request %d", requestCount)
		}

		if limit != expectedLimit {
			t.Errorf("request %d: expected limit=%d, got %d", requestCount, expectedLimit, limit)
		}

		if offset != expectedOffset {
			t.Errorf("request %d: expected offset=%d, got %d", requestCount, expectedOffset, offset)
		}

		// Return markets
		var numMarkets int
		switch requestCount {
		case 1:
			numMarkets = 100
		case 2:
			numMarkets = 50
		}

		markets := make([]types.Market, numMarkets)
		for i := 0; i < numMarkets; i++ {
			marketNum := offset + i + 1
			markets[i] = types.Market{
				ID:       fmt.Sprintf("market%d", marketNum),
				Slug:     fmt.Sprintf("market-%d", marketNum),
				Question: fmt.Sprintf("Question %d", marketNum),
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(markets)
	}))
	defer server.Close()

	client := NewClient(server.URL, logger)
	ctx := context.Background()

	// Request 150 markets starting at offset 50
	resp, err := client.FetchActiveMarkets(ctx, 150, 50, "volume24hr")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Count != 150 {
		t.Errorf("expected count=150, got %d", resp.Count)
	}

	if requestCount != 2 {
		t.Errorf("expected 2 requests, got %d", requestCount)
	}

	// Verify first market is offset+1 (market51)
	if resp.Data[0].ID != "market51" {
		t.Errorf("expected first market to be market51, got %s", resp.Data[0].ID)
	}
}

// TestClient_Pagination_PartialLastPage tests pagination when last page returns fewer results.
func TestClient_Pagination_PartialLastPage(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	requestCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		offsetStr := r.URL.Query().Get("offset")
		offset, _ := strconv.Atoi(offsetStr)

		// First request returns 100 markets
		// Second request returns only 30 markets (no more data)
		var numMarkets int
		switch requestCount {
		case 1:
			numMarkets = 100
		case 2:
			numMarkets = 30
		default:
			t.Errorf("unexpected request %d", requestCount)
		}

		markets := make([]types.Market, numMarkets)
		for i := 0; i < numMarkets; i++ {
			marketNum := offset + i + 1
			markets[i] = types.Market{
				ID:       fmt.Sprintf("market%d", marketNum),
				Slug:     fmt.Sprintf("market-%d", marketNum),
				Question: fmt.Sprintf("Question %d", marketNum),
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(markets)
	}))
	defer server.Close()

	client := NewClient(server.URL, logger)
	ctx := context.Background()

	// Request 200 markets, but API only has 130 total
	resp, err := client.FetchActiveMarkets(ctx, 200, 0, "volume24hr")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should get only 130 markets (stopped when last page returned < MaxBatchSize)
	if resp.Count != 130 {
		t.Errorf("expected count=130, got %d", resp.Count)
	}

	if requestCount != 2 {
		t.Errorf("expected 2 requests, got %d", requestCount)
	}
}

// TestClient_Pagination_Error tests that pagination stops on error.
func TestClient_Pagination_Error(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	requestCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		// First request succeeds
		if requestCount == 1 {
			markets := make([]types.Market, 100)
			for i := 0; i < 100; i++ {
				markets[i] = types.Market{
					ID:       fmt.Sprintf("market%d", i+1),
					Slug:     fmt.Sprintf("market-%d", i+1),
					Question: fmt.Sprintf("Question %d", i+1),
				}
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(markets)
			return
		}

		// Second request fails
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	client := NewClient(server.URL, logger)
	ctx := context.Background()

	// Request 200 markets, but second page will fail
	_, err := client.FetchActiveMarkets(ctx, 200, 0, "volume24hr")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if requestCount != 2 {
		t.Errorf("expected 2 requests before error, got %d", requestCount)
	}
}

// TestClient_Pagination_ExactMultiple tests pagination when limit is exact multiple of MaxBatchSize.
func TestClient_Pagination_ExactMultiple(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	requestCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		limitStr := r.URL.Query().Get("limit")
		offsetStr := r.URL.Query().Get("offset")

		limit, _ := strconv.Atoi(limitStr)
		offset, _ := strconv.Atoi(offsetStr)

		// Should make exactly 3 requests of 100 each
		if limit != 100 {
			t.Errorf("request %d: expected limit=100, got %d", requestCount, limit)
		}

		expectedOffset := (requestCount - 1) * 100
		if offset != expectedOffset {
			t.Errorf("request %d: expected offset=%d, got %d", requestCount, expectedOffset, offset)
		}

		markets := make([]types.Market, 100)
		for i := 0; i < 100; i++ {
			marketNum := offset + i + 1
			markets[i] = types.Market{
				ID:       fmt.Sprintf("market%d", marketNum),
				Slug:     fmt.Sprintf("market-%d", marketNum),
				Question: fmt.Sprintf("Question %d", marketNum),
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(markets)
	}))
	defer server.Close()

	client := NewClient(server.URL, logger)
	ctx := context.Background()

	// Request exactly 300 markets (3 * MaxBatchSize)
	resp, err := client.FetchActiveMarkets(ctx, 300, 0, "volume24hr")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Count != 300 {
		t.Errorf("expected count=300, got %d", resp.Count)
	}

	if requestCount != 3 {
		t.Errorf("expected 3 requests, got %d", requestCount)
	}
}
