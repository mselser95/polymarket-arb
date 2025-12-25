// +build integration

package app

import (
	"context"
	"testing"
	"time"

	"github.com/mselser95/polymarket-arb/internal/arbitrage"
	"github.com/mselser95/polymarket-arb/internal/discovery"
	"github.com/mselser95/polymarket-arb/internal/execution"
	"github.com/mselser95/polymarket-arb/internal/orderbook"
	"github.com/mselser95/polymarket-arb/internal/testutil"
	"github.com/mselser95/polymarket-arb/pkg/cache"
	"github.com/mselser95/polymarket-arb/pkg/types"
	"go.uber.org/zap"
)

// TestE2E_ArbitrageFlow tests the complete arbitrage flow:
// 1. Market discovery
// 2. Orderbook updates via mock channel
// 3. Arbitrage detection
// 4. Trade execution
func TestE2E_ArbitrageFlow(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// Create test market with YES and NO tokens
	market := testutil.CreateTestMarket("market1", "test-slug", "Will X happen?")
	yesToken := market.GetTokenByOutcome("YES")
	noToken := market.GetTokenByOutcome("NO")

	if yesToken == nil || noToken == nil {
		t.Fatal("test market missing YES or NO token")
	}

	// Setup mock Gamma API
	mockAPI := testutil.NewMockGammaAPI([]*types.Market{market})
	defer mockAPI.Close()

	// Setup cache
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

	// Setup discovery service
	discoveryClient := discovery.NewClient(mockAPI.URL, logger)
	discoverySvc := discovery.New(&discovery.Config{
		Client:       discoveryClient,
		Cache:        cacheInterface,
		PollInterval: 1 * time.Second,
		MarketLimit:  10,
		Logger:       logger,
	})

	// Setup channels
	wsMsgChan := make(chan *types.OrderbookMessage, 100)

	// Setup orderbook manager
	obMgr := orderbook.New(&orderbook.Config{
		Logger:         logger,
		MessageChannel: wsMsgChan,
	})

	// Setup mock storage
	storage := testutil.NewMockStorage()

	// Setup arbitrage detector
	detector := arbitrage.New(arbitrage.Config{
		Threshold:    0.995,
		MinTradeSize: 10.0,
		TakerFee:     0.01,
		Logger:       logger,
	}, obMgr, discoverySvc, storage)

	// Setup executor
	executor := execution.New(&execution.Config{
		Mode:               "paper",
		MaxPositionSize:    1000.0,
		Logger:             logger,
		OpportunityChannel: detector.OpportunityChan(),
	})

	// Start context
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Start components
	err = obMgr.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start orderbook manager: %v", err)
	}
	defer obMgr.Close()

	err = detector.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start detector: %v", err)
	}
	defer detector.Close()

	err = executor.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start executor: %v", err)
	}
	defer executor.Close()

	// Start discovery service (which performs initial poll)
	discoverCtx, discoverCancel := context.WithCancel(ctx)
	defer discoverCancel()

	go func() {
		_ = discoverySvc.Run(discoverCtx)
	}()

	// Wait for initial market discovery
	select {
	case <-discoverySvc.NewMarketsChan():
		// Market discovered
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for market discovery")
	}

	// Verify market was discovered
	subs := discoverySvc.GetSubscribedMarkets()
	if len(subs) != 1 {
		t.Fatalf("expected 1 subscribed market, got %d", len(subs))
	}

	// Send orderbook messages that create arbitrage
	// YES bid: 0.48 (buy YES for 0.48)
	// NO bid: 0.51 (buy NO for 0.51)
	// Total: 0.99 (below 0.995 threshold = arbitrage!)

	yesBookMsg := testutil.CreateTestBookMessage(yesToken.TokenID, market.ID)
	yesBookMsg.Bids = []types.PriceLevel{
		{Price: "0.48", Size: "100.0"},
	}
	yesBookMsg.Asks = []types.PriceLevel{
		{Price: "0.50", Size: "100.0"},
	}

	noBookMsg := testutil.CreateTestBookMessage(noToken.TokenID, market.ID)
	noBookMsg.Bids = []types.PriceLevel{
		{Price: "0.51", Size: "100.0"},
	}
	noBookMsg.Asks = []types.PriceLevel{
		{Price: "0.53", Size: "100.0"},
	}

	// Send messages
	wsMsgChan <- yesBookMsg
	wsMsgChan <- noBookMsg

	// Wait for arbitrage detection and execution
	// Note: Opportunities are consumed by the executor, so we check storage instead
	time.Sleep(500 * time.Millisecond)

	// Verify opportunity was stored
	stored := storage.GetOpportunities()
	if len(stored) == 0 {
		t.Fatal("expected at least one stored opportunity")
	}

	// Verify opportunity details
	opp := stored[0]
	if opp.MarketID != market.ID {
		t.Errorf("expected market ID %s, got %s", market.ID, opp.MarketID)
	}

	if opp.PriceSum >= 0.995 {
		t.Errorf("expected price sum below threshold, got %f", opp.PriceSum)
	}

	t.Logf("✓ Arbitrage opportunity detected: market=%s, profit=%d BPS", opp.MarketSlug, opp.ProfitBPS)
}

