// Package price 价格计算模块单元测试
package price

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"biya-oracle/internal/types"
)

// TestCalculateWeightedPrice_NormalWeights 测试正常权重计算（收盘价40% + Pyth30% + Gate30%）
func TestCalculateWeightedPrice_NormalWeights(t *testing.T) {
	calculator := NewCalculator()

	testCases := []struct {
		name        string
		symbol      string
		closePrice  float64
		pythPrice   float64
		gatePrice   float64
		pythStatus  types.OracleStatus
		gateStatus  types.OracleStatus
		expected    float64
		isDegraded  bool
	}{
		{
			name:        "正常权重计算",
			symbol:      "AAPLX",
			closePrice:  100.0,
			pythPrice:   101.0,
			gatePrice:   102.0,
			pythStatus:  types.OracleStatusNormal,
			gateStatus:  types.OracleStatusNormal,
			expected:    100.9, // 100.0*0.4 + 101.0*0.3 + 102.0*0.3 = 100.9
			isDegraded:  false,
		},
		{
			name:        "正常权重计算（不同价格）",
			symbol:      "NVDAX",
			closePrice:  200.0,
			pythPrice:   201.0,
			gatePrice:   202.0,
			pythStatus:  types.OracleStatusNormal,
			gateStatus:  types.OracleStatusNormal,
			expected:    200.9, // 200.0*0.4 + 201.0*0.3 + 202.0*0.3 = 200.9
			isDegraded:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := calculator.CalculateWeightedPrice(
				tc.symbol,
				tc.closePrice,
				tc.pythPrice,
				tc.gatePrice,
				tc.pythStatus,
				tc.gateStatus,
			)

			assert.Equal(t, tc.symbol, result.Symbol, "股票符号应该正确")
			assert.InDelta(t, tc.expected, result.Price, 0.01, "加权价格应该正确")
			assert.Equal(t, tc.isDegraded, result.IsDegraded, "降级标志应该正确")
			assert.Equal(t, 0.4, result.ClosePriceWeight, "收盘价权重应该为0.4")
			assert.Equal(t, 0.3, result.PythWeight, "Pyth权重应该为0.3")
			assert.Equal(t, 0.3, result.GateWeight, "Gate权重应该为0.3")
		})
	}
}

// TestCalculateWeightedPrice_PythAbnormal 测试Pyth异常降权（收盘价60% + Gate40%）
func TestCalculateWeightedPrice_PythAbnormal(t *testing.T) {
	calculator := NewCalculator()

	testCases := []struct {
		name        string
		symbol      string
		closePrice  float64
		pythPrice   float64
		gatePrice   float64
		pythStatus  types.OracleStatus
		gateStatus  types.OracleStatus
		expected    float64
		isDegraded  bool
		degradeReason string
	}{
		{
			name:        "Pyth异常降权",
			symbol:      "AAPLX",
			closePrice:  100.0,
			pythPrice:   0, // 异常
			gatePrice:   102.0,
			pythStatus:  types.OracleStatusAbnormal,
			gateStatus:  types.OracleStatusNormal,
			expected:    100.8, // 100.0*0.6 + 102.0*0.4 = 100.8
			isDegraded:  true,
			degradeReason: "Pyth价格异常",
		},
		{
			name:        "Pyth异常降权（不同价格）",
			symbol:      "TSLAX",
			closePrice:  300.0,
			pythPrice:   0,
			gatePrice:   302.0,
			pythStatus:  types.OracleStatusAbnormal,
			gateStatus:  types.OracleStatusNormal,
			expected:    300.8, // 300.0*0.6 + 302.0*0.4 = 300.8
			isDegraded:  true,
			degradeReason: "Pyth价格异常",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := calculator.CalculateWeightedPrice(
				tc.symbol,
				tc.closePrice,
				tc.pythPrice,
				tc.gatePrice,
				tc.pythStatus,
				tc.gateStatus,
			)

			assert.Equal(t, tc.symbol, result.Symbol, "股票符号应该正确")
			assert.InDelta(t, tc.expected, result.Price, 0.01, "加权价格应该正确")
			assert.True(t, result.IsDegraded, "应该标记为降级")
			assert.Equal(t, tc.degradeReason, result.DegradeReason, "降级原因应该正确")
			assert.Equal(t, 0.6, result.ClosePriceWeight, "收盘价权重应该为0.6")
			assert.Equal(t, 0.0, result.PythWeight, "Pyth权重应该为0")
			assert.Equal(t, 0.4, result.GateWeight, "Gate权重应该为0.4")
		})
	}
}

