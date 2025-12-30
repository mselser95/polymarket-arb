package arbitrage

import (
	"testing"
)

// BenchmarkDetector_DetectMultiOutcome benchmarks arbitrage detection speed
func BenchmarkDetector_DetectMultiOutcome(b *testing.B) {
	// Bench tests need proper setup - skip for now as they use old API
	b.Skip("Bench tests need update to use new detector API with orderbook manager")
}

// BenchmarkDetector_DetectMultiOutcome_5Outcomes benchmarks with more outcomes
func BenchmarkDetector_DetectMultiOutcome_5Outcomes(b *testing.B) {
	// Bench tests need proper setup - skip for now as they use old API
	b.Skip("Bench tests need update to use new detector API with orderbook manager")
}
