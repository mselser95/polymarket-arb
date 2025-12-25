package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

//nolint:gochecknoglobals // Cobra boilerplate
var rootCmd = &cobra.Command{
	Use:   "polymarket-arb",
	Short: "Polymarket arbitrage bot",
	Long: `Polymarket arbitrage bot that subscribes to new emerging markets,
detects arbitrage opportunities when YES bid + NO bid < 1.0,
and executes trades in paper trading mode.

The bot polls the Polymarket Gamma API for new markets, subscribes to their
orderbooks via WebSocket, and monitors for price inefficiencies.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

//nolint:gochecknoinits // Cobra boilerplate
func init() {
	// Flags can be added here if needed
}
