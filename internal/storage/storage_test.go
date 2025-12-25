package storage

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/mselser95/polymarket-arb/internal/arbitrage"
	"go.uber.org/zap"
)

// Helper function to create a test opportunity
func createTestOpportunity() *arbitrage.Opportunity {
	return &arbitrage.Opportunity{
		ID:               "test-opp-123",
		MarketID:         "market-123",
		MarketSlug:       "test-market",
		MarketQuestion:   "Will X happen?",
		YesTokenID:       "test-yes-token-123",
		NoTokenID:        "test-no-token-123",
		DetectedAt:       time.Now(),
		YesAskPrice:      0.48,
		YesAskSize:       100.0,
		NoAskPrice:       0.51,
		NoAskSize:        100.0,
		PriceSum:         0.99,
		ProfitMargin:     0.01,
		ProfitBPS:        100,
		MaxTradeSize:     100.0,
		EstimatedProfit:  1.0,
		TotalFees:        0.2,
		NetProfit:        0.8,
		NetProfitBPS:     80,
		ConfigThreshold:  0.995,
	}
}

// TestConsoleStorage tests the console storage implementation
func TestConsoleStorage_New(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	storage := NewConsoleStorage(logger)

	if storage == nil {
		t.Fatal("expected non-nil storage")
	}

	if storage.logger == nil {
		t.Error("expected non-nil logger")
	}
}

func TestConsoleStorage_StoreOpportunity(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	storage := NewConsoleStorage(logger)

	opp := createTestOpportunity()
	ctx := context.Background()

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := storage.StoreOpportunity(ctx, opp)

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Read captured output
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	// Verify output contains expected information
	if !bytes.Contains([]byte(output), []byte("ARBITRAGE OPPORTUNITY DETECTED")) {
		t.Error("expected output to contain 'ARBITRAGE OPPORTUNITY DETECTED'")
	}

	if !bytes.Contains([]byte(output), []byte(opp.MarketSlug)) {
		t.Errorf("expected output to contain market slug %s", opp.MarketSlug)
	}

	if !bytes.Contains([]byte(output), []byte(opp.MarketQuestion)) {
		t.Errorf("expected output to contain market question %s", opp.MarketQuestion)
	}
}

func TestConsoleStorage_Close(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	storage := NewConsoleStorage(logger)

	err := storage.Close()
	if err != nil {
		t.Errorf("expected no error on close, got %v", err)
	}
}

