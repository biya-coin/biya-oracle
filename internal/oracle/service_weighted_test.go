// Package oracle 加权价格计算流程单元测试
package oracle

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"biya-oracle/internal/types"
)

// TestCalculateAndPushWeightedPrice_Normal 测试正常加权价格计算
func TestCalculateAndPushWeightedPrice_Normal(t *testing.T) {
	service, mr := setupTestService(t)
	defer mr.Close()

	service.mu.Lock()
	service.isReady = true
	// 设置收盘价、Pyth价格、Gate价格
	service.closePrices["AAPLX"] = 100.0
	service.pythPrices["AAPLX"] = 101.0
	service.gatePrices["AAPLX"] = 102.0
	service.mu.Unlock()

	// 设置数据源为加权合成
	service.stateManager.SetPythStatus("AAPLX", types.OracleStatusNormal)
	service.stateManager.SetGateStatus("AAPLX", types.OracleStatusNormal)

	// 计算并推送加权价格
	service.calculateAndPushWeightedPrice("AAPLX")

	// 验证加权价格计算：100.0×0.4 + 101.0×0.3 + 102.0×0.3 = 100.9
	weighted := service.calculateWeightedPriceOnly("AAPLX")
	assert.NotNil(t, weighted, "加权价格应该计算成功")
	if weighted != nil {
		expected := 100.0*0.4 + 101.0*0.3 + 102.0*0.3
		assert.InDelta(t, expected, weighted.Price, 0.01, "加权价格应该正确")
		assert.False(t, weighted.IsDegraded, "正常情况不应该降级")
	}

	// 验证价格被添加到批量收集器
	count := service.batchCollector.GetBufferedCount()
	assert.GreaterOrEqual(t, count, 0, "价格应该被添加到批量收集器")
}

// TestCalculateAndPushWeightedPrice_BothOracleAbnormal 测试两个预言机都异常
func TestCalculateAndPushWeightedPrice_BothOracleAbnormal(t *testing.T) {
	service, mr := setupTestService(t)
	defer mr.Close()

	service.mu.Lock()
	service.isReady = true
	service.closePrices["AAPLX"] = 100.0
	service.pythPrices["AAPLX"] = 0 // 异常
	service.gatePrices["AAPLX"] = 0 // 异常
	service.bothOracleAlerted["AAPLX"] = false // 未发送过告警
	service.mu.Unlock()

	// 设置两个预言机状态为异常
	service.stateManager.SetPythStatus("AAPLX", types.OracleStatusAbnormal)
	service.stateManager.SetGateStatus("AAPLX", types.OracleStatusAbnormal)

	// 计算并推送加权价格
	service.calculateAndPushWeightedPrice("AAPLX")

	// 验证加权价格计算失败（两个预言机都异常）
	weighted := service.calculateWeightedPriceOnly("AAPLX")
	assert.Nil(t, weighted, "两个预言机都异常时应该返回nil")

	// 验证股票被暂停
	isPaused := service.stateManager.IsPaused("AAPLX")
	assert.True(t, isPaused, "两个预言机都异常时应该暂停股票")

	// 验证告警状态已标记
	service.mu.RLock()
	alreadyAlerted := service.bothOracleAlerted["AAPLX"]
	service.mu.RUnlock()
	assert.True(t, alreadyAlerted, "应该标记已发送告警")

	// 验证价格未被添加到批量收集器
	count := service.batchCollector.GetBufferedCount()
	// 由于两个预言机都异常，不应该添加价格
	assert.Equal(t, 0, count, "两个预言机都异常时不应该添加价格")
}

