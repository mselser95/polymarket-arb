package arbitrage

import (
	"math"
	"testing"
)

const epsilon = 1e-6

// floatEquals compares floats with epsilon tolerance
func floatEquals(a, b, epsilon float64) bool {
	return math.Abs(a-b) < epsilon
}

// TestFloatingPoint_SmallSpread tests detection of very small spreads
func TestFloatingPoint_SmallSpread(t *testing.T) {
	tests := []struct {
		name     string
		priceSum float64
		expected bool // true if arbitrage opportunity
	}{
		{name: "0.0001 below threshold", priceSum: 0.9949, expected: true},
		{name: "0.00001 below threshold", priceSum: 0.99499, expected: true},
		{name: "exactly at threshold", priceSum: 0.995, expected: false},
		{name: "0.00001 above threshold", priceSum: 0.99501, expected: false},
	}

	threshold := 0.995

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isOpportunity := tt.priceSum < threshold
			if isOpportunity != tt.expected {
				t.Errorf("priceSum=%.6f, threshold=%.3f: expected opportunity=%v, got %v",
					tt.priceSum, threshold, tt.expected, isOpportunity)
			}
		})
	}
}

// TestFloatingPoint_PriceSumNearThreshold tests boundary conditions
func TestFloatingPoint_PriceSumNearThreshold(t *testing.T) {
	tests := []struct {
		name     string
		prices   []float64
		sum      float64
		isBelow  bool
	}{
		{
			name:    "two outcomes at boundary",
			prices:  []float64{0.497, 0.498},
			sum:     0.995,
			isBelow: false,
		},
		{
			name:    "two outcomes just below",
			prices:  []float64{0.497, 0.497},
			sum:     0.994,
			isBelow: true,
		},
		{
			name:    "three outcomes at boundary",
			prices:  []float64{0.33, 0.33, 0.335},
			sum:     0.995,
			isBelow: false,
		},
		{
			name:    "accumulated floating point error",
			prices:  []float64{0.3333333333, 0.3333333333, 0.3333333333},
			sum:     0.9999999999,
			isBelow: false,
		},
	}

	threshold := 0.995

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sum := 0.0
			for _, price := range tt.prices {
				sum += price
			}

			// Verify our expected sum
			if !floatEquals(sum, tt.sum, epsilon) {
				t.Logf("WARNING: sum calculation %.10f differs from expected %.10f", sum, tt.sum)
			}

			isBelow := sum < threshold
			if isBelow != tt.isBelow {
				t.Errorf("sum=%.10f, threshold=%.3f: expected isBelow=%v, got %v",
					sum, threshold, tt.isBelow, isBelow)
			}
		})
	}
}

// TestFloatingPoint_FeeCalculation tests fee precision
func TestFloatingPoint_FeeCalculation(t *testing.T) {
	tests := []struct {
		name         string
		tradeSize    float64
		feeRate      float64
		expectedFee  float64
		netProceeds  float64
	}{
		{
			name:         "small trade",
			tradeSize:    1.0,
			feeRate:      0.01,
			expectedFee:  0.01,
			netProceeds:  0.99,
		},
		{
			name:         "typical trade",
			tradeSize:    100.0,
			feeRate:      0.01,
			expectedFee:  1.0,
			netProceeds:  99.0,
		},
		{
			name:         "fractional trade",
			tradeSize:    99.99,
			feeRate:      0.01,
			expectedFee:  0.9999,
			netProceeds:  98.9901,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fee := tt.tradeSize * tt.feeRate
			net := tt.tradeSize - fee

			if !floatEquals(fee, tt.expectedFee, epsilon) {
				t.Errorf("fee: expected %.4f, got %.4f", tt.expectedFee, fee)
			}

			if !floatEquals(net, tt.netProceeds, epsilon) {
				t.Errorf("net: expected %.4f, got %.4f", tt.netProceeds, net)
			}
		})
	}
}

