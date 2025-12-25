package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all application configuration.
type Config struct {
	// Application
	LogLevel string
	HTTPPort string

	// Polymarket API
	PolymarketWSURL      string
	PolymarketGammaURL   string
	PolymarketAPIKey     string
	PolymarketSecret     string
	PolymarketPassphrase string

	// Market Discovery
	DiscoveryPollInterval time.Duration
	DiscoveryMarketLimit  int

	// WebSocket
	WSDialTimeout           time.Duration
	WSPongTimeout           time.Duration
	WSPingInterval          time.Duration
	WSReconnectInitialDelay time.Duration
	WSReconnectMaxDelay     time.Duration
	WSReconnectBackoffMult  float64
	WSMessageBufferSize     int

	// Arbitrage Detection
	ArbThreshold         float64
	ArbMinTradeSize      float64
	ArbDetectionInterval time.Duration
	ArbMakerFee          float64
	ArbTakerFee          float64

	// Execution
	ExecutionMode            string
	ExecutionMaxPositionSize float64

	// Storage
	StorageMode  string // "postgres" or "console"
	PostgresHost string
	PostgresPort string
	PostgresUser string
	PostgresPass string
	PostgresDB   string
	PostgresSSL  string
}

// LoadFromEnv loads configuration from environment variables with defaults.
func LoadFromEnv() (*Config, error) {
	cfg := &Config{
		// Application defaults
		LogLevel: getEnvOrDefault("LOG_LEVEL", "info"),
		HTTPPort: getEnvOrDefault("HTTP_PORT", "8080"),

		// Polymarket API defaults
		PolymarketWSURL:      getEnvOrDefault("POLYMARKET_WS_URL", "wss://ws-subscriptions-clob.polymarket.com/ws/market"),
		PolymarketGammaURL:   getEnvOrDefault("POLYMARKET_GAMMA_API_URL", "https://gamma-api.polymarket.com"),
		PolymarketAPIKey:     os.Getenv("POLYMARKET_API_KEY"),
		PolymarketSecret:     os.Getenv("POLYMARKET_SECRET"),
		PolymarketPassphrase: os.Getenv("POLYMARKET_PASSPHRASE"),

		// Market Discovery defaults
		DiscoveryPollInterval: getDurationOrDefault("DISCOVERY_POLL_INTERVAL", 30*time.Second),
		DiscoveryMarketLimit:  getIntOrDefault("DISCOVERY_MARKET_LIMIT", 50),

		// WebSocket defaults
		WSDialTimeout:           getDurationOrDefault("WS_DIAL_TIMEOUT", 10*time.Second),
		WSPongTimeout:           getDurationOrDefault("WS_PONG_TIMEOUT", 15*time.Second),
		WSPingInterval:          getDurationOrDefault("WS_PING_INTERVAL", 10*time.Second),
		WSReconnectInitialDelay: getDurationOrDefault("WS_RECONNECT_INITIAL_DELAY", 1*time.Second),
		WSReconnectMaxDelay:     getDurationOrDefault("WS_RECONNECT_MAX_DELAY", 30*time.Second),
		WSReconnectBackoffMult:  getFloat64OrDefault("WS_RECONNECT_BACKOFF_MULTIPLIER", 2.0),
		WSMessageBufferSize:     getIntOrDefault("WS_MESSAGE_BUFFER_SIZE", 1000),

		// Arbitrage defaults
		ArbThreshold:         getFloat64OrDefault("ARB_THRESHOLD", 0.995),
		ArbMinTradeSize:      getFloat64OrDefault("ARB_MIN_TRADE_SIZE", 10.0),
		ArbDetectionInterval: getDurationOrDefault("ARB_DETECTION_INTERVAL", 100*time.Millisecond),
		ArbMakerFee:          getFloat64OrDefault("ARB_MAKER_FEE", 0.0000), // 0% maker fee on Polymarket
		ArbTakerFee:          getFloat64OrDefault("ARB_TAKER_FEE", 0.0100), // 1% taker fee

		// Execution defaults
		ExecutionMode:            getEnvOrDefault("EXECUTION_MODE", "paper"),
		ExecutionMaxPositionSize: getFloat64OrDefault("EXECUTION_MAX_POSITION_SIZE", 1000.0),

		// Storage defaults
		StorageMode:  getEnvOrDefault("STORAGE_MODE", "console"),
		PostgresHost: getEnvOrDefault("POSTGRES_HOST", "localhost"),
		PostgresPort: getEnvOrDefault("POSTGRES_PORT", "5432"),
		PostgresUser: getEnvOrDefault("POSTGRES_USER", "polymarket"),
		PostgresPass: getEnvOrDefault("POSTGRES_PASSWORD", "polymarket123"),
		PostgresDB:   getEnvOrDefault("POSTGRES_DB", "polymarket_arb"),
		PostgresSSL:  getEnvOrDefault("POSTGRES_SSLMODE", "disable"),
	}

	err := cfg.Validate()
	if err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return cfg, nil
}

// Validate checks that configuration values are valid.
func (c *Config) Validate() error {
	if c.HTTPPort == "" {
		return fmt.Errorf("HTTP_PORT cannot be empty")
	}

	if c.PolymarketWSURL == "" {
		return fmt.Errorf("POLYMARKET_WS_URL cannot be empty")
	}

	if c.PolymarketGammaURL == "" {
		return fmt.Errorf("POLYMARKET_GAMMA_API_URL cannot be empty")
	}

	if c.ArbThreshold <= 0 || c.ArbThreshold >= 1.0 {
		return fmt.Errorf("ARB_THRESHOLD must be between 0 and 1.0, got %f", c.ArbThreshold)
	}

	if c.ExecutionMode != "paper" && c.ExecutionMode != "live" {
		return fmt.Errorf("EXECUTION_MODE must be 'paper' or 'live', got %q", c.ExecutionMode)
	}

	return nil
}

func getEnvOrDefault(key string, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func getIntOrDefault(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}

	intVal, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}

	return intVal
}

func getFloat64OrDefault(key string, defaultValue float64) float64 {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}

	floatVal, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return defaultValue
	}

	return floatVal
}

func getDurationOrDefault(key string, defaultValue time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return defaultValue
	}

	return duration
}
