package app

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// Shutdown gracefully shuts down the application.
func (a *App) Shutdown() error {
	a.logger.Info("application-shutting-down")

	a.healthChecker.SetReady(false)

	// Cancel context to signal all components
	a.cancel()

	// Shutdown components in dependency order
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	// Shutdown HTTP server
	err := a.shutdownHTTPServer(shutdownCtx)
	if err != nil {
		a.logger.Error("http-server-shutdown-error", zap.Error(err))
	}

	// Close executor
	err = a.shutdownExecutor()
	if err != nil {
		a.logger.Error("executor-close-error", zap.Error(err))
	}

	// Close arbitrage detector
	err = a.shutdownArbitrageDetector()
	if err != nil {
		a.logger.Error("arbitrage-detector-close-error", zap.Error(err))
	}

	// Close storage
	err = a.shutdownStorage()
	if err != nil {
		a.logger.Error("storage-close-error", zap.Error(err))
	}

	// Close orderbook manager
	err = a.shutdownOrderbookManager()
	if err != nil {
		a.logger.Error("orderbook-manager-close-error", zap.Error(err))
	}

	// Close WebSocket manager
	err = a.shutdownWebSocketManager()
	if err != nil {
		a.logger.Error("websocket-manager-close-error", zap.Error(err))
	}

	// Wait for all goroutines
	a.wg.Wait()

	a.logger.Info("application-shutdown-complete")

	return nil
}

func (a *App) shutdownHTTPServer(ctx context.Context) error {
	return a.httpServer.Shutdown(ctx)
}

func (a *App) shutdownExecutor() error {
	return a.executor.Close()
}

func (a *App) shutdownArbitrageDetector() error {
	return a.arbDetector.Close()
}

func (a *App) shutdownStorage() error {
	return a.storage.Close()
}

func (a *App) shutdownOrderbookManager() error {
	return a.obManager.Close()
}

func (a *App) shutdownWebSocketManager() error {
	return a.wsManager.Close()
}
