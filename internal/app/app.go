package app

import (
	"context"
	"sync"

	"github.com/mselser95/polymarket-arb/internal/arbitrage"
	"github.com/mselser95/polymarket-arb/internal/discovery"
	"github.com/mselser95/polymarket-arb/internal/execution"
	"github.com/mselser95/polymarket-arb/internal/orderbook"
	"github.com/mselser95/polymarket-arb/pkg/config"
	"github.com/mselser95/polymarket-arb/pkg/healthprobe"
	"github.com/mselser95/polymarket-arb/pkg/httpserver"
	"github.com/mselser95/polymarket-arb/pkg/websocket"
	"go.uber.org/zap"
)

// App is the main application orchestrator.
type App struct {
	cfg              *config.Config
	logger           *zap.Logger
	healthChecker    *healthprobe.HealthChecker
	httpServer       *httpserver.Server
	discoveryService *discovery.Service
	wsManager        *websocket.Manager
	obManager        *orderbook.Manager
	arbDetector      *arbitrage.Detector
	executor         *execution.Executor
	storage          arbitrage.Storage
	ctx              context.Context
	cancel           context.CancelFunc
	wg               sync.WaitGroup
}

// Options holds application options.
type Options struct {
	SingleMarket string // For debugging: slug of single market to track
}