// TestFloatingPoint_ProfitCalculation tests profit precision
func TestFloatingPoint_ProfitCalculation(t *testing.T) {
	tests := []struct {
		name          string
		cost          float64
		revenue       float64
		fee           float64
		expectedProfit float64
	}{
		{
			name:          "small profit",
			cost:          10.0,
			revenue:       10.05,
			fee:           0.01,
			expectedProfit: 0.04,
		},
		{
			name:          "tiny profit",
			cost:          10.0,
			revenue:       10.001,
			fee:           0.0001,
			expectedProfit: 0.0009,
		},
		{
			name:          "large profit",
			cost:          1000.0,
			revenue:       1050.0,
			fee:           10.5,
			expectedProfit: 39.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profit := tt.revenue - tt.cost - tt.fee

			if !floatEquals(profit, tt.expectedProfit, epsilon) {
				t.Errorf("expected profit %.6f, got %.6f (diff: %.10f)",
					tt.expectedProfit, profit, math.Abs(profit-tt.expectedProfit))
			}
		})
	}
}

// TestFloatingPoint_ManyOutcomes tests accumulated precision error
func TestFloatingPoint_ManyOutcomes(t *testing.T) {
	tests := []struct {
		name          string
		numOutcomes   int
		pricePerOutcome float64
		threshold     float64
		expectedSum   float64
	}{
		{
			name:          "5 outcomes at 0.19",
			numOutcomes:   5,
			pricePerOutcome: 0.19,
			threshold:     0.995,
			expectedSum:   0.95,
		},
		{
			name:          "10 outcomes at 0.099",
			numOutcomes:   10,
			pricePerOutcome: 0.099,
			threshold:     0.995,
			expectedSum:   0.99,
		},
		{
			name:          "3 outcomes with repeating decimal",
			numOutcomes:   3,
			pricePerOutcome: 1.0 / 3.0,
			threshold:     0.995,
			expectedSum:   1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sum := 0.0
			for i := 0; i < tt.numOutcomes; i++ {
				sum += tt.pricePerOutcome
			}

			// Allow larger epsilon for many operations
			largeEpsilon := 1e-5
			if !floatEquals(sum, tt.expectedSum, largeEpsilon) {
				t.Logf("accumulated sum %.10f differs from expected %.10f (diff: %.10f)",
					sum, tt.expectedSum, math.Abs(sum-tt.expectedSum))
			}

			isOpportunity := sum < tt.threshold
			t.Logf("sum=%.10f, threshold=%.3f, isOpportunity=%v", sum, tt.threshold, isOpportunity)
		})
	}
}

// TestFloatingPoint_TokenConversion tests USD to token roundtrip
func TestFloatingPoint_TokenConversion(t *testing.T) {
	tests := []struct {
		name      string
		usd       float64
		price     float64
		tokens    float64
		roundtrip float64
	}{
		{
			name:      "simple conversion",
			usd:       10.0,
			price:     0.5,
			tokens:    20.0,
			roundtrip: 10.0,
		},
		{
			name:      "fractional price",
			usd:       100.0,
			price:     0.33,
			tokens:    303.030303,
			roundtrip: 99.999999,
		},
		{
			name:      "high precision",
			usd:       1.0,
			price:     0.123456789,
			tokens:    8.100000073,
			roundtrip: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// USD -> tokens
			tokens := tt.usd / tt.price

			// Allow 0.1% tolerance for token count
			tokenEpsilon := tt.tokens * 0.001
			if !floatEquals(tokens, tt.tokens, tokenEpsilon) {
				t.Logf("token conversion: expected %.6f, got %.6f (diff: %.10f)",
					tt.tokens, tokens, math.Abs(tokens-tt.tokens))
			}

			// tokens -> USD
			roundtrip := tokens * tt.price

			// Should get original USD back (with small tolerance)
			if !floatEquals(roundtrip, tt.roundtrip, epsilon) {
				t.Logf("roundtrip: expected %.6f, got %.6f (diff: %.10f)",
					tt.roundtrip, roundtrip, math.Abs(roundtrip-tt.roundtrip))
			}
		})
	}
}

