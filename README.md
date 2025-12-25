# Polymarket Arbitrage Bot

High-frequency trading bot for detecting and executing arbitrage opportunities on Polymarket prediction markets.

## Overview

This bot monitors Polymarket binary markets (YES/NO outcomes) in real-time and detects arbitrage opportunities when the sum of best bid prices is below a threshold (typically 0.995), accounting for fees.

**Example Arbitrage:**
- YES best bid: $0.48 (buy YES for $0.48)
- NO best bid: $0.51 (buy NO for $0.51)
- Total cost: $0.99 (guaranteed profit: $0.01 per share)

## Architecture

### Event-Driven Design

The bot uses a **fully event-driven architecture** optimized for low latency:

```
WebSocket → OrderbookManager → ArbitrageDetector → Executor
  (push)      (update events)   (opportunities)    (trades)
```

**Key Design Principles:**
1. **No Polling** - All components react to events, not timers
2. **Lock-Free Reads** - Orderbook updates parsed outside locks
3. **Non-Blocking Channels** - Buffered channels with overflow handling
4. **Fast JSON** - Uses `github.com/goccy/go-json` for 2-3x faster parsing

### Components

#### 1. Discovery Service (`internal/discovery/`)
- Polls Polymarket Gamma API for active markets
- Implements differential discovery (only new markets trigger subscriptions)
- Caches market metadata with Ristretto

#### 2. WebSocket Manager (`pkg/websocket/`)
- Maintains persistent connection to Polymarket orderbook feed
- Automatic reconnection with exponential backoff
- Subscribes to token orderbooks for discovered markets
- Ping/pong keep-alive

#### 3. Orderbook Manager (`internal/orderbook/`)
- Maintains in-memory orderbook snapshots (best bid/ask)
- **Event Emission:** Broadcasts updates to subscribers via channels
- **Optimized Locking:** Parses price strings outside critical section
- Handles both full snapshots (`book`) and incremental updates (`price_change`)

#### 4. Arbitrage Detector (`internal/arbitrage/`)
- **Event-Driven:** Listens to orderbook update events
- Checks for arbitrage when either YES or NO orderbook updates
- Validates minimum trade size and calculates net profit after fees
- Stores opportunities to database/console
- Emits opportunities to executor

#### 5. Executor (`internal/execution/`)
- Paper trading mode (default) - logs simulated trades
- Live trading mode (stub) - requires API credentials
- Tracks cumulative profit and metrics

## Performance Characteristics

### Latency Optimizations

**1. Lock Contention Reduction (50%+ improvement)**
```go
// OLD: Parse strings inside lock
m.mu.Lock()
bestBidPrice, _ := strconv.ParseFloat(msg.Bids[0].Price, 64)
m.mu.Unlock()

// NEW: Parse outside, only lock for map write
bestBidPrice, _ := strconv.ParseFloat(msg.Bids[0].Price, 64)
m.mu.Lock()
snapshot.BestBidPrice = bestBidPrice
m.mu.Unlock()
```

**2. Event-Driven vs Polling (100ms → <1ms)**
- OLD: Check all markets every 100ms
- NEW: React instantly to orderbook updates

**3. Fast JSON Parsing (2-3x faster)**
- Using `github.com/goccy/go-json` instead of `encoding/json`
- Critical for WebSocket message parsing hot path

**4. Non-Blocking Channels**
```go
select {
case updateChan <- snapshot:
default:
    // Drop update if consumer slow (prevents blocking producers)
}
```

### Memory Efficiency

- **Eliminated unnecessary allocations**: Removed `GetAllSnapshots()` polling pattern
- **Copy-on-read**: Return copies from `GetSnapshot()` to prevent races
- **Snapshot-only storage**: Only store best bid/ask, not full orderbook depth

### Throughput

