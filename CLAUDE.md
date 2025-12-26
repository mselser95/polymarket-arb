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

### WebSocket Connection Architecture

**Connection Model:**
- **Single persistent WebSocket connection** for all markets
- All token subscriptions multiplexed over one connection
- No connection pooling or cycling - one connection serves all data

**Subscription Flow:**
```
Discovery → Manager.Subscribe(tokenIDs) → WebSocket
     ↓                                           ↓
New Markets                              Single Connection
     ↓                                           ↓
TokenIDs: [YES, NO] × N markets         Multiplexed Stream
```

**Key Characteristics:**
- Each market requires 2 token subscriptions (YES + NO outcome tokens)
- Initial subscription uses `{"assets_ids": [...], "type": "market"}` message
- Dynamic subscriptions (adding markets) use `{"assets_ids": [...], "operation": "subscribe"}` message
- Messages received: `book` (full snapshot), `price_change` (incremental update), heartbeats (empty array `[]`)
- Heartbeats: Server sends empty arrays or minimal content periodically to keep connection alive

**Reconnection Strategy:**
- Exponential backoff: starts at 1s, doubles each attempt, caps at 30s
- Jitter (20%) added to prevent thundering herd
- On reconnect: automatically resubscribes to all previously tracked tokens
- Connection tracking: Prometheus metrics for active connections, duration, reconnect attempts

**Implementation Details** (`pkg/websocket/manager.go`):
- Single `Manager` struct per application instance
- `conn` field holds one `*websocket.Conn`
- `subscribed map[string]bool` tracks all active token subscriptions
- Reconnection handled in dedicated `reconnectLoop()` goroutine
- Message parsing: Attempts to unmarshal as `[]OrderbookMessage`, falls back to control message detection

**Scaling Implications:**
- No per-market connection overhead
- Memory usage: O(1) connections + O(N) token subscriptions
- Network: Single TCP connection reduces handshake overhead
- Limitation: All markets share same connection bandwidth (not an issue for orderbook updates)

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

### Position Redemption

After markets settle, winning positions can be redeemed for USDC at 1:1 ratio by calling the CTF contract's `redeemPositions` function.

```bash
# Preview redeemable positions (dry-run mode)
go run . redeem-positions --dry-run

# Redeem all settled positions
go run . redeem-positions

# Redeem specific market only
go run . redeem-positions --market <market-slug>

# Use custom RPC endpoint
go run . redeem-positions --rpc https://polygon-mainnet.g.alchemy.com/v2/YOUR_KEY
```

**Requirements:**
- `POLYMARKET_PRIVATE_KEY` in `.env`
- MATIC balance for gas (~$0.01 per market)
- Positions in settled markets (`closed=true`)

**How it works:**
1. Fetches all positions from Data API
2. Checks each market's settlement status via Gamma API
3. For settled markets, builds and signs `redeemPositions` transaction
4. Submits transaction to Polygon mainnet
5. Waits for confirmation and displays results

**On-chain details:**
- CTF Contract: `0x4bFb41d5B3570DeFd03C39a9A4D8dE6Bd8B8982E`
- Chain ID: 137 (Polygon)
- Gas limit: ~200,000 per redemption
- Winning outcome tokens are burned and USDC is released

## Code Organization

### Package Structure

```
cmd/                    # CLI commands (Cobra)
  approve.go           # USDC approval transaction
  balance.go           # Wallet balance checks
  derive_api_creds.go  # Generate Builder API credentials
  execute_arb.go       # Manual arbitrage execution
  redeem_positions.go  # Claim settled positions for USDC
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
- `ARB_MIN_TRADE_SIZE=1.0`: Minimum $1 trade (must meet per-market minimums)
- `ARB_MAX_TRADE_SIZE=2.0`: Maximum $2 trade (caps calculated size from orderbook)
- `ARB_TAKER_FEE=0.01`: Polymarket charges 1% taker fee
- `ARB_MAX_MARKET_DURATION=1h`: Only subscribe to markets expiring within this window (filters long-running markets)
- `EXECUTION_MODE=dry-run`: dry-run (detect only), paper (simulated), or live (real trades)
- `STORAGE_MODE=console`: console (stdout) or postgres

**Circuit Breaker (Balance Protection):**

The circuit breaker automatically stops trade execution when wallet balance is low and resumes when balance returns. It dynamically calculates thresholds based on your actual trading patterns.

- `CIRCUIT_BREAKER_ENABLED=true`: Enable/disable circuit breaker (default: true)
- `CIRCUIT_BREAKER_CHECK_INTERVAL=300s`: How often to check balance (default: 5 minutes)
- `CIRCUIT_BREAKER_TRADE_MULTIPLIER=3.0`: Disable threshold = avg trade size × multiplier (default: 3.0)
- `CIRCUIT_BREAKER_MIN_ABSOLUTE=5.0`: Absolute minimum USDC balance floor (default: $5)
- `CIRCUIT_BREAKER_HYSTERESIS_RATIO=1.5`: Re-enable at disable threshold × ratio (default: 1.5)
- `POLYGON_RPC_URL=https://polygon-rpc.com`: RPC endpoint for balance checks (optional)

