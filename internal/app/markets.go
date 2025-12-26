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
	// Validate market has at least 2 outcomes
	if len(market.Tokens) < 2 {
		a.logger.Warn("market-has-insufficient-outcomes",
			zap.String("market-id", market.ID),
			zap.String("slug", market.Slug),
			zap.Int("outcome-count", len(market.Tokens)))
		return
	}

	// Subscribe to ALL outcome token IDs (supports both binary and multi-outcome markets)
	tokenIDs := make([]string, 0, len(market.Tokens))
	outcomeNames := make([]string, 0, len(market.Tokens))

	for _, token := range market.Tokens {
		if token.TokenID == "" {
			a.logger.Warn("market-has-token-with-empty-id",
				zap.String("market-id", market.ID),
				zap.String("slug", market.Slug),
				zap.String("outcome", token.Outcome))
			continue
		}
		tokenIDs = append(tokenIDs, token.TokenID)
		outcomeNames = append(outcomeNames, token.Outcome)
	}

	if len(tokenIDs) < 2 {
		a.logger.Warn("market-has-insufficient-valid-tokens",
			zap.String("market-id", market.ID),
			zap.String("slug", market.Slug),
			zap.Int("valid-token-count", len(tokenIDs)))
		return
	}

	// Subscribe to all tokens via WebSocket
	err := a.wsPool.Subscribe(a.ctx, tokenIDs)
	if err != nil {
		a.logger.Error("subscribe-failed",
			zap.String("market-id", market.ID),
			zap.String("slug", market.Slug),
			zap.Strings("token-ids", tokenIDs),
			zap.Error(err))
		return
	}

	a.logger.Info("subscribed-to-market",
		zap.String("slug", market.Slug),
		zap.String("question", market.Question),
		zap.Int("outcome-count", len(tokenIDs)),
		zap.Strings("outcomes", outcomeNames))
}
