package discovery

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mselser95/polymarket-arb/pkg/cache"
	"github.com/mselser95/polymarket-arb/pkg/types"
	"go.uber.org/zap"
)

func TestNew(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cacheInterface, err := cache.NewRistrettoCache(&cache.RistrettoConfig{
		NumCounters: 1000,
		MaxCost:     100,
		BufferItems: 64,
		Logger:      logger,
	})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cacheInterface.Close()

	client := NewClient("https://api.example.com", logger)

	cfg := &Config{
		Client:       client,
		Cache:        cacheInterface,
		PollInterval: 30 * time.Second,
		MarketLimit:  50,
		Logger:       logger,
		SingleMarket: "",
	}

	svc := New(cfg)

	if svc == nil {
		t.Fatal("expected non-nil service")
	}

	if svc.client != client {
		t.Error("expected client to match")
	}

	if svc.cache != cacheInterface {
		t.Error("expected cache to match")
	}

	if svc.pollInterval != cfg.PollInterval {
		t.Errorf("expected poll interval %v, got %v", cfg.PollInterval, svc.pollInterval)
	}

	if svc.marketLimit != cfg.MarketLimit {
		t.Errorf("expected market limit %d, got %d", cfg.MarketLimit, svc.marketLimit)
	}

	if svc.subscribed == nil {
		t.Error("expected non-nil subscribed map")
	}

	if svc.newMarketsCh == nil {
		t.Error("expected non-nil new markets channel")
	}

	if cap(svc.newMarketsCh) != 100 {
		t.Errorf("expected channel capacity 100, got %d", cap(svc.newMarketsCh))
	}
}

func TestService_IdentifyNewMarkets(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	svc := &Service{
		logger:     logger,
		subscribed: make(map[string]*types.MarketSubscription),
	}

	markets := []types.Market{
		{
			ID:       "market1",
			Slug:     "market-1",
			Question: "Will X happen?",
			Tokens: []types.Token{
				{TokenID: "token1", Outcome: "YES"},
				{TokenID: "token2", Outcome: "NO"},
			},
		},
		{
			ID:       "market2",
			Slug:     "market-2",
			Question: "Will Y happen?",
			Tokens: []types.Token{
				{TokenID: "token3", Outcome: "YES"},
				{TokenID: "token4", Outcome: "NO"},
			},
		},
		{
			ID:       "market3",
			Slug:     "market-3",
			Question: "Invalid market (no NO token)",
			Tokens: []types.Token{
				{TokenID: "token5", Outcome: "YES"},
			},
		},
	}

	newMarkets := svc.identifyNewMarkets(markets)

	// Should identify 2 valid markets (market3 is invalid)
	if len(newMarkets) != 2 {
		t.Errorf("expected 2 new markets, got %d", len(newMarkets))
	}

	// Verify markets are tracked
	svc.mu.RLock()
	if len(svc.subscribed) != 2 {
		t.Errorf("expected 2 subscribed markets, got %d", len(svc.subscribed))
	}

	if _, exists := svc.subscribed["market-1"]; !exists {
		t.Error("expected market-1 to be subscribed")
	}

	if _, exists := svc.subscribed["market-2"]; !exists {
		t.Error("expected market-2 to be subscribed")
	}

	if _, exists := svc.subscribed["market-3"]; exists {
		t.Error("expected market-3 to not be subscribed (invalid)")
	}
	svc.mu.RUnlock()
}

func TestService_IdentifyNewMarkets_Duplicates(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	svc := &Service{
		logger:     logger,
		subscribed: make(map[string]*types.MarketSubscription),
	}

	// Pre-subscribe to market-1
	svc.subscribed["market-1"] = &types.MarketSubscription{
		MarketID:   "market1",
		MarketSlug: "market-1",
	}

	markets := []types.Market{
		{
			ID:       "market1",
			Slug:     "market-1",
			Question: "Will X happen?",
			Tokens: []types.Token{
				{TokenID: "token1", Outcome: "YES"},
				{TokenID: "token2", Outcome: "NO"},
			},
		},
		{
			ID:       "market2",
			Slug:     "market-2",
			Question: "Will Y happen?",
			Tokens: []types.Token{
				{TokenID: "token3", Outcome: "YES"},
				{TokenID: "token4", Outcome: "NO"},
			},
		},
	}

	newMarkets := svc.identifyNewMarkets(markets)

	// Should only identify market-2 (market-1 is already subscribed)
	if len(newMarkets) != 1 {
		t.Errorf("expected 1 new market, got %d", len(newMarkets))
	}

	if newMarkets[0].Slug != "market-2" {
		t.Errorf("expected market-2, got %s", newMarkets[0].Slug)
	}
}

