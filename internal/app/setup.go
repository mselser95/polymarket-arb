package app

import (
	"context"
	"fmt"

	"github.com/mselser95/polymarket-arb/internal/arbitrage"
	"github.com/mselser95/polymarket-arb/internal/discovery"
	"github.com/mselser95/polymarket-arb/internal/execution"
	"github.com/mselser95/polymarket-arb/internal/orderbook"
	"github.com/mselser95/polymarket-arb/internal/storage"
	"github.com/mselser95/polymarket-arb/pkg/cache"
	"github.com/mselser95/polymarket-arb/pkg/config"
	"github.com/mselser95/polymarket-arb/pkg/healthprobe"
	"github.com/mselser95/polymarket-arb/pkg/httpserver"
	"github.com/mselser95/polymarket-arb/pkg/websocket"
	"go.uber.org/zap"
)

// New creates a new application instance.
func New(cfg *config.Config, logger *zap.Logger, opts *Options) (*App, error) {
	if opts == nil {
		opts = &Options{}
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Initialize components
	healthChecker := setupHealthChecker()
	httpServer := setupHTTPServer(cfg, logger, healthChecker)

	// Setup cache
	marketCache, err := setupCache(logger)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("setup cache: %w", err)
	}

	discoveryService := setupDiscoveryService(cfg, logger, marketCache, opts)
	wsManager := setupWebSocketManager(cfg, logger)
	obManager := setupOrderbookManager(logger, wsManager)

	// Setup storage
	arbStorage, err := setupStorage(cfg, logger)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("setup storage: %w", err)
	}

	// Setup arbitrage detector
	arbDetector := setupArbitrageDetector(cfg, logger, obManager, discoveryService, arbStorage)

	// Setup executor
	executor := setupExecutor(cfg, logger, arbDetector)

	return &App{
		cfg:              cfg,
		logger:           logger,
		healthChecker:    healthChecker,
		httpServer:       httpServer,
		discoveryService: discoveryService,
		wsManager:        wsManager,
		obManager:        obManager,
		arbDetector:      arbDetector,
		executor:         executor,
		storage:          arbStorage,
		ctx:              ctx,
		cancel:           cancel,
	}, nil
}

func setupHealthChecker() *healthprobe.HealthChecker {
	return healthprobe.New()
}

func setupHTTPServer(cfg *config.Config, logger *zap.Logger, healthChecker *healthprobe.HealthChecker) *httpserver.Server {
	return httpserver.New(&httpserver.Config{
		Port:          cfg.HTTPPort,
		Logger:        logger,
		HealthChecker: healthChecker,
	})
}

func setupCache(logger *zap.Logger) (cache.Cache, error) {
	return cache.NewRistrettoCache(&cache.RistrettoConfig{
		NumCounters: 10000,    // 10x expected max items (1000 markets)
		MaxCost:     1000,     // Maximum 1000 items in cache
		BufferItems: 64,       // Buffer size for Get operations
		Logger:      logger,
	})
}

func setupDiscoveryService(cfg *config.Config, logger *zap.Logger, marketCache cache.Cache, opts *Options) *discovery.Service {
	discoveryClient := discovery.NewClient(cfg.PolymarketGammaURL, logger)
	return discovery.New(&discovery.Config{
		Client:       discoveryClient,
		Cache:        marketCache,
		PollInterval: cfg.DiscoveryPollInterval,
		MarketLimit:  cfg.DiscoveryMarketLimit,
		Logger:       logger,
		SingleMarket: opts.SingleMarket,
	})
}

func setupWebSocketManager(cfg *config.Config, logger *zap.Logger) *websocket.Manager {
	return websocket.New(websocket.Config{
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
}

func setupOrderbookManager(logger *zap.Logger, wsManager *websocket.Manager) *orderbook.Manager {
	return orderbook.New(&orderbook.Config{
		Logger:         logger,
		MessageChannel: wsManager.MessageChan(),
	})
}

func setupStorage(cfg *config.Config, logger *zap.Logger) (arbitrage.Storage, error) {
	if cfg.StorageMode == "postgres" {
		pgStorage, err := storage.NewPostgresStorage(&storage.PostgresConfig{
			Host:     cfg.PostgresHost,
			Port:     cfg.PostgresPort,
			User:     cfg.PostgresUser,
			Password: cfg.PostgresPass,
			Database: cfg.PostgresDB,
			SSLMode:  cfg.PostgresSSL,
			Logger:   logger,
		})
		if err != nil {
			return nil, fmt.Errorf("create postgres storage: %w", err)
		}
		return pgStorage, nil
	}

	return storage.NewConsoleStorage(logger), nil
}

func setupArbitrageDetector(
	cfg *config.Config,
	logger *zap.Logger,
	obManager *orderbook.Manager,
	discoveryService *discovery.Service,
	arbStorage arbitrage.Storage,
) *arbitrage.Detector {
	return arbitrage.New(
		arbitrage.Config{
			Threshold:    cfg.ArbThreshold,
			MinTradeSize: cfg.ArbMinTradeSize,
			TakerFee:     cfg.ArbTakerFee,
			Logger:       logger,
		},
		obManager,
		discoveryService,
		arbStorage,
	)
}

func setupExecutor(cfg *config.Config, logger *zap.Logger, arbDetector *arbitrage.Detector) *execution.Executor {
	return execution.New(&execution.Config{
		Mode:               cfg.ExecutionMode,
		MaxPositionSize:    cfg.ExecutionMaxPositionSize,
		Logger:             logger,
		OpportunityChannel: arbDetector.OpportunityChan(),
	})
}