**How it works:**
1. Tracks last 20 trades to calculate average trade size
2. Sets disable threshold: `max(avg_trade_size × multiplier, min_absolute)`
3. Sets enable threshold: `disable_threshold × hysteresis_ratio`
4. Background monitor checks balance every 5 minutes
5. Disables execution if balance < disable threshold
6. Re-enables execution if balance >= enable threshold

**Example scenario:**
- Your average trade: $10
- Multiplier: 3.0 → Disable at $30
- Hysteresis: 1.5 → Re-enable at $45
- This prevents execution with < $30, resumes at $45+

**Configuration profiles:**

*Conservative (keep large buffer):*
```bash
CIRCUIT_BREAKER_ENABLED=true
CIRCUIT_BREAKER_TRADE_MULTIPLIER=5.0      # 5x avg trade
CIRCUIT_BREAKER_MIN_ABSOLUTE=10.0          # $10 floor
CIRCUIT_BREAKER_HYSTERESIS_RATIO=2.0       # 2x to re-enable
```

*Aggressive (use more funds):*
```bash
CIRCUIT_BREAKER_ENABLED=true
CIRCUIT_BREAKER_TRADE_MULTIPLIER=2.0       # 2x avg trade
CIRCUIT_BREAKER_MIN_ABSOLUTE=3.0           # $3 floor
CIRCUIT_BREAKER_HYSTERESIS_RATIO=1.3       # 1.3x to re-enable
```

**Metrics exposed:**
- `polymarket_circuit_breaker_enabled`: Current state (1=enabled, 0=disabled)
- `polymarket_circuit_breaker_balance_usdc`: Last checked balance
- `polymarket_circuit_breaker_disable_threshold_usdc`: Current disable threshold
- `polymarket_circuit_breaker_enable_threshold_usdc`: Current enable threshold
- `polymarket_circuit_breaker_avg_trade_size_usdc`: Rolling average trade size
- `polymarket_circuit_breaker_state_changes_total`: Number of state changes
- `polymarket_execution_opportunities_skipped_total{reason="circuit_breaker"}`: Skipped opportunities

**Disabling the circuit breaker:**
```bash
CIRCUIT_BREAKER_ENABLED=false
```

### Why Fewer Subscriptions Than Markets Discovered?

The bot uses a three-layer filtering system:

1. **API Fetch** (`DISCOVERY_MARKET_LIMIT`): Fetches up to N markets from Gamma API
2. **Duration Filter** (`ARB_MAX_MARKET_DURATION`): Keeps only markets expiring soon
3. **Subscription**: Subscribes to filtered markets (2 tokens per market)

**Example with defaults:**
- Gamma API: 1,800 total active markets
- Fetched: 1000 markets (DISCOVERY_MARKET_LIMIT)
- Duration filter: 50 markets expire within 1 hour (ARB_MAX_MARKET_DURATION)
- Subscriptions: 100 WebSocket subs (50 markets × 2 tokens)

**Why focus on short-duration markets?**
- Higher arbitrage opportunity frequency
- More price volatility near expiry
- Lower WebSocket bandwidth usage
- Better capital efficiency

**To increase subscriptions:**
```bash
# Increase duration window
ARB_MAX_MARKET_DURATION=6h  # Subscribe to markets expiring within 6 hours

# Or disable duration filter entirely
ARB_MAX_MARKET_DURATION=720h  # 30 days (effectively no filter)
```

**Warning:** More subscriptions = more data = more CPU/memory usage

