package execution

import (
	"context"
	"testing"
	"time"

	"github.com/mselser95/polymarket-arb/internal/arbitrage"
	"go.uber.org/zap"
)

func TestNew(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	oppChan := make(chan *arbitrage.Opportunity, 10)

	cfg := &Config{
		Mode:               "paper",
		MaxPositionSize:    1000.0,
		Logger:             logger,
		OpportunityChannel: oppChan,
	}

	exec := New(cfg)

	if exec == nil {
		t.Fatal("expected non-nil executor")
	}

	if exec.mode != cfg.Mode {
		t.Errorf("expected mode %s, got %s", cfg.Mode, exec.mode)
	}

	if exec.logger == nil {
		t.Error("expected non-nil logger")
	}

	if exec.opportunityChan != oppChan {
		t.Error("expected opportunity channel to match")
	}

	if exec.cumulativeProfit != 0 {
		t.Errorf("expected cumulative profit to be 0, got %f", exec.cumulativeProfit)
	}
}

func TestExecutor_ExecutePaper(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	exec := &Executor{
		mode:   "paper",
		logger: logger,
	}

	opp := &arbitrage.Opportunity{
		ID:             "opp1",
		MarketSlug:     "test-market",
		MarketQuestion: "Will X happen?",
		YesAskPrice:    0.48,
		NoAskPrice:     0.51,
		PriceSum:       0.99,
		ProfitMargin:   0.01,
		ProfitBPS:      100,
		MaxTradeSize:   100.0,
		DetectedAt:     time.Now(),
	}

	result := exec.executePaper(opp)

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if !result.Success {
		t.Errorf("expected success to be true, got false")
	}

	if result.Error != nil {
		t.Errorf("expected no error, got %v", result.Error)
	}

	if result.OpportunityID != opp.ID {
		t.Errorf("expected opportunity ID %s, got %s", opp.ID, result.OpportunityID)
	}

	if result.MarketSlug != opp.MarketSlug {
		t.Errorf("expected market slug %s, got %s", opp.MarketSlug, result.MarketSlug)
	}

	// Check trades
	if result.YesTrade == nil {
		t.Fatal("expected YES trade to be non-nil")
	}

	if result.YesTrade.Outcome != "YES" {
		t.Errorf("expected YES outcome, got %s", result.YesTrade.Outcome)
	}

	if result.YesTrade.Side != "BUY" {
		t.Errorf("expected BUY side, got %s", result.YesTrade.Side)
	}

	if result.YesTrade.Price != opp.YesAskPrice {
		t.Errorf("expected price %f, got %f", opp.YesAskPrice, result.YesTrade.Price)
	}

	if result.YesTrade.Size != opp.MaxTradeSize {
		t.Errorf("expected size %f, got %f", opp.MaxTradeSize, result.YesTrade.Size)
	}

	if result.NoTrade == nil {
		t.Fatal("expected NO trade to be non-nil")
	}

	if result.NoTrade.Outcome != "NO" {
		t.Errorf("expected NO outcome, got %s", result.NoTrade.Outcome)
	}

	if result.NoTrade.Side != "BUY" {
		t.Errorf("expected BUY side, got %s", result.NoTrade.Side)
	}

	// Check profit calculation
	expectedProfit := opp.MaxTradeSize * opp.ProfitMargin // 100 * 0.01 = 1.0
	if result.RealizedProfit != expectedProfit {
		t.Errorf("expected profit %f, got %f", expectedProfit, result.RealizedProfit)
	}

	// Check cumulative profit
	exec.mu.Lock()
	cumulativeProfit := exec.cumulativeProfit
	exec.mu.Unlock()

	if cumulativeProfit != expectedProfit {
		t.Errorf("expected cumulative profit %f, got %f", expectedProfit, cumulativeProfit)
	}
}