// TestPostgresStorage tests the PostgreSQL storage implementation using sqlmock
func TestPostgresStorage_StoreOpportunity(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// Create mock database
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	storage := &PostgresStorage{
		db:     db,
		logger: logger,
	}

	opp := createTestOpportunity()
	ctx := context.Background()

	// Expect INSERT query
	mock.ExpectExec("INSERT INTO arbitrage_opportunities").
		WithArgs(
			opp.ID,
			opp.MarketID,
			opp.MarketSlug,
			opp.MarketQuestion,
			sqlmock.AnyArg(), // DetectedAt (time.Time is tricky)
			opp.YesAskPrice,
			opp.YesAskSize,
			opp.NoAskPrice,
			opp.NoAskSize,
			opp.PriceSum,
			opp.ProfitMargin,
			opp.ProfitBPS,
			opp.MaxTradeSize,
			opp.EstimatedProfit,
			opp.TotalFees,
			opp.NetProfit,
			opp.NetProfitBPS,
			opp.ConfigThreshold,
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = storage.StoreOpportunity(ctx, opp)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	// Verify all expectations met
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestPostgresStorage_StoreOpportunity_Error(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// Create mock database
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	storage := &PostgresStorage{
		db:     db,
		logger: logger,
	}

	opp := createTestOpportunity()
	ctx := context.Background()

	// Expect INSERT query to fail
	mock.ExpectExec("INSERT INTO arbitrage_opportunities").
		WithArgs(
			opp.ID,
			opp.MarketID,
			opp.MarketSlug,
			opp.MarketQuestion,
			sqlmock.AnyArg(),
			opp.YesAskPrice,
			opp.YesAskSize,
			opp.NoAskPrice,
			opp.NoAskSize,
			opp.PriceSum,
			opp.ProfitMargin,
			opp.ProfitBPS,
			opp.MaxTradeSize,
			opp.EstimatedProfit,
			opp.TotalFees,
			opp.NetProfit,
			opp.NetProfitBPS,
			opp.ConfigThreshold,
		).
		WillReturnError(sqlmock.ErrCancelled)

	err = storage.StoreOpportunity(ctx, opp)
	if err == nil {
		t.Error("expected error, got nil")
	}

	// Verify all expectations met
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestPostgresStorage_Close(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	// Create mock database
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}

	storage := &PostgresStorage{
		db:     db,
		logger: logger,
	}

	mock.ExpectClose()

	err = storage.Close()
	if err != nil {
		t.Errorf("expected no error on close, got %v", err)
	}

	// Verify all expectations met
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestNewPostgresStorage_ConnectionSuccess(t *testing.T) {
	// This test requires actual database connection, so it's skipped in unit tests
	t.Skip("Requires actual PostgreSQL database")

	logger, _ := zap.NewDevelopment()

	cfg := &PostgresConfig{
		Host:     "localhost",
		Port:     "5432",
		User:     "test",
		Password: "test",
		Database: "test_db",
		SSLMode:  "disable",
		Logger:   logger,
	}

	storage, err := NewPostgresStorage(cfg)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if storage == nil {
		t.Fatal("expected non-nil storage")
	}

	if storage.db == nil {
		t.Error("expected non-nil database connection")
	}

	if storage.logger == nil {
		t.Error("expected non-nil logger")
	}

	storage.Close()
}

func TestPostgresStorage_QueryStructure(t *testing.T) {
	// Test that the INSERT query has correct number of parameters
	logger, _ := zap.NewDevelopment()

	// Create mock database
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	storage := &PostgresStorage{
		db:     db,
		logger: logger,
	}

	opp := createTestOpportunity()
	ctx := context.Background()

	// Expect INSERT with exact parameter count (18 parameters)
	mock.ExpectExec("INSERT INTO arbitrage_opportunities").
		WithArgs(
			sqlmock.AnyArg(), // 1: ID
			sqlmock.AnyArg(), // 2: MarketID
			sqlmock.AnyArg(), // 3: MarketSlug
			sqlmock.AnyArg(), // 4: MarketQuestion
			sqlmock.AnyArg(), // 5: DetectedAt
			sqlmock.AnyArg(), // 6: YesAskPrice
			sqlmock.AnyArg(), // 7: YesAskSize
			sqlmock.AnyArg(), // 8: NoAskPrice
			sqlmock.AnyArg(), // 9: NoAskSize
			sqlmock.AnyArg(), // 10: PriceSum
			sqlmock.AnyArg(), // 11: ProfitMargin
			sqlmock.AnyArg(), // 12: ProfitBPS
			sqlmock.AnyArg(), // 13: MaxTradeSize
			sqlmock.AnyArg(), // 14: EstimatedProfit
			sqlmock.AnyArg(), // 15: TotalFees
			sqlmock.AnyArg(), // 16: NetProfit
			sqlmock.AnyArg(), // 17: NetProfitBPS
			sqlmock.AnyArg(), // 18: ConfigThreshold
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = storage.StoreOpportunity(ctx, opp)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	// Verify all expectations met
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestStorage_Interface(t *testing.T) {
	// Verify both implementations satisfy the Storage interface
	logger, _ := zap.NewDevelopment()

	var _ Storage = NewConsoleStorage(logger)

	db, _, _ := sqlmock.New()
	defer db.Close()

	var _ Storage = &PostgresStorage{db: db, logger: logger}
}
