package arbitrage

import (
	"context"
	"sync"
	"time"

	"github.com/mselser95/polymarket-arb/internal/discovery"
	"github.com/mselser95/polymarket-arb/internal/orderbook"
	"github.com/mselser95/polymarket-arb/pkg/types"
	"go.uber.org/zap"
)

// Storage is the interface for storing opportunities.
type Storage interface {
	StoreOpportunity(ctx context.Context, opp *Opportunity) error
	Close() error
}

// Detector detects arbitrage opportunities.
type Detector struct {
	obManager        *orderbook.Manager
	discoveryService *discovery.Service
	config           Config
	logger           *zap.Logger
	storage          Storage
	opportunityChan  chan *Opportunity
	obUpdateChan     <-chan *types.OrderbookSnapshot
	ctx              context.Context
	wg               sync.WaitGroup
}

// Config holds detector configuration.
type Config struct {
	Threshold    float64
	MinTradeSize float64
	TakerFee     float64
	Logger       *zap.Logger
}

// New creates a new arbitrage detector.
func New(cfg Config, obManager *orderbook.Manager, discoveryService *discovery.Service, storage Storage) *Detector {
	return &Detector{
		obManager:        obManager,
		discoveryService: discoveryService,
		config:           cfg,
		logger:           cfg.Logger,
		storage:          storage,
		opportunityChan:  make(chan *Opportunity, 50),
		obUpdateChan:     obManager.UpdateChan(),
	}
}

// Start starts the arbitrage detector.
func (d *Detector) Start(ctx context.Context) error {
	d.ctx = ctx
	d.logger.Info("arbitrage-detector-starting",
		zap.Float64("threshold", d.config.Threshold),
		zap.Float64("min-trade-size", d.config.MinTradeSize))

	d.wg.Add(1)
	go d.detectionLoop()

	return nil
}

// detectionLoop listens for orderbook updates and checks for arbitrage.
func (d *Detector) detectionLoop() {
	defer d.wg.Done()

	for {
		select {
		case <-d.ctx.Done():
			d.logger.Info("arbitrage-detector-stopping")
			close(d.opportunityChan)
			return
		case update := <-d.obUpdateChan:
			if update == nil {
				// Channel closed
				return
			}
			start := time.Now()
			d.checkArbitrageForToken(update)
			DetectionDurationSeconds.Observe(time.Since(start).Seconds())
		}
	}
}

// checkArbitrageForToken checks for arbitrage when a specific token's orderbook updates.
func (d *Detector) checkArbitrageForToken(update *types.OrderbookSnapshot) {
	// Find which market this token belongs to
	markets := d.discoveryService.GetSubscribedMarkets()

	var targetMarket *types.MarketSubscription
	var isYesToken bool

	for _, market := range markets {
		if market.TokenIDYes == update.TokenID {
			targetMarket = market
			isYesToken = true
			break
		} else if market.TokenIDNo == update.TokenID {
			targetMarket = market
			isYesToken = false
			break
		}
	}

	if targetMarket == nil {
		// Token not part of any subscribed market
		return
	}

	// Get both YES and NO orderbooks for this market
	yesSnapshot, yesExists := d.obManager.GetSnapshot(targetMarket.TokenIDYes)
	noSnapshot, noExists := d.obManager.GetSnapshot(targetMarket.TokenIDNo)

	if !yesExists || !noExists {
		// Need both orderbooks to check arbitrage
		return
	}

	// Check for arbitrage
	opp, exists := d.detect(targetMarket, yesSnapshot, noSnapshot)
	if !exists {
		return
	}

	// Store opportunity
	err := d.storage.StoreOpportunity(d.ctx, opp)
	if err != nil {
		d.logger.Error("failed-to-store-opportunity",
			zap.String("opportunity-id", opp.ID),
			zap.Error(err))
	}

	// Send opportunity (non-blocking)
	select {
	case d.opportunityChan <- opp:
		d.logger.Info("arbitrage-opportunity-detected",
			zap.String("opportunity-id", opp.ID),
			zap.String("market-slug", opp.MarketSlug),
			zap.Int("net-profit-bps", opp.NetProfitBPS),
			zap.Float64("net-profit", opp.NetProfit),
			zap.Bool("yes-token-updated", isYesToken))
	default:
		d.logger.Warn("opportunity-channel-full", zap.String("market-slug", targetMarket.MarketSlug))
	}
}

