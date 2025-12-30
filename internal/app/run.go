package app

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
)

// Run starts the application and blocks until shutdown.
func (a *App) Run() error {
	a.logger.Info("application-starting",
		zap.String("mode", a.cfg.ExecutionMode),
		zap.Float64("arb-max-price-sum", a.cfg.ArbMaxPriceSum),
		zap.String("log-level", a.cfg.LogLevel))

	// Start all components
	err := a.startComponents()
	if err != nil {
		return err
	}

	// Mark as ready
	a.healthChecker.SetReady(true)

	a.logger.Info("application-ready",
		zap.String("http-addr", ":"+a.cfg.HTTPPort),
		zap.String("ws-url", a.cfg.PolymarketWSURL))

	// Wait for shutdown signal
	return a.waitForShutdown()
}

func (a *App) startComponents() error {
	// Start HTTP server
	a.wg.Add(1)
	go a.runHTTPServer()

	// Give HTTP server a moment to start
	time.Sleep(100 * time.Millisecond)

	// Start discovery service
	a.wg.Add(1)
	go a.runDiscoveryService()

	// Start WebSocket manager
	err := a.startWebSocketManager()
	if err != nil {
		return fmt.Errorf("start websocket manager: %w", err)
	}

	// Start market subscription handler
	a.wg.Add(1)
	go a.handleNewMarkets()

	// Start orderbook manager
	err = a.startOrderbookManager()
	if err != nil {
		return fmt.Errorf("start orderbook manager: %w", err)
	}

	// Start arbitrage detector
	err = a.startArbitrageDetector()
	if err != nil {
		return fmt.Errorf("start arbitrage detector: %w", err)
	}

	// Start executor
	err = a.startExecutor()
	if err != nil {
		return fmt.Errorf("start executor: %w", err)
	}

	return nil
}

func (a *App) runHTTPServer() {
	defer a.wg.Done()
	err := a.httpServer.Start()
	if err != nil {
		a.logger.Error("http-server-error", zap.Error(err))
	}
}

func (a *App) runDiscoveryService() {
	defer a.wg.Done()
	err := a.discoveryService.Run(a.ctx)
	if err != nil && !errors.Is(err, a.ctx.Err()) {
		a.logger.Error("discovery-service-error", zap.Error(err))
	}
}

func (a *App) startWebSocketManager() error {
	return a.wsPool.Start()
}

func (a *App) startOrderbookManager() error {
	return a.obManager.Start(a.ctx)
}

func (a *App) startArbitrageDetector() error {
	return a.arbDetector.Start(a.ctx)
}

func (a *App) startExecutor() error {
	if a.executor == nil {
		a.logger.Info("executor-not-started",
			zap.String("mode", a.cfg.ExecutionMode),
			zap.String("reason", "dry-run mode - detection only"))
		return nil
	}

	return a.executor.Start(a.ctx)
}

func (a *App) waitForShutdown() error {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigChan:
		a.logger.Info("shutdown-signal-received", zap.String("signal", sig.String()))
	case <-a.ctx.Done():
		a.logger.Info("context-cancelled")
	}

	return a.Shutdown()
}
