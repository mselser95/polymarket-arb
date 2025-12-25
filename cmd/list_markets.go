package cmd

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/mselser95/polymarket-arb/internal/discovery"
	"github.com/mselser95/polymarket-arb/pkg/config"
	"github.com/spf13/cobra"
)

//nolint:gochecknoglobals // Cobra boilerplate
var listMarketsCmd = &cobra.Command{
	Use:   "list-markets",
	Short: "List active markets from Polymarket Gamma API",
	Long:  `Fetches and displays active markets from the Polymarket Gamma API for debugging purposes.`,
	RunE:  runListMarkets,
}

//nolint:gochecknoinits // Cobra boilerplate
func init() {
	rootCmd.AddCommand(listMarketsCmd)
	listMarketsCmd.Flags().IntP("limit", "l", 20, "Maximum number of markets to fetch")
	listMarketsCmd.Flags().BoolP("verbose", "v", false, "Show detailed market information")
	listMarketsCmd.Flags().StringP("sort", "s", "volume24hr", "Sort by: volume24hr, createdAt, endDate")
}

func runListMarkets(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
	limit, _ := cmd.Flags().GetInt("limit")
	verbose, _ := cmd.Flags().GetBool("verbose")
	sortBy, _ := cmd.Flags().GetString("sort")

	// Validate sort option
	validSorts := []string{"volume24hr", "createdAt", "endDate"}
	validSort := false
	for _, valid := range validSorts {
		if sortBy == valid {
			validSort = true
			break
		}
	}
	if !validSort {
		return fmt.Errorf("invalid sort option: %s. Valid options: volume24hr, createdAt, endDate", sortBy)
	}

	// Create client
	client := discovery.NewClient(cfg.PolymarketGammaURL, logger)

	// Fetch markets
	fmt.Printf("Fetching up to %d active markets from Polymarket...\n\n", limit)

	resp, err := client.FetchActiveMarkets(ctx, limit, 0, sortBy)
	if err != nil {
		return fmt.Errorf("fetch markets: %w", err)
	}

	if len(resp.Data) == 0 {
		fmt.Println("No active markets found.")
		return nil
	}

	// Display markets
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "SLUG\tQUESTION\tTOKENS\n")
	fmt.Fprintf(w, "----\t--------\t------\n")

	for i := range resp.Data {
		market := &resp.Data[i]

		yesToken := market.GetTokenByOutcome("YES")
		noToken := market.GetTokenByOutcome("NO")

		tokensStatus := "✓"
		if yesToken == nil || noToken == nil {
			tokensStatus = "✗ (missing YES/NO)"
		}

		question := market.Question
		if len(question) > 60 {
			question = question[:57] + "..."
		}

		fmt.Fprintf(w, "%s\t%s\t%s\n", market.Slug, question, tokensStatus)

		if verbose {
			fmt.Fprintf(w, "\tID: %s\n", market.ID)
			fmt.Fprintf(w, "\tClosed: %v, Active: %v\n", market.Closed, market.Active)
			if yesToken != nil {
				fmt.Fprintf(w, "\tYES Token: %s\n", yesToken.TokenID)
			}
			if noToken != nil {
				fmt.Fprintf(w, "\tNO Token: %s\n", noToken.TokenID)
			}
			fmt.Fprintf(w, "\n")
		}
	}

	w.Flush()

	fmt.Printf("\nTotal: %d markets (showing %d)\n", resp.Count, len(resp.Data))

	return nil
}