func TestService_GetSubscribedMarkets(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	svc := &Service{
		logger:     logger,
		subscribed: make(map[string]*types.MarketSubscription),
	}

	// Add subscriptions
	svc.subscribed["market-1"] = &types.MarketSubscription{
		MarketID:   "market1",
		MarketSlug: "market-1",
		Question:   "Will X happen?",
	}
	svc.subscribed["market-2"] = &types.MarketSubscription{
		MarketID:   "market2",
		MarketSlug: "market-2",
		Question:   "Will Y happen?",
	}

	subs := svc.GetSubscribedMarkets()

	if len(subs) != 2 {
		t.Errorf("expected 2 subscribed markets, got %d", len(subs))
	}

	// Verify subscriptions contain expected data
	found := 0
	for _, sub := range subs {
		if sub.MarketSlug == "market-1" && sub.Question == "Will X happen?" {
			found++
		}
		if sub.MarketSlug == "market-2" && sub.Question == "Will Y happen?" {
			found++
		}
	}

	if found != 2 {
		t.Errorf("expected to find 2 matching subscriptions, found %d", found)
	}
}

func TestService_CacheMarket(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cacheInterface, err := cache.NewRistrettoCache(&cache.RistrettoConfig{
		NumCounters: 1000,
		MaxCost:     100,
		BufferItems: 64,
		Logger:      logger,
	})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cacheInterface.Close()

	// Cast for Wait() method
	ristrettoCache := cacheInterface.(*cache.RistrettoCache)

	svc := &Service{
		logger: logger,
		cache:  cacheInterface,
	}

	market := &types.Market{
		ID:       "market1",
		Slug:     "market-1",
		Question: "Will X happen?",
	}

	svc.cacheMarket(market)
	ristrettoCache.Wait()

	// Retrieve from cache
	retrieved := svc.GetMarket("market1")
	if retrieved == nil {
		t.Fatal("expected market to be cached")
	}

	if retrieved.ID != market.ID {
		t.Errorf("expected market ID %s, got %s", market.ID, retrieved.ID)
	}

	if retrieved.Question != market.Question {
		t.Errorf("expected question %s, got %s", market.Question, retrieved.Question)
	}
}

func TestService_GetMarket_NotFound(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cacheInterface, err := cache.NewRistrettoCache(&cache.RistrettoConfig{
		NumCounters: 1000,
		MaxCost:     100,
		BufferItems: 64,
		Logger:      logger,
	})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cacheInterface.Close()

	svc := &Service{
		logger: logger,
		cache:  cacheInterface,
	}

	retrieved := svc.GetMarket("nonexistent")
	if retrieved != nil {
		t.Error("expected nil for nonexistent market")
	}
}

func TestService_GetMarket_NilCache(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	svc := &Service{
		logger: logger,
		cache:  nil,
	}

	retrieved := svc.GetMarket("market1")
	if retrieved != nil {
		t.Error("expected nil when cache is nil")
	}
}

func TestService_NewMarketsChan(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	svc := &Service{
		logger:       logger,
		newMarketsCh: make(chan *types.Market, 100),
	}

	ch := svc.NewMarketsChan()
	if ch == nil {
		t.Fatal("expected non-nil channel")
	}

	// Verify it's the same channel
	if ch != svc.newMarketsCh {
		t.Error("NewMarketsChan() returned different channel")
	}
}

func TestClient_FetchActiveMarkets(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request parameters
		if r.URL.Query().Get("closed") != "false" {
			t.Error("expected closed=false")
		}
		if r.URL.Query().Get("active") != "true" {
			t.Error("expected active=true")
		}
		if r.URL.Query().Get("limit") != "10" {
			t.Error("expected limit=10")
		}

		// Return mock response (Gamma API returns direct array, not wrapped)
		markets := []types.Market{
			{
				ID:         "market1",
				Slug:       "market-1",
				Question:   "Will X happen?",
				Outcomes:   `["Yes", "No"]`,
				ClobTokens: `["token1", "token2"]`,
			},
			{
				ID:         "market2",
				Slug:       "market-2",
				Question:   "Will Y happen?",
				Outcomes:   `["Yes", "No"]`,
				ClobTokens: `["token3", "token4"]`,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(markets)
	}))
	defer server.Close()

	client := NewClient(server.URL, logger)
	ctx := context.Background()

	resp, err := client.FetchActiveMarkets(ctx, 10, 0, "volume24hr")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if resp.Count != 2 {
		t.Errorf("expected count 2, got %d", resp.Count)
	}

	if len(resp.Data) != 2 {
		t.Errorf("expected 2 markets, got %d", len(resp.Data))
	}

	if resp.Data[0].ID != "market1" {
		t.Errorf("expected market1, got %s", resp.Data[0].ID)
	}
}

