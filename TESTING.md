# Testing Guide

This document describes the testing strategy, patterns, and best practices for the polymarket-arb codebase.

## Testing Philosophy

### Goals

1. **Confidence in Refactoring** - Tests should catch regressions
2. **Fast Feedback** - Unit tests run in <1s, integration tests in <20s
3. **Clear Failures** - Test names and assertions clearly indicate what broke
4. **Minimal Mocking** - Mock at boundaries (HTTP, WebSocket), not internal logic

### Test Levels

#### Unit Tests (Fast, Isolated)
- Test single components with mocked dependencies
- Run on every file save
- No network I/O, no goroutines if possible
- Located next to source: `pkg/cache/ristretto_test.go`

#### Integration Tests (Realistic, E2E)
- Test multiple components together
- Use real goroutines, channels, timing
- Mock only external dependencies (APIs, WebSocket)
- Located in `internal/app/integration_test.go`
- Tagged with `// +build integration`

## Running Tests

### Quick Commands

```bash
# Unit tests only (fast)
make test

# Integration tests only
make test-integration

# All tests
make test-all

# With race detector (slower, catches concurrency bugs)
go test -race ./...

# Specific package
go test -v ./internal/arbitrage/

# Specific test
go test -v ./internal/app/ -run TestE2E_ArbitrageFlow

# With coverage
go test -cover ./...
make test-coverage
```

### Benchmarks

```bash
# Run all benchmarks
go test -bench=. -benchmem ./...

# Specific benchmark
go test -bench=BenchmarkHandlePriceChange -benchmem ./internal/orderbook/

# Compare before/after optimization
go test -bench=. -benchmem ./internal/orderbook/ > old.txt
# ... make changes ...
go test -bench=. -benchmem ./internal/orderbook/ > new.txt
benchcmp old.txt new.txt
```

## Writing Unit Tests

### Structure

```go
func TestComponentName_MethodName(t *testing.T) {
    // Setup
    logger, _ := zap.NewDevelopment()
    component := NewComponent(&Config{Logger: logger})

    // Execute
    result, err := component.DoSomething(input)

    // Assert
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if result != expected {
        t.Errorf("expected %v, got %v", expected, result)
    }
}
```

### Good Practices

**DO:**
- Use table-driven tests for similar test cases
- Use `t.Helper()` for test helper functions
- Use `t.Parallel()` for independent tests
- Test error paths explicitly
- Use meaningful test names: `TestDetector_DetectOpportunity_BelowThreshold`

**DON'T:**
- Use `time.Sleep()` in unit tests (use channels or mocks)
- Share state between tests
- Test implementation details
- Assert on log messages (unless that's the component's job)

### Example: Table-Driven Test

```go
func TestExtractBestLevel(t *testing.T) {
    tests := []struct {
        name      string
        levels    []types.PriceLevel
        wantPrice float64
        wantSize  float64
        wantErr   bool
    }{
        {
            name:      "valid single level",
            levels:    []types.PriceLevel{{Price: "0.52", Size: "100.0"}},
            wantPrice: 0.52,
            wantSize:  100.0,
            wantErr:   false,
        },
        {
            name:    "empty levels",
            levels:  []types.PriceLevel{},
            wantErr: true,
        },
        {
            name:    "invalid price",
            levels:  []types.PriceLevel{{Price: "invalid", Size: "100.0"}},
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            price, size, err := extractBestLevel(tt.levels)

            if (err != nil) != tt.wantErr {
                t.Fatalf("wantErr=%v, got err=%v", tt.wantErr, err)
            }

            if !tt.wantErr {
                if price != tt.wantPrice {
                    t.Errorf("price: want %f, got %f", tt.wantPrice, price)
                }
                if size != tt.wantSize {
                    t.Errorf("size: want %f, got %f", tt.wantSize, size)
                }
            }
        })
    }
}
```

## Writing Integration Tests

### Build Tags