// TestCalculateWeightedPrice_GateAbnormal 测试Gate异常降权（收盘价60% + Pyth40%）
func TestCalculateWeightedPrice_GateAbnormal(t *testing.T) {
	calculator := NewCalculator()

	testCases := []struct {
		name        string
		symbol      string
		closePrice  float64
		pythPrice   float64
		gatePrice   float64
		pythStatus  types.OracleStatus
		gateStatus  types.OracleStatus
		expected    float64
		isDegraded  bool
		degradeReason string
	}{
		{
			name:        "Gate异常降权",
			symbol:      "AAPLX",
			closePrice:  100.0,
			pythPrice:   101.0,
			gatePrice:   0, // 异常
			pythStatus:  types.OracleStatusNormal,
			gateStatus:  types.OracleStatusAbnormal,
			expected:    100.4, // 100.0*0.6 + 101.0*0.4 = 100.4
			isDegraded:  true,
			degradeReason: "Gate价格异常",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := calculator.CalculateWeightedPrice(
				tc.symbol,
				tc.closePrice,
				tc.pythPrice,
				tc.gatePrice,
				tc.pythStatus,
				tc.gateStatus,
			)

			assert.Equal(t, tc.symbol, result.Symbol, "股票符号应该正确")
			assert.InDelta(t, tc.expected, result.Price, 0.01, "加权价格应该正确")
			assert.True(t, result.IsDegraded, "应该标记为降级")
			assert.Equal(t, tc.degradeReason, result.DegradeReason, "降级原因应该正确")
			assert.Equal(t, 0.6, result.ClosePriceWeight, "收盘价权重应该为0.6")
			assert.Equal(t, 0.4, result.PythWeight, "Pyth权重应该为0.4")
			assert.Equal(t, 0.0, result.GateWeight, "Gate权重应该为0")
		})
	}
}

// TestCalculateWeightedPrice_ClosePriceAbnormal 测试收盘价异常降权（Pyth50% + Gate50%）
func TestCalculateWeightedPrice_ClosePriceAbnormal(t *testing.T) {
	calculator := NewCalculator()

	testCases := []struct {
		name        string
		symbol      string
		closePrice  float64
		pythPrice   float64
		gatePrice   float64
		pythStatus  types.OracleStatus
		gateStatus  types.OracleStatus
		expected    float64
		isDegraded  bool
		degradeReason string
	}{
		{
			name:        "收盘价异常降权",
			symbol:      "AAPLX",
			closePrice:  0, // 异常
			pythPrice:   101.0,
			gatePrice:   102.0,
			pythStatus:  types.OracleStatusNormal,
			gateStatus:  types.OracleStatusNormal,
			expected:    101.5, // 101.0*0.5 + 102.0*0.5 = 101.5
			isDegraded:  true,
			degradeReason: "CEX收盘价异常",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := calculator.CalculateWeightedPrice(
				tc.symbol,
				tc.closePrice,
				tc.pythPrice,
				tc.gatePrice,
				tc.pythStatus,
				tc.gateStatus,
			)

			assert.Equal(t, tc.symbol, result.Symbol, "股票符号应该正确")
			assert.InDelta(t, tc.expected, result.Price, 0.01, "加权价格应该正确")
			assert.True(t, result.IsDegraded, "应该标记为降级")
			assert.Equal(t, tc.degradeReason, result.DegradeReason, "降级原因应该正确")
			assert.Equal(t, 0.0, result.ClosePriceWeight, "收盘价权重应该为0")
			assert.Equal(t, 0.5, result.PythWeight, "Pyth权重应该为0.5")
			assert.Equal(t, 0.5, result.GateWeight, "Gate权重应该为0.5")
		})
	}
}

