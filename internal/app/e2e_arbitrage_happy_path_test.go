package app

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/mselser95/polymarket-arb/internal/arbitrage"
	"github.com/mselser95/polymarket-arb/internal/discovery"
	"github.com/mselser95/polymarket-arb/internal/execution"
	"github.com/mselser95/polymarket-arb/internal/markets"
	"github.com/mselser95/polymarket-arb/internal/orderbook"
	"github.com/mselser95/polymarket-arb/internal/testutil"
	"github.com/mselser95/polymarket-arb/pkg/cache"
	"github.com/mselser95/polymarket-arb/pkg/types"
	"go.uber.org/zap/zaptest"
)

// TestE2E_ArbitrageHappyPath_WithProfitOutput demonstrates the complete
// arbitrage flow from orderbook updates through profit calculation.
//
// Flow:
// 1. Mock market discovery returns a binary YES/NO market
// 2. Mock WebSocket sends orderbook updates with arbitrage opportunity
// 3. ArbitrageDetector detects the opportunity (YES 0.48 + NO 0.51 = 0.99 < 0.995)
// 4. Executor receives opportunity and places orders (via mock)
// 5. Test verifies orders were placed correctly
// 6. Test prints detailed profit breakdown.
func TestE2E_ArbitrageHappyPath_WithProfitOutput(t *testing.T) {
	// Test scenario: Binary market with clear arbitrage
	// CRITICAL: We buy the SAME token count for ALL outcomes (core arbitrage requirement)
	//
	// Orderbook prices (detected opportunity):
	// - YES ask: $0.48
	// - NO ask:  $0.51
	// - Price sum: 0.99 < 0.995 threshold ✅ Arbitrage detected!
	//
	// Aggressive pricing (for fast fills):
	// - We submit orders at higher prices to ensure immediate execution
	// - Example: $0.49 for YES, $0.52 for NO
	// - BUT: Actual fill prices from API may differ (could be better/worse)
	//
	// CRITICAL: Profit calculated from ACTUAL FILL PRICES (from API execution reports)
	// - NOT from orderbook prices
	// - NOT from our submitted aggressive prices
	// - ONLY from what the API reports we actually paid
	//
	// Token count requirement:
	// - We buy the SAME token count for ALL outcomes (core arbitrage requirement)
	// - Arbitrage requires owning 1 complete "set" of outcomes per token
	// - Exactly ONE outcome wins and pays $1.00 per token
	// - If we buy 100 YES + 98 NO, we only have 98 complete sets (2 YES tokens wasted!)
	// - Must buy min(YES_tokens, NO_tokens) to maximize complete sets
	//
	// Example profit calculation (using ACTUAL fill prices from API):
	// Assume API reports we filled at $0.48 for YES, $0.51 for NO:
	// - Buy 100 YES tokens @ $0.48 (actual fill) = $48.00
	// - Buy 100 NO tokens @ $0.51 (actual fill) = $51.00
	// - Total cost: $99.00
	// - Guaranteed revenue: 100 sets × $1.00 = $100.00
	// - Gross profit: $100.00 - $99.00 = $1.00
	// - Fees (1%): $0.99
	// - Net profit: $1.00 - $0.99 = $0.01

	logger := zaptest.NewLogger(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// === SETUP: Create test market ===
	market := testutil.CreateTestMarket("test-binary-market", "test-slug", "Will Bitcoin hit $100k by EOY?")
	yesToken := market.GetTokenByOutcome("YES")
	noToken := market.GetTokenByOutcome("NO")

	if yesToken == nil || noToken == nil {
		t.Fatal("test market missing YES or NO token")
	}

	// === SETUP: Mock Gamma API ===
	mockAPI := testutil.NewMockGammaAPI([]*types.Market{market})
	defer mockAPI.Close()

	// === SETUP: Cache ===
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

	// === SETUP: Discovery service ===
	discoveryClient := discovery.NewClient(mockAPI.URL, logger)
	discoverySvc := discovery.New(&discovery.Config{
		Client:       discoveryClient,
		Cache:        cacheInterface,
		PollInterval: 1 * time.Second,
		MarketLimit:  10,
		Logger:       logger,
	})

	// === SETUP: WebSocket channel ===
	wsMsgChan := make(chan *types.OrderbookMessage, 100)

	// === SETUP: Orderbook manager ===
	obMgr := orderbook.New(&orderbook.Config{
		Logger:         logger,
		MessageChannel: wsMsgChan,
	})

	// === SETUP: Mock storage ===
	mockStorage := arbitrage.NewMockStorage()

	// === SETUP: Metadata client ===
	metadataClient := markets.NewMetadataClient()
	cachedMetadataClient := markets.NewCachedMetadataClient(metadataClient, nil)

	// === SETUP: Arbitrage detector ===
	detector := arbitrage.New(arbitrage.Config{
		MaxPriceSum:  0.995, // Threshold for arbitrage
		MinTradeSize: 1.0,
		MaxTradeSize: 50.0, // $50 max trade
		TakerFee:     0.01, // 1% fee
		Logger:       logger,
	}, obMgr, discoverySvc, mockStorage, cachedMetadataClient)

	// === SETUP: Mock OrderClient ===
	mockOrderClient := testutil.NewMockOrderClient()

	// === SETUP: Executor (LIVE mode to test order placement) ===
	executor := execution.New(&execution.Config{
		Mode:               "live", // Use live mode to test order placement
		MaxPositionSize:    50.0,
		Logger:             logger,
		OpportunityChannel: detector.OpportunityChan(),
		OrderClient:        mockOrderClient, // Inject mock
		AggressionTicks:    1,                // Add 1 tick for aggressive pricing
		FillTimeout:        5 * time.Second,
		FillRetryInitial:   100 * time.Millisecond,
		FillRetryMax:       1 * time.Second,
		FillRetryMult:      2.0,
		TakerFee:           0.01,
	})

	// === START COMPONENTS ===
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

	// Start discovery service
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

	// === INJECT ORDERBOOK UPDATES ===
	// Send book messages with arbitrage opportunity
	// YES ask: $0.45, NO ask: $0.48 → sum = $0.93 < threshold (wider spread for profitable arb)
	yesBookMsg := testutil.CreateTestBookMessage(yesToken.TokenID, market.ID)
	yesBookMsg.Asks = []types.PriceLevel{
		{Price: "0.45", Size: "200.0"}, // Best ask: $0.45
	}
	yesBookMsg.Bids = []types.PriceLevel{
		{Price: "0.44", Size: "100.0"}, // Best bid
	}

	noBookMsg := testutil.CreateTestBookMessage(noToken.TokenID, market.ID)
	noBookMsg.Asks = []types.PriceLevel{
		{Price: "0.48", Size: "200.0"}, // Best ask: $0.48
	}
	noBookMsg.Bids = []types.PriceLevel{
		{Price: "0.47", Size: "100.0"}, // Best bid
	}

	wsMsgChan <- yesBookMsg
	wsMsgChan <- noBookMsg

	// === WAIT FOR EXECUTION ===
	time.Sleep(2 * time.Second)

	// === VERIFY ORDERS PLACED ===
	placedOrders := mockOrderClient.GetPlacedOrders()
	if len(placedOrders) < 2 {
		t.Fatalf("expected at least 2 orders (YES + NO), got %d", len(placedOrders))
	}

	// === EXTRACT ORDER DETAILS (from first opportunity) ===
	// Note: Detector may find multiple opportunities during the wait period,
	// so we only analyze the first 2 orders (first opportunity)
	firstTwoOrders := placedOrders[:2]
	var yesOrder, noOrder testutil.MockPlacedOrder
	for _, order := range firstTwoOrders {
		if order.TokenID == yesToken.TokenID {
			yesOrder = order
		} else {
			noOrder = order
		}
	}

	// Store expected prices (from orderbook - used for opportunity detection)
	expectedYESPrice := 0.45
	expectedNOPrice := 0.48

	// Store adjusted prices (what we submitted to API - aggressive pricing)
	adjustedYESPrice := yesOrder.Price
	adjustedNOPrice := noOrder.Price

	// CRITICAL: Actual fill prices come from API execution reports
	// In a real scenario, we'd get these from fill verification / match events
	// For this mock test, we simulate that the API reports the order placement prices as fills
	// BUT in production, actual fills could differ from our submission prices!
	actualYESPrice := yesOrder.Price
	actualNOPrice := noOrder.Price

	// CRITICAL: In arbitrage, we buy the SAME number of tokens for each outcome
	// The token count is determined by the most expensive outcome (bottleneck)
	tokenCount := yesOrder.TokenCount
	if noOrder.TokenCount < tokenCount {
		tokenCount = noOrder.TokenCount
	}

	// Verify both orders have same token count (arbitrage requirement)
	if yesOrder.TokenCount != noOrder.TokenCount {
		t.Fatalf("ARBITRAGE VIOLATION: Token counts must be equal. YES: %.2f, NO: %.2f",
			yesOrder.TokenCount, noOrder.TokenCount)
	}

	// PROFIT CALCULATION - Uses ACTUAL fill prices from API
	// NOT orderbook prices, NOT our submission prices

	// Calculate costs using actual fill prices (from API execution reports)
	yesCost := tokenCount * actualYESPrice
	noCost := tokenCount * actualNOPrice
	totalCost := yesCost + noCost

	// Revenue: Since we own 1 complete "set" per token, we're guaranteed $1.00 per set
	// Exactly ONE outcome wins and pays $1.00 per token
	revenue := tokenCount * 1.0

	// Fees (applied to actual costs, not submission prices)
	takerFee := 0.01
	yesFee := yesCost * takerFee
	noFee := noCost * takerFee
	totalFees := yesFee + noFee

	// Final profit - based entirely on what the API says we paid
	grossProfit := revenue - totalCost
	netProfit := grossProfit - totalFees

	// === PRINT DETAILED PROFIT BREAKDOWN ===
	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("ARBITRAGE EXECUTION SUMMARY")
	fmt.Println(strings.Repeat("=", 70))
	fmt.Println()

	fmt.Printf("Market: %s\n", market.Question)
	fmt.Printf("Market ID: %s\n", market.ID)
	fmt.Println()

	fmt.Println("ORDERBOOK PRICES (Detected Opportunity):")
	fmt.Printf("  YES Ask:  $%.4f  (best available in orderbook)\n", expectedYESPrice)
	fmt.Printf("  NO Ask:   $%.4f  (best available in orderbook)\n", expectedNOPrice)
	fmt.Printf("  Sum:      $%.4f (threshold: $%.4f)\n", expectedYESPrice+expectedNOPrice, 0.995)
	fmt.Printf("  Spread:   $%.4f (%.2f%%)\n", 1.0-(expectedYESPrice+expectedNOPrice), (1.0-(expectedYESPrice+expectedNOPrice))*100)
	fmt.Println()

	fmt.Println("PRICE ADJUSTMENTS (Aggressive Pricing):")
	fmt.Printf("  Purpose: Ensure immediate fill, avoid hanging orders\n")
	fmt.Printf("  YES:  $%.4f → $%.4f (adjusted up by $%.4f)\n",
		expectedYESPrice, adjustedYESPrice, adjustedYESPrice-expectedYESPrice)
	fmt.Printf("  NO:   $%.4f → $%.4f (adjusted up by $%.4f)\n",
		expectedNOPrice, adjustedNOPrice, adjustedNOPrice-expectedNOPrice)
	fmt.Println()

	fmt.Println("ACTUAL FILLS (From API):")
	fmt.Printf("  Note: Same token count for all outcomes (arbitrage requirement)\n")
	fmt.Printf("  YES:  %.2f tokens @ $%.4f = $%.2f (Order ID: %s)\n",
		tokenCount, actualYESPrice, yesCost, yesOrder.OrderID)
	fmt.Printf("  NO:   %.2f tokens @ $%.4f = $%.2f (Order ID: %s)\n",
		tokenCount, actualNOPrice, noCost, noOrder.OrderID)
	fmt.Printf("  Total Cost: $%.2f\n", totalCost)
	fmt.Println()

	fmt.Println("PROFIT CALCULATION:")
	fmt.Printf("  Token Count:    %.2f tokens\n", tokenCount)
	fmt.Printf("  Revenue:        $%.2f (%.2f tokens × $1.00 payout)\n", revenue, tokenCount)
	fmt.Printf("  Total Cost:     $%.2f\n", totalCost)
	fmt.Printf("  Gross Profit:   $%.2f\n", grossProfit)
	fmt.Println()

	fmt.Println("FEES (1% taker fee):")
	fmt.Printf("  YES Fee:  $%.4f\n", yesFee)
	fmt.Printf("  NO Fee:   $%.4f\n", noFee)
	fmt.Printf("  Total:    $%.4f\n", totalFees)
	fmt.Println()

	fmt.Println("NET PROFIT:")
	fmt.Printf("  Net Profit:     $%.2f\n", netProfit)
	fmt.Printf("  ROI:            %.2f%%\n", (netProfit/totalCost)*100)
	fmt.Printf("  Profit per $:   $%.4f\n", netProfit/totalCost)
	fmt.Println()

	fmt.Println(strings.Repeat("=", 70))
	fmt.Println()

	// === VERIFY POSITIVE PROFIT ===
	if netProfit <= 0 {
		t.Errorf("Expected positive net profit, got $%.2f", netProfit)
	}
}
