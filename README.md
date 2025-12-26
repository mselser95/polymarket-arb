# Polymarket Arbitrage Bot

High-frequency trading bot for detecting and executing arbitrage opportunities on Polymarket prediction markets.

## Table of Contents

- [Overview](#overview)
- [Key Features](#key-features)
- [Architecture](#architecture)
- [Quick Start](#quick-start)
- [Installation](#installation)
- [Configuration](#configuration)
- [CLI Commands](#cli-commands)
- [Trading Workflow](#trading-workflow)
- [Market Metadata System](#market-metadata-system)
- [Order Placement](#order-placement)
- [Testing](#testing)
- [Performance](#performance)
- [Troubleshooting](#troubleshooting)
- [Development](#development)
- [API Reference](#api-reference)
- [Project Structure](#project-structure)
- [Deployment](#deployment)
- [Contributing](#contributing)
- [License](#license)

## Overview

This bot monitors Polymarket binary markets (YES/NO outcomes) in real-time and detects arbitrage opportunities when the sum of best ask prices is below 1.0, accounting for trading fees.

### What is Prediction Market Arbitrage?

Polymarket allows trading on binary outcomes (YES/NO). Each outcome is a token priced between $0 and $1. When a market resolves:
- Winning outcome tokens → $1.00
- Losing outcome tokens → $0.00

**Arbitrage opportunity exists when:**
```
YES ask price + NO ask price < 1.0 (minus fees)
```

**Example:**
```
Market: "Will Bitcoin hit $100K by end of 2024?"

YES token: Best ask = $0.48 (buy YES for $0.48)
NO token:  Best ask = $0.47 (buy NO for $0.47)

Total cost: $0.95
Guaranteed payout: $1.00 (one token will be worth $1)
Gross profit: $0.05 per dollar invested (5%)

After 1% taker fees on each side (~2% total):
Net profit: ~$0.03 per dollar (3%)
```

## Key Features

### Real-Time Arbitrage Detection
- **Event-driven architecture**: Reacts instantly to orderbook updates (<1ms latency)
- **WebSocket streaming**: Direct connection to Polymarket CLOB feed
- **Smart filtering**: Only processes markets with sufficient liquidity
- **Fee-aware**: Accounts for 1% taker fees in profit calculations

### Market Metadata Caching
- **Automatic tick size detection**: Fetches price precision requirements (0.01, 0.001, etc.)
- **Minimum order size validation**: Prevents orders below market minimums (typically 5 tokens)
- **24-hour cache**: Reduces API calls, improves performance
- **Fallback defaults**: Continues operation even if metadata fetch fails

### Order Execution
- **Paper trading mode**: Test strategies without risking capital
- **Live trading mode**: Execute real trades via Polymarket CLOB API
- **Batch order submission**: Submit YES and NO orders simultaneously
- **Order size validation**: Pre-submission checks against market minimums
- **Response parsing**: Handles all API response types (success, rejection, errors)

### Comprehensive Testing
- **E2E test suite**: Tests complete batch order workflow
- **Unit tests**: 85% coverage for markets package, 77% for storage
- **Integration tests**: Validates entire arbitrage detection pipeline
- **Mock infrastructure**: HTTP test servers for API simulation

### Monitoring & Observability
- **Prometheus metrics**: Track opportunities, trades, profits
- **Structured logging**: JSON logs with contextual information
- **Health checks**: HTTP endpoint for service monitoring
- **Performance profiling**: Built-in pprof support

## Architecture

### System Design

```
┌─────────────────┐
│  Gamma API      │  (Market Discovery)
│  gamma-api.pm   │
└────────┬────────┘
         │
         │ Poll every 30s
         │
         ▼
┌─────────────────────────────────────────────────────────────┐
│                    Discovery Service                         │
│  - Fetches active markets                                   │
│  - Differential discovery (only new markets)                │
│  - Market metadata caching (Ristretto)                      │
└────────┬────────────────────────────────────────────────────┘
         │
         │ Subscribe to new markets
         │
         ▼
┌─────────────────────────────────────────────────────────────┐
│                  WebSocket Manager                           │
│  - Persistent WSS connection to CLOB                        │
│  - Subscribe to token orderbooks                            │
│  - Ping/pong keep-alive                                     │
│  - Automatic reconnection (exponential backoff)             │
└────────┬────────────────────────────────────────────────────┘
         │
         │ Stream orderbook updates
         │ (book, price_change events)
         │
         ▼
┌─────────────────────────────────────────────────────────────┐
│                  Orderbook Manager                           │
│  - In-memory snapshots (best bid/ask only)                  │
│  - Event emission to subscribers                            │
│  - Lock-free parsing (parse outside critical section)       │
│  - Buffered channels (1000 messages)                        │
└────────┬────────────────────────────────────────────────────┘
         │
         │ Emit update events
         │
         ▼
┌─────────────────────────────────────────────────────────────┐
│                Arbitrage Detector                            │
│  - Event-driven (no polling)                                │
│  - Fetches market metadata (tick size, min size)           │
│  - Validates: price sum < threshold                         │
│  - Validates: size >= minimum                               │
│  - Calculates net profit after fees                         │
│  - Emits opportunities                                      │
└────────┬────────────────────────────────────────────────────┘
         │
         │ Emit opportunities
         │
         ▼
┌─────────────────────────────────────────────────────────────┐
│                    Executor                                  │
│  - Paper mode: Log simulated trades                         │
│  - Live mode: Submit orders via CLOB API                    │
│  - Track cumulative profit                                  │
│  - Prometheus metrics                                       │
└─────────────────────────────────────────────────────────────┘
         │
         │ Store results
         │
         ▼
┌─────────────────────────────────────────────────────────────┐
│                    Storage                                   │
│  - Console: Log to stdout                                   │
│  - PostgreSQL: Persist opportunities and trades             │
└─────────────────────────────────────────────────────────────┘
```

### Component Details

#### 1. Discovery Service (`internal/discovery/`)

**Responsibilities:**
- Poll Polymarket Gamma API for active markets
- Detect new markets (differential discovery)
- Cache market metadata
- Trigger WebSocket subscriptions

**Key Features:**
- Configurable poll interval (default: 30s)
- Market limit to control memory usage
- Ristretto cache for market data
- Tracks seen markets to avoid resubscription

**API Used:**
```
GET https://gamma-api.polymarket.com/markets
```

#### 2. WebSocket Manager (`pkg/websocket/`)

**Responsibilities:**
- Maintain persistent WSS connection to Polymarket CLOB
- Subscribe to orderbook updates for specific tokens
- Handle ping/pong keep-alive
- Automatic reconnection on disconnect

**Key Features:**
- Exponential backoff reconnection (1s → 32s)
- Configurable timeouts (dial, pong, ping)
- Non-blocking message broadcast
- Graceful shutdown

**WebSocket Endpoint:**
```
wss://ws-subscriptions-clob.polymarket.com/ws/market
```

**Message Types:**
- `subscribe`: Subscribe to token orderbook
- `book`: Full orderbook snapshot
- `price_change`: Incremental price update
- `ping`/`pong`: Keep-alive

#### 3. Orderbook Manager (`internal/orderbook/`)

**Responsibilities:**
- Maintain in-memory orderbook snapshots
- Extract best bid/ask from full orderbook
- Emit update events to subscribers
- Handle both snapshots and incremental updates

**Key Features:**
- **Snapshot-only storage**: Only stores best bid/ask (not full depth)
- **Lock optimization**: Parse strings outside critical section
- **Event emission**: Broadcasts updates via buffered channels
- **Copy-on-read**: Returns copies to prevent race conditions

**Performance:**
- Handles 1000+ messages/sec
- <1ms processing per update
- 1000-message channel buffer

#### 4. Arbitrage Detector (`internal/arbitrage/`)

**Responsibilities:**
- Listen to orderbook update events
- Fetch market metadata (tick size, minimum order size)
- Detect arbitrage opportunities
- Calculate net profit after fees
- Emit opportunities to executor

**Detection Logic:**
```go
// Check if opportunity exists
priceSum := yesAskPrice + noAskPrice
threshold := 0.995 // Configurable

if priceSum >= threshold {
    return nil, false // No arbitrage
}

// Calculate profit
grossProfit := 1.0 - priceSum
feePercent := 2 * takerFee // Fee on both sides
netProfit := grossProfit - (priceSum * feePercent)

// Validate size meets market minimums
yesTokens := orderSize / yesAskPrice
noTokens := orderSize / noAskPrice

if yesTokens < yesMinSize || noTokens < noMinSize {
    return nil, false // Below minimum
}
```

**Metadata Integration:**
- Fetches tick size and min order size from cache
- Falls back to defaults (0.01 tick, 5.0 min) if fetch fails
- Validates both YES and NO sides independently

#### 5. Executor (`internal/execution/`)

**Responsibilities:**
- Execute trades based on opportunities (paper/live modes)
- Track cumulative profit
- Emit metrics
- Skipped entirely in dry-run mode

**Execution Modes:**

**Dry-Run (Detection Only):**
- Executor not initialized
- Only detects and logs opportunities
- No trade execution (not even simulated)
- Use for: Market monitoring, testing detection logic

**Paper Trading:**
- Logs simulated trades to console
- Tracks hypothetical profit
- Updates execution metrics
- No API calls made
- Use for: Strategy testing, full flow validation

**Live Trading:**
- Submits orders via CLOB API
- Signs requests with private key
- Handles order responses and errors
- Validates order size before submission
- Use for: Production trading (requires credentials)

#### 6. Market Metadata System (`internal/markets/`)

**New in latest version** - Caches market-specific trading constraints.

**Components:**
- `MetadataClient`: Fetches tick size and min order size from CLOB API
- `CachedMetadataClient`: Wraps client with 24-hour TTL cache

**API Endpoints:**
```
GET /tick-size?token_id={token_id}
  → {"minimum_tick_size": 0.001}

GET /book?token_id={token_id}
  → {"min_size": 5.0, "market": {...}}
```

**Cache Strategy:**
- Key format: `metadata:{tokenID}`
- TTL: 24 hours
- Ristretto cache integration
- Async writes (requires Wait() in tests)

## Quick Start

### Prerequisites

- Go 1.22 or higher
- Git
- (Optional) PostgreSQL for persistent storage
- (Optional) Polymarket account with API credentials for live trading

### Install and Run

```bash
# Clone repository
git clone https://github.com/yourusername/polymarket-arb.git
cd polymarket-arb

# Install dependencies
go mod download

# Run in paper trading mode (safe, no real trades)
make run

# You should see output like:
# 2024-12-25T12:00:00.000Z INFO  app/app.go:45  starting-polymarket-arb
# 2024-12-25T12:00:01.123Z INFO  discovery/service.go:67  discovered-markets  {"count": 42}
# 2024-12-25T12:00:02.456Z INFO  orderbook/manager.go:89  orderbook-snapshot  {"token": "yes-token-123"}
# 2024-12-25T12:00:03.789Z INFO  arbitrage/detector.go:156  opportunity-detected  {"market": "btc-100k", "profit_bps": 304}
```

## Installation

### From Source

```bash
# Clone repository
git clone https://github.com/yourusername/polymarket-arb.git
cd polymarket-arb

# Build binary
make build

# Binary will be at ./bin/polymarket-arb
./bin/polymarket-arb --help
```

### Development Installation

```bash
# Install development tools
make install-tools

# This installs:
# - golangci-lint (linting)
# - gofumpt (formatting)
# - mockgen (test mocks)
```

## Configuration

### Environment Variables

Create a `.env` file in the project root:

```bash
# Copy template
cp .env.example .env

# Edit with your values
vim .env
```

#### Core Configuration

```bash
# Polymarket API URLs
POLYMARKET_WS_URL=wss://ws-subscriptions-clob.polymarket.com/ws/market
POLYMARKET_GAMMA_API_URL=https://gamma-api.polymarket.com
POLYMARKET_CLOB_API_URL=https://clob.polymarket.com

# Discovery Service
DISCOVERY_POLL_INTERVAL=30s           # How often to check for new markets
DISCOVERY_MARKET_LIMIT=100            # Max markets to track simultaneously (default: 100)

# WebSocket Configuration
WS_DIAL_TIMEOUT=10s                   # Connection timeout
WS_PONG_TIMEOUT=15s                   # How long to wait for pong response
WS_PING_INTERVAL=10s                  # How often to send ping
WS_MESSAGE_BUFFER_SIZE=1000           # Channel buffer size
WS_RECONNECT_MAX_ATTEMPTS=10          # Max reconnection attempts
WS_RECONNECT_BASE_DELAY=1s            # Initial reconnection delay
WS_RECONNECT_MAX_DELAY=32s            # Max reconnection delay

# Arbitrage Detection
ARB_THRESHOLD=0.995                   # Detect when YES + NO < 0.995
ARB_MIN_TRADE_SIZE=1.0                # Minimum $1 USDC trade
ARB_MAX_TRADE_SIZE=2.0                # Maximum $2 USDC trade (caps calculated size)
ARB_TAKER_FEE=0.01                    # 1% taker fee (0.01 = 1%)

# Execution
EXECUTION_MODE=dry-run                # dry-run, paper, or live
EXECUTION_MAX_POSITION_SIZE=1000.0    # Max $1000 per trade (paper/live only)
EXECUTION_RATE_LIMIT=10               # Max 10 trades per minute (live only)

# Storage
STORAGE_MODE=console                  # console or postgres
POSTGRES_HOST=localhost
POSTGRES_PORT=5432
POSTGRES_DB=polymarket_arb
POSTGRES_USER=postgres
POSTGRES_PASSWORD=yourpassword
POSTGRES_SSLMODE=disable

# HTTP Server (metrics/health)
HTTP_PORT=8080
HTTP_READ_TIMEOUT=10s
HTTP_WRITE_TIMEOUT=10s

# Logging
LOG_LEVEL=info                        # debug, info, warn, error
LOG_FORMAT=json                       # json or console
```

#### Live Trading Configuration

**⚠️ Required for live trading only**

```bash
# Polymarket Credentials
POLYMARKET_PRIVATE_KEY=your_private_key_without_0x_prefix
POLYMARKET_API_CREDS=your_api_credentials_json
POLYMARKET_CHAIN_ID=137                # Polygon mainnet

# Rate Limiting (recommended for live trading)
EXECUTION_RATE_LIMIT=10                # Max 10 orders per minute
EXECUTION_COOLDOWN=5s                  # Wait 5s between opportunities

# Risk Management
EXECUTION_MAX_POSITION_SIZE=100.0      # Start small!
EXECUTION_MIN_PROFIT_BPS=200           # Minimum 2% profit (200 basis points)
```

### Configuration Precedence

1. Command-line flags (highest priority)
2. Environment variables
3. `.env` file
4. Default values (lowest priority)

Example:
```bash
# Override threshold via flag
go run . --threshold 0.98

# Override via environment variable
ARB_THRESHOLD=0.98 go run .
```

## CLI Commands

The bot provides several CLI commands for different use cases.

### `run` - Start the Bot

Starts the full arbitrage detection and execution pipeline.

```bash
# Run with defaults (paper trading, console output)
go run .

# Run with custom config
go run . --threshold 0.98 --min-trade-size 20.0

# Run in live trading mode (requires credentials)
EXECUTION_MODE=live go run .

# Run with specific market (single market mode)
go run . --market will-bitcoin-hit-100k-by-dec-31
```

**Flags:**
- `--threshold <float>`: Arbitrage detection threshold (default: 0.995)
- `--min-trade-size <float>`: Minimum trade size in USD (default: 10.0)
- `--market <slug>`: Run on single market only
- `--log-level <level>`: Set log level (debug, info, warn, error)

### `list-markets` - Discover Active Markets

Queries Polymarket Gamma API and lists all active markets.

```bash
# List all active markets
go run . list-markets

# Output:
# Fetching markets from Polymarket...
# Found 156 active markets:
#
# Market: Will Bitcoin hit $100K by Dec 31?
#   Slug: will-bitcoin-hit-100k-by-dec-31
#   Market ID: 0x1234...
#   End Date: 2024-12-31T23:59:59Z
#   YES Token: 86076435890279485031516158085782
#   NO Token: 86076435890279485031516158085783
#
# Market: Will Trump win 2024 election?
#   ...
```

**Useful for:**
- Finding market slugs for single-market mode
- Checking market end dates
- Getting token IDs for manual testing

### `approve` - Approve USDC Spending

Sends an on-chain transaction to approve the Polymarket CTF Exchange contract to spend USDC from your wallet.

**⚠️ Required before live trading** - This is a one-time operation per wallet.

```bash
# Approve unlimited USDC spending (recommended)
go run . approve

# Approve specific amount
go run . approve --amount 1000.0

# Use custom RPC endpoint
go run . approve --rpc https://polygon-mainnet.g.alchemy.com/v2/YOUR_KEY
```

**What it does:**
1. Checks current allowance
2. Calculates gas cost
3. Submits ERC20 approve transaction
4. Waits for confirmation
5. Displays transaction hash and status

**Requirements:**
- `POLYMARKET_PRIVATE_KEY` in `.env`
- MATIC balance for gas (~$0.01)
- USDC balance to trade with

**Contract Addresses:**
- USDC: `0x2791Bca1f2de4661ED88A30C99A7a9449Aa84174`
- CTF Exchange: `0x4bFb41d5B3570DeFd03C39a9A4D8dE6Bd8B8982E`
- Chain: Polygon (137)

### `balance` - Check Wallet Balances

Queries on-chain balances for USDC and MATIC.

```bash
# Check balances
go run . balance

# Output:
# Wallet: 0x742d35Cc6634C0532925a3b844Bc454e4438f44e
#
# USDC Balance: 1,234.56 USDC
# MATIC Balance: 5.67890123 MATIC
#
# Allowance: unlimited
# Status: Ready to trade ✓
```

**Use cases:**
- Verify sufficient USDC for trading
- Check MATIC balance for gas
- Confirm approval status

### `place-orders` - Manual Order Placement

Place a single pair of YES/NO orders on a market.

**⚠️ Submits real orders to Polymarket**

```bash
# Place orders on a market
go run . place-orders <market-slug> \
  --size 10.0 \
  --yes-price 0.45 \
  --no-price 0.50

# Example: Place small test orders
go run . place-orders will-bitcoin-hit-100k \
  --size 5.0 \
  --yes-price 0.01 \
  --no-price 0.01
```

**Flags:**
- `--size <float>`: Order size in USDC (default: 1.0)
- `--yes-price <float>`: YES token price (default: market ask)
- `--no-price <float>`: NO token price (default: market ask)

**What it does:**
1. Fetches market details and token IDs
2. Fetches tick size and minimum order size
3. Validates order size meets minimum
4. Rounds prices to tick size
5. Creates and signs orders
6. Submits to CLOB API
7. Displays responses

**Order Response Fields:**
```
✅ YES order submitted!
  Order ID: 0x1234567890abcdef...
  Status: LIVE
  Token ID: 86076435890279485031516158085782
  Price: 0.4500
  Size: 22.22 (Filled: 0.00)
  Side: BUY
  Type: GTC
  Created: 2024-12-25T12:00:00Z
```

### `test-live-order` - Test Order Submission

Submit low-price test orders to verify API integration without risking capital.

```bash
# Submit test orders (will likely be rejected due to low price)
go run . test-live-order <market-slug> \
  --size 1.0 \
  --yes-price 0.01 \
  --no-price 0.01
```

**Expected outcome:**
Most test orders are rejected because:
- Prices too far from market (0.01 vs ~0.50)
- Orders outside valid range
- This is intentional - validates error handling

**Use cases:**
- Test API connectivity
- Verify authentication works
- Test error response parsing
- Debug order formatting issues

## Trading Workflow

### Dry-Run Mode (Detection Only - Safest)

Dry-run mode monitors markets and detects opportunities without any execution.

```bash
# Run in dry-run mode
EXECUTION_MODE=dry-run go run . run

# Or set in .env
echo "EXECUTION_MODE=dry-run" >> .env
make run
```

**What happens:**
1. Bot monitors active markets via WebSocket
2. Detects arbitrage opportunities
3. Logs opportunities to console:
   ```
   ========================================
   Arbitrage Opportunity Detected
   ========================================
   ID:               opp_abc123
   Market:           will-bitcoin-hit-100k
   Question:         Will Bitcoin hit $100k?
   ----------------------------------------
   Prices:
     YES ASK:        0.4800
     NO ASK:         0.4800
     Total Cost:     0.9600 < 0.9950 (threshold)
   ----------------------------------------
   Profit Analysis:
     Max Size:       $50.00
     Gross Profit:   $40.00 (400 bps)
     Fees (1%):      $0.96
     Net Profit:     $39.04 (390 bps)
   ========================================
   ```
4. **NO execution** (not even simulated)
5. Tracks detector metrics only

**Use for:**
- Market research and monitoring
- Testing detection logic and thresholds
- Debugging configuration issues
- Safe exploration without any risk

### Paper Trading (Simulation Mode)

Paper trading simulates trades without sending real orders.

```bash
# Run in paper mode (default)
make run

# Or explicitly set mode
EXECUTION_MODE=paper go run .
```

**What happens:**
1. Bot detects arbitrage opportunities
2. Logs simulated trades to console:
   ```
   📝 PAPER TRADE
   Market: will-bitcoin-hit-100k
   YES: Buy 22.22 tokens @ $0.45 = $10.00
   NO:  Buy 20.00 tokens @ $0.50 = $10.00
   Total cost: $20.00
   Expected profit: $0.60 (3%)
   ```
3. Tracks hypothetical profit
4. Emits metrics
5. **No actual orders submitted**

**Use for:**
- Strategy testing
- Performance benchmarking
- Understanding opportunity distribution
- Safe learning

### Live Trading (Real Money)

**⚠️ WARNING: Live trading risks real capital. Start small!**

#### Prerequisites

1. **Polymarket Account**
   - Create account at https://polymarket.com
   - Connect wallet (MetaMask, WalletConnect, etc.)
   - Fund wallet with USDC on Polygon

2. **API Credentials**
   - Go to Account Settings → API Keys
   - Create new API key
   - Save credentials securely

3. **Wallet Setup**
   - Export private key from MetaMask
   - Add to `.env`:
     ```bash
     POLYMARKET_PRIVATE_KEY=your_key_without_0x
     ```

4. **Approve USDC Spending**
   ```bash
   go run . approve
   ```

#### Start Live Trading

```bash
# Set mode to live
export EXECUTION_MODE=live

# Start bot
go run .
```

**What happens:**
1. Bot detects arbitrage opportunity
2. Fetches market metadata (tick size, min size)
3. Validates order sizes meet minimums
4. Creates signed order requests
5. Submits to CLOB API via HTTP POST
6. Parses responses
7. Logs results:
   ```
   💰 LIVE TRADE EXECUTED
   Market: will-bitcoin-hit-100k

   YES Order:
     ID: 0x1234...
     Status: LIVE
     Price: 0.4500
     Size: 22.22 tokens

   NO Order:
     ID: 0x5678...
     Status: LIVE
     Price: 0.5000
     Size: 20.00 tokens

   Total cost: $20.00
   Expected profit: $0.60 (3%)
   ```

### Execution Mode Comparison

| Feature | Dry-Run | Paper | Live |
|---------|---------|-------|------|
| **Detects Opportunities** | ✅ | ✅ | ✅ |
| **Logs Opportunities** | ✅ | ✅ | ✅ |
| **Simulates Execution** | ❌ | ✅ | ❌ |
| **Places Real Orders** | ❌ | ❌ | ✅ |
| **Updates Execution Metrics** | ❌ | ✅ | ✅ |
| **Tracks Profit** | ❌ | ✅ (simulated) | ✅ (real) |
| **Requires API Credentials** | ❌ | ❌ | ✅ |
| **Requires USDC Balance** | ❌ | ❌ | ✅ |
| **Risk Level** | None | None | High |
| **Best For** | Monitoring, research | Testing, validation | Production |

**Progression Path:**
1. Start with **dry-run** to understand opportunity frequency
2. Move to **paper** to test execution logic
3. Advance to **live** with small position sizes

#### Risk Management

**Start Conservative:**
```bash
# Small position size
EXECUTION_MAX_POSITION_SIZE=50.0

# Higher profit threshold
EXECUTION_MIN_PROFIT_BPS=300  # 3% minimum

# Rate limiting
EXECUTION_RATE_LIMIT=5         # Max 5 trades/minute
EXECUTION_COOLDOWN=10s         # 10s between opportunities
```

**Monitor Closely:**
- Watch logs for errors
- Check balances regularly: `go run . balance`
- Monitor Polymarket UI for order status
- Track cumulative profit in metrics

**Common Issues:**
- **"Size lower than minimum"**: Increase `ARB_MIN_TRADE_SIZE` or check per-market minimums
- **"No opportunities detected"**: Check `ARB_MAX_TRADE_SIZE >= ARB_MIN_TRADE_SIZE`
- **"Trades smaller than expected"**: `ARB_MAX_TRADE_SIZE` is capping calculated size (set `LOG_LEVEL=debug`)
- **"Insufficient balance"**: Check `go run . balance`
- **"Invalid signature"**: Verify `POLYMARKET_PRIVATE_KEY`
- **"Rate limit exceeded"**: Reduce `EXECUTION_RATE_LIMIT`
- **"Empty orderbook warnings"**: Normal for illiquid markets (logged at debug level only)

## Market Metadata System

The bot automatically fetches and caches market-specific trading constraints.

### Why Metadata Matters

Each Polymarket token has different requirements:
- **Tick size**: Minimum price increment (e.g., 0.01, 0.001)
- **Minimum order size**: Minimum tokens per order (e.g., 5, 10, 100)

Orders failing these constraints are rejected by the API:
```
❌ Error 400: Size (2.5) lower than the minimum: 5.0
❌ Error 400: Price 0.451 not aligned to tick size 0.001
```

### How It Works

1. **Fetch on Detection**
   When arbitrage is detected, bot fetches metadata:
   ```
   GET /tick-size?token_id=86076435890279485031516158085782
   → {"minimum_tick_size": 0.001}

   GET /book?token_id=86076435890279485031516158085782
   → {"min_size": 5.0}
   ```

2. **Cache for 24 Hours**
   Stored in Ristretto cache:
   ```
   Key: metadata:86076435890279485031516158085782
   Value: {tickSize: 0.001, minOrderSize: 5.0}
   TTL: 24h
   ```

3. **Validate Before Submission**
   ```go
   // Check minimum size
   yesTakerTokens := orderSize / yesPrice
   if yesTakerTokens < yesMinSize {
       return error("order too small")
   }

   // Round to tick size
   price := roundToTickSize(yesPrice, yesTickSize)
   size := roundToSizePrecision(yesTakerTokens, yesTickSize)
   ```

4. **Fallback to Defaults**
   If fetch fails, uses safe defaults:
   ```
   tickSize: 0.01
   minOrderSize: 5.0
   ```

### Rounding Configuration

Different tick sizes require different decimal precision:

| Tick Size | Size Precision | Amount Precision | Example |
|-----------|----------------|------------------|---------|
| 0.1       | 2 decimals     | 3 decimals       | 12.34 tokens, $1.234 |
| 0.01      | 2 decimals     | 4 decimals       | 12.34 tokens, $1.2345 |
| 0.001     | 2 decimals     | 5 decimals       | 12.34 tokens, $1.23456 |
| 0.0001    | 2 decimals     | 6 decimals       | 12.34 tokens, $1.234567 |

**Implementation:**
```go
func getRoundingConfig(tickSize float64) (sizePrec, amountPrec int) {
    switch tickSize {
    case 0.1:
        return 2, 3
    case 0.01:
        return 2, 4
    case 0.001:
        return 2, 5
    case 0.0001:
        return 2, 6
    default:
        return 2, 4
    }
}
```

### Cache Performance

**Metrics:**
- Hit rate: ~95% (most tokens fetched once)
- Miss rate: ~5% (new tokens, cache expiry)
- Fetch time: ~50-100ms (on miss)
- Cache lookup: <1ms (on hit)

**Monitor cache stats:**
```bash
curl http://localhost:8080/metrics | grep cache

# Output:
cache_hits_total 1847
cache_misses_total 93
cache_sets_total 93
```

## Order Placement

### Order Structure

Polymarket uses EIP-712 signed orders:

```json
{
  "tokenID": "86076435890279485031516158085782",
  "price": "0.4500",
  "size": "22.22",
  "side": "BUY",
  "feeRateBps": "100",
  "nonce": "1703001234567",
  "signer": "0x742d35Cc6634C0532925a3b844Bc454e4438f44e",
  "signature": "0x1234567890abcdef...",
  "expiration": "0"
}
```

### Order Signing

**Process:**
1. Build order struct
2. Create EIP-712 typed data
3. Hash typed data
4. Sign with private key (ECDSA)
5. Encode signature (r, s, v)

**Implementation uses:**
- `github.com/polymarket/go-order-utils` for signing
- EIP-712 domain separator
- Polygon chain ID (137)

### Batch Submission

The bot submits YES and NO orders in a single batch for efficiency:

```go
POST /order
Content-Type: application/json

[
  {
    "tokenID": "yes-token-id",
    "price": "0.4500",
    ...
  },
  {
    "tokenID": "no-token-id",
    "price": "0.5000",
    ...
  }
]
```

**Response format:**
```json
[
  {
    "orderID": "0x1234...",
    "status": "LIVE",
    "asset_id": "yes-token-id",
    "price": "0.4500",
    "original_size": "22.22",
    "size_matched": "0.00",
    "side": "BUY",
    "type": "GTC",
    "created_at": "2024-12-25T12:00:00Z"
  },
  {
    "orderID": "0x5678...",
    "status": "LIVE",
    ...
  }
]
```

### Order States

| Status | Description | Next States |
|--------|-------------|-------------|
| `LIVE` | Order on book, waiting for fill | `MATCHED`, `CANCELLED` |
| `MATCHED` | Order partially filled | `MATCHED`, `LIVE` |
| `FILLED` | Order completely filled | Terminal |
| `CANCELLED` | Order cancelled by user | Terminal |
| `REJECTED` | Order rejected by exchange | Terminal |

### Error Handling

**Common Errors:**

| Error Code | Reason | Solution |
|------------|--------|----------|
| 400 | Invalid request format | Check order structure |
| 400 | Size below minimum | Increase order size |
| 400 | Price not aligned to tick | Round to tick size |
| 401 | Invalid signature | Check private key |
| 403 | Insufficient balance | Add USDC to wallet |
| 409 | Duplicate nonce | Retry with new nonce |
| 422 | Market closed | Skip market |
| 429 | Rate limit exceeded | Add delay between requests |

**Error Response:**
```json
{
  "error": "Invalid order payload",
  "message": "Size (2.5) lower than the minimum: 5.0",
  "code": 400
}
```

## Testing

### Unit Tests

Run unit tests for specific packages:

```bash
# All unit tests
go test ./...

# Specific package
go test ./internal/arbitrage -v

# With coverage
go test ./internal/markets -coverprofile=coverage.out
go tool cover -html=coverage.out

# With race detector
go test -race ./...
```

### E2E Tests

Comprehensive end-to-end tests for batch order placement:

```bash
# Run E2E tests
go test ./internal/execution -v -run TestE2E

# Tests included:
# - Batch order placement (6 scenarios)
# - Metadata fetching
# - Order response parsing
# - Rounding precision
```

**E2E Test Scenarios:**

1. **Valid orders (both sides above minimum)**
   ```
   Size: $10, YES: $0.40, NO: $0.55
   YES tokens: 25.00 (> 5.0 ✓)
   NO tokens: 18.18 (> 5.0 ✓)
   Result: Both orders accepted
   ```

2. **YES order below minimum**
   ```
   Size: $2, YES: $0.99, NO: $0.10
   YES tokens: 2.02 (< 5.0 ✗)
   Result: YES rejected, NO accepted
   ```

3. **Edge case (exactly at minimum)**
   ```
   Size: $5, YES: $1.00, NO: $1.00
   YES tokens: 5.00 (= 5.0 ✓)
   NO tokens: 5.00 (= 5.0 ✓)
   Result: Both accepted
   ```

### Integration Tests

Full pipeline tests with mocked dependencies:

```bash
# Run integration tests
go test ./internal/app -v

# Includes:
# - Complete arbitrage flow
# - Market discovery
# - Orderbook processing
```

**Integration Test Flow:**
```
Mock Gamma API → Discovery → Mock WebSocket → Orderbook Manager
→ Arbitrage Detector → Mock Executor → Assertions
```

### Test Coverage

Current coverage by package:

| Package | Coverage | Key Tests |
|---------|----------|-----------|
| **Infrastructure (pkg/)** |||
| `pkg/wallet` | Full | Balance/position tracking, metrics |
| `pkg/httpserver` | Full | Health, metrics, orderbook endpoints |
| `pkg/healthprobe` | Full | Liveness, readiness, state management |
| `pkg/cache` | 84.0% | Ristretto operations |
| **Core Logic (internal/)** |||
| `internal/markets` | 85.0% | Metadata fetching, caching |
| `internal/storage` | 77.3% | Postgres, console output |
| `internal/discovery` | 75.7% | Market polling |
| `internal/orderbook` | 51.0% | Snapshot updates |
| `internal/arbitrage` | 33.1% | Opportunity detection |
| `internal/execution` | 26.9% | Order execution |

**View full coverage:**
```bash
go test ./... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

### Benchmarks

```bash
# Run benchmarks
go test -bench=. -benchmem ./internal/orderbook

# Example output:
BenchmarkHandleMessage/book-8         1000  1.2 ms/op  2048 B/op  12 allocs/op
BenchmarkHandleMessage/price_change-8 5000  0.3 ms/op   512 B/op   3 allocs/op
```

### Mock Infrastructure

**HTTP Test Servers:**
```go
// Mock Gamma API
mockAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    json.NewEncoder(w).Encode(markets)
}))

// Mock CLOB API
mockCLOB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    json.NewEncoder(w).Encode(orderResponse)
}))
```

**WebSocket Mocks:**
```go
// Mock WebSocket messages
mockWS.SendMessage(`{
    "event_type": "book",
    "market": "test-market",
    "asset_id": "test-token",
    "bids": [["0.48", "100"]],
    "asks": [["0.52", "100"]]
}`)
```

## Performance

### Latency Metrics

| Operation | Latency | Target |
|-----------|---------|--------|
| WebSocket message parse | 0.1-0.3ms | <1ms |
| Orderbook update | 0.2-0.5ms | <1ms |
| Arbitrage detection | 0.3-0.8ms | <1ms |
| Metadata cache lookup | <0.1ms | <1ms |
| Order submission | 50-200ms | <500ms |

### Throughput

- **Orderbook updates**: 1000+ messages/sec
- **Arbitrage checks**: 500+ per second
- **Order submissions**: 10 per minute (rate limited)

### Memory Usage

Typical memory footprint:

```
Heap: 20-40 MB
Goroutines: 10-20
Connections: 1 WebSocket, N HTTP (pooled)
Cache: 5-10 MB (Ristretto)
```

**Memory by component:**
- Orderbook snapshots: ~1-2 MB (50 markets × 40 KB)
- Market cache: ~2-5 MB
- Metadata cache: ~1-2 MB
- WebSocket buffers: ~2 MB
- HTTP connection pool: ~1 MB

### Optimization Techniques

**1. Lock Contention Reduction**
```go
// Parse outside lock (50%+ improvement)
price, _ := strconv.ParseFloat(msg.Price, 64)
m.mu.Lock()
snapshot.BestBidPrice = price
m.mu.Unlock()
```

**2. Fast JSON Parsing**
```go
// 2-3x faster than encoding/json
import "github.com/goccy/go-json"
```

**3. Non-Blocking Channels**
```go
// Drop updates if consumer slow
select {
case ch <- update:
default:
    metrics.DroppedUpdates.Inc()
}
```

**4. Snapshot-Only Storage**
```go
// Store only best bid/ask, not full depth
type OrderbookSnapshot struct {
    BestBidPrice float64
    BestBidSize  float64
    BestAskPrice float64
    BestAskSize  float64
}
```

**5. Connection Pooling**
```go
// Reuse HTTP connections
httpClient := &http.Client{
    Transport: &http.Transport{
        MaxIdleConns:        100,
        MaxIdleConnsPerHost: 10,
        IdleConnTimeout:     90 * time.Second,
    },
}
```

### Profiling

**CPU Profiling:**
```bash
go run . --cpuprofile cpu.prof
go tool pprof -http=:8081 cpu.prof
```

**Memory Profiling:**
```bash
go run . --memprofile mem.prof
go tool pprof -http=:8081 mem.prof
```

**Goroutine Profiling:**
```bash
curl http://localhost:8080/debug/pprof/goroutine > goroutine.prof
go tool pprof goroutine.prof
```

## Troubleshooting

### No Opportunities Detected

**Symptoms:**
- Bot starts successfully
- Orderbook updates received
- No arbitrage opportunities logged

**Causes & Solutions:**

1. **Threshold too strict**
   ```bash
   # Current threshold: 0.995 (0.5% spread)
   # Try relaxing:
   ARB_THRESHOLD=0.98 go run .  # 2% spread
   ```

2. **Min/max trade size misconfigured**
   ```bash
   # Check trade size configuration
   ARB_MIN_TRADE_SIZE=1.0 ARB_MAX_TRADE_SIZE=10.0 go run .
   ```

3. **Efficient markets**
   - Polymarket spreads are typically tight (0.5-2%)
   - Arbitrage opportunities are rare
   - Try paper trading to see detection in action

4. **WebSocket not connected**
   ```bash
   # Check logs for:
   grep "websocket-connected" logs.txt
   grep "subscription-confirmed" logs.txt
   ```

### Orders Rejected

**"Size lower than minimum: 5.0"**
```bash
# Solution 1: Increase order size
go run . place-orders <market> --size 10.0

# Solution 2: Check actual minimum
curl "https://clob.polymarket.com/book?token_id=<token>" | jq .min_size

# Solution 3: Increase global minimum
ARB_MIN_TRADE_SIZE=20.0 go run .
```

**"Price not aligned to tick size"**
```bash
# Solution: Let bot fetch tick size automatically
# Or manually round:
# tick_size=0.001 → round to 3 decimals (0.451)
# tick_size=0.01  → round to 2 decimals (0.45)
```

**"Invalid signature"**
```bash
# Check private key format
echo $POLYMARKET_PRIVATE_KEY | wc -c
# Should be 64 characters (no 0x prefix)

# Verify key in .env
grep POLYMARKET_PRIVATE_KEY .env

# Test signing
go run . balance
```

**"Insufficient balance"**
```bash
# Check balances
go run . balance

# Add USDC to wallet on Polygon:
# 1. Bridge USDC to Polygon
# 2. Or swap MATIC → USDC on Polygon DEX
```

### High CPU Usage

**Symptoms:**
- CPU usage > 50%
- High goroutine count
- Slow orderbook updates

**Diagnostic:**
```bash
# Check goroutine count
curl http://localhost:8080/debug/pprof/goroutine?debug=1 | grep goroutine | wc -l

# Profile CPU
go run . --cpuprofile cpu.prof
go tool pprof -http=:8081 cpu.prof
```

**Solutions:**

1. **Reduce market count**
   ```bash
   DISCOVERY_MARKET_LIMIT=20 go run .
   ```

2. **Increase channel buffers**
   ```bash
   WS_MESSAGE_BUFFER_SIZE=2000 go run .
   ```

3. **Check for goroutine leaks**
   ```bash
   # Look for growing count
   watch 'curl -s localhost:8080/debug/pprof/goroutine?debug=1 | grep "goroutine profile:" '
   ```

### WebSocket Disconnects

**Symptoms:**
- Frequent reconnections in logs
- Missing orderbook updates
- "connection reset" errors

**Causes & Solutions:**

1. **Network instability**
   ```bash
   # Increase timeouts
   WS_PONG_TIMEOUT=30s go run .
   WS_RECONNECT_MAX_DELAY=60s go run .
   ```

2. **Server-side rate limits**
   ```bash
   # Reduce subscription count
   DISCOVERY_MARKET_LIMIT=20 go run .

   # Add delay between subscriptions
   time.Sleep(100 * time.Millisecond)
   ```

3. **Firewall/proxy issues**
   ```bash
   # Test WebSocket connectivity
   wscat -c wss://ws-subscriptions-clob.polymarket.com/ws/market

   # Check for HTTP proxy interference
   unset HTTP_PROXY HTTPS_PROXY
   ```

### Postgres Connection Issues

**"connection refused"**
```bash
# Check Postgres running
pg_isready -h localhost -p 5432

# Test connection
psql -h localhost -U postgres -d polymarket_arb
```

**"authentication failed"**
```bash
# Verify credentials in .env
grep POSTGRES .env

# Update pg_hba.conf for local connections
echo "host all all 127.0.0.1/32 md5" >> /etc/postgresql/14/main/pg_hba.conf
sudo systemctl restart postgresql
```

**"database does not exist"**
```bash
# Create database
createdb -h localhost -U postgres polymarket_arb

# Or via SQL
psql -h localhost -U postgres -c "CREATE DATABASE polymarket_arb;"
```

### Rate Limit Errors (429)

**Symptoms:**
- "Rate limit exceeded" errors
- Many orders rejected
- HTTP 429 responses

**Solutions:**
```bash
# Reduce order rate
EXECUTION_RATE_LIMIT=5 go run .

# Add cooldown between opportunities
EXECUTION_COOLDOWN=10s go run .

# Check current rate
curl http://localhost:8080/metrics | grep execution_trades_total
```

## Development

### Project Layout

```
polymarket-arb/
├── cmd/
│   └── polymarket-arb/
│       └── main.go              # CLI entry point
├── internal/
│   ├── app/
│   │   ├── app.go               # Application setup
│   │   ├── setup.go             # Dependency injection
│   │   └── integration_test.go  # E2E integration tests
│   ├── arbitrage/
│   │   ├── detector.go          # Arbitrage detection logic
│   │   ├── opportunity.go       # Opportunity data model
│   │   └── detector_test.go
│   ├── discovery/
│   │   ├── service.go           # Market discovery
│   │   └── service_test.go
│   ├── execution/
│   │   ├── executor.go          # Trade execution
│   │   ├── order_client.go      # CLOB API client
│   │   ├── order_placement_e2e_test.go
│   │   └── executor_test.go
│   ├── markets/
│   │   ├── metadata.go          # Fetch tick size, min size
│   │   ├── cache.go             # Metadata caching
│   │   ├── metadata_test.go
│   │   └── cache_test.go
│   ├── orderbook/
│   │   ├── manager.go           # Orderbook state management
│   │   └── manager_test.go
│   ├── storage/
│   │   ├── storage.go           # Storage interface
│   │   ├── postgres.go          # Postgres implementation
│   │   ├── console.go           # Console implementation
│   │   └── storage_test.go
│   └── testutil/
│       ├── mocks.go             # Test mocks
│       └── factories.go         # Test data factories
└── pkg/
    ├── cache/
    │   ├── cache.go             # Cache interface
    │   ├── ristretto.go         # Ristretto implementation
    │   ├── metrics.go           # Cache metrics
    │   └── cache_test.go
    ├── config/
    │   └── config.go            # Configuration
    ├── healthprobe/
    │   └── probe.go             # Health checks
    ├── httpserver/
    │   └── server.go            # HTTP server
    ├── types/
    │   ├── market.go            # Market types
    │   └── orderbook.go         # Orderbook types
    └── websocket/
        ├── client.go            # WebSocket client
        └── client_test.go
```

### Adding a New Feature

**Example: Add support for limit orders**

1. **Define types** (`pkg/types/order.go`):
   ```go
   type OrderType string

   const (
       OrderTypeMarket OrderType = "MARKET"
       OrderTypeLimit  OrderType = "LIMIT"
   )
   ```

2. **Extend opportunity** (`internal/arbitrage/opportunity.go`):
   ```go
   type Opportunity struct {
       // ... existing fields
       OrderType OrderType
   }
   ```

3. **Update detector** (`internal/arbitrage/detector.go`):
   ```go
   func (d *Detector) detect(...) (*Opportunity, bool) {
       // ... existing detection
       opp.OrderType = OrderTypeLimit
       return opp, true
   }
   ```

4. **Implement execution** (`internal/execution/executor.go`):
   ```go
   func (e *Executor) executeLimitOrder(opp *Opportunity) error {
       // Implementation
   }
   ```

5. **Add tests**:
   ```go
   func TestExecuteLimitOrder(t *testing.T) {
       // Test implementation
   }
   ```

6. **Update config** (`pkg/config/config.go`):
   ```go
   type ExecutionConfig struct {
       // ... existing fields
       DefaultOrderType OrderType `env:"EXECUTION_ORDER_TYPE" default:"MARKET"`
   }
   ```

### Code Style

**Follow standard Go conventions:**

```go
// Package comment
package arbitrage

// Import grouping: stdlib, external, internal
import (
    "context"
    "time"

    "github.com/goccy/go-json"
    "go.uber.org/zap"

    "github.com/mselser95/polymarket-arb/pkg/types"
)

// Type comment explains purpose
type Detector struct {
    config Config
    logger *zap.Logger
}

// Function comment describes behavior
// Uses named return values for clarity
func (d *Detector) detect(
    market *types.Market,
    yesBook, noBook *types.OrderbookSnapshot,
) (opp *Opportunity, exists bool) {
    // Early returns for validation
    if yesBook.BestAskPrice <= 0 {
        return nil, false
    }

    // Clear variable names
    priceSum := yesBook.BestAskPrice + noBook.BestAskPrice

    return opp, true
}
```

**Linting:**
```bash
# Run linter
golangci-lint run

# Auto-fix issues
golangci-lint run --fix

# Check specific file
golangci-lint run internal/arbitrage/detector.go
```

### Debugging

**Enable debug logging:**
```bash
LOG_LEVEL=debug make run
```

**Attach debugger (Delve):**
```bash
# Install delve
go install github.com/go-delve/delve/cmd/dlv@latest

# Start with debugger
dlv debug cmd/polymarket-arb/main.go

# Set breakpoint
(dlv) break internal/arbitrage/detector.go:156
(dlv) continue
```

**Print debug info:**
```go
d.logger.Debug("arbitrage-check",
    zap.String("market", market.Slug),
    zap.Float64("yes-ask", yesBook.BestAskPrice),
    zap.Float64("no-ask", noBook.BestAskPrice),
    zap.Float64("sum", priceSum),
    zap.Float64("threshold", d.config.Threshold))
```

### Performance Testing

**Benchmark orderbook updates:**
```bash
go test -bench=BenchmarkOrderbookUpdate -benchmem ./internal/orderbook
```

**Load test:**
```bash
# Generate synthetic orderbook updates
go run test/loadgen/main.go --rate 1000
```

**Memory leak detection:**
```bash
# Run with memory profiling
go run . --memprofile mem.prof

# Check for growing allocations
go tool pprof -alloc_space mem.prof
```

## API Reference

### Polymarket Gamma API

**Base URL:** `https://gamma-api.polymarket.com`

**GET /markets**

List active markets.

Query parameters:
- `limit` (int): Max results (default: 100)
- `offset` (int): Pagination offset
- `active` (bool): Filter active markets
- `closed` (bool): Filter closed markets

Response:
```json
[
  {
    "id": "0x1234...",
    "question": "Will Bitcoin hit $100K by Dec 31?",
    "slug": "will-bitcoin-hit-100k-by-dec-31",
    "active": true,
    "closed": false,
    "tokens": [
      {
        "token_id": "86076435890279485031516158085782",
        "outcome": "Yes",
        "price": 0.48
      },
      {
        "token_id": "86076435890279485031516158085783",
        "outcome": "No",
        "price": 0.52
      }
    ],
    "end_date_iso": "2024-12-31T23:59:59Z"
  }
]
```

### Polymarket CLOB API

**Base URL:** `https://clob.polymarket.com`

**GET /tick-size**

Get minimum tick size for a token.

Query parameters:
- `token_id` (string): Token ID

Response:
```json
{
  "minimum_tick_size": 0.001
}
```

**GET /book**

Get orderbook for a token.

Query parameters:
- `token_id` (string): Token ID

Response:
```json
{
  "market": "will-bitcoin-hit-100k",
  "asset_id": "86076435890279485031516158085782",
  "min_size": 5.0,
  "bids": [
    {"price": "0.48", "size": "100.50"},
    {"price": "0.47", "size": "50.25"}
  ],
  "asks": [
    {"price": "0.52", "size": "75.00"},
    {"price": "0.53", "size": "200.00"}
  ]
}
```

**POST /order**

Submit order(s).

Request:
```json
[
  {
    "tokenID": "86076435890279485031516158085782",
    "price": "0.4500",
    "size": "22.22",
    "side": "BUY",
    "feeRateBps": "100",
    "nonce": "1703001234567",
    "signer": "0x742d35Cc...",
    "signature": "0x1234...",
    "expiration": "0"
  }
]
```

Response:
```json
[
  {
    "orderID": "0x1234567890abcdef",
    "status": "LIVE",
    "asset_id": "86076435890279485031516158085782",
    "price": "0.4500",
    "original_size": "22.22",
    "size_matched": "0.00",
    "side": "BUY",
    "type": "GTC",
    "created_at": "2024-12-25T12:00:00Z",
    "maker_address": "0x742d35Cc..."
  }
]
```

### WebSocket API

**Endpoint:** `wss://ws-subscriptions-clob.polymarket.com/ws/market`

**Subscribe to orderbook:**
```json
{
  "type": "subscribe",
  "channel": "market",
  "market": "0x1234...",
  "assets_ids": ["token-id-1", "token-id-2"]
}
```

**Orderbook snapshot:**
```json
{
  "event_type": "book",
  "market": "0x1234...",
  "asset_id": "token-id-1",
  "timestamp": "1703001234567",
  "hash": "0xabcd...",
  "bids": [
    ["0.48", "100.50"],
    ["0.47", "50.25"]
  ],
  "asks": [
    ["0.52", "75.00"],
    ["0.53", "200.00"]
  ]
}
```

**Price update:**
```json
{
  "event_type": "price_change",
  "market": "0x1234...",
  "asset_id": "token-id-1",
  "timestamp": "1703001234568",
  "price": "0.485",
  "side": "BUY"
}
```

### Bot API Endpoints

The bot exposes HTTP endpoints for health checks and real-time orderbook data.

**GET /health**

Health check endpoint.

Response:
```json
{
  "status": "ok"
}
```

**GET /ready**

Readiness check (returns 200 when bot is fully initialized).

**GET /metrics**

Prometheus metrics endpoint (see [Monitoring & Observability](#monitoring--observability)).

**GET /api/orderbook?slug=<market-slug>**

Get live orderbook data for all outcomes in a subscribed market.

Query parameters:
- `slug` (string, required): Market slug (e.g., `will-bitcoin-hit-100k`)

Response:
```json
{
  "market_id": "0x1234...",
  "market_slug": "will-bitcoin-hit-100k",
  "question": "Will Bitcoin hit $100k in 2025?",
  "outcomes": [
    {
      "outcome": "Yes",
      "token_id": "86076435890279485031516158085782",
      "best_bid_price": 0.48,
      "best_bid_size": 150.0,
      "best_ask_price": 0.52,
      "best_ask_size": 120.0
    },
    {
      "outcome": "No",
      "token_id": "86076435890279485031516158085783",
      "best_bid_price": 0.47,
      "best_bid_size": 200.0,
      "best_ask_price": 0.53,
      "best_ask_size": 180.0
    }
  ]
}
```

Error responses:
- `400`: Missing `slug` parameter
- `404`: Market not found or not currently subscribed

**Notes:**
- Only returns data for markets currently subscribed by the bot
- Multi-outcome markets (elections, sports) return N outcome objects
- Orderbook data updates in real-time from WebSocket feed
- Best bid/ask represent top of book only (no depth)

**Example usage:**
```bash
# Query binary market
curl "http://localhost:8080/api/orderbook?slug=will-bitcoin-hit-100k"

# Query multi-outcome market (election)
curl "http://localhost:8080/api/orderbook?slug=2024-presidential-election"

# Error: market not subscribed
curl "http://localhost:8080/api/orderbook?slug=nonexistent-market"
# {"error":"market not found or not subscribed"}
```

## Deployment

### Docker

**Dockerfile:**
```dockerfile
FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /polymarket-arb cmd/polymarket-arb/main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/

COPY --from=builder /polymarket-arb .

EXPOSE 8080
CMD ["./polymarket-arb"]
```

**Build and run:**
```bash
# Build image
docker build -t polymarket-arb:latest .

# Run container
docker run -d \
  --name polymarket-arb \
  -p 8080:8080 \
  -e EXECUTION_MODE=paper \
  -e LOG_LEVEL=info \
  polymarket-arb:latest

# View logs
docker logs -f polymarket-arb

# Check metrics
curl http://localhost:8080/metrics
```

### Docker Compose

**docker-compose.yml:**
```yaml
version: '3.8'

services:
  postgres:
    image: postgres:15-alpine
    environment:
      POSTGRES_DB: polymarket_arb
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: yourpassword
    volumes:
      - postgres_data:/var/lib/postgresql/data
    ports:
      - "5432:5432"

  bot:
    build: .
    depends_on:
      - postgres
    environment:
      EXECUTION_MODE: paper
      STORAGE_MODE: postgres
      POSTGRES_HOST: postgres
      POSTGRES_DB: polymarket_arb
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: yourpassword
      LOG_LEVEL: info
    ports:
      - "8080:8080"
    restart: unless-stopped

volumes:
  postgres_data:
```

**Run with compose:**
```bash
# Start services
docker-compose up -d

# View logs
docker-compose logs -f bot

# Stop services
docker-compose down
```

### Kubernetes

**deployment.yaml:**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: polymarket-arb
spec:
  replicas: 1
  selector:
    matchLabels:
      app: polymarket-arb
  template:
    metadata:
      labels:
        app: polymarket-arb
    spec:
      containers:
      - name: bot
        image: polymarket-arb:latest
        env:
        - name: EXECUTION_MODE
          value: "paper"
        - name: STORAGE_MODE
          value: "postgres"
        - name: POSTGRES_HOST
          value: "postgres-service"
        - name: POSTGRES_PASSWORD
          valueFrom:
            secretKeyRef:
              name: postgres-secret
              key: password
        ports:
        - containerPort: 8080
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
        resources:
          requests:
            memory: "128Mi"
            cpu: "100m"
          limits:
            memory: "256Mi"
            cpu: "500m"
---
apiVersion: v1
kind: Service
metadata:
  name: polymarket-arb-service
spec:
  selector:
    app: polymarket-arb
  ports:
  - port: 8080
    targetPort: 8080
  type: ClusterIP
```

**Deploy:**
```bash
# Create secret for Postgres password
kubectl create secret generic postgres-secret \
  --from-literal=password=yourpassword

# Apply deployment
kubectl apply -f deployment.yaml

# Check status
kubectl get pods
kubectl logs -f deployment/polymarket-arb

# Port forward for metrics
kubectl port-forward deployment/polymarket-arb 8080:8080

# Check metrics
curl http://localhost:8080/metrics
```

### Monitoring

**Comprehensive Grafana Dashboard Suite**

The bot includes 7 production-ready Grafana dashboards covering all 65 metrics with 67+ panels:

1. **Trading Performance** (10 panels) - Detection → Execution → Profit pipeline
   - Opportunity detection rate, profit distribution, trade execution
   - URL: `http://localhost:3000/d/trading-performance`

2. **System Health** (15 panels) - Operational health monitoring
   - WebSocket status, connection pool metrics, message processing latency
   - URL: `http://localhost:3000/d/system-health`

3. **Wallet Metrics** (8 panels) - Balance and P&L tracking
   - USDC/MATIC balances, active positions, unrealized P&L with circuit breaker thresholds
   - URL: `http://localhost:3000/d/wallet-metrics`

4. **Orderbook Processing** (12 panels) - HFT latency performance
   - E2E arbitrage detection latency (<1ms target), orderbook update processing
   - URL: `http://localhost:3000/d/orderbook-processing`

5. **Circuit Breaker** (11 panels) - Balance protection system
   - State indicator, safety margin, balance vs thresholds, state change history
   - URL: `http://localhost:3000/d/circuit-breaker`

6. **Error Analysis** (11 panels) - Debugging and failure classification
   - Execution errors by type, rejection reasons, skipped opportunities
   - URL: `http://localhost:3000/d/error-analysis`

7. **Cache & API Performance** (12 panels) - Cache efficiency and API latency
   - Cache hit rate (>80% target), operation latency, metadata fetch duration
   - URL: `http://localhost:3000/d/cache-api`

**Prometheus scrape config:**
```yaml
scrape_configs:
  - job_name: 'arb-bot'
    static_configs:
      - targets: ['localhost:8080']
    metrics_path: '/metrics'
    scrape_interval: 15s
    labels:
      service: 'arb-bot'

  - job_name: 'wallet-tracker'
    static_configs:
      - targets: ['localhost:8081']
    metrics_path: '/metrics'
    scrape_interval: 60s
    labels:
      service: 'wallet-tracker'
```

**Key metrics to monitor:**
- **E2E Latency (p99)**: <1ms for HFT performance (Dashboard 04)
- **Cache Hit Rate**: >80% for optimal performance (Dashboard 07, 02)
- **Circuit Breaker State**: Understand execution halts (Dashboard 05)
- **Error Ratio**: <1% for healthy operation (Dashboard 06)
- **WebSocket Pool**: All 5 connections active (Dashboard 02)

**Docker Compose Monitoring Stack:**
```bash
# Start Prometheus + Grafana + bot
docker-compose up -d

# Access dashboards
open http://localhost:3000  # Grafana (admin/admin)
open http://localhost:9090  # Prometheus
```

**Comprehensive Guide:**
See [docs/MONITORING.md](docs/MONITORING.md) for detailed dashboard usage, troubleshooting, and alerting setup.

## Contributing

### Development Workflow

1. **Fork and clone**
   ```bash
   git clone https://github.com/yourusername/polymarket-arb.git
   cd polymarket-arb
   ```

2. **Create feature branch**
   ```bash
   git checkout -b feature/add-limit-orders
   ```

3. **Make changes**
   - Write code
   - Add tests
   - Update documentation

4. **Run checks**
   ```bash
   # Tests
   make test

   # Linter
   golangci-lint run

   # Format
   gofumpt -l -w .

   # Build
   make build
   ```

5. **Commit and push**
   ```bash
   git add .
   git commit -m "Add support for limit orders"
   git push origin feature/add-limit-orders
   ```

6. **Create pull request**
   - Open PR on GitHub
   - Describe changes
   - Link related issues

### Code Review Checklist

- [ ] Tests pass (`make test`)
- [ ] Linter passes (`golangci-lint run`)
- [ ] Code formatted (`gofumpt -l .`)
- [ ] Documentation updated
- [ ] Breaking changes noted
- [ ] Performance implications considered
- [ ] Error handling correct
- [ ] Logging appropriate
- [ ] Metrics added if needed

### Testing Requirements

- **New features**: Require unit tests
- **Bug fixes**: Require regression test
- **Performance changes**: Require benchmarks
- **API changes**: Require integration tests

### Documentation Requirements

- **New features**: Update README
- **Configuration**: Update .env.example
- **API changes**: Update API Reference section
- **Breaking changes**: Update CHANGELOG

## License

MIT License

Copyright (c) 2024

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

## Acknowledgments

- [Polymarket](https://polymarket.com) for providing the prediction market platform
- [go-order-utils](https://github.com/polymarket/go-order-utils) for order signing utilities
- [Ristretto](https://github.com/dgraph-io/ristretto) for high-performance caching
- [go-json](https://github.com/goccy/go-json) for fast JSON parsing

## Contact

- GitHub Issues: [Create an issue](https://github.com/yourusername/polymarket-arb/issues)
- Discussions: [GitHub Discussions](https://github.com/yourusername/polymarket-arb/discussions)

---

**Disclaimer:** This software is for educational purposes only. Trading prediction markets involves financial risk. Use at your own risk. The authors are not responsible for any financial losses incurred.