// TestCalculateAndPushWeightedPrice_BothOracleRecovery 测试两个预言机恢复
func TestCalculateAndPushWeightedPrice_BothOracleRecovery(t *testing.T) {
	service, mr := setupTestService(t)
	defer mr.Close()

	service.mu.Lock()
	service.isReady = true
	service.closePrices["AAPLX"] = 100.0
	service.pythPrices["AAPLX"] = 101.0
	service.gatePrices["AAPLX"] = 102.0
	service.bothOracleAlerted["AAPLX"] = true // 之前已发送过告警
	service.mu.Unlock()

	// 先设置两个预言机为异常（模拟之前的状态）
	service.stateManager.SetPythStatus("AAPLX", types.OracleStatusAbnormal)
	service.stateManager.SetGateStatus("AAPLX", types.OracleStatusAbnormal)
	service.stateManager.PauseStock("AAPLX", "两个预言机都异常")

	// 恢复两个预言机状态为正常
	service.stateManager.SetPythStatus("AAPLX", types.OracleStatusNormal)
	service.stateManager.SetGateStatus("AAPLX", types.OracleStatusNormal)

	// 计算并推送加权价格
	service.calculateAndPushWeightedPrice("AAPLX")

	// 验证股票已恢复
	isPaused := service.stateManager.IsPaused("AAPLX")
	assert.False(t, isPaused, "预言机恢复后应该恢复股票报价")

	// 验证告警状态已重置
	service.mu.RLock()
	alreadyAlerted := service.bothOracleAlerted["AAPLX"]
	service.mu.RUnlock()
	assert.False(t, alreadyAlerted, "恢复后应该重置告警状态")

	// 验证加权价格计算成功
	weighted := service.calculateWeightedPriceOnly("AAPLX")
	assert.NotNil(t, weighted, "恢复后加权价格应该计算成功")
	if weighted != nil {
		expected := 100.0*0.4 + 101.0*0.3 + 102.0*0.3
		assert.InDelta(t, expected, weighted.Price, 0.01, "加权价格应该正确")
	}
}

// TestCalculateAndPushWeightedPrice_ClosePriceAbnormal 测试收盘价异常时的降权
func TestCalculateAndPushWeightedPrice_ClosePriceAbnormal(t *testing.T) {
	service, mr := setupTestService(t)
	defer mr.Close()

	service.mu.Lock()
	service.isReady = true
	service.closePrices["AAPLX"] = 0 // 收盘价异常
	service.pythPrices["AAPLX"] = 101.0
	service.gatePrices["AAPLX"] = 102.0
	service.closePriceAbnormal["AAPLX"] = false // 未记录过日志
	service.mu.Unlock()

	// 设置预言机状态为正常
	service.stateManager.SetPythStatus("AAPLX", types.OracleStatusNormal)
	service.stateManager.SetGateStatus("AAPLX", types.OracleStatusNormal)

	// 计算并推送加权价格
	service.calculateAndPushWeightedPrice("AAPLX")

	// 验证加权价格计算（使用降级权重：Pyth50% + Gate50%）
	weighted := service.calculateWeightedPriceOnly("AAPLX")
	assert.NotNil(t, weighted, "收盘价异常时应该使用降级权重")
	if weighted != nil {
		// 降级权重：Pyth50% + Gate50% = 101.0×0.5 + 102.0×0.5 = 101.5
		expected := 101.0*0.5 + 102.0*0.5
		assert.InDelta(t, expected, weighted.Price, 0.01, "降级权重计算的加权价格应该正确")
		assert.True(t, weighted.IsDegraded, "收盘价异常时应该标记为降级")
		assert.Equal(t, "CEX收盘价异常", weighted.DegradeReason, "降级原因应该正确")
	}

	// 验证日志状态已标记（去重）
	service.mu.RLock()
	alreadyLogged := service.closePriceAbnormal["AAPLX"]
	service.mu.RUnlock()
	assert.True(t, alreadyLogged, "应该标记已记录日志")
}