// TestCalculateWeightedPrice_BothOracleAbnormal 测试两个预言机都异常
func TestCalculateWeightedPrice_BothOracleAbnormal(t *testing.T) {
	calculator := NewCalculator()

	testCases := []struct {
		name        string
		symbol      string
		closePrice  float64
		pythPrice   float64
		gatePrice   float64
		pythStatus  types.OracleStatus
		gateStatus  types.OracleStatus
		expected    float64
		isDegraded  bool
		degradeReason string
	}{
		{
			name:        "两个预言机都异常",
			symbol:      "AAPLX",
			closePrice:  100.0,
			pythPrice:   0, // 异常
			gatePrice:   0, // 异常
			pythStatus:  types.OracleStatusAbnormal,
			gateStatus:  types.OracleStatusAbnormal,
			expected:    0, // 无效价格
			isDegraded:  true,
			degradeReason: "两个预言机都异常",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := calculator.CalculateWeightedPrice(
				tc.symbol,
				tc.closePrice,
				tc.pythPrice,
				tc.gatePrice,
				tc.pythStatus,
				tc.gateStatus,
			)

			assert.Equal(t, tc.symbol, result.Symbol, "股票符号应该正确")
			assert.Equal(t, tc.expected, result.Price, "价格应该为0（无效）")
			assert.True(t, result.IsDegraded, "应该标记为降级")
			assert.Equal(t, tc.degradeReason, result.DegradeReason, "降级原因应该正确")
		})
	}
}

// TestCalculateWeightedPrice_BoundaryValues 测试边界值
func TestCalculateWeightedPrice_BoundaryValues(t *testing.T) {
	calculator := NewCalculator()

	testCases := []struct {
		name        string
		symbol      string
		closePrice  float64
		pythPrice   float64
		gatePrice   float64
		pythStatus  types.OracleStatus
		gateStatus  types.OracleStatus
		expected    float64
	}{
		{
			name:        "极小价格值",
			symbol:      "AAPLX",
			closePrice:  0.0001,
			pythPrice:   0.0002,
			gatePrice:   0.0003,
			pythStatus:  types.OracleStatusNormal,
			gateStatus:  types.OracleStatusNormal,
			expected:    0.0002, // 0.0001*0.4 + 0.0002*0.3 + 0.0003*0.3 = 0.00019 ≈ 0.0002
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := calculator.CalculateWeightedPrice(
				tc.symbol,
				tc.closePrice,
				tc.pythPrice,
				tc.gatePrice,
				tc.pythStatus,
				tc.gateStatus,
			)

			assert.InDelta(t, tc.expected, result.Price, 0.0001, "边界值计算应该正确")
		})
	}
}

// TestCheckPriceJump_NormalJump 测试正常价格跳变（跳变 ≤ 10%）
func TestCheckPriceJump_NormalJump(t *testing.T) {
	calculator := NewCalculator()

	testCases := []struct {
		name      string
		oldPrice  float64
		newPrice  float64
		expected  float64
		isAbnormal bool
	}{
		{
			name:      "正常价格变动 5%",
			oldPrice:  100.0,
			newPrice:  105.0,
			expected:  0.05,
			isAbnormal: false,
		},
		{
			name:      "正常价格变动 10%（边界值）",
			oldPrice:  100.0,
			newPrice:  110.0,
			expected:  0.10,
			isAbnormal: false,
		},
		{
			name:      "正常价格变动 1%",
			oldPrice:  100.0,
			newPrice:  101.0,
			expected:  0.01,
			isAbnormal: false,
		},
		{
			name:      "价格跳变异常 15%",
			oldPrice:  100.0,
			newPrice:  115.0,
			expected:  0.15,
			isAbnormal: true,
		},
		{
			name:      "价格跳变异常 20%",
			oldPrice:  100.0,
			newPrice:  120.0,
			expected:  0.20,
			isAbnormal: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			jump, isAbnormal := calculator.CheckPriceJump(tc.oldPrice, tc.newPrice)
			assert.InDelta(t, tc.expected, jump, 0.0001, "跳变百分比应该正确")
			assert.Equal(t, tc.isAbnormal, isAbnormal, "跳变判断应该正确")
		})
	}
}