### Execution Modes

The bot supports three execution modes, controlled by the `EXECUTION_MODE` environment variable.

#### dry-run: Detection Only Mode

**What runs:**
- Discovery Service ✅
- WebSocket Manager ✅
- Orderbook Manager ✅
- Arbitrage Detector ✅
- Storage (logs opportunities) ✅
- Executor ❌ (nil, not initialized)

**Architecture:**
```
WebSocket → Orderbook → Detector → Storage (console/postgres)
                            ↓
                    (logs opportunity)
                    [Executor disabled]
```

**What you see:**
```
========================================
Arbitrage Opportunity Detected
========================================
ID:               opp_abc123
Market:           will-bitcoin-hit-100k
Prices:
  YES ASK:        0.4800
  NO ASK:         0.4800
Profit:           $39.04 (390 bps)
========================================
```

**Metrics tracked:**
- `arbitrage_opportunities_detected_total`
- `arbitrage_opportunity_profit_bps`
- `orderbook_updates_total`

**Use cases:**
- Market research: "How many opportunities per day?"
- Configuration testing: "Is my threshold too strict?"
- Detection debugging: "Why aren't opportunities detected?"
- Safe monitoring: Zero risk, no credentials needed

---

#### paper: Simulation Mode

**What runs:**
- All dry-run components ✅
- Executor ✅ (simulates trades)

**Architecture:**
```
WebSocket → Orderbook → Detector → Storage
                            ↓
                       Opportunity
                            ↓
                       Executor.executePaper()
                            ↓
                    (logs simulated trade)
```

**What you see:**
```
📝 PAPER TRADE EXECUTED
Market: will-bitcoin-hit-100k
YES: Buy 22.22 tokens @ $0.45 = $10.00
NO:  Buy 20.00 tokens @ $0.50 = $10.00
Profit: $0.60 (3%)
Cumulative: $12.45
```

**Metrics tracked:**
- All detector metrics +
- `execution_trades_total{mode="paper"}`
- `execution_profit_usd_total{mode="paper"}`

**Use cases:**
- Strategy validation: "Is this profitable over time?"
- Execution logic testing: "Does the executor handle all cases?"
- Performance benchmarking: "How many trades per hour?"
- Learning: Safe way to understand the full flow

---

#### live: Real Trading Mode

**What runs:**
- All paper mode components ✅
- OrderClient ✅ (real API calls)

**Architecture:**
```
WebSocket → Orderbook → Detector → Storage
                            ↓
                       Opportunity
                            ↓
                       Executor.executeLive()
                            ↓
                       OrderClient.PlaceOrdersBatch()
                            ↓
                    (POST to CLOB API)
                            ↓
                    Real orders on Polymarket
```

**What you see:**
```
💰 LIVE TRADE EXECUTED
Market: will-bitcoin-hit-100k
YES Order ID: 0x1234...
NO Order ID: 0x5678...
Status: LIVE
Profit: $0.60 (3%)
```

**Metrics tracked:**
- All paper metrics +
- Real profit tracking
- Order success/failure rates

**Requirements:**
- `POLYMARKET_API_KEY` (from Builder API)
- `POLYMARKET_PRIVATE_KEY` (for signing)
- USDC balance in wallet
- One-time USDC approval: `go run . approve`

**Use cases:**
- Production trading: Real profit opportunities
- **CAUTION**: Real money at risk

---

#### Mode Comparison Table

| Component | dry-run | paper | live |
|-----------|---------|-------|------|
| Detector | ✅ | ✅ | ✅ |
| Storage | ✅ | ✅ | ✅ |
| Executor | ❌ | ✅ (sim) | ✅ (real) |
| OrderClient | ❌ | ❌ | ✅ |
| Credentials | ❌ | ❌ | ✅ |
| Risk | None | None | High |

**Recommended progression:**
1. **dry-run** for 24hrs → understand opportunity frequency
2. **paper** for 1 week → validate strategy and profitability
3. **live** with $50 max → test with real money (small)
4. **live** scaled up → production trading

#### Switching Between Modes

**Option 1: Environment variable**
```bash
# Quick test
EXECUTION_MODE=dry-run go run . run

# Switch modes on the fly
EXECUTION_MODE=paper go run . run
EXECUTION_MODE=live go run . run
```