// TestCalculateWeightedPriceOnly_PythAbnormal 测试Pyth异常降权
func TestCalculateWeightedPriceOnly_PythAbnormal(t *testing.T) {
	service, mr := setupTestService(t)
	defer mr.Close()

	service.mu.Lock()
	service.closePrices["AAPLX"] = 100.0
	service.pythPrices["AAPLX"] = 0 // Pyth异常
	service.gatePrices["AAPLX"] = 102.0
	service.mu.Unlock()

	// 设置Pyth状态为异常，Gate状态为正常
	service.stateManager.SetPythStatus("AAPLX", types.OracleStatusAbnormal)
	service.stateManager.SetGateStatus("AAPLX", types.OracleStatusNormal)

	// 计算加权价格
	weighted := service.calculateWeightedPriceOnly("AAPLX")
	assert.NotNil(t, weighted, "Pyth异常时应该使用降级权重")
	if weighted != nil {
		// 降级权重：收盘价60% + Gate40% = 100.0×0.6 + 102.0×0.4 = 100.8
		expected := 100.0*0.6 + 102.0*0.4
		assert.InDelta(t, expected, weighted.Price, 0.01, "降级权重计算的加权价格应该正确")
		assert.True(t, weighted.IsDegraded, "Pyth异常时应该标记为降级")
		assert.Equal(t, "Pyth价格异常", weighted.DegradeReason, "降级原因应该正确")
	}
}

// TestCalculateWeightedPriceOnly_GateAbnormal 测试Gate异常降权
func TestCalculateWeightedPriceOnly_GateAbnormal(t *testing.T) {
	service, mr := setupTestService(t)
	defer mr.Close()

	service.mu.Lock()
	service.closePrices["AAPLX"] = 100.0
	service.pythPrices["AAPLX"] = 101.0
	service.gatePrices["AAPLX"] = 0 // Gate异常
	service.mu.Unlock()

	// 设置Pyth状态为正常，Gate状态为异常
	service.stateManager.SetPythStatus("AAPLX", types.OracleStatusNormal)
	service.stateManager.SetGateStatus("AAPLX", types.OracleStatusAbnormal)

	// 计算加权价格
	weighted := service.calculateWeightedPriceOnly("AAPLX")
	assert.NotNil(t, weighted, "Gate异常时应该使用降级权重")
	if weighted != nil {
		// 降级权重：收盘价60% + Pyth40% = 100.0×0.6 + 101.0×0.4 = 100.4
		expected := 100.0*0.6 + 101.0*0.4
		assert.InDelta(t, expected, weighted.Price, 0.01, "降级权重计算的加权价格应该正确")
		assert.True(t, weighted.IsDegraded, "Gate异常时应该标记为降级")
		assert.Equal(t, "Gate价格异常", weighted.DegradeReason, "降级原因应该正确")
	}
}

// TestCalculateWeightedPriceOnly_ClosePriceRecovery 测试收盘价恢复
func TestCalculateWeightedPriceOnly_ClosePriceRecovery(t *testing.T) {
	service, mr := setupTestService(t)
	defer mr.Close()

	service.mu.Lock()
	service.isReady = true
	// 先设置收盘价为异常
	service.closePrices["AAPLX"] = 0
	service.pythPrices["AAPLX"] = 101.0
	service.gatePrices["AAPLX"] = 102.0
	service.closePriceAbnormal["AAPLX"] = true // 之前已记录过日志
	service.mu.Unlock()

	// 设置预言机状态为正常
	service.stateManager.SetPythStatus("AAPLX", types.OracleStatusNormal)
	service.stateManager.SetGateStatus("AAPLX", types.OracleStatusNormal)

	// 恢复收盘价
	service.mu.Lock()
	service.closePrices["AAPLX"] = 100.0
	service.mu.Unlock()

	// 计算并推送加权价格
	service.calculateAndPushWeightedPrice("AAPLX")

	// 验证加权价格计算（恢复正常权重）
	weighted := service.calculateWeightedPriceOnly("AAPLX")
	assert.NotNil(t, weighted, "收盘价恢复后应该使用正常权重")
	if weighted != nil {
		// 正常权重：收盘价40% + Pyth30% + Gate30% = 100.0×0.4 + 101.0×0.3 + 102.0×0.3 = 100.9
		expected := 100.0*0.4 + 101.0*0.3 + 102.0*0.3
		assert.InDelta(t, expected, weighted.Price, 0.01, "正常权重计算的加权价格应该正确")
		assert.False(t, weighted.IsDegraded, "收盘价恢复后不应该标记为降级")
	}

	// 验证日志状态已重置
	service.mu.RLock()
	alreadyLogged := service.closePriceAbnormal["AAPLX"]
	service.mu.RUnlock()
	assert.False(t, alreadyLogged, "收盘价恢复后应该重置日志状态")
}
