package types

import (
	"encoding/json"
	"time"
)

// Market represents a Polymarket market from the Gamma API.
type Market struct {
	ID          string    `json:"id"`
	Question    string    `json:"question"`
	Slug        string    `json:"slug"`
	Closed      bool      `json:"closed"`
	Active      bool      `json:"active"`
	Tokens      []Token   `json:"-"` // Populated from outcomes + clobTokenIds
	CreatedAt   time.Time `json:"createdAt"`
	EndDate     time.Time `json:"endDate"`
	Description string    `json:"description"`
	Outcomes    string    `json:"outcomes"`       // JSON string: "[\"Yes\", \"No\"]"
	ClobTokens  string    `json:"clobTokenIds"`   // JSON string: "[\"token1\", \"token2\"]"

	// Trading constraints (fetched separately from CLOB API)
	MinOrderSize float64 `json:"min_order_size"` // Minimum order size in tokens
	TickSize     float64 `json:"tick_size"`      // Price tick size
}

// UnmarshalJSON custom unmarshaler to parse outcomes and clobTokenIds into Tokens.
func (m *Market) UnmarshalJSON(data []byte) error {
	type Alias Market
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(m),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// Parse outcomes and clobTokenIds to populate Tokens
	if m.Outcomes != "" && m.ClobTokens != "" {
		var outcomes []string
		var tokenIDs []string

		if err := json.Unmarshal([]byte(m.Outcomes), &outcomes); err == nil {
			if err := json.Unmarshal([]byte(m.ClobTokens), &tokenIDs); err == nil {
				m.Tokens = make([]Token, 0, len(outcomes))
				for i, outcome := range outcomes {
					if i < len(tokenIDs) {
						m.Tokens = append(m.Tokens, Token{
							TokenID: tokenIDs[i],
							Outcome: outcome,
						})
					}
				}
			}
		}
	}

	return nil
}

// Token represents a market outcome token (YES or NO).
type Token struct {
	TokenID      string  `json:"token_id"`
	Outcome      string  `json:"outcome"`
	Price        float64 `json:"price,omitempty"`
	MinOrderSize float64 `json:"min_order_size,omitempty"` // Minimum order size for this token
	TickSize     float64 `json:"tick_size,omitempty"`      // Tick size for this token
}

// GetTokenByOutcome returns the token for a specific outcome (YES or NO).
// Case-insensitive matching (accepts YES/Yes, NO/No).
func (m *Market) GetTokenByOutcome(outcome string) *Token {
	for i := range m.Tokens {
		// Normalize comparison: YES/Yes, NO/No
		tokenOutcome := m.Tokens[i].Outcome
		if tokenOutcome == outcome ||
			(outcome == "YES" && tokenOutcome == "Yes") ||
			(outcome == "NO" && tokenOutcome == "No") {
			return &m.Tokens[i]
		}
	}
	return nil
}

// OutcomeToken represents a single outcome in a market subscription.
// For binary markets: Outcome = "YES" or "NO"
// For multi-outcome markets: Outcome = "Candidate A", "Team 1", etc.
type OutcomeToken struct {
	TokenID string // CLOB token ID for this outcome
	Outcome string // Human-readable outcome name
}

// MarketSubscription tracks subscription state for a market.
// Supports both binary (2 outcomes) and multi-outcome (3+) markets.
type MarketSubscription struct {
	MarketID     string
	MarketSlug   string
	Question     string
	Outcomes     []OutcomeToken // All outcomes for this market (2+ outcomes)
	SubscribedAt time.Time
}

// MarketsResponse represents the response from Gamma API /events endpoint.
type MarketsResponse struct {
	Data     []Market `json:"data"`
	Count    int      `json:"count"`
	NextPage string   `json:"next_page,omitempty"`
	Limit    int      `json:"limit"`
	Offset   int      `json:"offset"`
}
