package app

import (
	"github.com/mselser95/polymarket-arb/pkg/types"
	"go.uber.org/zap"
)

// handleNewMarkets subscribes to new markets as they are discovered.
func (a *App) handleNewMarkets() {
	defer a.wg.Done()

	for {
		select {
		case <-a.ctx.Done():
			return
		case market, ok := <-a.discoveryService.NewMarketsChan():
			if !ok {
				return
			}

			a.subscribeToMarket(market)
		}
	}
}

func (a *App) subscribeToMarket(market *types.Market) {
	// Get YES and NO token IDs
	yesToken := market.GetTokenByOutcome("YES")
	noToken := market.GetTokenByOutcome("NO")

	if yesToken == nil || noToken == nil {
		a.logger.Warn("market-missing-tokens",
			zap.String("market-id", market.ID),
			zap.String("slug", market.Slug))
		return
	}

	// Subscribe to both tokens
	tokenIDs := []string{yesToken.TokenID, noToken.TokenID}
	err := a.wsManager.Subscribe(a.ctx, tokenIDs)
	if err != nil {
		a.logger.Error("subscribe-failed",
			zap.String("market-id", market.ID),
			zap.String("slug", market.Slug),
			zap.Error(err))
		return
	}

	a.logger.Info("subscribed-to-market",
		zap.String("slug", market.Slug),
		zap.String("question", market.Question))
}
