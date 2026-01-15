// Package price 价格处理模块
package price

import (
	"biya-oracle/internal/types"
)

// 正常权重
const (
	WeightClosePrice = 0.4 // CEX收盘价权重
	WeightPyth       = 0.3 // Pyth权重
	WeightGate       = 0.3 // Gate权重
)

// 降级权重
const (
	DegradeWeightClosePrice   = 0.6 // 收盘价降级权重
	DegradeWeightSingleOracle = 0.4 // 单一预言机降级权重
	DegradeWeightEqualOracle  = 0.5 // 两个预言机平分权重
)

// Calculator 价格计算器
type Calculator struct {
	validator *Validator
}

// NewCalculator 创建价格计算器
func NewCalculator() *Calculator {
	return &Calculator{
		validator: NewValidator(),
	}
}

// CalculateWeightedPrice 计算加权合成价格
// 输入: closePrice(CEX收盘价), pythPrice(Pyth价格), gatePrice(Gate价格)
// 输入: pythStatus, gateStatus 预言机状态
// 输出: 加权价格信息
func (c *Calculator) CalculateWeightedPrice(
	symbol string,
	closePrice, pythPrice, gatePrice float64,
	pythStatus, gateStatus types.OracleStatus,
) *types.WeightedPrice {

	result := &types.WeightedPrice{
		Symbol:     symbol,
		ClosePrice: closePrice,
		PythPrice:  pythPrice,
		GatePrice:  gatePrice,
	}

	// 判断各数据源状态
	closePriceValid := closePrice > 0
	pythValid := pythStatus == types.OracleStatusNormal && pythPrice > 0
	gateValid := gateStatus == types.OracleStatusNormal && gatePrice > 0

	// 根据状态计算权重和价格
	switch {
	case closePriceValid && pythValid && gateValid:
		// 正常情况: 收盘价40% + Pyth30% + Gate30%
		result.ClosePriceWeight = WeightClosePrice
		result.PythWeight = WeightPyth
		result.GateWeight = WeightGate
		result.Price = closePrice*WeightClosePrice + pythPrice*WeightPyth + gatePrice*WeightGate
		result.IsDegraded = false

	case closePriceValid && !pythValid && gateValid:
		// Pyth异常: 收盘价60% + Gate40%
		result.ClosePriceWeight = DegradeWeightClosePrice
		result.PythWeight = 0
		result.GateWeight = DegradeWeightSingleOracle
		result.Price = closePrice*DegradeWeightClosePrice + gatePrice*DegradeWeightSingleOracle
		result.IsDegraded = true
		result.DegradeReason = "Pyth价格异常"

	case closePriceValid && pythValid && !gateValid:
		// Gate异常: 收盘价60% + Pyth40%
		result.ClosePriceWeight = DegradeWeightClosePrice
		result.PythWeight = DegradeWeightSingleOracle
		result.GateWeight = 0
		result.Price = closePrice*DegradeWeightClosePrice + pythPrice*DegradeWeightSingleOracle
		result.IsDegraded = true
		result.DegradeReason = "Gate价格异常"

	case !closePriceValid && pythValid && gateValid:
		// 收盘价异常: Pyth50% + Gate50%
		result.ClosePriceWeight = 0
		result.PythWeight = DegradeWeightEqualOracle
		result.GateWeight = DegradeWeightEqualOracle
		result.Price = pythPrice*DegradeWeightEqualOracle + gatePrice*DegradeWeightEqualOracle
		result.IsDegraded = true
		result.DegradeReason = "CEX收盘价异常"

	case closePriceValid && !pythValid && !gateValid:
		// 两个预言机都异常，只有收盘价
		// 这种情况应该暂停报价，返回0表示无效
		result.Price = 0
		result.IsDegraded = true
		result.DegradeReason = "两个预言机都异常"

	default:
		// 其他异常情况
		result.Price = 0
		result.IsDegraded = true
		result.DegradeReason = "数据源异常"
	}

	// 格式化价格
	if result.Price > 0 {
		result.Price = FormatPrice(result.Price)
	}

	return result
}

// CheckPriceJump 检查价格跳变
// 返回: 跳变百分比, 是否异常（跳变>10%）
func (c *Calculator) CheckPriceJump(lastPrice, currentPrice float64) (float64, bool) {
	jump, isNormal := c.validator.ValidatePriceJump(lastPrice, currentPrice)
	return jump, !isNormal // 返回是否异常
}

// IsBothOracleAbnormal 判断是否两个预言机都异常
func (c *Calculator) IsBothOracleAbnormal(pythStatus, gateStatus types.OracleStatus) bool {
	pythAbnormal := pythStatus != types.OracleStatusNormal
	gateAbnormal := gateStatus != types.OracleStatusNormal
	return pythAbnormal && gateAbnormal
}