**Option 2: .env file**
```bash
# Edit .env
echo "EXECUTION_MODE=dry-run" > .env
go run . run

# Change mode
sed -i 's/EXECUTION_MODE=.*/EXECUTION_MODE=paper/' .env
go run . run
```

**Option 3: Configuration profiles**
```bash
# .env.dryrun
EXECUTION_MODE=dry-run
ARB_THRESHOLD=0.98

# .env.paper
EXECUTION_MODE=paper
ARB_THRESHOLD=0.995

# .env.live
EXECUTION_MODE=live
ARB_THRESHOLD=0.995
ARB_MAX_TRADE_SIZE=10.0
```

#### Common Configuration Patterns

**Aggressive detection (dry-run):**
```bash
EXECUTION_MODE=dry-run
ARB_THRESHOLD=0.98          # 2% spread (more opportunities)
ARB_MIN_TRADE_SIZE=1.0      # Low minimum
ARB_MAX_TRADE_SIZE=100.0    # High cap (irrelevant for dry-run)
LOG_LEVEL=info              # Clean output
```

**Conservative testing (paper):**
```bash
EXECUTION_MODE=paper
ARB_THRESHOLD=0.995         # 0.5% spread (quality opportunities)
ARB_MIN_TRADE_SIZE=5.0
ARB_MAX_TRADE_SIZE=20.0
LOG_LEVEL=debug             # Detailed logging
```

**Production (live):**
```bash
EXECUTION_MODE=live
ARB_THRESHOLD=0.995
ARB_MIN_TRADE_SIZE=10.0     # Above market minimums
ARB_MAX_TRADE_SIZE=50.0     # Risk management
LOG_LEVEL=info
STORAGE_MODE=postgres       # Persistent logging
```

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

### Adjusting Trade Size Limits

**Configuration interaction:**
- `ARB_MIN_TRADE_SIZE`: Global minimum, must be >= per-market minimums (~$2.50)
- `ARB_MAX_TRADE_SIZE`: Caps calculated size from orderbook liquidity
- **CRITICAL**: `MAX` must be >= `MIN`, validated at startup

**Example configurations:**

Testing/learning (small positions):
```bash
ARB_MIN_TRADE_SIZE=1.0
ARB_MAX_TRADE_SIZE=5.0
```

Production (larger positions):
```bash
ARB_MIN_TRADE_SIZE=10.0
ARB_MAX_TRADE_SIZE=100.0
```

**Debug logging:**
- Set `LOG_LEVEL=debug` to see when trades are capped by max limit
- Look for `trade-size-capped-by-max` log entries

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

### Common Operational Issues

**Empty Orderbook Warnings**
- **Symptom:** Many warnings about "no price levels" at startup
- **Cause:** Normal - many Polymarket markets have thin/no liquidity
- **Solution:** These are now logged at DEBUG level only (use `LOG_LEVEL=debug` to see them)
- **Impact:** None - detector only processes markets with valid orderbooks

**Market Discovery**
- **Default limit:** 1000 markets fetched from API (configurable via `DISCOVERY_MARKET_LIMIT`)
- **Duration filter:** Only markets expiring within `ARB_MAX_MARKET_DURATION` (default: 1h) are subscribed
- **Example:** 1000 markets fetched → 50 expiring within 1h → 100 WebSocket subscriptions (50×2 tokens)
- **Trade-off:** More subscriptions = more opportunities but higher resource usage

**Detector Not Finding Opportunities**
- Check thresholds: `ARB_THRESHOLD=0.98` (2% spread) finds more opportunities
- Verify min/max trade sizes: `ARB_MIN_TRADE_SIZE <= ARB_MAX_TRADE_SIZE`
- Use dry-run mode first: `EXECUTION_MODE=dry-run`
- Enable debug logging: `LOG_LEVEL=debug`

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

### Grafana Dashboards

**7 comprehensive dashboards** covering all 65 metrics with 67+ panels total:

#### 01 - Trading Performance (10 panels)
**Focus**: Detection → Execution → Profit pipeline
- Opportunities detected rate
- Execution success rate
- Cumulative profit by mode (paper/live)
- Profit margin distribution (heatmap)
- Trade size distribution
- Opportunities rejected by reason
- Trade distribution (YES vs NO)
- Markets with opportunities
- Opportunity conversion rate

**Use**: Track profitability, execution efficiency, and trading patterns

