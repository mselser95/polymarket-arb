# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

High-frequency trading bot for detecting and executing arbitrage opportunities on Polymarket prediction markets. Uses event-driven architecture with WebSocket feeds, optimized for low latency (<1ms arbitrage detection).

**Key Arbitrage Strategy:** When YES best bid + NO best bid < threshold (typically 0.995), both positions guarantee profit since exactly one pays out $1.00.

## Architecture

### Event-Driven Design (Critical)

The bot is **fully event-driven** - no polling loops. All components react to channel events:

```
WebSocket → OrderbookManager → ArbitrageDetector → Executor
  (push)      (update events)   (opportunities)    (trades)
```

**Performance Optimizations:**
- Lock-free reads: Parse price strings outside mutex critical sections
- Non-blocking channels: Buffered with overflow handling (drop updates if consumer slow)
- Fast JSON: Uses `github.com/goccy/go-json` (2-3x faster than stdlib)
- Snapshot-only storage: Only track best bid/ask, not full orderbook depth

### Key Components

1. **Discovery Service** (`internal/discovery/`): Polls Gamma API for active markets, implements differential discovery
2. **WebSocket Manager** (`pkg/websocket/`): Persistent connection with auto-reconnect, exponential backoff
3. **Orderbook Manager** (`internal/orderbook/`): In-memory snapshots, emits update events to subscribers
4. **Arbitrage Detector** (`internal/arbitrage/`): Event-driven, checks opportunities on orderbook updates
5. **Executor** (`internal/execution/`): Paper/live trading modes, tracks cumulative profit

## Commands

### Development

```bash
# Build
make build
go build -o polymarket-arb .

# Run (paper trading, console output)
make run
go run . run

# Run single market (debugging)
go run . run --single-market <market-slug>

# List active markets
make list-markets
go run . list-markets --limit 20

# Watch orderbook updates
go run . watch-orderbook <market-slug>
```

### Linting

```bash
# Run golangci-lint (REQUIRED before commits)
make lint
golangci-lint run --timeout=5m ./...
```

**CRITICAL:** This project uses a strict `.golangci.yml` with 50+ enabled linters. All code must pass linting. Pay special attention to:
- `gochecknoglobals` / `gochecknoinits`: Use `//nolint` with explanation for Cobra boilerplate
- `funlen` / `gocognit`: Extract helper functions for complex logic
- `noinlineerr`: No inline error handling (`if err := ...; err != nil`)
- `protogetter`: Always use `Get()` methods for proto fields
- `intrange`: Use Go 1.22+ range syntax (`for range count`)
- `noctx`: Use `ListenConfig` for network listeners

### Testing

```bash
# Unit tests (fast, no external deps)
make test
go test -v -race -cover ./...

# Integration tests (E2E flows, tagged)
make test-integration
go test -tags=integration -v -race ./...

# Specific package
go test -v ./internal/arbitrage/

# Specific test
go test -v ./internal/app/ -run TestE2E_ArbitrageFlow

# Coverage report
make test-coverage
# Opens coverage.html

# Benchmarks
make test-bench
go test -bench=. -benchmem ./...
```

**Testing Patterns:**
- Unit tests: Mock dependencies at boundaries (HTTP, WebSocket), not internal logic
- Integration tests: Use `internal/testutil/` mocks (MockGammaAPI, MockStorage)
- Async testing: Use channel selects with timeouts, avoid `time.Sleep()` in unit tests
- Cache testing: Call `cache.Wait()` after `Set()` (Ristretto uses async admission)
- Race detector: Run with `-race` regularly to catch concurrency bugs

### Credentials & Authentication

```bash
# Derive Builder API credentials from private key
go run . derive-api-creds

# Approve USDC spending (one-time on-chain transaction)
go run . approve [--amount <USDC>] [--rpc <URL>]

# Check wallet balances (MATIC, USDC, positions)
go run . balance [--rpc <URL>]

# Track balance/P&L over time with Prometheus metrics
go run . track-balance                     # Update every 1 minute (default)
go run . track-balance --interval 30s      # Update every 30 seconds
go run . track-balance --port 8081         # Use custom port for metrics

# Test live order submission (⚠️ submits real orders!)
go run . test-live-order <market-slug> --size 1.0 --yes-price 0.01 --no-price 0.01
```

**Wallet Tracking (`track-balance`):**
- Continuously monitors wallet balance and positions
- Exposes Prometheus metrics at `http://localhost:8080/metrics`
- Updates every minute by default (configurable with `--interval`)
- Tracks: MATIC, USDC, allowances, position values, unrealized P&L, portfolio value
- Designed for Grafana dashboards and long-term monitoring
- No authentication required (uses Data API)
- Graceful shutdown on SIGINT/SIGTERM