Integration tests use build tags to separate them from fast unit tests:

```go
// +build integration

package app

import "testing"

func TestE2E_ArbitrageFlow(t *testing.T) {
    // ...
}
```

Run with: `go test -tags=integration`

### Test Structure

Integration tests follow the **Given-When-Then** pattern:

```go
func TestE2E_ArbitrageFlow(t *testing.T) {
    // GIVEN: A running system with mocked external dependencies
    logger, _ := zap.NewDevelopment()
    market := testutil.CreateTestMarket("market1", "test-slug", "Will X happen?")
    mockAPI := testutil.NewMockGammaAPI([]*types.Market{market})
    defer mockAPI.Close()

    // ... setup components ...

    // WHEN: Orderbook messages create an arbitrage opportunity
    wsMsgChan <- yesBookMsg
    wsMsgChan <- noBookMsg
    time.Sleep(500 * time.Millisecond)  // Allow async processing

    // THEN: Opportunity is detected and stored
    stored := storage.GetOpportunities()
    if len(stored) == 0 {
        t.Fatal("expected at least one stored opportunity")
    }

    opp := stored[0]
    if opp.PriceSum >= 0.995 {
        t.Errorf("expected arbitrage opportunity, got price sum %f", opp.PriceSum)
    }
}
```

### Timing in Tests

**Challenges:**
- Components run in separate goroutines
- Events are asynchronous
- Test needs to wait for processing

**Patterns:**

1. **Channel-based synchronization** (preferred):
```go
select {
case market := <-discoverySvc.NewMarketsChan():
    // Test the market
case <-time.After(2 * time.Second):
    t.Fatal("timeout waiting for market")
}
```

2. **Polling with timeout** (for components without channels):
```go
deadline := time.Now().Add(2 * time.Second)
for time.Now().Before(deadline) {
    if storage.GetOpportunities() > 0 {
        break  // Success
    }
    time.Sleep(50 * time.Millisecond)
}
```

3. **Fixed delay** (last resort):
```go
time.Sleep(500 * time.Millisecond)  // Allow async processing
```

### Cleanup

Always clean up resources:

```go
func TestE2E_Something(t *testing.T) {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()  // Always cancel context

    mockAPI := testutil.NewMockGammaAPI(markets)
    defer mockAPI.Close()  // Always close server

    detector := arbitrage.New(...)
    defer detector.Close()  // Always close components
}
```

## Test Utilities

### Mock Implementations (`internal/testutil/mocks.go`)

**MockGammaAPI** - HTTP test server for Polymarket API:
```go
mockAPI := testutil.NewMockGammaAPI([]*types.Market{market1, market2})
defer mockAPI.Close()

// Simulate new market appearing
mockAPI.AddMarket(market3)
```

**MockStorage** - In-memory opportunity storage:
```go
storage := testutil.NewMockStorage()
storage.StoreOpportunity(ctx, opp)
opportunities := storage.GetOpportunities()
```

### Test Fixtures (`internal/testutil/fixtures.go`)

**Factory Functions:**
```go
// Create test market with YES/NO tokens
market := testutil.CreateTestMarket("market1", "test-slug", "Will X happen?")

// Create orderbook messages
bookMsg := testutil.CreateTestBookMessage("token-1", "market-1")
priceChangeMsg := testutil.CreateTestPriceChangeMessage("token-1", "market-1")

// Create pre-configured orderbooks with arbitrage
yesBook, noBook := testutil.CreateArbitrageOrderbooks(
    "market1", "yes-token", "no-token",
)
```

## Testing Patterns

### Testing Concurrent Code

Use `-race` flag to detect data races:

```bash
go test -race ./...
```

**Common Issues:**
- Reading/writing maps without locks
- Accessing struct fields from multiple goroutines
- Closing channels that might still have writers

**Solutions:**
- Use `sync.RWMutex` for maps
- Return copies, not pointers to internal state
- Use context cancellation to signal shutdown

