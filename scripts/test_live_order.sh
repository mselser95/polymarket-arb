#!/bin/bash

# Test live order submission
# Usage: ./scripts/test_live_order.sh [--paper|--live|--mock]

set -e

MODE="${1:---paper}"

echo "=== Polymarket Live Order Tests ==="
echo ""

case "$MODE" in
  --paper)
    echo "Testing Paper Mode (Safe - No Real API Calls)"
    echo "----------------------------------------------"
    go run . test-live-order will-trump-release-the-epstein-files-by-december-22 \
      --paper \
      --size 1.0 \
      --yes-price 0.01 \
      --no-price 0.01
    ;;

  --live)
    echo "Testing Live Mode (Real Orders!)"
    echo "----------------------------------------------"
    echo "⚠️  WARNING: This will submit real orders!"
    echo "Press Ctrl+C to cancel, or wait 3 seconds..."
    sleep 3
    go run . test-live-order will-trump-release-the-epstein-files-by-december-22 \
      --live \
      --size 1.0 \
      --yes-price 0.01 \
      --no-price 0.01
    ;;

  --mock)
    echo "Testing Mock Mode (Saved Responses)"
    echo "----------------------------------------------"
    go run . test-live-order will-trump-release-the-epstein-files-by-december-22 \
      --mock \
      --size 1.0 \
      --yes-price 0.01 \
      --no-price 0.01
    ;;

  *)
    echo "Usage: $0 [--paper|--live|--mock]"
    echo ""
    echo "Modes:"
    echo "  --paper : Paper trading (simulated, no API calls)"
    echo "  --live  : Live trading (real orders, real money!)"
    echo "  --mock  : Mock mode (uses saved API responses)"
    exit 1
    ;;
esac
