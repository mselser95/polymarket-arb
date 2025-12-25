package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/mselser95/polymarket-arb/internal/arbitrage"
	"github.com/mselser95/polymarket-arb/internal/discovery"
	"github.com/mselser95/polymarket-arb/pkg/config"
	"github.com/mselser95/polymarket-arb/pkg/types"
	"github.com/mselser95/polymarket-arb/pkg/websocket"
	"github.com/spf13/cobra"
)

//nolint:gochecknoglobals // Cobra boilerplate
var executeArbCmd = &cobra.Command{
	Use:   "execute-arb <market-slug>",
	Short: "Execute a paper arbitrage trade on a specific market",
	Long: `Connects to a market, fetches current orderbook prices, and executes a paper
arbitrage trade if conditions are met. Useful for testing arbitrage logic.

Example:
  polymarket-arb execute-arb fed-increases-interest-rates-by-25-bps-after-january-2026-meeting`,
	Args: cobra.ExactArgs(1),
	RunE: runExecuteArb,
}

//nolint:gochecknoinits // Cobra boilerplate
func init() {
	rootCmd.AddCommand(executeArbCmd)
	executeArbCmd.Flags().Float64P("threshold", "t", 0.995, "Arbitrage threshold (price sum must be below this)")
	executeArbCmd.Flags().Float64P("size", "s", 100.0, "Trade size in USD")
	executeArbCmd.Flags().Float64P("fee", "f", 0.01, "Taker fee (0.01 = 1%)")
}

