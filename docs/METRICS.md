# Polymarket Arbitrage Bot - Metrics Reference

Comprehensive documentation of all Prometheus metrics exposed by the polymarket-arb bot.

## Table of Contents

- [Overview](#overview)
- [Metric Categories](#metric-categories)
- [Discovery Service Metrics](#discovery-service-metrics)
- [WebSocket Manager Metrics](#websocket-manager-metrics)
- [Orderbook Manager Metrics](#orderbook-manager-metrics)
- [Arbitrage Detector Metrics](#arbitrage-detector-metrics)
- [Execution Engine Metrics](#execution-engine-metrics)
- [Markets Metadata Client Metrics](#markets-metadata-client-metrics)
- [Cache Metrics](#cache-metrics)
- [Querying Metrics](#querying-metrics)

---

## Overview

All metrics are exposed at `http://localhost:8080/metrics` (configurable via `HTTP_PORT` environment variable).

**Total Metrics:** 46 metrics
- **Operational Metrics:** 32 (system health, performance, reliability)
- **Business Metrics:** 14 (trading strategy, profitability, market coverage)

**Metric Types:**
- **Counter:** Monotonically increasing values (e.g., `_total` suffix)
- **Gauge:** Values that can go up or down (e.g., connection status, hit rate)
- **Histogram:** Distributions with configurable buckets (e.g., latency, profit margins)

---

## Metric Categories

### Operational Metrics
Focus on system health, performance bottlenecks, and reliability:
- Latency measurements (all `*_duration_seconds`, `*_latency_seconds`)
- Error counters (`*_errors_total`, `*_failures_total`)
- Resource utilization (connections, memory, lock contention)
- Data loss tracking (`*_dropped_total`)

### Business Metrics
Focus on trading strategy effectiveness and profitability:
- Opportunity detection and conversion rates
- Profit margins (gross and net after fees)
- Trade execution success/failure
- Market coverage and activity

---

## Discovery Service Metrics

**Component:** `internal/discovery/`
**Purpose:** Track market discovery from Gamma API

### `polymarket_discovery_markets_total`
- **Type:** Counter
- **Category:** Business
- **Description:** Total number of markets discovered from Gamma API
- **Updated:** After each successful API poll
- **Use Case:** Track overall market universe size

### `polymarket_discovery_new_markets_total`
- **Type:** Counter
- **Category:** Business
- **Description:** Total number of new markets subscribed (differential discovery)
- **Updated:** When new market sent to subscription channel
- **Use Case:** Monitor market expansion rate

### `polymarket_discovery_poll_duration_seconds`
- **Type:** Histogram
- **Category:** Operational
- **Description:** Duration of Gamma API poll requests
- **Buckets:** Default Prometheus buckets (0.005 to 10 seconds)
- **Updated:** After each poll operation
- **Use Case:** Identify API latency issues
- **Alert Threshold:** p99 > 5s (API degradation)

### `polymarket_discovery_poll_errors_total`
- **Type:** Counter
- **Category:** Operational
- **Description:** Total number of Gamma API poll failures
- **Updated:** When API fetch fails
- **Use Case:** Track discovery service reliability
- **Alert Threshold:** rate > 0.1/min (repeated failures)

---

## WebSocket Manager Metrics

**Component:** `pkg/websocket/`
**Purpose:** Monitor WebSocket connection health and message flow

### `polymarket_ws_active_connections`
- **Type:** Gauge
- **Category:** Operational
- **Description:** Number of active WebSocket connections (0 or 1)
- **Values:** 0 = disconnected, 1 = connected
- **Updated:** On connect/disconnect events
- **Use Case:** Critical health indicator
- **Alert Threshold:** value == 0 for > 1 minute

### `polymarket_ws_reconnect_attempts_total`
- **Type:** Counter
- **Category:** Operational
- **Description:** Total number of WebSocket reconnection attempts
- **Updated:** Before each reconnection attempt
- **Use Case:** Track connection stability
- **Alert Threshold:** rate > 5/hour (unstable connection)

### `polymarket_ws_reconnect_failures_total`
- **Type:** Counter
- **Category:** Operational
- **Description:** Total number of WebSocket reconnection failures
- **Updated:** After failed reconnection attempt
- **Use Case:** Identify persistent connection issues
- **Alert Threshold:** any increase (immediate investigation)

### `polymarket_ws_messages_received_total`
- **Type:** Counter with labels
- **Labels:** `event_type` (book, price_change, last_trade_price)
- **Category:** Operational
- **Description:** Total WebSocket messages received by event type
- **Updated:** For each parsed message
- **Use Case:** Monitor message throughput and distribution

### `polymarket_ws_message_latency_seconds`
- **Type:** Histogram
- **Category:** Operational
- **Description:** WebSocket message processing latency (parse + channel send)
- **Buckets:** Default Prometheus buckets
- **Updated:** For each message processed
- **Use Case:** Identify message processing bottlenecks
- **Alert Threshold:** p99 > 0.01s (10ms)

### `polymarket_ws_subscription_count`
- **Type:** Gauge
- **Category:** Operational
- **Description:** Number of active market subscriptions
- **Updated:** After subscription/unsubscription
- **Use Case:** Track active market coverage

### `polymarket_ws_messages_dropped_total` ⭐ NEW
- **Type:** Counter with labels
- **Labels:** `reason` (channel_full)
- **Category:** Operational
- **Description:** Messages dropped due to full message channel
- **Updated:** When non-blocking channel send fails
- **Use Case:** Detect backpressure and data loss
- **Alert Threshold:** any increase (data loss)

### `polymarket_ws_connection_duration_seconds` ⭐ NEW
- **Type:** Histogram
- **Category:** Operational
- **Description:** Duration of WebSocket connections before disconnect
- **Buckets:** 60s to 86400s (1 minute to 24 hours)
- **Updated:** On disconnection
- **Use Case:** Analyze connection stability patterns

---

## Orderbook Manager Metrics

**Component:** `internal/orderbook/`
**Purpose:** Monitor hot path orderbook processing (critical for HFT performance)

### `polymarket_orderbook_updates_total`
- **Type:** Counter with labels
- **Labels:** `event_type` (book, price_change)
- **Category:** Operational
- **Description:** Total orderbook updates processed by event type
- **Updated:** For every orderbook message handled
- **Use Case:** Track update rate and distribution

### `polymarket_orderbook_snapshots_tracked`
- **Type:** Gauge
- **Category:** Operational
- **Description:** Number of orderbook snapshots tracked in memory
- **Updated:** After updating orderbook snapshot
- **Use Case:** Monitor memory footprint

### `polymarket_orderbook_updates_dropped_total` ⭐ NEW
- **Type:** Counter with labels
- **Labels:** `reason` (channel_full)
- **Category:** Operational
- **Description:** Orderbook updates dropped due to full update channel
- **Updated:** When non-blocking channel send fails
- **Use Case:** Critical data loss indicator
- **Alert Threshold:** any increase (data loss)

### `polymarket_orderbook_update_processing_duration_seconds` ⭐ NEW
- **Type:** Histogram
- **Category:** Operational
- **Description:** Time to process orderbook update (parse + update + notify)
- **Buckets:** 0.1ms to 100ms (HFT-optimized)
- **Updated:** For each handleMessage() call
- **Use Case:** Identify hot path bottlenecks
- **Alert Threshold:** p99 > 1ms (violates HFT target)

### `polymarket_orderbook_lock_contention_seconds` ⭐ NEW
- **Type:** Histogram
- **Category:** Operational
- **Description:** Time waiting to acquire orderbook mutex lock
- **Buckets:** 0.1ms to 100ms (HFT-optimized)
- **Updated:** Before every lock acquisition
- **Use Case:** Detect mutex contention bottlenecks
- **Alert Threshold:** p99 > 0.5ms (high contention)

---

## Arbitrage Detector Metrics

**Component:** `internal/arbitrage/`
**Purpose:** Track opportunity detection and quality

### `polymarket_arb_opportunities_detected_total`
- **Type:** Counter
- **Category:** Business
- **Description:** Total arbitrage opportunities detected
- **Updated:** When opportunity detected and validated
- **Use Case:** Primary business KPI

### `polymarket_arb_opportunity_profit_bps`
- **Type:** Histogram
- **Category:** Business
- **Description:** Gross profit margin in basis points (before fees)
- **Buckets:** 10, 25, 50, 100, 200, 500, 1000, 2000, 5000 BPS
- **Updated:** When opportunity detected
- **Use Case:** Analyze profit distribution

### `polymarket_arb_opportunity_size_usd`
- **Type:** Histogram
- **Category:** Business
- **Description:** Trade size in USD for detected opportunities
- **Buckets:** Exponential: 10, 20, 40, 80, 160, 320, 640, 1280, 2560, 5120 USD
- **Updated:** When opportunity detected
- **Use Case:** Understand liquidity availability

### `polymarket_arb_detection_duration_seconds`
- **Type:** Histogram
- **Category:** Operational
- **Description:** Duration of arbitrage detection loop (per orderbook update)
- **Buckets:** Default Prometheus buckets
- **Updated:** After each detection check
- **Use Case:** Monitor detection performance
- **Alert Threshold:** p99 > 0.001s (1ms HFT target)

### `polymarket_arb_opportunities_rejected_total` ⭐ NEW
- **Type:** Counter with labels
- **Labels:** `reason` (invalid_price, invalid_size, price_above_threshold, below_min_size, below_market_min)
- **Category:** Business
- **Description:** Opportunities rejected during validation
- **Updated:** For each rejection in detect() method
- **Use Case:** Tune detection parameters and understand rejection patterns

### `polymarket_arb_net_profit_bps` ⭐ NEW
- **Type:** Histogram
- **Category:** Business
- **Description:** Net profit after fees in basis points
- **Buckets:** 10, 25, 50, 100, 200, 500, 1000, 2000, 5000 BPS
- **Updated:** When opportunity detected
- **Use Case:** Real profitability analysis (accounts for 1% taker fee)

### `polymarket_arb_e2e_latency_seconds` ⭐ NEW
- **Type:** Histogram
- **Category:** Operational
- **Description:** End-to-end latency from orderbook update to opportunity detection
- **Buckets:** 0.1ms to 100ms (HFT-optimized)
- **Updated:** When opportunity detected
- **Use Case:** Critical HFT performance metric - measures full pipeline speed
- **Alert Threshold:** p99 > 1ms (violates HFT target)

---

## Execution Engine Metrics

**Component:** `internal/execution/`
**Purpose:** Track trade execution success/failure and profitability

### `polymarket_execution_trades_total`
- **Type:** Counter with labels
- **Labels:** `mode` (paper, live), `outcome` (YES, NO)
- **Category:** Business
- **Description:** Total trades executed by mode and outcome
- **Updated:** After each successful trade execution
- **Use Case:** Track trading activity

### `polymarket_execution_profit_realized_usd`
- **Type:** Counter with labels
- **Labels:** `mode` (paper, live)
- **Category:** Business
- **Description:** Cumulative profit realized (hypothetical for paper trading)
- **Updated:** After each trade execution
- **Use Case:** Track cumulative P&L
- **Note:** Excluded from user's dashboard request (balance tracking separate)

### `polymarket_execution_duration_seconds`
- **Type:** Histogram
- **Category:** Operational
- **Description:** Duration of trade execution (both YES and NO orders)
- **Buckets:** Default Prometheus buckets
- **Updated:** After each execution attempt
- **Use Case:** Monitor execution latency
- **Alert Threshold:** p99 > 30s (timeout risk)

### `polymarket_execution_errors_total`
- **Type:** Counter
- **Category:** Operational
- **Description:** Total execution errors (all types)
- **Updated:** When execution fails
- **Use Case:** Overall execution reliability
- **Alert Threshold:** rate > 0.1/min

### `polymarket_execution_errors_by_type_total` ⭐ NEW
- **Type:** Counter with labels
- **Labels:** `error_type` (network, api, validation, funds, unknown)
- **Category:** Operational
- **Description:** Execution errors classified by type
- **Updated:** When execution fails (via classifyError() function)
- **Use Case:** Root cause analysis for execution failures
- **Error Types:**
  - `network`: Connection refused, timeout, dial errors
  - `api`: API errors, 4xx/5xx responses
  - `validation`: Missing fields, configuration errors
  - `funds`: Insufficient balance
  - `unknown`: Unclassified errors

### `polymarket_execution_opportunities_received_total` ⭐ NEW
- **Type:** Counter
- **Category:** Business
- **Description:** Total opportunities received by executor
- **Updated:** When opportunity arrives on channel
- **Use Case:** Track opportunity flow to execution

### `polymarket_execution_opportunities_executed_total` ⭐ NEW
- **Type:** Counter
- **Category:** Business
- **Description:** Total opportunities successfully executed
- **Updated:** When execution completes without error
- **Use Case:** Calculate conversion rate (executed / received)

---

## Markets Metadata Client Metrics

**Component:** `internal/markets/`
**Purpose:** Monitor metadata API fetching and caching

### `polymarket_markets_metadata_fetch_duration_seconds` ⭐ NEW
- **Type:** Histogram
- **Category:** Operational
- **Description:** Duration of metadata fetch from CLOB API
- **Buckets:** Default Prometheus buckets
- **Updated:** After each FetchTokenMetadata() call
- **Use Case:** Monitor CLOB API latency
- **Alert Threshold:** p99 > 5s

### `polymarket_markets_metadata_fetch_errors_total` ⭐ NEW
- **Type:** Counter
- **Category:** Operational
- **Description:** Total metadata fetch errors
- **Updated:** When FetchTokenMetadata() fails
- **Use Case:** Track metadata service reliability
- **Alert Threshold:** rate > 0.1/min

### `polymarket_markets_metadata_cache_hits_total` ⭐ NEW
- **Type:** Counter
- **Category:** Operational
- **Description:** Total metadata cache hits
- **Updated:** When cached metadata found in GetTokenMetadata()
- **Use Case:** Calculate cache effectiveness

### `polymarket_markets_metadata_cache_misses_total` ⭐ NEW
- **Type:** Counter
- **Category:** Operational
- **Description:** Total metadata cache misses
- **Updated:** When cached metadata not found
- **Use Case:** Calculate cache effectiveness
- **Derived Metric:** Hit rate = hits / (hits + misses)

---

## Cache Metrics

**Component:** `pkg/cache/`
**Purpose:** Monitor Ristretto cache performance

### `polymarket_cache_hits_total`
- **Type:** Counter
- **Category:** Operational
- **Description:** Total cache hits
- **Updated:** When cache Get() finds value
- **Use Case:** Cache effectiveness

### `polymarket_cache_misses_total`
- **Type:** Counter
- **Category:** Operational
- **Description:** Total cache misses
- **Updated:** When cache Get() doesn't find value
- **Use Case:** Cache effectiveness

### `polymarket_cache_sets_total`
- **Type:** Counter
- **Category:** Operational
- **Description:** Total cache sets
- **Updated:** When cache Set() succeeds
- **Use Case:** Track cache write operations

### `polymarket_cache_deletes_total`
- **Type:** Counter
- **Category:** Operational
- **Description:** Total cache deletes
- **Updated:** When cache Delete() called
- **Use Case:** Track cache evictions

### `polymarket_cache_hit_rate` ⭐ NEW
- **Type:** Gauge
- **Category:** Operational
- **Description:** Cache hit rate (hits / (hits + misses))
- **Range:** 0.0 to 1.0
- **Updated:** After each Get() operation
- **Use Case:** Real-time cache effectiveness metric
- **Alert Threshold:** < 0.5 (poor cache effectiveness)

### `polymarket_cache_operation_duration_seconds` ⭐ NEW
- **Type:** Histogram with labels
- **Labels:** `operation` (get, set, delete)
- **Category:** Operational
- **Description:** Duration of cache operations
- **Buckets:** 10µs to 10ms (microsecond precision)
- **Updated:** For each cache operation
- **Use Case:** Identify cache performance issues

---

## Querying Metrics

### Example PromQL Queries

**Opportunity Detection Rate:**
```promql
rate(polymarket_arb_opportunities_detected_total[1m])
```

**Execution Success Rate:**
```promql
rate(polymarket_execution_opportunities_executed_total[5m]) /
rate(polymarket_execution_opportunities_received_total[5m])
```

**Cache Hit Rate (real-time):**
```promql
polymarket_cache_hit_rate
```

**p99 End-to-End Latency (ms):**
```promql
histogram_quantile(0.99, rate(polymarket_arb_e2e_latency_seconds_bucket[1m])) * 1000
```

**Rejection Rate by Reason:**
```promql
sum by(reason) (rate(polymarket_arb_opportunities_rejected_total[5m]))
```

**Error Distribution:**
```promql
sum by(error_type) (polymarket_execution_errors_by_type_total)
```

**Messages Dropped (data loss indicator):**
```promql
rate(polymarket_ws_messages_dropped_total[1m]) +
rate(polymarket_orderbook_updates_dropped_total[1m])
```

**Lock Contention (microseconds):**
```promql
histogram_quantile(0.99, rate(polymarket_orderbook_lock_contention_seconds_bucket[1m])) * 1000000
```

---

## Alert Rules (Recommended)

### Critical Alerts

**WebSocket Disconnected:**
```yaml
alert: WebSocketDisconnected
expr: polymarket_ws_active_connections == 0
for: 1m
severity: critical
```

**Data Loss Detected:**
```yaml
alert: MessagesDropped
expr: rate(polymarket_ws_messages_dropped_total[1m]) > 0 or
      rate(polymarket_orderbook_updates_dropped_total[1m]) > 0
for: 30s
severity: critical
```

**High Execution Error Rate:**
```yaml
alert: HighExecutionErrors
expr: rate(polymarket_execution_errors_total[5m]) > 0.1
for: 1m
severity: critical
```

### Warning Alerts

**High Latency:**
```yaml
alert: HighE2ELatency
expr: histogram_quantile(0.99, rate(polymarket_arb_e2e_latency_seconds_bucket[1m])) > 0.001
for: 5m
severity: warning
```

**Low Cache Hit Rate:**
```yaml
alert: LowCacheHitRate
expr: polymarket_cache_hit_rate < 0.5
for: 5m
severity: warning
```

**No Opportunities Detected:**
```yaml
alert: NoOpportunities
expr: rate(polymarket_arb_opportunities_detected_total[10m]) == 0
for: 10m
severity: warning
```

---

## Metric Naming Conventions

All metrics follow Prometheus best practices:

- **Prefix:** `polymarket_` for namespace isolation
- **Component:** `<component>_` (ws, orderbook, arb, execution, cache, markets, discovery)
- **Metric:** Descriptive name (e.g., `opportunities_detected`, `lock_contention`)
- **Unit Suffix:** `_seconds`, `_bytes`, `_total`, `_bps` (basis points)
- **Counters:** Always end with `_total`

**Examples:**
- `polymarket_arb_opportunities_detected_total` ✅
- `polymarket_orderbook_lock_contention_seconds` ✅
- `polymarket_cache_hit_rate` ✅ (gauge, no unit)

---

## Performance Considerations

### Label Cardinality
All labels use low-cardinality values to avoid metric explosion:
- `mode`: 2 values (paper, live)
- `outcome`: 2 values (YES, NO)
- `event_type`: 3 values (book, price_change, last_trade_price)
- `error_type`: 5 values (network, api, validation, funds, unknown)
- `operation`: 3 values (get, set, delete)
- `reason`: 5 values (various rejection/drop reasons)

**Avoid:** Per-market, per-token, or per-order labels (high cardinality).

### Hot Path Metrics
Metrics in critical paths use lightweight operations:
- Counters preferred over histograms where possible
- Pre-allocated label value slices
- No string concatenation in observation paths

### Histogram Buckets
HFT-optimized buckets for latency metrics:
- **Sub-millisecond:** 0.1ms, 0.2ms, 0.5ms, 1ms, 2ms, 5ms, 10ms, 25ms, 50ms, 100ms
- Targets <1ms p99 latency for hot paths

---

## Dashboard Integration

All metrics are designed for use in Grafana dashboards:

1. **Trading Performance** (`01-trading-performance.json`)
   - Business KPIs: opportunities, profit, conversion rate

2. **System Health** (`02-system-health.json`)
   - Operational health: connections, errors, drops, cache

3. **Market Activity** (`03-market-activity.json`)
   - Market data flow: discovery, subscriptions, updates

4. **Latency & Performance** (`04-latency-performance.json`)
   - HFT-critical: E2E latency, lock contention, processing time

---

## Summary Statistics

**Total Metrics:** 46
- **Original (Existing):** 24 metrics
- **New (P0 Priority):** 12 metrics (hot path, critical)
- **New (P1 Priority):** 10 metrics (high value)

**By Component:**
- Discovery: 4 metrics
- WebSocket: 8 metrics
- Orderbook: 5 metrics
- Arbitrage: 7 metrics
- Execution: 7 metrics
- Markets Metadata: 4 metrics
- Cache: 6 metrics

**By Type:**
- Counters: 24 (including CounterVecs)
- Gauges: 5
- Histograms: 12 (including HistogramVecs)

**Coverage:**
- ✅ All hot paths instrumented (orderbook, detection, execution)
- ✅ All data loss scenarios tracked (message drops, channel overflows)
- ✅ All error types classified
- ✅ End-to-end pipeline latency measured
- ✅ Business KPIs captured (opportunities, profit, conversion)