#### 02 - System Health (15 panels)
**Focus**: Operational health of all components
- WebSocket connection status
- WebSocket pool status (5 connections)
- Pool subscription distribution
- Pool multiplex latency
- Message processing latency (p99)
- Cache hit rate
- Active subscriptions
- Execution errors by type
- Messages dropped by component
- Orderbook lock contention
- WebSocket reconnection attempts
- Cache operations latency
- WebSocket connection duration
- Discovery poll duration
- Orderbook snapshots tracked

**Use**: Identify system bottlenecks, connection issues, cache efficiency

#### 03 - Wallet Metrics (8 panels)
**Focus**: Balance, positions, and P&L tracking
- USDC balance (with circuit breaker thresholds)
- MATIC balance (gas)
- Portfolio value
- Active positions
- Unrealized P&L (USD and %)
- USDC balance over time (shows disable/enable thresholds as threshold lines)
- Portfolio value over time

**Use**: Monitor wallet health, position performance, circuit breaker context

#### 04 - Orderbook Processing (12 panels)
**Focus**: HFT latency and throughput
- Orderbooks tracked
- Orderbook update rate
- WebSocket message rate
- Processing latency (p99)
- WebSocket message rate by type
- Orderbook update rate by type
- Processing duration (p50/p99)
- End-to-end latency (p99) - **CRITICAL** for HFT
- Messages dropped/sec
- Updates dropped/sec
- Lock contention (p99)
- Opportunities detected/sec

**Use**: Performance tuning, ensure <1ms processing, detect bottlenecks

#### 05 - Circuit Breaker (11 panels)
**Focus**: Balance protection and execution control
- Large state indicator (ENABLED ✅ / DISABLED ⛔)
- Safety margin gauge (balance - disable threshold)
- Balance vs thresholds timeline (visual comparison)
- Current USDC balance
- Disable threshold (trading stops)
- Enable threshold (trading resumes)
- Average trade size (rolling, basis for thresholds)
- State change history (timeline)
- State changes count (detect flapping)
- Opportunities skipped due to circuit breaker

**Use**: Understand why trading stopped/resumed, adjust threshold configuration

**Threshold Logic**:
- Disable = max(avg_trade_size × 3.0, $5.00)
- Enable = disable_threshold × 1.5 (hysteresis prevents flapping)

#### 06 - Error Analysis & Debugging (11 panels)
**Focus**: Failure classification and debugging
- Execution errors by type (pie chart: network, api, validation, funds)
- Error ratio (errors vs success %)
- Execution error rate
- Execution errors timeline (stacked by type)
- Opportunities rejected (by reason: min_size, max_size, low_profit)
- Rejection rate timeline
- Opportunities skipped (by reason: circuit_breaker, balance_low)
- Skipped opportunity rate
- WebSocket errors
- Discovery errors (API failures)
- Wallet update errors (RPC failures)

**Use**: Debug execution failures, understand rejection reasons, identify error patterns

#### 07 - Cache & API Performance (12 panels)
**Focus**: Cache efficiency and external API latency
- Cache hit rate gauge (target: >80%)
- Total cache hits (last hour)
- Total cache misses (last hour)
- Cache operations rate
- Cache sets/sec
- Cache hit rate timeline
- Cache hits vs misses (stacked)
- Cache operation latency by type (get/set/delete, p95)
- Market metadata fetch rate
- Metadata fetch duration (p50/p95)
- Metadata cache efficiency (pie chart)
- Discovery poll duration (p95)

**Use**: Optimize caching strategy, monitor API responsiveness, reduce latency

### Dashboard Access
- **Grafana**: `http://localhost:3000` (if using Docker Compose)
- **Prometheus**: `http://localhost:9090`
- **Metrics Endpoint**: `http://localhost:8080/metrics`

### Key Metrics to Watch
- **E2E Latency (p99)**: Should be <1ms for HFT performance (Dashboard 04)
- **Cache Hit Rate**: Should be >80% for optimal performance (Dashboard 07, 02)
- **Circuit Breaker State**: Understand execution halts (Dashboard 05)
- **Error Ratio**: Should be <1% (Dashboard 06)
- **WebSocket Pool**: All 5 connections active (Dashboard 02)
- **Opportunities Skipped**: Identify lost trading chances (Dashboard 06)

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
