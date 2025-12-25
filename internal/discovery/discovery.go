package discovery

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/mselser95/polymarket-arb/pkg/cache"
	"github.com/mselser95/polymarket-arb/pkg/types"
	"go.uber.org/zap"
)

// Service discovers new markets by polling the Gamma API.
type Service struct {
	client       *Client
	cache        cache.Cache
	pollInterval time.Duration
	marketLimit  int
	logger       *zap.Logger
	subscribed   map[string]*types.MarketSubscription
	mu           sync.RWMutex
	newMarketsCh chan *types.Market
	singleMarket string // For debugging: if set, only track this one market
}

// Config holds discovery service configuration.
type Config struct {
	Client       *Client
	Cache        cache.Cache
	PollInterval time.Duration
	MarketLimit  int
	Logger       *zap.Logger
	SingleMarket string // For debugging: slug of single market to track
}

// New creates a new discovery service.
func New(cfg *Config) *Service {
	return &Service{
		client:       cfg.Client,
		cache:        cfg.Cache,
		pollInterval: cfg.PollInterval,
		marketLimit:  cfg.MarketLimit,
		logger:       cfg.Logger,
		subscribed:   make(map[string]*types.MarketSubscription),
		newMarketsCh: make(chan *types.Market, 100),
		singleMarket: cfg.SingleMarket,
	}
}

// Run starts the discovery polling loop.
func (s *Service) Run(ctx context.Context) error {
	s.logger.Info("discovery-service-starting",
		zap.Duration("poll-interval", s.pollInterval),
		zap.Int("market-limit", s.marketLimit),
		zap.String("single-market", s.singleMarket))

	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	// Initial poll
	err := s.poll(ctx)
	if err != nil {
		s.logger.Error("initial-poll-failed", zap.Error(err))
	}

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("discovery-service-stopping")
			close(s.newMarketsCh)
			return ctx.Err()
		case <-ticker.C:
			err := s.poll(ctx)
			if err != nil {
				s.logger.Error("poll-failed", zap.Error(err))
			}
		}
	}
}

// poll fetches markets from the API and identifies new ones.
func (s *Service) poll(ctx context.Context) error {
	start := time.Now()
	defer func() {
		PollDurationSeconds.Observe(time.Since(start).Seconds())
	}()

	// Single market mode for debugging
	if s.singleMarket != "" {
		return s.pollSingleMarket(ctx)
	}

	// Fetch active markets sorted by volume (most active first)
	resp, err := s.client.FetchActiveMarkets(ctx, s.marketLimit, 0, "volume24hr")
	if err != nil {
		PollErrorsTotal.Inc()
		return fmt.Errorf("fetch active markets: %w", err)
	}

	MarketsDiscoveredTotal.Add(float64(len(resp.Data)))

	// Identify new markets
	newMarkets := s.identifyNewMarkets(resp.Data)

	// Cache and send new markets to channel (non-blocking)
	for i := range newMarkets {
		// Cache the market
		s.cacheMarket(newMarkets[i])

		select {
		case s.newMarketsCh <- newMarkets[i]:
			NewMarketsTotal.Inc()
			s.logger.Info("new-market-discovered",
				zap.String("market-id", newMarkets[i].ID),
				zap.String("question", newMarkets[i].Question))
		default:
			s.logger.Warn("new-markets-channel-full",
				zap.String("market-id", newMarkets[i].ID))
		}
	}

	s.logger.Debug("poll-complete",
		zap.Int("total-markets", len(resp.Data)),
		zap.Int("new-markets", len(newMarkets)),
		zap.Duration("duration", time.Since(start)))

	return nil
}

