package storage

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
	"github.com/mselser95/polymarket-arb/internal/arbitrage"
	"go.uber.org/zap"
)

// PostgresStorage implements Storage using PostgreSQL.
type PostgresStorage struct {
	db     *sql.DB
	logger *zap.Logger
}

// PostgresConfig holds PostgreSQL configuration.
type PostgresConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Database string
	SSLMode  string
	Logger   *zap.Logger
}

// NewPostgresStorage creates a new PostgreSQL storage.
func NewPostgresStorage(cfg *PostgresConfig) (*PostgresStorage, error) {
	connStr := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Database, cfg.SSLMode,
	)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Test connection
	err = db.Ping()
	if err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	cfg.Logger.Info("postgres-storage-connected",
		zap.String("host", cfg.Host),
		zap.String("database", cfg.Database))

	return &PostgresStorage{
		db:     db,
		logger: cfg.Logger,
	}, nil
}

// StoreOpportunity stores an arbitrage opportunity in PostgreSQL.
// NOTE: Postgres schema needs migration to support multi-outcome markets properly.
// For now, storing summary data only (no individual outcome details).
func (p *PostgresStorage) StoreOpportunity(ctx context.Context, opp *arbitrage.Opportunity) error {
	// For backward compatibility with existing schema, store binary-equivalent data
	// TODO: Migrate schema to support N outcomes with JSONB column
	var firstPrice, secondPrice, firstSize, secondSize float64
	if len(opp.Outcomes) >= 2 {
		firstPrice = opp.Outcomes[0].AskPrice
		firstSize = opp.Outcomes[0].AskSize
		secondPrice = opp.Outcomes[1].AskPrice
		secondSize = opp.Outcomes[1].AskSize
	}

	query := `
		INSERT INTO arbitrage_opportunities (
			id, market_id, market_slug, market_question, detected_at,
			yes_bid_price, yes_bid_size, no_bid_price, no_bid_size,
			price_sum, profit_margin, profit_bps, max_trade_size,
			estimated_profit, total_fees, net_profit, net_profit_bps,
			config_threshold
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18
		)
	`

	_, err := p.db.ExecContext(ctx, query,
		opp.ID,
		opp.MarketID,
		opp.MarketSlug,
		opp.MarketQuestion,
		opp.DetectedAt,
		firstPrice,  // Reuse yes_bid_price column for first outcome
		firstSize,   // Reuse yes_bid_size column for first outcome
		secondPrice, // Reuse no_bid_price column for second outcome
		secondSize,  // Reuse no_bid_size column for second outcome
		opp.TotalPriceSum,
		opp.ProfitMargin,
		opp.ProfitBPS,
		opp.MaxTradeSize,
		opp.EstimatedProfit,
		opp.TotalFees,
		opp.NetProfit,
		opp.NetProfitBPS,
		opp.ConfigMaxPriceSum,
	)

	if err != nil {
		return fmt.Errorf("insert opportunity: %w", err)
	}

	p.logger.Debug("opportunity-stored",
		zap.String("opportunity-id", opp.ID),
		zap.String("market-slug", opp.MarketSlug),
		zap.Int("outcome-count", len(opp.Outcomes)))

	return nil
}

// Close closes the database connection.
func (p *PostgresStorage) Close() error {
	p.logger.Info("closing-postgres-storage")
	return p.db.Close()
}