**Required Environment Variables:**
```bash
# .env file (copy from .env.example)
POLYMARKET_PRIVATE_KEY=<hex_without_0x>  # For signing transactions/orders
POLYMARKET_API_KEY=<builder_api_key>     # From derive-api-creds or Polymarket UI
POLYMARKET_SECRET=<api_secret>
POLYMARKET_PASSPHRASE=<api_passphrase>
```

**Signature Types:**
- `POLYMARKET_SIGNATURE_TYPE=0`: EOA (default, most common)
- `POLYMARKET_SIGNATURE_TYPE=1`: POLY_PROXY
- `POLYMARKET_SIGNATURE_TYPE=2`: POLY_GNOSIS_SAFE

### Order Execution

```bash
# Place orders manually
go run . place-orders <market-slug> \
  --yes-price <price> --yes-size <size> \
  --no-price <price> --no-size <size>

# Execute arbitrage (if opportunity exists)
go run . execute-arb <market-slug> --size <size>
```

## Code Organization

### Package Structure

```
cmd/                    # CLI commands (Cobra)
  approve.go           # USDC approval transaction
  balance.go           # Wallet balance checks
  derive_api_creds.go  # Generate Builder API credentials
  execute_arb.go       # Manual arbitrage execution
  list_markets.go      # Market discovery
  place_orders.go      # Manual order placement
  track_balance.go     # Continuous wallet/P&L tracking with Prometheus metrics
  test_live_order.go   # Order submission testing
  watch_orderbook.go   # Real-time orderbook monitoring

internal/
  app/                 # Application lifecycle & orchestration
  arbitrage/           # Opportunity detection logic
  discovery/           # Market discovery service
  execution/           # Trade execution (paper/live)
  orderbook/           # Orderbook state management
  markets/             # Market metadata caching
  storage/             # Console & Postgres storage
  testutil/            # Test mocks & fixtures

pkg/
  cache/               # Ristretto cache wrapper
  config/              # Environment-based configuration
  healthprobe/         # Health checks
  httpserver/          # Metrics & health HTTP server
  types/               # Shared data types
  wallet/              # Wallet balance tracking with Prometheus metrics
  websocket/           # WebSocket client with reconnection
```

### Configuration (`pkg/config/`)

All config loaded via environment variables with defaults. See `LoadFromEnv()` for full list.

**Critical Settings:**
- `ARB_THRESHOLD=0.995`: Detect when YES + NO < threshold (accounting for fees)
- `ARB_MIN_TRADE_SIZE=10.0`: Minimum $10 trade
- `ARB_TAKER_FEE=0.01`: Polymarket charges 1% taker fee
- `EXECUTION_MODE=paper`: paper (logs only) or live (real trades)
- `STORAGE_MODE=console`: console (stdout) or postgres

## Blockchain Integration

**On-Chain vs Off-Chain:**
- **On-chain (one-time):** USDC approval via `approve` command (Polygon mainnet)
- **Off-chain (all trades):** Order placement via Builder API signatures (no gas fees)

**Contract Addresses (Polygon):**
- USDC: `0x2791Bca1f2de4661ED88A30C99A7a9449Aa84174`
- CTF Exchange: `0x4bFb41d5B3570DeFd03C39a9A4D8dE6Bd8B8982E`

**Order Signing:**
- Uses `github.com/polymarket/go-order-utils` for EIP-712 signature generation
- Signs with `POLYMARKET_PRIVATE_KEY` (ECDSA)
- Submits signed orders to CLOB API (no blockchain transaction)

## Common Development Tasks

### Adding a New CLI Command

1. Create `cmd/<command>.go` with Cobra command struct
2. Add command to `rootCmd` in `init()` function
3. Use `//nolint:gochecknoglobals,gochecknoinits` comments for Cobra boilerplate
4. Load config via `config.LoadFromEnv()` or `.env` file
5. Use structured logging with `zap.Logger`

### Modifying Orderbook Processing

**CRITICAL:** Orderbook updates are in hot path (1000+ msg/sec). Optimize carefully:
- Parse price strings **outside** mutex locks
- Use non-blocking channel sends with `select/default`
- Return copies, not pointers to internal state
- Emit events to subscribers via buffered channels

### Adding New Linters

This project is strict on code quality. When adding new checks:
- Update `.golangci.yml` with linter config
- Run `make lint` to catch violations
- Extract helper functions for high complexity
- Use `//nolint:<linter> // reason` sparingly with explanation

### Debugging WebSocket Issues

- Check `websocket.Manager` logs for connection state
- Monitor `orderbook_updates_total` metric (Prometheus)
- Use `watch-orderbook` command to see live messages
- Verify subscription messages in WebSocket logs

### Testing Async/Event-Driven Code

