package types

import (
	"encoding/json"
	"testing"
)

func TestPriceChangeMessage_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantErr   bool
		checkFunc func(*testing.T, *PriceChangeMessage)
	}{
		{
			name: "valid_price_change_single_asset",
			input: `{
				"event_type": "price_change",
				"market": "0xabc123",
				"timestamp": "1234567890000",
				"price_changes": [
					{
						"asset_id": "token1",
						"best_bid": "0.52",
						"best_ask": "0.53"
					}
				]
			}`,
			wantErr: false,
			checkFunc: func(t *testing.T, msg *PriceChangeMessage) {
				if msg.EventType != "price_change" {
					t.Errorf("EventType = %q, want %q", msg.EventType, "price_change")
				}
				if msg.Market != "0xabc123" {
					t.Errorf("Market = %q, want %q", msg.Market, "0xabc123")
				}
				if msg.Timestamp != 1234567890000 {
					t.Errorf("Timestamp = %d, want %d", msg.Timestamp, 1234567890000)
				}
				if len(msg.PriceChanges) != 1 {
					t.Fatalf("len(PriceChanges) = %d, want 1", len(msg.PriceChanges))
				}
				pc := msg.PriceChanges[0]
				if pc.AssetID != "token1" {
					t.Errorf("PriceChanges[0].AssetID = %q, want %q", pc.AssetID, "token1")
				}
				if pc.BestBid != "0.52" {
					t.Errorf("PriceChanges[0].BestBid = %q, want %q", pc.BestBid, "0.52")
				}
				if pc.BestAsk != "0.53" {
					t.Errorf("PriceChanges[0].BestAsk = %q, want %q", pc.BestAsk, "0.53")
				}
			},
		},
		{
			name: "valid_price_change_multiple_assets",
			input: `{
				"event_type": "price_change",
				"market": "0xdef456",
				"timestamp": "1234567890000",
				"price_changes": [
					{
						"asset_id": "token1",
						"best_bid": "0.52",
						"best_ask": "0.53"
					},
					{
						"asset_id": "token2",
						"best_bid": "0.48",
						"best_ask": "0.49"
					}
				]
			}`,
			wantErr: false,
			checkFunc: func(t *testing.T, msg *PriceChangeMessage) {
				if len(msg.PriceChanges) != 2 {
					t.Fatalf("len(PriceChanges) = %d, want 2", len(msg.PriceChanges))
				}
				// Check first asset
				pc1 := msg.PriceChanges[0]
				if pc1.AssetID != "token1" {
					t.Errorf("PriceChanges[0].AssetID = %q, want %q", pc1.AssetID, "token1")
				}
				// Check second asset
				pc2 := msg.PriceChanges[1]
				if pc2.AssetID != "token2" {
					t.Errorf("PriceChanges[1].AssetID = %q, want %q", pc2.AssetID, "token2")
				}
			},
		},
		{
			name: "valid_price_change_empty_array",
			input: `{
				"event_type": "price_change",
				"market": "0xghi789",
				"timestamp": "1234567890000",
				"price_changes": []
			}`,
			wantErr: false,
			checkFunc: func(t *testing.T, msg *PriceChangeMessage) {
				if len(msg.PriceChanges) != 0 {
					t.Errorf("len(PriceChanges) = %d, want 0", len(msg.PriceChanges))
				}
			},
		},
		{
			name: "valid_price_change_no_timestamp",
			input: `{
				"event_type": "price_change",
				"market": "0xjkl012",
				"price_changes": [
					{
						"asset_id": "token1",
						"best_bid": "0.52",
						"best_ask": "0.53"
					}
				]
			}`,
			wantErr: false,
			checkFunc: func(t *testing.T, msg *PriceChangeMessage) {
				if msg.Timestamp != 0 {
					t.Errorf("Timestamp = %d, want 0 (not provided)", msg.Timestamp)
				}
			},
		},
		{
			name: "invalid_json",
			input: `{
				"event_type": "price_change",
				"market": "0xabc",
				"price_changes": [INVALID
			}`,
			wantErr: true,
		},
		{
			name: "invalid_timestamp_format",
			input: `{
				"event_type": "price_change",
				"market": "0xabc",
				"timestamp": "not_a_number",
				"price_changes": []
			}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var msg PriceChangeMessage
			err := json.Unmarshal([]byte(tt.input), &msg)
			if (err != nil) != tt.wantErr {
				t.Errorf("Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.checkFunc != nil {
				tt.checkFunc(t, &msg)
			}
		})
	}
}

func TestPriceChange_Fields(t *testing.T) {
	// Test that PriceChange struct fields are properly mapped
	input := `{
		"asset_id": "test_token_id",
		"best_bid": "0.4999",
		"best_ask": "0.5001"
	}`

	var pc PriceChange
	err := json.Unmarshal([]byte(input), &pc)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if pc.AssetID != "test_token_id" {
		t.Errorf("AssetID = %q, want %q", pc.AssetID, "test_token_id")
	}
	if pc.BestBid != "0.4999" {
		t.Errorf("BestBid = %q, want %q", pc.BestBid, "0.4999")
	}
	if pc.BestAsk != "0.5001" {
		t.Errorf("BestAsk = %q, want %q", pc.BestAsk, "0.5001")
	}
}

// TestPriceChangeMessage_RealWorldExample tests with actual CLOB API format from documentation
func TestPriceChangeMessage_RealWorldExample(t *testing.T) {
	// Example from https://docs.polymarket.com/developers/CLOB/websocket/market-channel
	realWorldMsg := `{
		"market": "0x5f65177b394277fd294cd75650044e32ba009a95022d88a0c1d565897d72f8f1",
		"price_changes": [
			{
				"asset_id": "71321045679252212594626385532706912750332728571942532289631379312455583992563",
				"price": "0.5",
				"size": "200",
				"side": "BUY",
				"hash": "56621a121a47ed9333273e21c83b660cff37ae50",
				"best_bid": "0.5",
				"best_ask": "1"
			},
			{
				"asset_id": "52114319501245915516055106046884209969926127482827954674443846427813813222426",
				"price": "0.5",
				"size": "200",
				"side": "SELL",
				"hash": "1895759e4df7a796bf4f1c5a5950b748306923e2",
				"best_bid": "0",
				"best_ask": "0.5"
			}
		],
		"timestamp": "1757908892351",
		"event_type": "price_change"
	}`

	var msg PriceChangeMessage
	err := json.Unmarshal([]byte(realWorldMsg), &msg)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	// Verify structure
	if msg.EventType != "price_change" {
		t.Errorf("EventType = %q, want %q", msg.EventType, "price_change")
	}
	if msg.Market != "0x5f65177b394277fd294cd75650044e32ba009a95022d88a0c1d565897d72f8f1" {
		t.Errorf("Market = %q, want %q", msg.Market, "0x5f65177b394277fd294cd75650044e32ba009a95022d88a0c1d565897d72f8f1")
	}
	if len(msg.PriceChanges) != 2 {
		t.Fatalf("len(PriceChanges) = %d, want 2", len(msg.PriceChanges))
	}

	// Verify timestamp parsing
	expectedTimestamp := int64(1757908892351)
	if msg.Timestamp != expectedTimestamp {
		t.Errorf("Timestamp = %d, want %d", msg.Timestamp, expectedTimestamp)
	}

	// Verify first price change (BUY side)
	pc1 := msg.PriceChanges[0]
	if pc1.AssetID != "71321045679252212594626385532706912750332728571942532289631379312455583992563" {
		t.Errorf("PriceChanges[0].AssetID mismatch")
	}
	if pc1.Price != "0.5" {
		t.Errorf("PriceChanges[0].Price = %q, want %q", pc1.Price, "0.5")
	}
	if pc1.Size != "200" {
		t.Errorf("PriceChanges[0].Size = %q, want %q", pc1.Size, "200")
	}
	if pc1.Side != "BUY" {
		t.Errorf("PriceChanges[0].Side = %q, want %q", pc1.Side, "BUY")
	}
	if pc1.Hash != "56621a121a47ed9333273e21c83b660cff37ae50" {
		t.Errorf("PriceChanges[0].Hash = %q, want %q", pc1.Hash, "56621a121a47ed9333273e21c83b660cff37ae50")
	}
	if pc1.BestBid != "0.5" {
		t.Errorf("PriceChanges[0].BestBid = %q, want %q", pc1.BestBid, "0.5")
	}
	if pc1.BestAsk != "1" {
		t.Errorf("PriceChanges[0].BestAsk = %q, want %q", pc1.BestAsk, "1")
	}

	// Verify second price change (SELL side)
	pc2 := msg.PriceChanges[1]
	if pc2.AssetID != "52114319501245915516055106046884209969926127482827954674443846427813813222426" {
		t.Errorf("PriceChanges[1].AssetID mismatch")
	}
	if pc2.Price != "0.5" {
		t.Errorf("PriceChanges[1].Price = %q, want %q", pc2.Price, "0.5")
	}
	if pc2.Size != "200" {
		t.Errorf("PriceChanges[1].Size = %q, want %q", pc2.Size, "200")
	}
	if pc2.Side != "SELL" {
		t.Errorf("PriceChanges[1].Side = %q, want %q", pc2.Side, "SELL")
	}
	if pc2.Hash != "1895759e4df7a796bf4f1c5a5950b748306923e2" {
		t.Errorf("PriceChanges[1].Hash = %q, want %q", pc2.Hash, "1895759e4df7a796bf4f1c5a5950b748306923e2")
	}
	if pc2.BestBid != "0" {
		t.Errorf("PriceChanges[1].BestBid = %q, want %q", pc2.BestBid, "0")
	}
	if pc2.BestAsk != "0.5" {
		t.Errorf("PriceChanges[1].BestAsk = %q, want %q", pc2.BestAsk, "0.5")
	}
}