// pollSingleMarket polls only a specific market (for debugging).
func (s *Service) pollSingleMarket(ctx context.Context) error {
	// Check if already subscribed
	s.mu.RLock()
	_, exists := s.subscribed[s.singleMarket]
	s.mu.RUnlock()

	if exists {
		// Already subscribed, nothing to do
		return nil
	}

	// Fetch the single market
	market, err := s.client.FetchMarketBySlug(ctx, s.singleMarket)
	if err != nil {
		PollErrorsTotal.Inc()
		return fmt.Errorf("fetch market by slug %q: %w", s.singleMarket, err)
	}

	MarketsDiscoveredTotal.Inc()

	// Check if market has YES and NO tokens
	yesToken := market.GetTokenByOutcome("YES")
	noToken := market.GetTokenByOutcome("NO")

	if yesToken == nil || noToken == nil {
		return fmt.Errorf("market %q missing YES or NO token", s.singleMarket)
	}

	// Mark as subscribed
	s.mu.Lock()
	s.subscribed[market.Slug] = &types.MarketSubscription{
		MarketID:     market.ID,
		MarketSlug:   market.Slug,
		Question:     market.Question,
		TokenIDYes:   yesToken.TokenID,
		TokenIDNo:    noToken.TokenID,
		SubscribedAt: time.Now(),
	}
	s.mu.Unlock()

	// Cache and send to channel
	s.cacheMarket(market)

	select {
	case s.newMarketsCh <- market:
		NewMarketsTotal.Inc()
		s.logger.Info("single-market-discovered",
			zap.String("slug", market.Slug),
			zap.String("question", market.Question))
	default:
		s.logger.Warn("new-markets-channel-full")
	}

	return nil
}

// identifyNewMarkets returns markets that haven't been subscribed yet.
func (s *Service) identifyNewMarkets(markets []types.Market) []*types.Market {
	s.mu.Lock()
	defer s.mu.Unlock()

	var newMarkets []*types.Market

	for i := range markets {
		market := &markets[i]

		// Check if already subscribed
		if _, exists := s.subscribed[market.Slug]; exists {
			continue
		}

		// Check if market is valid (has YES and NO tokens)
		yesToken := market.GetTokenByOutcome("YES")
		noToken := market.GetTokenByOutcome("NO")

		if yesToken == nil || noToken == nil {
			s.logger.Debug("skipping-market-missing-tokens",
				zap.String("market-id", market.ID),
				zap.String("question", market.Question))
			continue
		}

		// Mark as subscribed
		s.subscribed[market.Slug] = &types.MarketSubscription{
			MarketID:     market.ID,
			MarketSlug:   market.Slug,
			Question:     market.Question,
			TokenIDYes:   yesToken.TokenID,
			TokenIDNo:    noToken.TokenID,
			SubscribedAt: time.Now(),
		}

		newMarkets = append(newMarkets, market)
	}

	return newMarkets
}

// NewMarketsChan returns the channel for receiving new markets.
func (s *Service) NewMarketsChan() <-chan *types.Market {
	return s.newMarketsCh
}

// GetSubscribedMarkets returns all currently subscribed markets.
func (s *Service) GetSubscribedMarkets() []*types.MarketSubscription {
	s.mu.RLock()
	defer s.mu.RUnlock()

	subs := make([]*types.MarketSubscription, 0, len(s.subscribed))
	for _, sub := range s.subscribed {
		subs = append(subs, sub)
	}

	return subs
}

// cacheMarket stores a market in the cache.
func (s *Service) cacheMarket(market *types.Market) {
	if s.cache == nil {
		return
	}

	// Cache by market ID with 24 hour TTL
	const cacheTTL = 24 * time.Hour
	success := s.cache.Set(market.ID, market, cacheTTL)
	if !success {
		s.logger.Warn("failed-to-cache-market", zap.String("market-id", market.ID))
	}
}

// GetMarket retrieves a market from cache or returns nil if not found.
func (s *Service) GetMarket(marketID string) *types.Market {
	if s.cache == nil {
		return nil
	}

	value, found := s.cache.Get(marketID)
	if !found {
		return nil
	}

	market, ok := value.(*types.Market)
	if !ok {
		s.logger.Warn("invalid-market-type-in-cache",
			zap.String("market-id", marketID))
		return nil
	}

	return market
}