// TestCheckPriceJump_FirstPrice 测试首次价格（无上次价格）
func TestCheckPriceJump_FirstPrice(t *testing.T) {
	calculator := NewCalculator()

	testCases := []struct {
		name     string
		oldPrice float64
		newPrice float64
		isAbnormal bool
	}{
		{
			name:     "首次价格（oldPrice = 0）",
			oldPrice: 0,
			newPrice: 100.0,
			isAbnormal: false,
		},
		{
			name:     "首次价格（oldPrice < 0）",
			oldPrice: -1.0,
			newPrice: 100.0,
			isAbnormal: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			jump, isAbnormal := calculator.CheckPriceJump(tc.oldPrice, tc.newPrice)
			assert.Equal(t, 0.0, jump, "首次价格跳变应该为0")
			assert.False(t, isAbnormal, "首次价格应该认为正常")
		})
	}
}

// TestCheckPriceJump_PriceDecrease 测试价格下跌跳变
func TestCheckPriceJump_PriceDecrease(t *testing.T) {
	calculator := NewCalculator()

	testCases := []struct {
		name     string
		oldPrice float64
		newPrice float64
		expected float64
		isAbnormal bool
	}{
		{
			name:     "价格下跌 5%",
			oldPrice: 100.0,
			newPrice: 95.0,
			expected: 0.05,
			isAbnormal: false,
		},
		{
			name:     "价格下跌 15%",
			oldPrice: 100.0,
			newPrice: 85.0,
			expected: 0.15,
			isAbnormal: true,
		},
		{
			name:     "价格下跌 10%（边界值）",
			oldPrice: 100.0,
			newPrice: 90.0,
			expected: 0.10,
			isAbnormal: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			jump, isAbnormal := calculator.CheckPriceJump(tc.oldPrice, tc.newPrice)
			assert.InDelta(t, tc.expected, jump, 0.0001, "跳变百分比应该正确（取绝对值）")
			assert.Equal(t, tc.isAbnormal, isAbnormal, "跳变判断应该正确")
		})
	}
}

// TestIsBothOracleAbnormal 测试判断两个预言机是否都异常
func TestIsBothOracleAbnormal(t *testing.T) {
	calculator := NewCalculator()

	testCases := []struct {
		name        string
		pythStatus  types.OracleStatus
		gateStatus  types.OracleStatus
		expected    bool
	}{
		{
			name:       "两个都正常",
			pythStatus: types.OracleStatusNormal,
			gateStatus: types.OracleStatusNormal,
			expected:   false,
		},
		{
			name:       "Pyth异常，Gate正常",
			pythStatus: types.OracleStatusAbnormal,
			gateStatus: types.OracleStatusNormal,
			expected:   false,
		},
		{
			name:       "Pyth正常，Gate异常",
			pythStatus: types.OracleStatusNormal,
			gateStatus: types.OracleStatusAbnormal,
			expected:   false,
		},
		{
			name:       "两个都异常",
			pythStatus: types.OracleStatusAbnormal,
			gateStatus: types.OracleStatusAbnormal,
			expected:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := calculator.IsBothOracleAbnormal(tc.pythStatus, tc.gateStatus)
			assert.Equal(t, tc.expected, result, "两个预言机异常判断应该正确")
		})
	}
}
