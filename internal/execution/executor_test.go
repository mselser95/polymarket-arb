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

	opp := arbitrage.CreateTestOpportunity("test-market", "test-slug")

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
		t.Fatal("expected first trade to be non-nil")
	}

	if result.YesTrade.Side != "BUY" {
		t.Errorf("expected BUY side, got %s", result.YesTrade.Side)
	}

	if result.NoTrade == nil {
		t.Fatal("expected second trade to be non-nil")
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

func TestExecutor_ExecuteLive(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	exec := &Executor{
		mode:   "live",
		logger: logger,
	}

	opp := arbitrage.CreateTestOpportunity("test-market", "test-slug")

	result := exec.executeLive(opp)

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Live trading not configured, should return error
	if result.Success {
		t.Error("expected success to be false for unconfigured live trading")
	}

	if result.Error == nil {
		t.Error("expected error for unconfigured live trading")
	}
}

func TestExecutor_Execute_UnknownMode(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	exec := &Executor{
		mode:   "unknown",
		logger: logger,
	}

	opp := arbitrage.CreateTestOpportunity("test-market", "test-slug")

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
	opp := arbitrage.CreateTestOpportunity("test-market", "test-slug")
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
		opp := arbitrage.CreateTestOpportunity("test-market", "test-slug")
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