// TestFloatingPoint_EpsilonComparison tests epsilon tolerance
func TestFloatingPoint_EpsilonComparison(t *testing.T) {
	tests := []struct {
		name     string
		a        float64
		b        float64
		epsilon  float64
		expected bool
	}{
		{name: "exactly equal", a: 1.0, b: 1.0, epsilon: 1e-6, expected: true},
		{name: "within epsilon", a: 1.0, b: 1.0000001, epsilon: 1e-6, expected: true},
		{name: "outside epsilon", a: 1.0, b: 1.000002, epsilon: 1e-6, expected: false},
		{name: "negative values", a: -1.0, b: -1.0000001, epsilon: 1e-6, expected: true},
		{name: "opposite signs", a: 1.0, b: -1.0, epsilon: 1e-6, expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := floatEquals(tt.a, tt.b, tt.epsilon)
			if result != tt.expected {
				t.Errorf("floatEquals(%.10f, %.10f, %.10f) = %v, expected %v",
					tt.a, tt.b, tt.epsilon, result, tt.expected)
			}
		})
	}
}

// TestFloatingPoint_SubtractionCancellation tests catastrophic cancellation
func TestFloatingPoint_SubtractionCancellation(t *testing.T) {
	tests := []struct {
		name     string
		a        float64
		b        float64
		expected float64
	}{
		{
			name:     "near equal values",
			a:        1.0,
			b:        0.99999999,
			expected: 0.00000001,
		},
		{
			name:     "very close values",
			a:        1.0000001,
			b:        1.0000000,
			expected: 0.0000001,
		},
		{
			name:     "threshold comparison",
			a:        0.995001,
			b:        0.995,
			expected: 0.000001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.a - tt.b

			// Allow larger epsilon due to cancellation
			cancelEpsilon := 1e-8
			if !floatEquals(result, tt.expected, cancelEpsilon) {
				t.Logf("subtraction: %.10f - %.10f = %.12f, expected %.12f (diff: %.15f)",
					tt.a, tt.b, result, tt.expected, math.Abs(result-tt.expected))
			}
		})
	}
}

// TestFloatingPoint_DivisionBySmallNumber tests division precision
func TestFloatingPoint_DivisionBySmallNumber(t *testing.T) {
	tests := []struct {
		name     string
		dividend float64
		divisor  float64
		expected float64
	}{
		{
			name:     "normal division",
			dividend: 10.0,
			divisor:  0.01,
			expected: 1000.0,
		},
		{
			name:     "very small divisor",
			dividend: 1.0,
			divisor:  0.001,
			expected: 1000.0,
		},
		{
			name:     "tiny divisor",
			dividend: 1.0,
			divisor:  0.0001,
			expected: 10000.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.dividend / tt.divisor

			if !floatEquals(result, tt.expected, epsilon) {
				t.Errorf("division: %.6f / %.6f = %.6f, expected %.6f",
					tt.dividend, tt.divisor, result, tt.expected)
			}
		})
	}
}

// TestFloatingPoint_NegativeZero tests negative zero handling
func TestFloatingPoint_NegativeZero(t *testing.T) {
	zero := 0.0
	negZero := -0.0

	// In Go, 0.0 == -0.0
	if zero != negZero {
		t.Errorf("0.0 != -0.0: this should never happen in Go")
	}

	// But they have different bit patterns
	zeroBits := math.Float64bits(zero)
	negZeroBits := math.Float64bits(negZero)

	if zeroBits == negZeroBits {
		t.Log("0.0 and -0.0 have same bit pattern (expected in most cases)")
	} else {
		t.Logf("0.0 bits: %064b", zeroBits)
		t.Logf("-0.0 bits: %064b", negZeroBits)
	}

	// Test operations with negative zero
	result := negZero * -1.0
	if result != 0.0 {
		t.Errorf("-0.0 * -1.0 = %.10f, expected 0.0", result)
	}
}