func TestClient_FetchActiveMarkets_Error(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// Create mock server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	defer server.Close()

	client := NewClient(server.URL, logger)
	ctx := context.Background()

	_, err := client.FetchActiveMarkets(ctx, 10, 0, "volume24hr")
	if err == nil {
		t.Error("expected error for 500 status")
	}
}

func TestClient_FetchMarketBySlug(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// Create mock server - FetchMarketBySlug now uses FetchActiveMarkets
	// which expects an array response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify endpoint
		if r.URL.Path != "/markets" {
			t.Errorf("expected path /markets, got %s", r.URL.Path)
		}

		// Return mock response as array
		markets := []types.Market{
			{
				ID:       "market1",
				Slug:     "test-market",
				Question: "Will X happen?",
				Tokens: []types.Token{
					{TokenID: "token1", Outcome: "YES"},
					{TokenID: "token2", Outcome: "NO"},
				},
			},
			{
				ID:       "market2",
				Slug:     "other-market",
				Question: "Will Y happen?",
				Tokens: []types.Token{
					{TokenID: "token3", Outcome: "YES"},
					{TokenID: "token4", Outcome: "NO"},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(markets)
	}))
	defer server.Close()

	client := NewClient(server.URL, logger)
	ctx := context.Background()

	market, err := client.FetchMarketBySlug(ctx, "test-market")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if market.ID != "market1" {
		t.Errorf("expected market1, got %s", market.ID)
	}

	if market.Slug != "test-market" {
		t.Errorf("expected test-market, got %s", market.Slug)
	}
}

func TestClient_FetchMarketBySlug_NotFound(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// Create mock server that returns 404
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("market not found"))
	}))
	defer server.Close()

	client := NewClient(server.URL, logger)
	ctx := context.Background()

	_, err := client.FetchMarketBySlug(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for 404 status")
	}
}

func TestService_Poll_Integration(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// Create mock server (Gamma API returns direct array, not wrapped)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		markets := []types.Market{
			{
				ID:         "market1",
				Slug:       "market-1",
				Question:   "Will X happen?",
				Outcomes:   `["Yes", "No"]`,
				ClobTokens: `["token1", "token2"]`,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(markets)
	}))
	defer server.Close()

	client := NewClient(server.URL, logger)
	cacheInterface, err := cache.NewRistrettoCache(&cache.RistrettoConfig{
		NumCounters: 1000,
		MaxCost:     100,
		BufferItems: 64,
		Logger:      logger,
	})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cacheInterface.Close()

	svc := New(&Config{
		Client:       client,
		Cache:        cacheInterface,
		PollInterval: 30 * time.Second,
		MarketLimit:  10,
		Logger:       logger,
	})

	ctx := context.Background()
	err = svc.poll(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Check that market was sent to channel
	select {
	case market := <-svc.newMarketsCh:
		if market.ID != "market1" {
			t.Errorf("expected market1, got %s", market.ID)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for market")
	}

	// Verify market is subscribed
	subs := svc.GetSubscribedMarkets()
	if len(subs) != 1 {
		t.Errorf("expected 1 subscribed market, got %d", len(subs))
	}
}

func TestService_PollSingleMarket(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// Create mock server - FetchMarketBySlug now uses FetchActiveMarkets
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/markets" {
			t.Errorf("expected path /markets, got %s", r.URL.Path)
		}

		// Return array of markets
		markets := []types.Market{
			{
				ID:         "market1",
				Slug:       "single-market",
				Question:   "Will X happen?",
				Outcomes:   `["Yes", "No"]`,
				ClobTokens: `["token1", "token2"]`,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(markets)
	}))
	defer server.Close()

	client := NewClient(server.URL, logger)
	cacheInterface, err := cache.NewRistrettoCache(&cache.RistrettoConfig{
		NumCounters: 1000,
		MaxCost:     100,
		BufferItems: 64,
		Logger:      logger,
	})
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}
	defer cacheInterface.Close()

	svc := New(&Config{
		Client:       client,
		Cache:        cacheInterface,
		PollInterval: 30 * time.Second,
		MarketLimit:  10,
		Logger:       logger,
		SingleMarket: "single-market",
	})

	ctx := context.Background()
	err = svc.pollSingleMarket(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Check that market was sent to channel
	select {
	case market := <-svc.newMarketsCh:
		if market.Slug != "single-market" {
			t.Errorf("expected single-market, got %s", market.Slug)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for market")
	}

	// Second poll should do nothing (already subscribed)
	err = svc.pollSingleMarket(ctx)
	if err != nil {
		t.Fatalf("expected no error on second poll, got %v", err)
	}

	// Verify only one subscription
	subs := svc.GetSubscribedMarkets()
	if len(subs) != 1 {
		t.Errorf("expected 1 subscribed market, got %d", len(subs))
	}
}
