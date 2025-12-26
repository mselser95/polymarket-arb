package httpserver

import (
	"encoding/json"
	"net/http"

	"github.com/mselser95/polymarket-arb/internal/discovery"
	"github.com/mselser95/polymarket-arb/internal/orderbook"
	"go.uber.org/zap"
)

// OrderbookHandler handles HTTP requests for orderbook data.
type OrderbookHandler struct {
	obManager         *orderbook.Manager
	discoveryService  *discovery.Service
	logger            *zap.Logger
}

// NewOrderbookHandler creates a new orderbook handler.
func NewOrderbookHandler(obMgr *orderbook.Manager, discSvc *discovery.Service, logger *zap.Logger) *OrderbookHandler {
	return &OrderbookHandler{
		obManager:        obMgr,
		discoveryService: discSvc,
		logger:           logger,
	}
}

// OutcomeOrderbook represents orderbook data for a single outcome.
type OutcomeOrderbook struct {
	Outcome      string  `json:"outcome"`
	TokenID      string  `json:"token_id"`
	BestBidPrice float64 `json:"best_bid_price"`
	BestBidSize  float64 `json:"best_bid_size"`
	BestAskPrice float64 `json:"best_ask_price"`
	BestAskSize  float64 `json:"best_ask_size"`
}

// OrderbookResponse represents the HTTP response for orderbook data.
type OrderbookResponse struct {
	MarketID   string              `json:"market_id"`
	MarketSlug string              `json:"market_slug"`
	Question   string              `json:"question"`
	Outcomes   []OutcomeOrderbook  `json:"outcomes"`
}

// ErrorResponse represents an HTTP error response.
type ErrorResponse struct {
	Error string `json:"error"`
}

// HandleOrderbook handles GET /api/orderbook?slug=<market-slug> requests.
func (h *OrderbookHandler) HandleOrderbook(w http.ResponseWriter, r *http.Request) {
	// Only allow GET requests
	if r.Method != http.MethodGet {
		h.writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get slug from query parameter
	slug := r.URL.Query().Get("slug")
	if slug == "" {
		h.writeError(w, "missing required query parameter: slug", http.StatusBadRequest)
		return
	}

	h.logger.Debug("orderbook-request-received", zap.String("slug", slug))

	// Look up market by slug
	marketSub, exists := h.discoveryService.GetMarketBySlug(slug)
	if !exists {
		h.writeError(w, "market not found or not subscribed", http.StatusNotFound)
		return
	}

	// Build response with orderbook data for each outcome
	outcomes := make([]OutcomeOrderbook, 0, len(marketSub.Outcomes))

	for _, outcome := range marketSub.Outcomes {
		snapshot, found := h.obManager.GetSnapshot(outcome.TokenID)
		if !found {
			// Orderbook not available for this outcome yet
			h.logger.Debug("orderbook-not-available",
				zap.String("token-id", outcome.TokenID),
				zap.String("outcome", outcome.Outcome))
			continue
		}

		outcomes = append(outcomes, OutcomeOrderbook{
			Outcome:      outcome.Outcome,
			TokenID:      outcome.TokenID,
			BestBidPrice: snapshot.BestBidPrice,
			BestBidSize:  snapshot.BestBidSize,
			BestAskPrice: snapshot.BestAskPrice,
			BestAskSize:  snapshot.BestAskSize,
		})
	}

	response := OrderbookResponse{
		MarketID:   marketSub.MarketID,
		MarketSlug: marketSub.MarketSlug,
		Question:   marketSub.Question,
		Outcomes:   outcomes,
	}

	// Write JSON response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	err := json.NewEncoder(w).Encode(response)
	if err != nil {
		h.logger.Error("failed-to-encode-response", zap.Error(err))
	}
}

// writeError writes a JSON error response.
func (h *OrderbookHandler) writeError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := ErrorResponse{Error: message}
	err := json.NewEncoder(w).Encode(response)
	if err != nil {
		h.logger.Error("failed-to-encode-error-response", zap.Error(err))
	}
}