### Testing Event-Driven Systems

**Challenge:** Events flow through channels asynchronously.

**Pattern:**
```go
// 1. Start components
obMgr.Start(ctx)
detector.Start(ctx)

// 2. Send input event
wsMsgChan <- orderBookMsg

// 3. Wait for output event with timeout
select {
case opp := <-detector.OpportunityChan():
    // Success - verify opportunity
case <-time.After(1 * time.Second):
    t.Fatal("timeout waiting for opportunity")
}
```

### Testing Error Paths

Don't just test the happy path:

```go
func TestDetector_Detect_InvalidOrderbook(t *testing.T) {
    detector := arbitrage.New(config, obMgr, discovery, storage)

    // Send invalid data
    invalidSnapshot := &types.OrderbookSnapshot{
        BestBidPrice: -1.0,  // Invalid
    }

    _, exists := detector.detect(market, invalidSnapshot, noSnapshot)
    if exists {
        t.Error("expected no opportunity for invalid orderbook")
    }
}
```

### Testing Cache Behavior

Ristretto cache uses async admission, so tests must wait:

```go
cache := cache.NewRistrettoCache(config)

cache.Set("key", value, 1*time.Hour)
cache.Wait()  // CRITICAL: Wait for async admission

val, exists := cache.Get("key")
if !exists {
    t.Fatal("expected cache hit after Wait()")
}
```

## Coverage

### Measuring Coverage

```bash
# Per-package coverage
go test -cover ./...

# HTML coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Makefile target
make test-coverage
```

### Coverage Goals

- **Critical paths:** 80%+ (arbitrage detection, orderbook updates)
- **Infrastructure:** 60%+ (config, HTTP server, health checks)
- **Overall:** 60%+ (achievable with current test suite)

**Not everything needs 100% coverage:**
- Logging statements
- Error paths that can't realistically occur
- One-time initialization code

## Debugging Test Failures

### Enable Verbose Output

```bash
go test -v ./internal/app/ -run TestE2E_ArbitrageFlow
```

### Enable Debug Logging

```go
// In test setup
logger, _ := zap.NewDevelopment()  // Debug level by default
```

### Check Goroutine Leaks

```bash
# Run with race detector
go test -race -v ./internal/app/

# Look for warnings like:
# "WARNING: DATA RACE"
# "goroutine X [running]:"
```

### Common Failures

**"timeout waiting for X"**
- Component didn't start properly
- Channel buffer full (check logs for "channel-full")
- Logic bug in event handling

**"expected X, got Y"**
- Check test setup (did you create the right test data?)
- Check async timing (did you wait long enough?)
- Check component initialization (are all dependencies set up?)

**"panic: send on closed channel"**
- Component shutdown race condition
- Always check `ctx.Done()` before sending on channels

## Best Practices

### DO

- Write tests before fixing bugs (TDD for bug fixes)
- Test one thing per test function
- Use descriptive test names
- Clean up resources with `defer`
- Use `t.Helper()` for assertion helpers
- Run tests with `-race` regularly

### DON'T

- Skip tests (`t.Skip()`) without a good reason
- Use global state in tests
- Depend on test execution order
- Use hardcoded ports (use `httptest` instead)
- Assert on exact timing (tests run slower in CI)
- Ignore failing tests

## CI/CD Integration

### GitHub Actions Example

```yaml
name: Test
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.22'

      - name: Unit Tests
        run: go test -race -cover ./...

      - name: Integration Tests
        run: go test -tags=integration -race -v ./...

      - name: Upload Coverage
        run: |
          go test -coverprofile=coverage.out ./...
          go tool cover -html=coverage.out -o coverage.html
```

## Resources

- [Go Testing Documentation](https://golang.org/pkg/testing/)
- [Table Driven Tests](https://github.com/golang/go/wiki/TableDrivenTests)
- [Testify Library](https://github.com/stretchr/testify) (optional, not currently used)
