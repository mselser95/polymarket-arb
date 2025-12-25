package storage

import (
	"context"
	"fmt"

	"github.com/mselser95/polymarket-arb/internal/arbitrage"
	"go.uber.org/zap"
)

// ConsoleStorage implements Storage by pretty-printing to console.
type ConsoleStorage struct {
	logger *zap.Logger
}

// NewConsoleStorage creates a new console storage.
func NewConsoleStorage(logger *zap.Logger) *ConsoleStorage {
	logger.Info("console-storage-initialized")
	return &ConsoleStorage{
		logger: logger,
	}
}

// StoreOpportunity pretty-prints an arbitrage opportunity to console.
func (c *ConsoleStorage) StoreOpportunity(ctx context.Context, opp *arbitrage.Opportunity) error {
	fmt.Println("\n" + "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("🎯 ARBITRAGE OPPORTUNITY DETECTED\n")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("ID:       %s\n", opp.ID[:8])
	fmt.Printf("Market:   %s\n", opp.MarketSlug)
	fmt.Printf("Question: %s\n", opp.MarketQuestion)
	fmt.Printf("Time:     %s\n", opp.DetectedAt.Format("2006-01-02 15:04:05"))
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("📊 PRICES\n")
	fmt.Printf("  YES Bid:  %.4f @ %.2f size\n", opp.YesAskPrice, opp.YesAskSize)
	fmt.Printf("  NO Bid:   %.4f @ %.2f size\n", opp.NoAskPrice, opp.NoAskSize)
	fmt.Printf("  Sum:      %.4f (threshold: %.4f)\n", opp.PriceSum, opp.ConfigThreshold)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("💰 PROFIT ANALYSIS\n")
	fmt.Printf("  Trade Size:      $%.2f\n", opp.MaxTradeSize)
	fmt.Printf("  Gross Profit:    $%.2f (%d bps)\n", opp.EstimatedProfit, opp.ProfitBPS)
	fmt.Printf("  Fees (1%%):       $%.2f\n", opp.TotalFees)
	fmt.Printf("  Net Profit:      $%.2f (%d bps)\n", opp.NetProfit, opp.NetProfitBPS)
	if opp.NetProfit > 0 {
		fmt.Printf("  ✅ PROFITABLE after fees!\n")
	} else {
		fmt.Printf("  ❌ NOT profitable after fees\n")
	}
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	return nil
}

// Close is a no-op for console storage.
func (c *ConsoleStorage) Close() error {
	c.logger.Info("closing-console-storage")
	return nil
}
