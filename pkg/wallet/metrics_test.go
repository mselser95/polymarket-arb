package wallet

import (
	"testing"
)

// TestMetrics_Registration tests all metrics are initialized
func TestMetrics_Registration(t *testing.T) {
	if MATICBalance == nil {
		t.Error("MATICBalance not registered")
	}

	if USDCBalance == nil {
		t.Error("USDCBalance not registered")
	}

	if USDCAllowance == nil {
		t.Error("USDCAllowance not registered")
	}

	if ActivePositions == nil {
		t.Error("ActivePositions not registered")
	}

	if TotalPositionValue == nil {
		t.Error("TotalPositionValue not registered")
	}

	if TotalPositionCost == nil {
		t.Error("TotalPositionCost not registered")
	}

	if UnrealizedPnL == nil {
		t.Error("UnrealizedPnL not registered")
	}

	if UnrealizedPnLPercent == nil {
		t.Error("UnrealizedPnLPercent not registered")
	}

	if PortfolioValue == nil {
		t.Error("PortfolioValue not registered")
	}

	if UpdateErrorsTotal == nil {
		t.Error("UpdateErrorsTotal not registered")
	}

	if UpdateDuration == nil {
		t.Error("UpdateDuration not registered")
	}

	if LastUpdateTimestamp == nil {
		t.Error("LastUpdateTimestamp not registered")
	}
}

// TestMetrics_CounterIncrement tests counter can be incremented
func TestMetrics_CounterIncrement(t *testing.T) {
	UpdateErrorsTotal.Inc()
}

// TestMetrics_GaugeSet tests gauge can be set
func TestMetrics_GaugeSet(t *testing.T) {
	MATICBalance.Set(10.5)
	USDCBalance.Set(100.0)
	USDCAllowance.Set(1000.0)
	ActivePositions.Set(5)
	TotalPositionValue.Set(50.0)
	TotalPositionCost.Set(45.0)
	UnrealizedPnL.Set(5.0)
	UnrealizedPnLPercent.Set(11.11)
	PortfolioValue.Set(150.0)
	LastUpdateTimestamp.Set(1234567890)
}

// TestMetrics_HistogramObserve tests histogram can observe values
func TestMetrics_HistogramObserve(t *testing.T) {
	UpdateDuration.Observe(0.5)
}