- **Orderbook updates:** 1000+ messages/sec
- **Arbitrage detection:** <1ms per update
- **Channel buffers:**
  - Orderbook updates: 1000 messages
  - Opportunities: 50 opportunities

## Configuration

Environment variables (see `pkg/config/config.go`):

```bash
# Polymarket API
POLYMARKET_WS_URL=wss://ws-subscriptions-clob.polymarket.com/ws/market
POLYMARKET_GAMMA_API_URL=https://gamma-api.polymarket.com

# Discovery
DISCOVERY_POLL_INTERVAL=30s
DISCOVERY_MARKET_LIMIT=50

# WebSocket
WS_DIAL_TIMEOUT=10s
WS_PONG_TIMEOUT=15s
WS_PING_INTERVAL=10s
WS_MESSAGE_BUFFER_SIZE=1000

# Arbitrage Detection
ARB_THRESHOLD=0.995          # Detect when YES + NO < 0.995
ARB_MIN_TRADE_SIZE=10.0      # Minimum $10 trade size
ARB_TAKER_FEE=0.01           # 1% taker fee on Polymarket

# Execution
EXECUTION_MODE=paper         # paper or live
EXECUTION_MAX_POSITION_SIZE=1000.0

# Storage
STORAGE_MODE=console         # console or postgres
POSTGRES_HOST=localhost
POSTGRES_PORT=5432
POSTGRES_DB=polymarket_arb
```

## Running

### Prerequisites

```bash
go 1.22+
```

### Development

```bash
# Run with default config (paper trading, console output)
make run

# Run with single market mode
go run cmd/polymarket-arb/main.go --market <market-slug>

# List active markets
make list-markets
```

### Testing

```bash
# Run unit tests
make test

# Run integration tests (includes E2E flows)
make test-integration

# Run execution package tests
make test-execution

# Run with race detector
go test -race ./...

# Run benchmarks
go test -bench=. -benchmem ./...
```

### Testing Live Order Submission

The `test-live-order` command allows you to test actual order submission to the Polymarket CLOB API and verify response parsing. This is useful for:
- Testing API connectivity and authentication
- Verifying order request/response formats
- Testing order rejection scenarios
- Debugging API integration issues

**⚠️ WARNING: This submits real orders to Polymarket. Use small amounts for testing!**

#### Setup Credentials

1. Copy the environment template:
```bash
cp .env.example .env
```

2. Obtain Polymarket API credentials:
   - Go to https://polymarket.com
   - Connect your wallet
   - Navigate to Account Settings → API Keys
   - Create a new API key
   - Save the API Key, Private Key, and Passphrase

3. Edit `.env` and fill in your credentials:
```bash
POLYMARKET_API_KEY=your_api_key
POLYMARKET_PRIVATE_KEY=your_private_key_without_0x
POLYMARKET_API_PASSPHRASE=your_passphrase
```

4. Load credentials:
```bash
source .env
```

#### Submit Test Orders

```bash
# Test order submission on a market
# Uses very low prices (0.01) to avoid accidental fills
go run . test-live-order <market-slug> \
  --size 1.0 \
  --yes-price 0.01 \
  --no-price 0.01

# Example: Test on Trump/Epstein files market
go run . test-live-order will-trump-release-the-epstein-files-by-december-22 \
  --size 1.0 \
  --yes-price 0.01 \
  --no-price 0.01
```

#### What Gets Tested

The command will:
1. ✅ Fetch market details and token IDs
2. ✅ Create order requests for YES and NO tokens
3. ✅ Sign requests with your private key
4. ✅ Submit orders to the CLOB API
5. ✅ Parse and display responses
6. ✅ Show order status, IDs, and any errors

#### Expected Outcomes

**Orders Rejected (Expected):**
Most test orders will be rejected because:
- Prices too far from market (0.01 vs market price ~0.50)
- Orders outside valid price range
- Insufficient balance for settlement

**Orders Accepted:**
If orders are accepted, you'll see:
- Order ID
- Status: "open", "filled", "partially_filled"
- Token ID
- Actual filled size