// detectOpportunities scans all markets for arbitrage opportunities.
// DEPRECATED: Use event-driven detection via checkArbitrageForToken instead.
func (d *Detector) detectOpportunities() {
	// Get all subscribed markets
	markets := d.discoveryService.GetSubscribedMarkets()

	for _, market := range markets {
		// Get YES and NO orderbooks
		yesSnapshot, yesExists := d.obManager.GetSnapshot(market.TokenIDYes)
		noSnapshot, noExists := d.obManager.GetSnapshot(market.TokenIDNo)

		if !yesExists || !noExists {
			continue
		}

		// Check for arbitrage
		opp, exists := d.detect(market, yesSnapshot, noSnapshot)
		if !exists {
			continue
		}

		// Store opportunity
		err := d.storage.StoreOpportunity(d.ctx, opp)
		if err != nil {
			d.logger.Error("failed-to-store-opportunity",
				zap.String("opportunity-id", opp.ID),
				zap.Error(err))
		}

		// Send opportunity (non-blocking)
		select {
		case d.opportunityChan <- opp:
			d.logger.Info("arbitrage-opportunity-detected",
				zap.String("opportunity-id", opp.ID),
				zap.String("market-slug", opp.MarketSlug),
				zap.Int("net-profit-bps", opp.NetProfitBPS),
				zap.Float64("net-profit", opp.NetProfit))
		default:
			d.logger.Warn("opportunity-channel-full", zap.String("market-slug", market.MarketSlug))
		}
	}
}

// detect checks if an arbitrage opportunity exists for a market.
func (d *Detector) detect(
	market *types.MarketSubscription,
	yesBook *types.OrderbookSnapshot,
	noBook *types.OrderbookSnapshot,
) (*Opportunity, bool) {
	// Validate orderbooks - use ASK prices since we're buying
	if yesBook.BestAskPrice <= 0 || noBook.BestAskPrice <= 0 {
		return nil, false
	}

	if yesBook.BestAskSize <= 0 || noBook.BestAskSize <= 0 {
		return nil, false
	}

	// Calculate price sum using ASK prices (what we pay to buy)
	priceSum := yesBook.BestAskPrice + noBook.BestAskPrice

	// Check if arbitrage exists
	if priceSum >= d.config.Threshold {
		return nil, false
	}

	// Calculate trade size using ASK sizes (available liquidity to buy)
	maxSize := yesBook.BestAskSize
	if noBook.BestAskSize < maxSize {
		maxSize = noBook.BestAskSize
	}

	// Check minimum trade size
	if maxSize < d.config.MinTradeSize {
		d.logger.Debug("opportunity-below-min-size",
			zap.String("market-slug", market.MarketSlug),
			zap.Float64("size", maxSize),
			zap.Float64("min-size", d.config.MinTradeSize))
		return nil, false
	}

	// Create opportunity using ASK prices (what we pay to buy)
	opp := NewOpportunity(
		market.MarketID,
		market.MarketSlug,
		market.Question,
		market.TokenIDYes,
		market.TokenIDNo,
		yesBook.BestAskPrice,
		yesBook.BestAskSize,
		noBook.BestAskPrice,
		noBook.BestAskSize,
		d.config.Threshold,
		d.config.TakerFee,
	)

	// Update metrics
	OpportunitiesDetectedTotal.Inc()
	OpportunityProfitBPS.Observe(float64(opp.ProfitBPS))
	OpportunitySizeUSD.Observe(opp.MaxTradeSize)

	return opp, true
}

// OpportunityChan returns the channel for receiving opportunities.
func (d *Detector) OpportunityChan() <-chan *Opportunity {
	return d.opportunityChan
}

// Close gracefully closes the detector.
func (d *Detector) Close() error {
	d.logger.Info("closing-arbitrage-detector")
	d.wg.Wait()
	d.logger.Info("arbitrage-detector-closed")
	return nil
}