**Pattern for integration tests:**
```go
// Start components
obMgr.Start(ctx)
detector.Start(ctx)

// Send input event
wsMsgChan <- orderBookMsg

// Wait for output with timeout
select {
case opp := <-detector.OpportunityChan():
    // Success - verify opportunity
case <-time.After(1 * time.Second):
    t.Fatal("timeout waiting for opportunity")
}
```

**Avoid:**
- Polling loops in unit tests
- Fixed `time.Sleep()` delays (use channels)
- Asserting on exact timing (flaky in CI)

## Important Implementation Details

### Cobra Command Structure

All CLI commands follow this pattern:
```go
//nolint:gochecknoglobals // Cobra boilerplate
var commandCmd = &cobra.Command{
    Use:   "command",
    Short: "Brief description",
    Long:  "Detailed description...",
    RunE:  runCommand,
}

//nolint:gochecknoinits // Cobra boilerplate
func init() {
    rootCmd.AddCommand(commandCmd)
    commandCmd.Flags().StringVarP(&flag, "flag", "f", "default", "description")
}
```

### Ethereum Transaction Pattern

When sending on-chain transactions:
1. Load private key: `crypto.HexToECDSA(privateKeyHex)`
2. Get nonce: `client.PendingNonceAt(ctx, address)`
3. Get gas price: `client.SuggestGasPrice(ctx)`
4. Build transaction with EIP-155 signer (chainID 137 for Polygon)
5. Sign with `types.SignTx(tx, signer, privateKey)`
6. Send and wait: `client.SendTransaction()` then `bind.WaitMined()`

### Order Submission Pattern

For off-chain order placement:
1. Fetch market metadata (token IDs) from Gamma API
2. Build order struct with price/size/expiration
3. Sign with `go-order-utils` EIP-712 domain + types
4. Submit to CLOB API: `POST https://clob.polymarket.com/order`
5. Parse response for order ID or error details

### Error Handling

- All errors wrapped with context: `fmt.Errorf("action failed: %w", err)`
- No inline error handling: split `err := f()` and `if err != nil` to separate lines
- Log errors with structured context: `logger.Error("msg", zap.Error(err), zap.String("market", slug))`

## Metrics & Observability

**Prometheus metrics exposed on `:8080/metrics`:**

*Arbitrage Bot:*
- `orderbook_updates_total{event_type}`: WebSocket message counts
- `arbitrage_opportunities_detected_total`: Opportunity count
- `arbitrage_opportunity_profit_bps`: Profit in basis points
- `execution_trades_total{mode}`: Trade counts by mode
- `execution_profit_usd_total{mode}`: Cumulative profit

*Wallet Tracker (`track-balance` command):*
- `polymarket_wallet_matic_balance`: MATIC balance for gas
- `polymarket_wallet_usdc_balance`: USDC balance for trading
- `polymarket_wallet_usdc_allowance`: USDC approved to CTF Exchange
- `polymarket_wallet_active_positions`: Number of open positions
- `polymarket_wallet_total_position_value`: Sum of position values (USD)
- `polymarket_wallet_total_position_cost`: Sum of initial costs (USD)
- `polymarket_wallet_unrealized_pnl`: Total unrealized P&L (USD)
- `polymarket_wallet_unrealized_pnl_percent`: P&L percentage
- `polymarket_wallet_portfolio_value`: USDC + positions (USD)
- `polymarket_wallet_update_errors_total`: Failed update attempts
- `polymarket_wallet_update_duration_seconds`: Update latency
- `polymarket_wallet_last_update_timestamp`: Unix timestamp of last update

**Health check:** `GET http://localhost:8080/health`

**Debugging:**
- Set `LOG_LEVEL=debug` for verbose logging
- Check Prometheus metrics for throughput
- Use pprof for profiling: `go run . --cpuprofile cpu.prof`

## Security Considerations

- **Never commit `.env` file** - contains private keys
- Private keys stored without `0x` prefix
- All API credentials from environment variables
- On-chain transactions require MATIC for gas (~$0.01)
- Off-chain orders are free (no gas)
- Use low prices (0.01) when testing order submission to avoid accidental fills

## External Dependencies

**Critical Libraries:**
- `github.com/goccy/go-json`: Fast JSON parser (hot path)
- `github.com/polymarket/go-order-utils`: EIP-712 order signing
- `github.com/ethereum/go-ethereum`: Ethereum client & crypto
- `github.com/gorilla/websocket`: WebSocket client
- `github.com/dgraph-io/ristretto`: High-performance cache
- `github.com/spf13/cobra`: CLI framework
- `go.uber.org/zap`: Structured logging

## References

- README.md: High-level architecture and performance characteristics
- TESTING.md: Comprehensive testing guide with patterns and examples
- .golangci.yml: Linter configuration (strict, 50+ enabled linters)
- CREDENTIALS_TROUBLESHOOTING.md: Authentication debugging guide
