-- Drop view
DROP VIEW IF EXISTS profitable_opportunities;

-- Drop indexes
DROP INDEX IF EXISTS idx_arbitrage_opportunities_created_at;
DROP INDEX IF EXISTS idx_arbitrage_opportunities_net_profit;
DROP INDEX IF EXISTS idx_arbitrage_opportunities_detected_at;
DROP INDEX IF EXISTS idx_arbitrage_opportunities_market_slug;

-- Drop table
DROP TABLE IF EXISTS arbitrage_opportunities;