func TestExecutor_ExecutePaper_MultipleTrades(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	exec := &Executor{
		mode:   "paper",
		logger: logger,
	}

	// Execute multiple trades
	for i := 0; i < 3; i++ {
		opp := &arbitrage.Opportunity{
			ID:             "opp1",
			MarketSlug:     "test-market",
			MarketQuestion: "Will X happen?",
			YesAskPrice:    0.48,
			NoAskPrice:     0.51,
			PriceSum:       0.99,
			ProfitMargin:   0.01,
			ProfitBPS:      100,
			MaxTradeSize:   100.0,
			DetectedAt:     time.Now(),
		}

		exec.executePaper(opp)
	}

	// Check cumulative profit
	exec.mu.Lock()
	cumulativeProfit := exec.cumulativeProfit
	exec.mu.Unlock()

	expectedProfit := 3.0 // 3 trades * 100 * 0.01
	if cumulativeProfit != expectedProfit {
		t.Errorf("expected cumulative profit %f, got %f", expectedProfit, cumulativeProfit)
	}
}

func TestExecutor_ExecuteLive(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	exec := &Executor{
		mode:   "live",
		logger: logger,
	}

	opp := &arbitrage.Opportunity{
		ID:             "opp1",
		MarketSlug:     "test-market",
		MarketQuestion: "Will X happen?",
		YesAskPrice:    0.48,
		NoAskPrice:     0.51,
		PriceSum:       0.99,
		ProfitMargin:   0.01,
		ProfitBPS:      100,
		MaxTradeSize:   100.0,
		DetectedAt:     time.Now(),
	}

	result := exec.executeLive(opp)

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Live trading not implemented, should return error
	if result.Success {
		t.Error("expected success to be false for unimplemented live trading")
	}

	if result.Error == nil {
		t.Error("expected error for unimplemented live trading")
	}
}

func TestExecutor_Execute_UnknownMode(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	exec := &Executor{
		mode:   "unknown",
		logger: logger,
	}

	opp := &arbitrage.Opportunity{
		ID:             "opp1",
		MarketSlug:     "test-market",
		MarketQuestion: "Will X happen?",
		YesAskPrice:    0.48,
		NoAskPrice:     0.51,
		PriceSum:       0.99,
		ProfitMargin:   0.01,
		ProfitBPS:      100,
		MaxTradeSize:   100.0,
		DetectedAt:     time.Now(),
	}

	result := exec.execute(opp)

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if result.Success {
		t.Error("expected success to be false for unknown mode")
	}

	if result.Error == nil {
		t.Error("expected error for unknown mode")
	}
}

func TestExecutor_ExecutionLoop(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	oppChan := make(chan *arbitrage.Opportunity, 10)

	exec := &Executor{
		mode:            "paper",
		logger:          logger,
		opportunityChan: oppChan,
	}

	ctx, cancel := context.WithCancel(context.Background())
	exec.ctx = ctx

	// Start execution loop
	exec.wg.Add(1)
	go exec.executionLoop()

	// Send an opportunity
	opp := &arbitrage.Opportunity{
		ID:             "opp1",
		MarketSlug:     "test-market",
		MarketQuestion: "Will X happen?",
		YesAskPrice:    0.48,
		NoAskPrice:     0.51,
		PriceSum:       0.99,
		ProfitMargin:   0.01,
		ProfitBPS:      100,
		MaxTradeSize:   100.0,
		DetectedAt:     time.Now(),
	}

	oppChan <- opp

	// Give it time to process
	time.Sleep(100 * time.Millisecond)

	// Check cumulative profit
	exec.mu.Lock()
	cumulativeProfit := exec.cumulativeProfit
	exec.mu.Unlock()

	expectedProfit := 1.0 // 100 * 0.01
	if cumulativeProfit != expectedProfit {
		t.Errorf("expected cumulative profit %f, got %f", expectedProfit, cumulativeProfit)
	}

	// Stop execution loop
	cancel()
	exec.wg.Wait()
}