#### Response Parsing Verification

The command demonstrates proper parsing of:
- **Success responses**: Order ID, status, prices, sizes
- **Error responses**: Error codes, messages, reasons
- **HTTP status codes**: 200/201 (success), 400 (bad request), 401 (auth), 422 (validation)

This validates that your order submission and response handling code works correctly.

### Production

```bash
# Build
make build

# Run with environment config
export EXECUTION_MODE=live
export POLYMARKET_API_KEY=your-key
export STORAGE_MODE=postgres
./bin/polymarket-arb
```

## Testing Strategy

### Unit Tests
- Component-level tests with mocked dependencies
- Coverage for critical paths: orderbook updates, arbitrage detection
- Race detection enabled in CI

### Integration Tests (`internal/app/integration_test.go`)

**TestE2E_ArbitrageFlow:**
- Complete flow: discovery → orderbook → detection → execution
- Validates event-driven architecture
- Verifies opportunity storage and profit calculation

**TestE2E_MarketDiscoveryFlow:**
- Tests initial market poll
- Tests differential discovery (no duplicates)
- Validates cache integration

**TestE2E_OrderbookProcessing:**
- Tests full snapshot (`book`) processing
- Tests incremental updates (`price_change`)
- Validates best bid/ask extraction

### Test Helpers (`internal/testutil/`)
- `MockGammaAPI`: HTTP test server for Gamma API
- `MockStorage`: In-memory opportunity storage
- Factory functions for test data

## Metrics

Prometheus metrics exposed on `:8080/metrics`:

```
# Orderbook
orderbook_updates_total{event_type}
orderbook_snapshots_tracked

# Arbitrage
arbitrage_opportunities_detected_total
arbitrage_opportunity_profit_bps
arbitrage_opportunity_size_usd
arbitrage_detection_duration_seconds

# Execution
execution_trades_total{mode}
execution_profit_usd_total{mode}
execution_duration_seconds
```

Health check: `http://localhost:8080/health`

## Project Structure

```
.
├── cmd/
│   └── polymarket-arb/       # Main entry point
├── internal/
│   ├── app/                  # Application setup and lifecycle
│   ├── arbitrage/            # Arbitrage detection logic
│   ├── discovery/            # Market discovery service
│   ├── execution/            # Trade execution (paper/live)
│   ├── orderbook/            # Orderbook state management
│   ├── storage/              # Postgres and console storage
│   └── testutil/             # Test helpers and mocks
└── pkg/
    ├── cache/                # Ristretto cache wrapper
    ├── config/               # Configuration management
    ├── healthprobe/          # Health checks
    ├── httpserver/           # HTTP server for metrics
    ├── types/                # Shared types
    └── websocket/            # WebSocket client with reconnection

```

## Development Notes

### Adding New Components

1. Define interface in relevant package
2. Implement with struct + config pattern
3. Add `Start(ctx)` and `Close()` lifecycle methods
4. Create unit tests with mocked dependencies
5. Add integration test if component interacts with others

### Debugging

```bash
# Enable debug logging
LOG_LEVEL=debug make run

# View metrics
curl http://localhost:8080/metrics

# Check health
curl http://localhost:8080/health
```

### Common Issues

**No opportunities detected:**
- Check `ARB_THRESHOLD` - market spreads may be tight
- Verify `ARB_MIN_TRADE_SIZE` - insufficient liquidity
- Check WebSocket connection status in logs

**High CPU usage:**
- Check orderbook update rate in metrics
- Verify channel buffer sizes aren't overflowing
- Run with pprof: `go run cmd/polymarket-arb/main.go --cpuprofile cpu.prof`

## License

MIT

## Contributing

1. Ensure all tests pass: `make test test-integration`
2. Run linter: `golangci-lint run`
3. Add tests for new features
4. Update documentation
