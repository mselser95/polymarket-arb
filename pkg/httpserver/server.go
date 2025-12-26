package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/mselser95/polymarket-arb/internal/discovery"
	"github.com/mselser95/polymarket-arb/internal/orderbook"
	"github.com/mselser95/polymarket-arb/pkg/healthprobe"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

// Server provides HTTP endpoints for metrics and health checks.
type Server struct {
	server        *http.Server
	logger        *zap.Logger
	healthChecker *healthprobe.HealthChecker
}

// Config holds server configuration.
type Config struct {
	Port             string
	Logger           *zap.Logger
	HealthChecker    *healthprobe.HealthChecker
	OrderbookManager *orderbook.Manager
	DiscoveryService *discovery.Service
}

// New creates a new HTTP server.
func New(cfg *Config) *Server {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	// Routes
	r.Get("/metrics", promhttp.Handler().ServeHTTP)
	r.Get("/health", cfg.HealthChecker.Health())
	r.Get("/ready", cfg.HealthChecker.Ready())

	// Orderbook API endpoint (if components provided)
	if cfg.OrderbookManager != nil && cfg.DiscoveryService != nil {
		obHandler := NewOrderbookHandler(cfg.OrderbookManager, cfg.DiscoveryService, cfg.Logger)
		r.Get("/api/orderbook", obHandler.HandleOrderbook)
	}

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           r,
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	return &Server{
		server:        server,
		logger:        cfg.Logger,
		healthChecker: cfg.HealthChecker,
	}
}

// Start starts the HTTP server.
// This is a blocking call that returns when the server stops or encounters an error.
func (s *Server) Start() error {
	s.logger.Info("http-server-starting", zap.String("addr", s.server.Addr))

	err := s.server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("listen and serve: %w", err)
	}

	return nil
}

// Shutdown gracefully shuts down the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("http-server-shutting-down")

	err := s.server.Shutdown(ctx)
	if err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}

	s.logger.Info("http-server-shutdown-complete")
	return nil
}