// TestE2E_MarketDiscoveryFlow tests the market discovery and subscription flow.
func TestE2E_MarketDiscoveryFlow(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// Create multiple test markets
	market1 := testutil.CreateTestMarket("market1", "market-1", "Will A happen?")
	market2 := testutil.CreateTestMarket("market2", "market-2", "Will B happen?")
	market3 := testutil.CreateTestMarket("market3", "market-3", "Will C happen?")

	// Setup mock Gamma API
	mockAPI := testutil.NewMockGammaAPI([]*types.Market{market1, market2})
	defer mockAPI.Close()

	// Setup cache
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

	// Setup discovery service
	discoveryClient := discovery.NewClient(mockAPI.URL, logger)
	discoverySvc := discovery.New(&discovery.Config{
		Client:       discoveryClient,
		Cache:        cacheInterface,
		PollInterval: 500 * time.Millisecond,
		MarketLimit:  10,
		Logger:       logger,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Start discovery service
	go func() {
		_ = discoverySvc.Run(ctx)
	}()

	// Wait for initial market discovery (2 markets)
	marketsDiscovered := 0
	timeout := time.After(3 * time.Second)

discoveryLoop:
	for marketsDiscovered < 2 {
		select {
		case <-discoverySvc.NewMarketsChan():
			marketsDiscovered++
		case <-timeout:
			t.Fatalf("timeout waiting for initial market discovery (got %d/2)", marketsDiscovered)
		case <-ctx.Done():
			break discoveryLoop
		}
	}

	subs := discoverySvc.GetSubscribedMarkets()
	if len(subs) != 2 {
		t.Errorf("expected 2 subscribed markets after first poll, got %d", len(subs))
	}

	t.Logf("✓ Initial discovery: %d markets", marketsDiscovered)

	// Add a third market to the API (simulating new market appearing)
	mockAPI.AddMarket(market3)

	// Wait for next poll to discover the new market (poll interval is 500ms)
	select {
	case market := <-discoverySvc.NewMarketsChan():
		if market.Slug != "market-3" {
			t.Errorf("expected market-3, got %s", market.Slug)
		}
		t.Logf("✓ Differential discovery: %s", market.Slug)
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for differential market")
	}

	subs = discoverySvc.GetSubscribedMarkets()
	if len(subs) != 3 {
		t.Errorf("expected 3 subscribed markets after differential discovery, got %d", len(subs))
	}

	// Verify no duplicate discoveries in the next interval
	select {
	case <-discoverySvc.NewMarketsChan():
		t.Error("unexpected market from channel after all markets discovered")
	case <-time.After(1 * time.Second):
		// Expected - no new markets
		t.Log("✓ No duplicate markets discovered")
	}
}

// TestE2E_OrderbookProcessing tests orderbook message processing flow.
func TestE2E_OrderbookProcessing(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// Setup channels
	wsMsgChan := make(chan *types.OrderbookMessage, 100)

	// Setup orderbook manager
	obMgr := orderbook.New(&orderbook.Config{
		Logger:         logger,
		MessageChannel: wsMsgChan,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start orderbook manager
	err := obMgr.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start orderbook manager: %v", err)
	}
	defer obMgr.Close()

	// Send book message
	bookMsg := testutil.CreateTestBookMessage("token-1", "market-1")
	wsMsgChan <- bookMsg

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Verify snapshot was created
	snapshot, exists := obMgr.GetSnapshot("token-1")
	if !exists {
		t.Fatal("expected orderbook snapshot to exist")
	}

	if snapshot.BestBidPrice != 0.52 {
		t.Errorf("expected best bid 0.52, got %f", snapshot.BestBidPrice)
	}

	t.Log("✓ Book message processed correctly")

	// Send price change message
	priceChangeMsg := testutil.CreateTestPriceChangeMessage("token-1", "market-1")
	priceChangeMsg.Bids = []types.PriceLevel{
		{Price: "0.51", Size: "150.0"},
	}
	wsMsgChan <- priceChangeMsg

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Verify snapshot was updated
	snapshot, exists = obMgr.GetSnapshot("token-1")
	if !exists {
		t.Fatal("expected orderbook snapshot to exist after update")
	}

	if snapshot.BestBidPrice != 0.51 {
		t.Errorf("expected updated best bid 0.51, got %f", snapshot.BestBidPrice)
	}

	t.Log("✓ Price change message processed correctly")
}
