#!/bin/bash
set -e

echo "🧪 Testing Data Persistence"
echo ""
echo "This script demonstrates that data persists across docker-compose restarts."
echo ""

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${BLUE}Step 1: Starting services...${NC}"
docker-compose up -d
sleep 10

echo ""
echo -e "${BLUE}Step 2: Checking initial state...${NC}"
INITIAL_COUNT=$(docker-compose exec -T postgres psql -U polymarket -d polymarket_arb -t -c "SELECT COUNT(*) FROM arbitrage_opportunities;" | xargs)
echo "Current row count: $INITIAL_COUNT"

echo ""
echo -e "${BLUE}Step 3: Inserting test data...${NC}"
docker-compose exec -T postgres psql -U polymarket -d polymarket_arb << 'SQL'
INSERT INTO arbitrage_opportunities (
    id, market_id, market_slug, market_question, detected_at,
    yes_bid_price, yes_bid_size, no_bid_price, no_bid_size,
    price_sum, profit_margin, profit_bps, max_trade_size,
    estimated_profit, total_fees, net_profit, net_profit_bps, config_threshold
) VALUES (
    'test-' || extract(epoch from now())::text,
    'test-market-123',
    'test-market-slug',
    'Will data persist across restarts?',
    now(),
    0.45, 100.0, 0.50, 100.0,
    0.95, 0.05, 500, 50.0,
    2.50, 0.25, 2.25, 450, 0.995
);
SQL

NEW_COUNT=$(docker-compose exec -T postgres psql -U polymarket -d polymarket_arb -t -c "SELECT COUNT(*) FROM arbitrage_opportunities;" | xargs)
echo -e "${GREEN}✅ Data inserted. New row count: $NEW_COUNT${NC}"

echo ""
echo -e "${YELLOW}Step 4: Stopping services (docker-compose down)...${NC}"
docker-compose down
echo -e "${GREEN}✅ Services stopped. Volumes remain intact.${NC}"

echo ""
echo -e "${BLUE}Step 5: Starting services again...${NC}"
docker-compose up -d
sleep 10

echo ""
echo -e "${BLUE}Step 6: Verifying data persisted...${NC}"
FINAL_COUNT=$(docker-compose exec -T postgres psql -U polymarket -d polymarket_arb -t -c "SELECT COUNT(*) FROM arbitrage_opportunities;" | xargs)

if [ "$FINAL_COUNT" -eq "$NEW_COUNT" ]; then
    echo -e "${GREEN}✅ SUCCESS! Data persisted across restart.${NC}"
    echo "   - Before: $INITIAL_COUNT rows"
    echo "   - After insert: $NEW_COUNT rows"
    echo "   - After restart: $FINAL_COUNT rows"
    echo ""
    echo "Latest entry:"
    docker-compose exec -T postgres psql -U polymarket -d polymarket_arb -c "SELECT market_question, detected_at, net_profit FROM arbitrage_opportunities ORDER BY created_at DESC LIMIT 1;"
else
    echo -e "${YELLOW}⚠️  Row count mismatch. Expected $NEW_COUNT, got $FINAL_COUNT${NC}"
    exit 1
fi

echo ""
echo -e "${BLUE}Step 7: Checking volumes...${NC}"
docker volume ls | grep polymarket

echo ""
echo -e "${GREEN}✅ Test complete! Data persistence verified.${NC}"
echo ""
echo "To clean up test data:"
echo "  docker-compose exec postgres psql -U polymarket -d polymarket_arb -c \"DELETE FROM arbitrage_opportunities WHERE id LIKE 'test-%';\""
