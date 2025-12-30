# Monitoring & Observability Guide

Comprehensive guide to monitoring the Polymarket arbitrage bot using Grafana dashboards and Prometheus metrics.

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Dashboard Guide](#dashboard-guide)
  - [1. Trading Performance](#1-trading-performance)
  - [2. System Health](#2-system-health)
  - [3. Wallet Metrics](#3-wallet-metrics)
  - [4. Orderbook Processing](#4-orderbook-processing)
  - [5. Circuit Breaker](#5-circuit-breaker)
  - [6. Error Analysis](#6-error-analysis)
  - [7. Cache & API Performance](#7-cache--api-performance)
- [Key Metrics Reference](#key-metrics-reference)
- [Troubleshooting](#troubleshooting)
- [Performance Tuning](#performance-tuning)
- [Alerting Setup](#alerting-setup)

## Overview

The bot exposes **65 Prometheus metrics** across **7 comprehensive Grafana dashboards** with **67+ panels** total.

**Monitoring Stack:**
- **Prometheus**: Metric collection and storage
- **Grafana**: Visualization and dashboards
- **HTTP Server**: Exposes metrics on `:8080` (arb-bot) and `:8081` (wallet-tracker)

**Architecture:**
```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│  Arb Bot     │────▶│  Prometheus  │────▶│   Grafana    │
│  :8080       │     │  :9090       │     │   :3000      │
└──────────────┘     └──────────────┘     └──────────────┘
       │
       │
┌──────────────┐
│ Wallet       │
│ Tracker      │
│ :8081        │
└──────────────┘
```

## Quick Start

### Using Docker Compose

```bash
# Start full monitoring stack (Prometheus + Grafana + bot)
docker-compose up -d

# Access Grafana dashboards
open http://localhost:3000
# Login: admin / admin

# View raw metrics
curl http://localhost:8080/metrics | grep polymarket
```

### Manual Setup

```bash
# Terminal 1: Start bot
go run . run

# Terminal 2: Start wallet tracker (optional)
go run . track-balance --interval 60s --port 8081

# Terminal 3: Check metrics
curl http://localhost:8080/metrics
```

## Dashboard Guide

### 1. Trading Performance

**Purpose**: Core arbitrage detection and execution monitoring

**URL**: `http://localhost:3000/d/trading-performance`

**Refresh**: 10s | **Time Range**: 6h

#### Panels (10 total)

1. **Opportunities Detected (Rate)** - Time series
   - Shows opportunities/sec detected in real-time
   - **Target**: >0.1 ops/sec in active markets
   - **Alert**: If 0 for >10min, check market filters or WebSocket connection

2. **Execution Success Rate** - Gauge
   - Ratio of executed trades vs received opportunities
   - **Green**: >80% (healthy)
   - **Yellow**: 50-80% (investigate rejections)
   - **Red**: <50% (check errors dashboard)

3. **Cumulative Profit (by Mode)** - Time series
   - Total profit by execution mode (paper/live)
   - Use template variable to filter by mode
   - **Track**: Steady upward trend in live mode

4. **Profit Margin Distribution (BPS)** - Heatmap
   - Distribution of profit margins in basis points
   - **Healthy**: Cluster around 30-100 bps
   - **Warning**: If too low (<10 bps), increase `ARB_MAX_PRICE_SUM`

5. **Net Profit After Fees (BPS)** - Heatmap
   - Profit after 2% total taker fees
   - **Healthy**: Positive values only
   - **Warning**: Negative values indicate unprofitable trades

6. **Trade Size Distribution (USD)** - Histogram
   - Distribution of trade sizes
   - **Check**: Within `ARB_MIN_TRADE_SIZE` and `ARB_MAX_TRADE_SIZE` bounds

7. **Opportunities Rejected (by Reason)** - Pie chart
   - Breakdown: `min_size`, `max_size`, `low_profit`
   - **High `min_size` rejections**: Markets have high minimums (>$5)
   - **High `low_profit` rejections**: Consider lowering `ARB_MAX_PRICE_SUM`

8. **Trades Executed (by Outcome)** - Stat
   - Count by YES vs NO outcome
   - **Healthy**: Roughly balanced distribution

9. **Markets with Opportunities** - Stat
   - Number of markets showing arbitrage
   - **Green**: >10 markets
   - **Yellow**: 5-10 markets
   - **Red**: <5 markets (expand market discovery)

10. **Opportunity Conversion Rate** - Gauge
    - % of detected opportunities actually executed
    - **Green**: >70%
    - **Yellow**: 30-70%
    - **Red**: <30% (investigate execution errors)

#### Key Insights

**Question**: "How profitable is the bot?"
- Check **Cumulative Profit** panel (mode=live)

**Question**: "Why aren't trades executing?"
- Check **Execution Success Rate** + **Opportunities Rejected** pie chart
- Cross-reference with Error Analysis dashboard

**Question**: "Are opportunities real or artifacts?"
- Check **Profit Margin Distribution** - should be >0 after fees
- Verify **Net Profit After Fees** is positive

### 2. System Health

**Purpose**: Operational health of all bot components

**URL**: `http://localhost:3000/d/system-health`

**Refresh**: 5s | **Time Range**: 1h

#### Panels (15 total)

1. **WebSocket Connection Status** - Stat
   - Binary indicator: CONNECTED (green) / DISCONNECTED (red)
   - **Critical**: If disconnected, no orderbook updates received

2. **WebSocket Pool Status** - Gauge
   - Number of active pool connections
   - **Target**: 5/5 connections active
   - **Warning**: <3 connections (degraded performance)

3. **Message Processing Latency (p99)** - Stat
   - 99th percentile of WebSocket message handling
   - **Green**: <1ms
   - **Yellow**: 1-10ms
   - **Red**: >10ms (bottleneck in processing)

4. **Cache Hit Rate** - Gauge
   - Percentage of cache hits vs total requests
   - **Green**: >80%
   - **Orange**: 50-80% (suboptimal)
   - **Red**: <50% (cache ineffective)

5. **Active Subscriptions** - Stat
   - Total WebSocket subscriptions across pool
   - **Target**: Matches number of tracked markets
   - **Warning**: If 0, check market discovery

6. **Pool Subscription Distribution** - Heatmap
   - Distribution of subscriptions per connection
   - **Healthy**: Even distribution (each connection ~20% of total)
   - **Warning**: Uneven distribution indicates hash collision

7. **Pool Multiplex Latency (p99)** - Stat
   - Latency of routing messages through pool
   - **Green**: <100µs
   - **Yellow**: 100-1000µs
   - **Red**: >1ms (pool bottleneck)

8. **Execution Errors by Type** - Time series (stacked)
   - Rate of errors by type: network, api, validation, funds
   - **Action**: Investigate spike in specific error type

9. **Messages Dropped by Component** - Time series (stacked)
   - WebSocket vs Orderbook dropped messages
   - **Healthy**: 0 dropped messages
   - **Warning**: >0 indicates buffer overflow or slow consumer

10. **Orderbook Lock Contention** - Time series (p50/p95/p99)
    - Time spent waiting for mutex locks
    - **Target**: <0.5ms (p99)
    - **Warning**: >1ms indicates lock contention issues

11. **WebSocket Reconnection Attempts** - Time series
    - Attempts vs failures
    - **Healthy**: Attempts = 0 (stable connection)
    - **Warning**: Frequent reconnects indicate network issues

12. **Cache Operations Latency** - Time series (p99)
    - Get/Set operation latency
    - **Target**: <10µs
    - **Warning**: >100µs indicates cache performance degradation

13. **WebSocket Connection Duration** - Time series (median/p95)
    - How long connections stay alive
    - **Healthy**: Hours to days
    - **Warning**: <5min indicates instability

14. **Discovery Poll Duration** - Time series (p50/p99)
    - Latency of Gamma API market discovery
    - **Target**: <1s (p99)
    - **Warning**: >5s indicates API slowness

15. **Orderbook Snapshots Tracked** - Time series
    - Number of markets with orderbook state
    - **Target**: Matches subscriptions count

#### Key Insights

**Question**: "Is the bot healthy?"
- **WebSocket Connection Status**: CONNECTED
- **WebSocket Pool Status**: 5/5 active
- **Cache Hit Rate**: >80%
- **Messages Dropped**: 0

**Question**: "Why is latency high?"
- Check **Message Processing Latency** vs **Orderbook Lock Contention**
- If lock contention high: Reduce concurrent access or increase buffer sizes

**Question**: "Why are reconnections happening?"
- Check **WebSocket Reconnection Attempts** timeline
- Correlate with network events or rate limiting

### 3. Wallet Metrics

**Purpose**: Track wallet balance, positions, and P&L

**URL**: `http://localhost:3000/d/wallet-metrics`

**Refresh**: 10s | **Time Range**: 24h

#### Panels (8 total)

1. **USDC Balance** - Stat
   - Current USDC balance for trading
   - **Warning**: If near circuit breaker disable threshold

2. **MATIC Balance** - Stat
   - MATIC for gas fees
   - **Warning**: <0.1 MATIC (insufficient for gas)

3. **Portfolio Value** - Stat
   - Total: USDC + position values
   - Track overall portfolio growth

4. **Active Positions** - Stat
   - Number of open positions
   - **Note**: Positions remain until markets settle

5. **Unrealized P&L** - Stat (with thresholds)
   - Dollar value of unrealized gains/losses
   - **Green**: Positive
   - **Red**: Negative

6. **Unrealized P&L %** - Gauge
   - Percentage return on positions
   - **Range**: -100% to +100%
   - **Thresholds**: Red (<-10%), Yellow (-10% to 0%), Green (>0%)

7. **USDC Balance Over Time (with Circuit Breaker Thresholds)** - Time series
   - **Blue solid**: Current balance
   - **Red dashed**: Disable threshold (trading stops)
   - **Yellow dashed**: Enable threshold (trading resumes)
   - **Critical**: Watch for balance approaching disable threshold

8. **Portfolio Value Over Time** - Time series
   - Historical portfolio value
   - **Target**: Upward trend in live mode

#### Key Insights

**Question**: "How much profit have I made?"
- Check **Unrealized P&L** for open positions
- Check Trading Performance dashboard for realized profit

**Question**: "Why did trading stop?"
- Check **USDC Balance Over Time** against circuit breaker thresholds
- If balance < red line, circuit breaker disabled trading
- Need to add USDC to resume

**Question**: "Do I have enough gas?"
- Check **MATIC Balance**
- Need >0.1 MATIC for on-chain operations (rare)

### 4. Orderbook Processing

**Purpose**: HFT latency performance monitoring

**URL**: `http://localhost:3000/d/orderbook-processing`

**Refresh**: 5s | **Time Range**: 1h

#### Panels (12 total)

1. **E2E Arbitrage Detection Latency (P50/P95/P99)** - Time series
   - **CRITICAL METRIC**: Time from orderbook update → opportunity detection
   - **Target**: <1ms (p99) for HFT performance
   - **Warning**: >5ms indicates performance degradation

2. **E2E Latency Heatmap** - Heatmap
   - Distribution of E2E latencies
   - **Healthy**: Cluster <1ms
   - **Warning**: Long tail >10ms

3. **Orderbook Update Processing (P50/P95/P99)** - Time series
   - Time to process and update orderbook state
   - **Target**: <1ms (p99)

4. **Mutex Lock Contention (P50/P95/P99)** - Time series
   - Time waiting for orderbook locks
   - **Target**: <0.5ms (p99)
   - **Warning**: High contention slows detection

5. **WebSocket Pool Multiplex Latency (P50/P95/P99)** - Time series
   - Time to route messages through pool
   - **Target**: <100µs (p99)

6. **WebSocket Message Latency (P50/P95/P99)** - Time series
   - Network + parsing latency
   - **Target**: <10ms (p99)
   - **Note**: Includes network RTT to Polymarket

7. **Arbitrage Detection Duration** - Gauge
   - Average detection computation time
   - **Target**: <100µs

8. **Execution Duration (Paper vs Live)** - Time series (p99)
   - Time to execute trades
   - **Paper**: <1ms (simulated)
   - **Live**: 100-500ms (network + API)

9. **Discovery Poll Duration** - Time series (p95)
   - Gamma API latency
   - **Target**: <1s

10. **Metadata Fetch Duration** - Time series (p95)
    - Time to fetch market metadata
    - **Target**: <2s (with 24h cache)

11. **Orderbook Update Throughput** - Stat
    - Messages processed per second
    - **High volume**: >100 msg/sec indicates active markets

12. **Latency Budget Breakdown** - Stacked bar
    - Where time is spent: network, parsing, processing, detection
    - **Optimize**: Focus on largest component

#### Key Insights

**Question**: "Is the bot fast enough for HFT?"
- **E2E Latency p99**: Must be <1ms
- If >1ms, check lock contention and processing time

**Question**: "Where is the bottleneck?"
- Compare **Orderbook Update Processing** vs **Lock Contention** vs **Message Latency**
- **Network-bound**: High WebSocket Message Latency
- **CPU-bound**: High Orderbook Update Processing
- **Contention-bound**: High Lock Contention

**Question**: "How to optimize performance?"
- Reduce subscriptions if throughput >1000 msg/sec
- Increase buffer sizes if messages dropping
- Profile code if lock contention high

### 5. Circuit Breaker

**Purpose**: Monitor balance protection system

**URL**: `http://localhost:3000/d/circuit-breaker`

**Refresh**: 10s | **Time Range**: 6h

#### Panels (11 total)

1. **Circuit Breaker State** - Stat (large)
   - **ENABLED ✅** (green): Trading active
   - **DISABLED ⛔** (red): Trading stopped due to low balance

2. **Safety Margin** - Gauge
   - `current_balance - disable_threshold`
   - **Green**: >$10 buffer
   - **Yellow**: $0-$10 buffer
   - **Red**: Negative (trading disabled)

3. **Balance vs Thresholds** - Time series
   - **Blue solid**: Current balance
   - **Red dashed**: Disable threshold
   - **Yellow dashed**: Enable threshold
   - **Visual**: Easy to see proximity to thresholds

4. **Current USDC Balance** - Stat
   - Real-time balance from RPC
   - **Thresholds**: Red (<$5), Yellow ($5-$20), Green (>$20)

5. **Disable Threshold** - Stat
   - `max(avg_trade_size × 3.0, $5.00)`
   - **Dynamic**: Adjusts based on your trading patterns

6. **Enable Threshold** - Stat
   - `disable_threshold × 1.5`
   - **Hysteresis**: Prevents flapping

7. **Average Trade Size (Rolling)** - Stat
   - Average of last 20 trades
   - **Basis**: For calculating thresholds

8. **Average Trade Size Over Time** - Time series
   - Historical trade size
   - **Monitor**: If increasing, thresholds will rise

9. **State Change History** - State timeline
   - Visual timeline of enable/disable events
   - **Healthy**: Few state changes
   - **Warning**: Frequent flapping (adjust hysteresis)

10. **State Changes (Total)** - Stat (24h)
    - Count of state transitions
    - **Green**: <5 changes/day
    - **Yellow**: 5-20 changes
    - **Red**: >20 changes (too much flapping)

11. **Opportunities Skipped (Circuit Breaker)** - Time series
    - Opportunities not executed due to circuit breaker
    - **Impact metric**: Shows cost of protection

#### Key Insights

**Question**: "Why isn't the bot trading?"
- Check **Circuit Breaker State**
- If DISABLED ⛔, check **Current USDC Balance** vs **Disable Threshold**
- **Action**: Add USDC until balance > enable threshold (yellow line)

**Question**: "How much buffer do I have?"
- Check **Safety Margin** gauge
- **Green**: >$10 buffer (safe)
- **Red**: Approaching disable threshold

**Question**: "What are my thresholds?"
- **Disable Threshold**: Trading stops below this
- **Enable Threshold**: Trading resumes above this
- **Formula**: Based on your average trade size × 3.0

**Question**: "Is the circuit breaker too aggressive?"
- Check **State Changes** count
- If >20 changes/day, increase `CIRCUIT_BREAKER_HYSTERESIS_RATIO` from 1.5 to 2.0

**Configuration Tuning:**

```bash
# Conservative (large buffer)
CIRCUIT_BREAKER_TRADE_MULTIPLIER=5.0      # 5x avg trade
CIRCUIT_BREAKER_MIN_ABSOLUTE=10.0          # $10 floor
CIRCUIT_BREAKER_HYSTERESIS_RATIO=2.0       # 2x to re-enable

# Aggressive (use more funds)
CIRCUIT_BREAKER_TRADE_MULTIPLIER=2.0       # 2x avg trade
CIRCUIT_BREAKER_MIN_ABSOLUTE=3.0           # $3 floor
CIRCUIT_BREAKER_HYSTERESIS_RATIO=1.3       # 1.3x to re-enable
```

### 6. Error Analysis

**Purpose**: Debugging and failure classification

**URL**: `http://localhost:3000/d/error-analysis`

**Refresh**: 10s | **Time Range**: 6h

#### Panels (11 total)

1. **Execution Errors by Type** - Pie chart
   - Breakdown: `network`, `api`, `validation`, `funds`, `unknown`
   - **Focus**: Largest slice indicates root cause

2. **Error Ratio (Errors vs Success)** - Gauge
   - `errors / (errors + success) × 100`
   - **Green**: <1%
   - **Yellow**: 1-5%
   - **Red**: >5% (investigation required)

3. **Execution Error Rate** - Time series
   - Errors per minute
   - **Alert**: Spike indicates systemic issue

4. **Execution Errors Timeline (by Type)** - Stacked time series
   - Error rate over time by type
   - **Pattern detection**: Are errors correlated with events?

5. **Opportunities Rejected** - Bar chart (by reason)
   - `min_size`, `max_size`, `low_profit`
   - **Actionable**: Adjust thresholds based on rejections

6. **Rejection Rate Timeline (by Reason)** - Stacked area
   - Historical rejection patterns
   - **Monitor**: If rejections increasing, market conditions changing

7. **Opportunities Skipped** - Pie chart (by reason)
   - `circuit_breaker`, `balance_low`, `size_invalid`
   - **Most common**: circuit_breaker when balance low

8. **Skipped Opportunity Rate (by Reason)** - Time series
   - Opportunities/min skipped by reason
   - **Cost analysis**: How many opportunities lost to protection?

9. **WebSocket Errors** - Time series
   - Rate of WebSocket connection errors
   - **Healthy**: 0 errors
   - **Warning**: >0 indicates network instability

10. **Discovery Errors (API Failures)** - Time series
    - Rate of Gamma API failures
    - **Healthy**: 0 errors
    - **Warning**: >0 indicates API issues

11. **Wallet Update Errors (RPC Failures)** - Time series
    - Rate of Polygon RPC errors (balance checks)
    - **Warning**: >0 indicates RPC endpoint issues

#### Key Insights

**Question**: "Why are trades failing?"
- Check **Execution Errors by Type** pie chart
- **Network errors**: Check internet connection, CLOB API status
- **API errors**: Check Polymarket API status, credentials
- **Validation errors**: Check order sizes against market minimums
- **Funds errors**: Check USDC balance and allowance

**Question**: "Why are so many opportunities rejected?"
- Check **Opportunities Rejected** bar chart
- **High `low_profit`**: Markets tight, consider lowering `ARB_MAX_PRICE_SUM`
- **High `min_size`**: Markets have high minimums, increase `ARB_MIN_TRADE_SIZE`
- **High `max_size`**: Hit position limits, increase `ARB_MAX_TRADE_SIZE`

**Question**: "Is the error rate acceptable?"
- Check **Error Ratio** gauge
- **<1%**: Healthy
- **1-5%**: Monitor but acceptable
- **>5%**: Investigate immediately

### 7. Cache & API Performance

**Purpose**: Cache efficiency and external API monitoring

**URL**: `http://localhost:3000/d/cache-api`

**Refresh**: 10s | **Time Range**: 6h

#### Panels (12 total)

1. **Cache Hit Rate** - Gauge
   - `hits / (hits + misses) × 100`
   - **Green**: >80%
   - **Orange**: 50-80%
   - **Red**: <50% (cache ineffective)

2. **Total Cache Hits (Last Hour)** - Stat
   - Count of cache hits
   - **Monitor**: Should be much higher than misses

3. **Total Cache Misses (Last Hour)** - Stat
   - Count of cache misses
   - **Healthy**: <20% of hits

4. **Cache Operations Rate** - Stat
   - Total operations per second (gets + sets + deletes)
   - **High load**: >100 ops/sec indicates heavy usage

5. **Cache Sets/sec** - Stat
   - Rate of cache writes
   - **Pattern**: Spike at startup, steady state low

6. **Cache Hit Rate Timeline** - Time series
   - Hit rate % over time
   - **Healthy**: Stable >80% after warmup

7. **Cache Hits vs Misses** - Stacked area
   - Visual comparison of hits vs misses
   - **Target**: Hits should dominate

8. **Cache Operation Latency (by Type)** - Time series (p95)
   - Latency for get/set/delete operations
   - **Target**: <10µs for all operations
   - **Warning**: >100µs indicates Ristretto performance issue

9. **Market Metadata Fetch Rate** - Time series
   - API calls per minute to fetch metadata
   - **Healthy**: Low rate (<1 call/min) due to 24h cache

10. **Metadata Fetch Duration (p50/p95)** - Time series
    - Latency of Gamma API metadata fetches
    - **Target**: <2s (p95)
    - **Warning**: >5s indicates API slowness

11. **Metadata Cache Efficiency** - Pie chart
    - Hits vs misses for metadata cache
    - **Target**: >95% hits (24h TTL is effective)

12. **Discovery Poll Duration (p95)** - Time series
    - Latency of Gamma API market discovery
    - **Target**: <1s
    - **Warning**: Correlated slowness with metadata fetches indicates Gamma API degradation

#### Key Insights

**Question**: "Is the cache working?"
- Check **Cache Hit Rate** gauge
- If <80%, check:
  - Cache size configuration
  - TTL settings (24h for metadata)
  - Are keys being invalidated too frequently?

**Question**: "Is Gamma API slow?"
- Check **Metadata Fetch Duration** and **Discovery Poll Duration**
- Both should be <2s
- If >5s, Polymarket API may be experiencing issues

**Question**: "How to improve cache performance?"
- **Low hit rate**: Increase cache size or TTL
- **High latency**: Check Ristretto configuration
- **High miss rate**: Are cache keys stable?

**Optimization:**

```bash
# Increase metadata cache TTL if markets change infrequently
MARKETS_CACHE_TTL=48h  # Default: 24h

# Adjust Ristretto cache size
# (done in code, pkg/cache/ristretto.go)
```

## Key Metrics Reference

### Arbitrage Detection

| Metric | Type | Labels | Description | Target |
|--------|------|--------|-------------|--------|
| `polymarket_arb_opportunities_detected_total` | Counter | - | Total opportunities detected | - |
| `polymarket_arb_opportunity_profit_bps` | Histogram | - | Profit in basis points | 30-100 bps |
| `polymarket_arb_net_profit_bps` | Histogram | - | Profit after fees | >0 bps |
| `polymarket_arb_opportunity_size_usd` | Histogram | - | Trade size in USD | $1-$100 |
| `polymarket_arb_opportunities_rejected_total` | Counter | `reason` | Rejected opportunities | - |
| `polymarket_arb_e2e_latency_seconds` | Histogram | - | End-to-end detection latency | <1ms (p99) |
| `polymarket_arb_detection_duration_seconds` | Histogram | - | Detection computation time | <100µs |

### Execution

| Metric | Type | Labels | Description | Target |
|--------|------|--------|-------------|--------|
| `polymarket_execution_opportunities_received_total` | Counter | - | Opportunities received | - |
| `polymarket_execution_opportunities_executed_total` | Counter | - | Successfully executed | >70% |
| `polymarket_execution_opportunities_skipped_total` | Counter | `reason` | Skipped opportunities | - |
| `polymarket_execution_trades_total` | Counter | `mode`, `outcome` | Trade count | - |
| `polymarket_execution_profit_realized_usd` | Gauge | `mode` | Cumulative profit | Growing |
| `polymarket_execution_errors_total` | Counter | - | Total errors | <1% |
| `polymarket_execution_errors_by_type_total` | Counter | `error_type` | Errors by type | - |
| `polymarket_execution_duration_seconds` | Histogram | `mode` | Execution latency | Paper <1ms, Live <500ms |

### WebSocket

| Metric | Type | Labels | Description | Target |
|--------|------|--------|-------------|--------|
| `polymarket_ws_active_connections` | Gauge | - | WebSocket connected (0/1) | 1 |
| `polymarket_ws_subscription_count` | Gauge | - | Total subscriptions | Matches markets |
| `polymarket_ws_messages_received_total` | Counter | `event_type` | Messages received | - |
| `polymarket_ws_messages_dropped_total` | Counter | - | Dropped messages | 0 |
| `polymarket_ws_message_latency_seconds` | Histogram | - | Message processing latency | <1ms (p99) |
| `polymarket_ws_pool_active_connections` | Gauge | - | Pool connections | 5 |
| `polymarket_ws_pool_subscription_distribution` | Histogram | - | Subscriptions per connection | Even |
| `polymarket_ws_pool_multiplex_latency_seconds` | Histogram | - | Pool routing latency | <100µs (p99) |
| `polymarket_ws_errors_total` | Counter | - | WebSocket errors | 0 |
| `polymarket_ws_reconnect_attempts_total` | Counter | - | Reconnection attempts | 0 |
| `polymarket_ws_reconnect_failures_total` | Counter | - | Failed reconnections | 0 |

### Orderbook

| Metric | Type | Labels | Description | Target |
|--------|------|--------|-------------|--------|
| `polymarket_orderbook_snapshots_tracked` | Gauge | - | Tracked orderbooks | Matches subscriptions |
| `polymarket_orderbook_updates_total` | Counter | `event_type` | Update count | - |
| `polymarket_orderbook_updates_dropped_total` | Counter | - | Dropped updates | 0 |
| `polymarket_orderbook_update_processing_duration_seconds` | Histogram | - | Processing latency | <1ms (p99) |
| `polymarket_orderbook_lock_contention_seconds` | Histogram | - | Lock wait time | <0.5ms (p99) |

### Circuit Breaker

| Metric | Type | Labels | Description | Target |
|--------|------|--------|-------------|--------|
| `polymarket_circuit_breaker_enabled` | Gauge | - | State (0=disabled, 1=enabled) | 1 |
| `polymarket_circuit_breaker_balance_usdc` | Gauge | - | Current balance | >disable threshold |
| `polymarket_circuit_breaker_disable_threshold_usdc` | Gauge | - | Stop trading below | - |
| `polymarket_circuit_breaker_enable_threshold_usdc` | Gauge | - | Resume trading above | - |
| `polymarket_circuit_breaker_avg_trade_size_usdc` | Gauge | - | Rolling average trade size | - |
| `polymarket_circuit_breaker_state_changes_total` | Counter | - | State transitions | <5/day |

### Cache

| Metric | Type | Labels | Description | Target |
|--------|------|--------|-------------|--------|
| `polymarket_cache_hit_rate` | Gauge | - | Hit rate (0.0-1.0) | >0.8 |
| `polymarket_cache_hits_total` | Counter | - | Cache hits | >> misses |
| `polymarket_cache_misses_total` | Counter | - | Cache misses | Low |
| `polymarket_cache_sets_total` | Counter | - | Cache writes | - |
| `polymarket_cache_deletes_total` | Counter | - | Cache deletions | - |
| `polymarket_cache_evictions_total` | Counter | - | Cache evictions | Low |
| `polymarket_cache_operation_duration_seconds` | Histogram | `operation` | Operation latency | <10µs |

### Wallet (via `track-balance` command)

| Metric | Type | Labels | Description | Target |
|--------|------|--------|-------------|--------|
| `polymarket_wallet_matic_balance` | Gauge | - | MATIC balance | >0.1 |
| `polymarket_wallet_usdc_balance` | Gauge | - | USDC balance | >disable threshold |
| `polymarket_wallet_usdc_allowance` | Gauge | - | Approved USDC | Unlimited |
| `polymarket_wallet_active_positions` | Gauge | - | Open positions | - |
| `polymarket_wallet_total_position_value` | Gauge | - | Position value (USD) | - |
| `polymarket_wallet_total_position_cost` | Gauge | - | Cost basis (USD) | - |
| `polymarket_wallet_unrealized_pnl` | Gauge | - | Unrealized P&L (USD) | Positive |
| `polymarket_wallet_unrealized_pnl_percent` | Gauge | - | P&L percentage | Positive |
| `polymarket_wallet_portfolio_value` | Gauge | - | Total portfolio (USD) | Growing |
| `polymarket_wallet_update_errors_total` | Counter | - | Update errors | 0 |

### Discovery & Markets

| Metric | Type | Labels | Description | Target |
|--------|------|--------|-------------|--------|
| `polymarket_discovery_markets_total` | Gauge | - | Discovered markets | - |
| `polymarket_discovery_markets_filtered_by_end_date_total` | Counter | - | Markets filtered by date | - |
| `polymarket_discovery_poll_duration_seconds` | Histogram | - | Poll latency | <1s |
| `polymarket_discovery_errors_total` | Counter | - | API errors | 0 |
| `polymarket_markets_metadata_fetched_total` | Counter | - | Metadata fetches | - |
| `polymarket_markets_metadata_fetch_duration_seconds` | Histogram | - | Fetch latency | <2s |
| `polymarket_markets_metadata_cache_hits_total` | Counter | - | Metadata cache hits | High |
| `polymarket_markets_metadata_cache_misses_total` | Counter | - | Metadata cache misses | Low |

## Troubleshooting

### No Opportunities Detected

**Symptoms**: `polymarket_arb_opportunities_detected_total` = 0 for >10min

**Possible Causes**:
1. **No markets tracked**
   - Check: `polymarket_discovery_markets_total` > 0
   - Fix: Increase `DISCOVERY_MARKET_LIMIT` or adjust `ARB_MAX_MARKET_DURATION`

2. **WebSocket disconnected**
   - Check: `polymarket_ws_active_connections` = 1
   - Fix: Check network, restart bot

3. **Threshold too strict**
   - Check: `ARB_MAX_PRICE_SUM` value
   - Fix: Lower to 0.98 (2% spread) temporarily to test

4. **Orderbook empty**
   - Check: `polymarket_orderbook_snapshots_tracked` > 0
   - Fix: Markets may have no liquidity (normal for some markets)

### High Execution Errors

**Symptoms**: `polymarket_execution_errors_total` > 5% of trades

**Diagnosis**:
1. Check **Error Analysis** dashboard → **Execution Errors by Type**
2. Common types:
   - **network**: Check internet, Polymarket API status
   - **api**: Check credentials, API key validity
   - **validation**: Check trade sizes against market minimums
   - **funds**: Check USDC balance and allowance

**Fixes**:
```bash
# Validation errors (order too small)
ARB_MIN_TRADE_SIZE=10.0  # Increase to $10

# Funds errors (insufficient balance)
go run . balance  # Check balance
# Add USDC to wallet

# API errors (invalid credentials)
go run . derive-api-creds  # Regenerate credentials
```

### Circuit Breaker Keeps Disabling

**Symptoms**: `polymarket_circuit_breaker_state_changes_total` > 20/day

**Cause**: Balance oscillating around disable threshold (flapping)

**Fix**: Increase hysteresis ratio
```bash
# Current: enable_threshold = disable × 1.5
CIRCUIT_BREAKER_HYSTERESIS_RATIO=2.0  # Change to 2.0

# Or increase buffer
CIRCUIT_BREAKER_TRADE_MULTIPLIER=5.0  # Increase from 3.0 to 5.0
```

### High WebSocket Reconnections

**Symptoms**: `polymarket_ws_reconnect_attempts_total` increasing frequently

**Possible Causes**:
1. **Network instability**
   - Check: Internet connection stability
   - Fix: Use stable network, avoid WiFi

2. **Rate limiting**
   - Check: `DISCOVERY_MARKET_LIMIT` value
   - Fix: Reduce limit to <100 markets

3. **Polymarket API issues**
   - Check: Polymarket status page
   - Fix: Wait for API recovery

### Low Cache Hit Rate

**Symptoms**: `polymarket_cache_hit_rate` < 0.5

**Possible Causes**:
1. **Cache not warmed up**
   - Check: Just started bot?
   - Fix: Wait 5-10 minutes for cache warmup

2. **TTL too short**
   - Check: `MARKETS_CACHE_TTL` setting
   - Fix: Increase to 48h if markets change infrequently

3. **Keys not stable**
   - Check: Are market slugs changing?
   - Fix: Investigate key generation logic

### High Latency (>1ms)

**Symptoms**: `polymarket_arb_e2e_latency_seconds` (p99) > 1ms

**Diagnosis**: Use **Orderbook Processing** dashboard

**Possible Causes**:
1. **Lock contention**
   - Check: `polymarket_orderbook_lock_contention_seconds` (p99)
   - Fix: Reduce concurrent access or optimize critical sections

2. **High message volume**
   - Check: `polymarket_ws_messages_received_total` rate
   - Fix: Reduce subscriptions or increase buffer sizes

3. **CPU-bound processing**
   - Check: Orderbook update processing duration
   - Fix: Profile code, optimize hot paths

## Performance Tuning

### For Maximum Throughput

**Goal**: Process as many markets as possible

```bash
# Configuration
DISCOVERY_MARKET_LIMIT=200           # Track 200 markets
WS_POOL_SIZE=10                      # 10 WebSocket connections
ARB_MAX_PRICE_SUM=0.99                   # Relaxed threshold
ARB_MIN_TRADE_SIZE=1.0               # Low minimum

# Trade-offs
# - Higher CPU usage
# - More WebSocket connections
# - More potential opportunities (but lower quality)
```

### For Minimum Latency

**Goal**: Detect and execute as fast as possible (<1ms)

```bash
# Configuration
DISCOVERY_MARKET_LIMIT=20            # Only 20 high-volume markets
WS_POOL_SIZE=5                       # Standard pool
ARB_MAX_PRICE_SUM=0.995                  # Strict threshold
LOG_LEVEL=warn                       # Minimal logging

# Code optimizations
# - Reduce lock contention
# - Increase channel buffer sizes
# - Profile hot paths
```

### For Profitability

**Goal**: Maximize net profit after fees

```bash
# Configuration
ARB_MAX_PRICE_SUM=0.995                  # 0.5% spread minimum
ARB_MIN_TRADE_SIZE=10.0              # Meet market minimums
ARB_MAX_TRADE_SIZE=100.0             # Large positions
EXECUTION_MODE=live                  # Real trading

# Strategy
# - Focus on quality over quantity
# - Use circuit breaker for protection
# - Monitor Error Ratio dashboard
```

### For Safety (Paper Trading)

**Goal**: Test strategies without risk

```bash
# Configuration
EXECUTION_MODE=paper                 # Simulated trading
ARB_MAX_PRICE_SUM=0.98                   # Aggressive (more opportunities)
ARB_MIN_TRADE_SIZE=1.0               # Low minimum
STORAGE_MODE=postgres                # Persistent logging

# Monitoring
# - Run for 1 week
# - Check cumulative profit in Trading Performance dashboard
# - Analyze rejection reasons in Error Analysis dashboard
```

## Alerting Setup

While the user requested no alerting for now, here's a reference configuration for future use.

### Prometheus Alert Rules

```yaml
# /etc/prometheus/rules/polymarket-arb.yml
groups:
  - name: polymarket-arb-critical
    interval: 30s
    rules:
      - alert: WebSocketDisconnected
        expr: polymarket_ws_active_connections{service="arb-bot"} == 0
        for: 1m
        labels:
          severity: critical
          component: websocket
        annotations:
          summary: "WebSocket connection lost"
          description: "No orderbook updates being received"

      - alert: CircuitBreakerDisabled
        expr: polymarket_circuit_breaker_enabled{service="arb-bot"} == 0
        for: 5m
        labels:
          severity: warning
          component: circuit-breaker
        annotations:
          summary: "Trading halted due to low balance"
          description: "USDC balance below disable threshold: {{ $value }}"

      - alert: HighErrorRate
        expr: |
          (
            rate(polymarket_execution_errors_total{service="arb-bot"}[5m])
            /
            (rate(polymarket_execution_trades_total{service="arb-bot"}[5m]) + rate(polymarket_execution_errors_total{service="arb-bot"}[5m]))
          ) > 0.05
        for: 10m
        labels:
          severity: warning
          component: execution
        annotations:
          summary: "Execution error rate >5%"
          description: "Error rate: {{ $value | humanizePercentage }}"

  - name: polymarket-arb-performance
    interval: 1m
    rules:
      - alert: HighLatency
        expr: |
          histogram_quantile(0.99, rate(polymarket_arb_e2e_latency_seconds_bucket{service="arb-bot"}[1m])) > 0.005
        for: 5m
        labels:
          severity: warning
          component: performance
        annotations:
          summary: "E2E latency >5ms (p99)"
          description: "Latency: {{ $value | humanizeDuration }}"

      - alert: LowCacheHitRate
        expr: polymarket_cache_hit_rate{service="arb-bot"} < 0.5
        for: 10m
        labels:
          severity: warning
          component: cache
        annotations:
          summary: "Cache hit rate <50%"
          description: "Hit rate: {{ $value | humanizePercentage }}"

      - alert: NoOpportunitiesDetected
        expr: rate(polymarket_arb_opportunities_detected_total{service="arb-bot"}[10m]) == 0
        for: 30m
        labels:
          severity: info
          component: detection
        annotations:
          summary: "No arbitrage opportunities in 30 minutes"
          description: "Check market conditions and thresholds"
```

### AlertManager Configuration

```yaml
# /etc/alertmanager/alertmanager.yml
route:
  receiver: 'default'
  group_by: ['alertname', 'component']
  group_wait: 10s
  group_interval: 10s
  repeat_interval: 12h
  routes:
    - match:
        severity: critical
      receiver: 'pagerduty'
      continue: true
    - match:
        severity: warning
      receiver: 'slack'
    - match:
        severity: info
      receiver: 'email'

receivers:
  - name: 'default'
    webhook_configs:
      - url: 'http://localhost:5001/'

  - name: 'pagerduty'
    pagerduty_configs:
      - service_key: '<YOUR_PAGERDUTY_KEY>'

  - name: 'slack'
    slack_configs:
      - api_url: '<YOUR_SLACK_WEBHOOK>'
        channel: '#polymarket-alerts'

  - name: 'email'
    email_configs:
      - to: 'alerts@example.com'
        from: 'alertmanager@example.com'
        smarthost: 'smtp.gmail.com:587'
```

### Grafana Alerts (Alternative)

Grafana 9+ supports unified alerting. Create alerts directly in dashboards:

1. **Navigate to dashboard panel**
2. **Edit panel → Alert tab**
3. **Set conditions**:
   - Example: `polymarket_ws_active_connections < 1`
   - Evaluation: Every 1m for 1m
4. **Configure notification channel** (Slack, email, PagerDuty)
5. **Save dashboard**

## Summary

This monitoring suite provides:
- **67+ dashboard panels** across 7 dashboards
- **65 Prometheus metrics** covering all bot components
- **Sub-millisecond observability** for HFT performance
- **Comprehensive debugging** with error classification
- **Balance protection** monitoring via circuit breaker dashboard

**Key Takeaways**:
1. Start with **Trading Performance** dashboard for business metrics
2. Use **System Health** to verify operational status
3. Monitor **Circuit Breaker** to avoid surprise halts
4. Debug with **Error Analysis** when issues occur
5. Optimize with **Orderbook Processing** latency metrics

**Need Help?**
- Check [CLAUDE.md](../CLAUDE.md) for bot architecture
- Review [TESTING.md](../TESTING.md) for testing patterns
- See [README.md](../README.md) for general usage
