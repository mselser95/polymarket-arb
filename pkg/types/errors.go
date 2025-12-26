package types

import "fmt"

// OrderError represents an error that occurred during order placement or execution.
type OrderError struct {
	Code    string // API error code or internal error code
	Message string // Human-readable error message
	OrderID string // Order ID if available
	Side    string // YES or NO
}

func (e *OrderError) Error() string {
	if e.OrderID != "" {
		return fmt.Sprintf("%s order failed (ID: %s): %s (%s)", e.Side, e.OrderID, e.Message, e.Code)
	}

	return fmt.Sprintf("%s order failed: %s (%s)", e.Side, e.Message, e.Code)
}

// Known Polymarket CLOB API error codes
const (
	ErrInvalidMinTickSize = "INVALID_ORDER_MIN_TICK_SIZE"
	ErrNotEnoughBalance   = "INVALID_ORDER_NOT_ENOUGH_BALANCE"
	ErrFOKNotFilled       = "FOK_ORDER_NOT_FILLED_ERROR"
	ErrMarketNotReady     = "MARKET_NOT_READY"
	ErrUnmatched          = "UNMATCHED"
	ErrUnknownStatus      = "UNKNOWN_STATUS"
)