func runExecuteArb(cmd *cobra.Command, args []string) error {
	marketSlug := args[0]

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Load config
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Create logger
	logger, err := config.NewLogger()
	if err != nil {
		return fmt.Errorf("create logger: %w", err)
	}
	defer func() {
		_ = logger.Sync()
	}()

	// Get flags
	threshold, _ := cmd.Flags().GetFloat64("threshold")
	tradeSize, _ := cmd.Flags().GetFloat64("size")
	takerFee, _ := cmd.Flags().GetFloat64("fee")

	fmt.Printf("=== Polymarket Arbitrage Executor (Paper Mode) ===\n\n")
	fmt.Printf("Market: %s\n", marketSlug)
	fmt.Printf("Threshold: %.3f\n", threshold)
	fmt.Printf("Trade Size: $%.2f\n", tradeSize)
	fmt.Printf("Taker Fee: %.2f%%\n\n", takerFee*100)

	// Fetch market info
	client := discovery.NewClient(cfg.PolymarketGammaURL, logger)
	market, err := client.FetchMarketBySlug(ctx, marketSlug)
	if err != nil {
		return fmt.Errorf("fetch market: %w", err)
	}

	fmt.Printf("Question: %s\n", market.Question)
	fmt.Printf("Market ID: %s\n\n", market.ID)

	// Get YES and NO tokens
	yesToken := market.GetTokenByOutcome("YES")
	noToken := market.GetTokenByOutcome("NO")

	if yesToken == nil || noToken == nil {
		return fmt.Errorf("market missing YES or NO token")
	}

	fmt.Printf("YES Token: %s\n", yesToken.TokenID)
	fmt.Printf("NO Token: %s\n\n", noToken.TokenID)

	// Create WebSocket manager
	wsManager := websocket.New(websocket.Config{
		URL:                   cfg.PolymarketWSURL,
		DialTimeout:           cfg.WSDialTimeout,
		PongTimeout:           cfg.WSPongTimeout,
		PingInterval:          cfg.WSPingInterval,
		ReconnectInitialDelay: cfg.WSReconnectInitialDelay,
		ReconnectMaxDelay:     cfg.WSReconnectMaxDelay,
		ReconnectBackoffMult:  cfg.WSReconnectBackoffMult,
		MessageBufferSize:     cfg.WSMessageBufferSize,
		Logger:                logger,
	})

	// Start WebSocket
	err = wsManager.Start()
	if err != nil {
		return fmt.Errorf("start websocket: %w", err)
	}
	defer wsManager.Close()

	// Subscribe to tokens
	tokenIDs := []string{yesToken.TokenID, noToken.TokenID}
	err = wsManager.Subscribe(ctx, tokenIDs)
	if err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}

	fmt.Println("Subscribed to orderbook. Waiting for prices...")

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Track orderbook snapshots
	orderbooks := make(map[string]*types.OrderbookSnapshot)
	msgChan := wsManager.MessageChan()

	// Wait for orderbook data with timeout
	timeout := time.After(30 * time.Second)

	for {
		select {
		case <-sigChan:
			fmt.Println("\nShutdown requested")
			return nil

		case <-timeout:
			return fmt.Errorf("timeout waiting for orderbook data")

		case msg, ok := <-msgChan:
			if !ok {
				return fmt.Errorf("message channel closed")
			}

			// Only process book messages
			if msg.EventType != "book" {
				continue
			}

			// Update orderbook snapshot
			snapshot := parseOrderbookSnapshot(msg, market.ID)
			if snapshot != nil {
				orderbooks[msg.AssetID] = snapshot

				// Display update
				outcome := "UNKNOWN"
				if msg.AssetID == yesToken.TokenID {
					outcome = "YES"
				} else if msg.AssetID == noToken.TokenID {
					outcome = "NO"
				}

				fmt.Printf("[%s] %s - Bid: %.3f (size: %.2f), Ask: %.3f (size: %.2f)\n",
					time.Now().Format("15:04:05"),
					outcome,
					snapshot.BestBidPrice,
					snapshot.BestBidSize,
					snapshot.BestAskPrice,
					snapshot.BestAskSize)

				// Check if we have both orderbooks
				yesBook, hasYes := orderbooks[yesToken.TokenID]
				noBook, hasNo := orderbooks[noToken.TokenID]

				if hasYes && hasNo {
					// Calculate arbitrage
					fmt.Println("\n=== Arbitrage Analysis ===")

					// Use ASK prices - this is what you PAY to BUY
					yesAsk := yesBook.BestAskPrice
					noAsk := noBook.BestAskPrice
					priceSum := yesAsk + noAsk

					fmt.Printf("YES Ask: %.4f (you buy at this price)\n", yesAsk)
					fmt.Printf("NO Ask:  %.4f (you buy at this price)\n", noAsk)
					fmt.Printf("Price Sum: %.4f\n", priceSum)
					fmt.Printf("Threshold: %.4f\n\n", threshold)

					if priceSum >= threshold {
						fmt.Printf("❌ No arbitrage opportunity (price sum %.4f >= threshold %.4f)\n", priceSum, threshold)
						fmt.Println("\nTip: Try a market with more price inefficiency, or adjust --threshold")
						return nil
					}

					fmt.Printf("✅ Arbitrage opportunity detected!\n\n")

					// Use ASK prices and sizes for the opportunity
					// In arbitrage, you buy at the ask price
					opp := arbitrage.NewOpportunity(
						market.ID,
						market.Slug,
						market.Question,
						yesToken.TokenID,
						noToken.TokenID,
						yesAsk,
						yesBook.BestAskSize,
						noAsk,
						noBook.BestAskSize,
						threshold,
						takerFee,
					)

					// Display results
					fmt.Println("=== Trade Execution (Paper Mode) ===")
					fmt.Printf("Buy YES at Ask: %.4f\n", opp.YesAskPrice)
					fmt.Printf("Buy NO at Ask:  %.4f\n", opp.NoAskPrice)
					fmt.Printf("Trade Size: $%.2f\n\n", opp.MaxTradeSize)

					fmt.Println("=== Profit Calculation ===")
					fmt.Printf("Gross Profit: $%.4f (%.2f BPS)\n", opp.EstimatedProfit, float64(opp.ProfitBPS))
					fmt.Printf("Total Fees:   $%.4f\n", opp.TotalFees)
					fmt.Printf("Net Profit:   $%.4f (%.2f BPS)\n\n", opp.NetProfit, float64(opp.NetProfitBPS))

					if opp.NetProfit <= 0 {
						fmt.Printf("⚠️  WARNING: Net profit is negative after fees!\n")
						fmt.Printf("   This trade would lose money. The market spread is too narrow.\n\n")
					} else {
						fmt.Printf("✅ Profitable trade! Net profit: $%.4f\n\n", opp.NetProfit)
					}

					fmt.Println("=== Breakdown ===")
					fmt.Printf("Total Cost:     $%.4f (%.4f + %.4f)\n", opp.YesAskPrice+opp.NoAskPrice, opp.YesAskPrice, opp.NoAskPrice)
					fmt.Printf("Payout:         $1.0000 (always pays $1 when both resolve)\n")
					fmt.Printf("Profit Margin:  %.4f (1.0 - %.4f)\n", opp.ProfitMargin, priceSum)
					fmt.Printf("ROI:            %.2f%%\n\n", (opp.NetProfit/(opp.YesAskPrice+opp.NoAskPrice))*100)

					return nil
				}
			}
		}
	}
}

func parseOrderbookSnapshot(msg *types.OrderbookMessage, marketID string) *types.OrderbookSnapshot {
	if len(msg.Bids) == 0 || len(msg.Asks) == 0 {
		return nil
	}

	// Parse best bid
	bestBid, err := strconv.ParseFloat(msg.Bids[0].Price, 64)
	if err != nil {
		return nil
	}
	bidSize, err := strconv.ParseFloat(msg.Bids[0].Size, 64)
	if err != nil {
		return nil
	}

	// Parse best ask
	bestAsk, err := strconv.ParseFloat(msg.Asks[0].Price, 64)
	if err != nil {
		return nil
	}
	askSize, err := strconv.ParseFloat(msg.Asks[0].Size, 64)
	if err != nil {
		return nil
	}

	return &types.OrderbookSnapshot{
		MarketID:     marketID,
		TokenID:      msg.AssetID,
		BestBidPrice: bestBid,
		BestBidSize:  bidSize,
		BestAskPrice: bestAsk,
		BestAskSize:  askSize,
		LastUpdated:  time.Now(),
	}
}