func TestExecutor_ExecutionLoop_ChannelClose(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	oppChan := make(chan *arbitrage.Opportunity, 10)

	exec := &Executor{
		mode:            "paper",
		logger:          logger,
		opportunityChan: oppChan,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	exec.ctx = ctx

	// Start execution loop
	exec.wg.Add(1)
	go exec.executionLoop()

	// Close channel
	close(oppChan)

	// Wait for execution loop to stop
	exec.wg.Wait()

	// Should exit cleanly
}

func TestExecutor_Start_And_Close(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	oppChan := make(chan *arbitrage.Opportunity, 10)

	cfg := &Config{
		Mode:               "paper",
		MaxPositionSize:    1000.0,
		Logger:             logger,
		OpportunityChannel: oppChan,
	}

	exec := New(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := exec.Start(ctx)
	if err != nil {
		t.Fatalf("expected no error on start, got %v", err)
	}

	// Send an opportunity
	opp := &arbitrage.Opportunity{
		ID:             "opp1",
		MarketSlug:     "test-market",
		MarketQuestion: "Will X happen?",
		YesAskPrice:    0.48,
		NoAskPrice:     0.51,
		PriceSum:       0.99,
		ProfitMargin:   0.01,
		ProfitBPS:      100,
		MaxTradeSize:   100.0,
		DetectedAt:     time.Now(),
	}

	oppChan <- opp

	// Give it time to process
	time.Sleep(100 * time.Millisecond)

	// Stop
	cancel()

	// Close
	err = exec.Close()
	if err != nil {
		t.Errorf("expected no error on close, got %v", err)
	}

	// Verify profit was tracked
	exec.mu.Lock()
	cumulativeProfit := exec.cumulativeProfit
	exec.mu.Unlock()

	expectedProfit := 1.0
	if cumulativeProfit != expectedProfit {
		t.Errorf("expected cumulative profit %f, got %f", expectedProfit, cumulativeProfit)
	}
}

func TestExecutor_ConcurrentExecution(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	oppChan := make(chan *arbitrage.Opportunity, 100)

	exec := &Executor{
		mode:            "paper",
		logger:          logger,
		opportunityChan: oppChan,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	exec.ctx = ctx

	// Start execution loop
	exec.wg.Add(1)
	go exec.executionLoop()

	// Send multiple opportunities
	numOpps := 10
	for i := 0; i < numOpps; i++ {
		opp := &arbitrage.Opportunity{
			ID:             "opp1",
			MarketSlug:     "test-market",
			MarketQuestion: "Will X happen?",
			YesAskPrice:    0.48,
			NoAskPrice:     0.51,
			PriceSum:       0.99,
			ProfitMargin:   0.01,
			ProfitBPS:      100,
			MaxTradeSize:   100.0,
			DetectedAt:     time.Now(),
		}

		oppChan <- opp
	}

	// Give it time to process
	time.Sleep(200 * time.Millisecond)

	// Check cumulative profit
	exec.mu.Lock()
	cumulativeProfit := exec.cumulativeProfit
	exec.mu.Unlock()

	expectedProfit := 10.0 // 10 opportunities * 100 * 0.01
	if cumulativeProfit != expectedProfit {
		t.Errorf("expected cumulative profit %f, got %f", expectedProfit, cumulativeProfit)
	}

	// Stop execution loop
	cancel()
	exec.wg.Wait()
}

func TestExecutor_ProfitCalculation(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	exec := &Executor{
		mode:   "paper",
		logger: logger,
	}

	tests := []struct {
		name           string
		maxTradeSize   float64
		profitMargin   float64
		expectedProfit float64
	}{
		{"small trade", 10.0, 0.02, 0.2},
		{"medium trade", 100.0, 0.01, 1.0},
		{"large trade", 1000.0, 0.005, 5.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opp := &arbitrage.Opportunity{
				ID:             "opp1",
				MarketSlug:     "test-market",
				MarketQuestion: "Will X happen?",
				YesAskPrice:    0.48,
				NoAskPrice:     0.51,
				PriceSum:       0.99,
				ProfitMargin:   tt.profitMargin,
				ProfitBPS:      int(tt.profitMargin * 10000),
				MaxTradeSize:   tt.maxTradeSize,
				DetectedAt:     time.Now(),
			}

			result := exec.executePaper(opp)

			if result.RealizedProfit != tt.expectedProfit {
				t.Errorf("expected profit %f, got %f", tt.expectedProfit, result.RealizedProfit)
			}
		})
	}
}
