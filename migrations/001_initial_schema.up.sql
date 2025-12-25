-- Create arbitrage_opportunities table
CREATE TABLE IF NOT EXISTS arbitrage_opportunities (
    id VARCHAR(36) PRIMARY KEY,
    market_id VARCHAR(255) NOT NULL,
    market_slug VARCHAR(255) NOT NULL,
    market_question TEXT NOT NULL,
    detected_at TIMESTAMP NOT NULL,
    yes_bid_price DECIMAL(18, 8) NOT NULL,
    yes_bid_size DECIMAL(18, 8) NOT NULL,
    no_bid_price DECIMAL(18, 8) NOT NULL,
    no_bid_size DECIMAL(18, 8) NOT NULL,
    price_sum DECIMAL(18, 8) NOT NULL,
    profit_margin DECIMAL(18, 8) NOT NULL,
    profit_bps INTEGER NOT NULL,
    max_trade_size DECIMAL(18, 8) NOT NULL,
    estimated_profit DECIMAL(18, 8) NOT NULL,
    total_fees DECIMAL(18, 8) NOT NULL,
    net_profit DECIMAL(18, 8) NOT NULL,
    net_profit_bps INTEGER NOT NULL,
    config_threshold DECIMAL(18, 8) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes for common queries
CREATE INDEX idx_arbitrage_opportunities_market_slug ON arbitrage_opportunities(market_slug);
CREATE INDEX idx_arbitrage_opportunities_detected_at ON arbitrage_opportunities(detected_at DESC);
CREATE INDEX idx_arbitrage_opportunities_net_profit ON arbitrage_opportunities(net_profit DESC);
CREATE INDEX idx_arbitrage_opportunities_created_at ON arbitrage_opportunities(created_at DESC);

-- Create view for profitable opportunities
CREATE VIEW profitable_opportunities AS
SELECT
    id,
    market_slug,
    market_question,
    detected_at,
    yes_bid_price,
    no_bid_price,
    price_sum,
    net_profit,
    net_profit_bps,
    max_trade_size
FROM arbitrage_opportunities
WHERE net_profit > 0
ORDER BY detected_at DESC;
