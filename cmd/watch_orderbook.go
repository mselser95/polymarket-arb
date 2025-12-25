package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/mselser95/polymarket-arb/internal/discovery"
	"github.com/mselser95/polymarket-arb/pkg/config"
	"github.com/mselser95/polymarket-arb/pkg/types"
	"github.com/mselser95/polymarket-arb/pkg/websocket"
	"github.com/spf13/cobra"
)

//nolint:gochecknoglobals // Cobra boilerplate
var watchOrderbookCmd = &cobra.Command{
	Use:   "watch-orderbook <market-slug>",
	Short: "Watch orderbook updates for a specific market",
	Long: `Connects to Polymarket WebSocket and displays real-time orderbook updates
for a specific market. Useful for debugging and understanding market dynamics.

Example:
  polymarket-arb watch-orderbook trump-popular-vote-2024`,
	Args: cobra.ExactArgs(1),
	RunE: runWatchOrderbook,
}

//nolint:gochecknoinits // Cobra boilerplate
func init() {
	rootCmd.AddCommand(watchOrderbookCmd)
	watchOrderbookCmd.Flags().BoolP("json", "j", false, "Output raw JSON messages")
}

func runWatchOrderbook(cmd *cobra.Command, args []string) error {
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
	jsonOutput, _ := cmd.Flags().GetBool("json")

	// Fetch market info
	client := discovery.NewClient(cfg.PolymarketGammaURL, logger)
	market, err := client.FetchMarketBySlug(ctx, marketSlug)
	if err != nil {
		return fmt.Errorf("fetch market: %w", err)
	}

	fmt.Printf("Market: %s\n", market.Question)
	fmt.Printf("Slug: %s\n", market.Slug)
	fmt.Printf("ID: %s\n\n", market.ID)

	// Get YES and NO tokens
	yesToken := market.GetTokenByOutcome("YES")
	noToken := market.GetTokenByOutcome("NO")

	if yesToken == nil || noToken == nil {
		return fmt.Errorf("market missing YES or NO token")
	}

	fmt.Printf("YES Token ID: %s\n", yesToken.TokenID)
	fmt.Printf("NO Token ID: %s\n\n", noToken.TokenID)

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

	fmt.Println("Subscribed! Watching for orderbook updates...")

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Create tabwriter for formatted output
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	// Process messages
	msgChan := wsManager.MessageChan()

	for {
		select {
		case <-sigChan:
			fmt.Println("\nShutting down...")
			return nil
		case msg, ok := <-msgChan:
			if !ok {
				return fmt.Errorf("message channel closed")
			}

			if jsonOutput {
				// Print raw JSON
				jsonBytes, _ := json.MarshalIndent(msg, "", "  ")
				fmt.Println(string(jsonBytes))
			} else {
				// Print formatted
				printFormattedMessage(w, msg, yesToken.TokenID, noToken.TokenID)
			}
		}
	}
}

func printFormattedMessage(w *tabwriter.Writer, msg *types.OrderbookMessage, yesTokenID string, noTokenID string) {
	outcome := "UNKNOWN"
	if msg.AssetID == yesTokenID {
		outcome = "YES"
	} else if msg.AssetID == noTokenID {
		outcome = "NO"
	}

	timestamp := time.Unix(msg.Timestamp/1000, 0).Format("15:04:05")

	fmt.Fprintf(w, "[%s] %s\t%s\t", timestamp, outcome, msg.EventType)

	if msg.EventType == "book" || msg.EventType == "price_change" {
		// Extract best bid/ask
		bestBid := "N/A"
		bestAsk := "N/A"

		if len(msg.Bids) > 0 {
			bestBid = fmt.Sprintf("%s@%s", msg.Bids[0].Price, msg.Bids[0].Size)
		}

		if len(msg.Asks) > 0 {
			bestAsk = fmt.Sprintf("%s@%s", msg.Asks[0].Price, msg.Asks[0].Size)
		}

		fmt.Fprintf(w, "Bid: %s\tAsk: %s\n", bestBid, bestAsk)
	} else {
		fmt.Fprintf(w, "\n")
	}

	w.Flush()
}
