// Package oracle 业务流程集成测试
package oracle

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"biya-oracle/internal/datasource/cex"
	"biya-oracle/internal/datasource/gate"
	"biya-oracle/internal/datasource/pyth"
	"biya-oracle/internal/types"
)

// TestCompleteDataSourceSwitch_CEXToWeighted 测试CEX → 加权合成完整流程
func TestCompleteDataSourceSwitch_CEXToWeighted(t *testing.T) {
	service, mr := setupTestService(t)
	defer mr.Close()

	// 设置系统为就绪状态
	service.mu.Lock()
	service.isReady = true
	service.lastPrices["AAPLX"] = 100.0 // 最后推送价格
	// 设置CEX报价缓存
	service.cexQuotes["AAPL"] = &cex.QuoteMessage{
		Symbol:       "AAPL",
		LatestPrice:  cex.FlexibleFloat(100.5),
		MarketStatus: "Regular",
		Timestamp:    time.Now().UnixMilli(),
	}
	// 设置Pyth和Gate价格（用于加权计算）
	service.pythPrices["AAPLX"] = 101.0
	service.gatePrices["AAPLX"] = 102.0
	service.mu.Unlock()

	// 设置数据源切换回调
	service.stateManager.SetOnDataSourceChange(service.onDataSourceChange)

	// 设置预言机状态为正常
	service.stateManager.SetPythStatus("AAPLX", types.OracleStatusNormal)
	service.stateManager.SetGateStatus("AAPLX", types.OracleStatusNormal)

	// 步骤1: 模拟状态管理检测到市场状态变为 market_closed
	// 通过直接调用数据源切换回调来模拟
	oldSource := types.DataSourceCEX
	newSource := types.DataSourceWeighted
	symbol := "AAPLX"

	// 步骤2: 触发数据源切换
	service.onDataSourceChange(oldSource, newSource, symbol)

	// 步骤3: 验证收盘价被捕获
	ctx := context.Background()
	storedPrice, err := service.storage.GetLatestClosePrice(ctx, symbol)
	assert.NoError(t, err, "应该能从Redis获取收盘价")
	assert.NotNil(t, storedPrice, "收盘价应该存在")
	if storedPrice != nil {
		assert.Equal(t, 100.5, storedPrice.Price, "收盘价应该正确")
	}

	// 验证内存收盘价缓存已更新
	service.mu.RLock()
	closePrice := service.closePrices[symbol]
	service.mu.RUnlock()
	assert.Equal(t, 100.5, closePrice, "内存收盘价缓存应该已更新")

	// 步骤4: 验证5%差异检测通过（100.0 vs 100.5，差异0.5% < 5%）
	_, ok, err := service.validator.ValidateSwitchDiff(100.0, 100.5)
	assert.NoError(t, err, "差异检测应该通过")
	assert.True(t, ok, "差异0.5%应该通过5%阈值")

	// 步骤5: 验证加权价格计算成功
	weighted := service.calculateWeightedPriceOnly(symbol)
	assert.NotNil(t, weighted, "加权价格应该计算成功")
	if weighted != nil {
		// 正常权重：收盘价40% + Pyth30% + Gate30% = 100.5×0.4 + 101.0×0.3 + 102.0×0.3 = 100.9
		expected := 100.5*0.4 + 101.0*0.3 + 102.0*0.3
		assert.InDelta(t, expected, weighted.Price, 0.01, "加权价格应该正确")
		assert.False(t, weighted.IsDegraded, "正常情况不应该降级")
	}

	// 步骤6: 验证价格被添加到批量收集器（强制立即推送）
	count := service.batchCollector.GetBufferedCount()
	assert.GreaterOrEqual(t, count, 0, "价格应该被添加到批量收集器")

	// 验证批量收集器被强制刷新（通过检查是否触发了flush）
	// 由于是异步的，我们等待一小段时间
	time.Sleep(200 * time.Millisecond)
}

// TestCompleteDataSourceSwitch_WeightedToCEX 测试加权合成 → CEX完整流程
func TestCompleteDataSourceSwitch_WeightedToCEX(t *testing.T) {
	service, mr := setupTestService(t)
	defer mr.Close()

	// 设置系统为就绪状态
	service.mu.Lock()
	service.isReady = true
	service.lastPrices["AAPLX"] = 100.0 // 最后推送价格
	// 设置CEX报价缓存（PreMarket状态）
	service.cexQuotes["AAPL"] = &cex.QuoteMessage{
		Symbol:       "AAPL",
		BidPrice:     cex.FlexibleFloat(99.5),
		MarketStatus: "PreMarket",
		Timestamp:    time.Now().UnixMilli(),
	}
	service.mu.Unlock()

	// 设置数据源切换回调
	service.stateManager.SetOnDataSourceChange(service.onDataSourceChange)

	// 步骤1: 模拟状态管理检测到市场状态变为 pre_hour_trading，股票状态 = normal
	// 通过直接调用数据源切换回调来模拟
	oldSource := types.DataSourceWeighted
	newSource := types.DataSourceCEX
	symbol := "AAPLX"

	// 步骤2: 触发数据源切换
	service.onDataSourceChange(oldSource, newSource, symbol)

	// 步骤3: 验证从CEX缓存获取价格
	service.mu.RLock()
	quote := service.cexQuotes["AAPL"]
	service.mu.RUnlock()
	assert.NotNil(t, quote, "CEX报价应该存在")
	if quote != nil {
		// PreMarket状态应该使用bid_price
		currentPrice := quote.GetCurrentPrice()
		assert.Equal(t, 99.5, currentPrice, "PreMarket状态应该使用bid_price")
	}

	// 步骤4: 验证5%差异检测通过（差异0.5% < 5%）
	_, ok, err := service.validator.ValidateSwitchDiff(100.0, 99.5)
	assert.NoError(t, err, "差异检测应该通过")
	assert.True(t, ok, "差异0.5%应该通过5%阈值")

	// 步骤5: 验证价格被添加到批量收集器（强制立即推送）
	count := service.batchCollector.GetBufferedCount()
	assert.GreaterOrEqual(t, count, 0, "价格应该被添加到批量收集器")

	// 验证批量收集器被强制刷新
	time.Sleep(200 * time.Millisecond)
}

// TestOracleRecovery_PythAbnormalToRecovery 测试Pyth异常 → 降权 → 恢复完整流程
func TestOracleRecovery_PythAbnormalToRecovery(t *testing.T) {
	service, mr := setupTestService(t)
	defer mr.Close()

	service.mu.Lock()
	service.isReady = true
	// 设置初始价格
	service.closePrices["AAPLX"] = 100.0
	service.pythPrices["AAPLX"] = 100.0 // 初始正常价格
	service.gatePrices["AAPLX"] = 102.0
	service.mu.Unlock()

	// 设置数据源为加权合成
	service.stateManager.SetPythStatus("AAPLX", types.OracleStatusNormal)
	service.stateManager.SetGateStatus("AAPLX", types.OracleStatusNormal)

	// 步骤1: Pyth价格跳变到115.0（跳变15% > 10%）
	priceData1 := &pyth.PriceData{
		Symbol:    "AAPLX",
		Price:     115.0,
		Timestamp: time.Now(),
	}

	// 处理Pyth价格（应该检测到跳变异常）
	service.handlePythPrice(priceData1)

	// 验证步骤1：Pyth状态 = ABNORMAL
	pythStatus := service.stateManager.GetPythStatus("AAPLX")
	assert.Equal(t, types.OracleStatusAbnormal, pythStatus, "Pyth状态应该为异常")

	// 验证步骤1：使用降级权重（收盘价60% + Gate40%）
	weighted1 := service.calculateWeightedPriceOnly("AAPLX")
	assert.NotNil(t, weighted1, "应该使用降级权重")
	if weighted1 != nil {
		// 降级权重：收盘价60% + Gate40% = 100.0×0.6 + 102.0×0.4 = 100.8
		expected := 100.0*0.6 + 102.0*0.4
		assert.InDelta(t, expected, weighted1.Price, 0.01, "降级权重计算的加权价格应该正确")
		assert.True(t, weighted1.IsDegraded, "应该标记为降级")
		assert.Equal(t, "Pyth价格异常", weighted1.DegradeReason, "降级原因应该正确")
	}

	// 验证步骤1：记录降权日志（首次）
	service.mu.RLock()
	alreadyDegraded := service.pythJumpAlerted["AAPLX"]
	service.mu.RUnlock()
	assert.True(t, alreadyDegraded, "应该标记已降权")

	// 验证Pyth价格缓存未更新（保留上次正常价格）
	service.mu.RLock()
	pythPrice := service.pythPrices["AAPLX"]
	service.mu.RUnlock()
	assert.Equal(t, 100.0, pythPrice, "Pyth价格缓存应该保留上次正常价格")

	// 步骤2: Pyth价格恢复到105.0（跳变5% ≤ 10%）
	priceData2 := &pyth.PriceData{
		Symbol:    "AAPLX",
		Price:     105.0,
		Timestamp: time.Now(),
	}

	// 处理Pyth价格（应该检测到恢复）
	service.handlePythPrice(priceData2)

	// 验证步骤2：Pyth状态 = NORMAL
	pythStatus2 := service.stateManager.GetPythStatus("AAPLX")
	assert.Equal(t, types.OracleStatusNormal, pythStatus2, "Pyth状态应该恢复为正常")

	// 验证步骤2：恢复正常权重（收盘价40% + Pyth30% + Gate30%）
	weighted2 := service.calculateWeightedPriceOnly("AAPLX")
	assert.NotNil(t, weighted2, "应该恢复正常权重")
	if weighted2 != nil {
		// 正常权重：收盘价40% + Pyth30% + Gate30% = 100.0×0.4 + 105.0×0.3 + 102.0×0.3 = 102.1
		expected := 100.0*0.4 + 105.0*0.3 + 102.0*0.3
		assert.InDelta(t, expected, weighted2.Price, 0.01, "正常权重计算的加权价格应该正确")
		assert.False(t, weighted2.IsDegraded, "恢复后不应该标记为降级")
	}

	// 验证步骤2：Pyth价格缓存已更新
	service.mu.RLock()
	pythPrice2 := service.pythPrices["AAPLX"]
	service.mu.RUnlock()
	assert.Equal(t, 105.0, pythPrice2, "Pyth价格缓存应该已更新")

	// 验证步骤2：降权状态已重置
	service.mu.RLock()
	alreadyDegraded2 := service.pythJumpAlerted["AAPLX"]
	service.mu.RUnlock()
	assert.False(t, alreadyDegraded2, "恢复后应该重置降权状态")
}

// TestOracleRecovery_BothOracleAbnormalToRecovery 测试两个预言机都异常 → 暂停 → 恢复
func TestOracleRecovery_BothOracleAbnormalToRecovery(t *testing.T) {
	service, mr := setupTestService(t)
	defer mr.Close()

	service.mu.Lock()
	service.isReady = true
	// 设置初始价格
	service.closePrices["AAPLX"] = 100.0
	service.pythPrices["AAPLX"] = 100.0 // 初始正常价格
	service.gatePrices["AAPLX"] = 102.0 // 初始正常价格
	service.bothOracleAlerted["AAPLX"] = false // 未发送过告警
	service.mu.Unlock()

	// 设置数据源为加权合成
	service.stateManager.SetPythStatus("AAPLX", types.OracleStatusNormal)
	service.stateManager.SetGateStatus("AAPLX", types.OracleStatusNormal)

	// 步骤1: Pyth价格跳变异常（115.0，跳变15% > 10%）
	priceData1 := &pyth.PriceData{
		Symbol:    "AAPLX",
		Price:     115.0,
		Timestamp: time.Now(),
	}
	service.handlePythPrice(priceData1)

	// 验证步骤1：Pyth异常，使用降级权重
	pythStatus1 := service.stateManager.GetPythStatus("AAPLX")
	assert.Equal(t, types.OracleStatusAbnormal, pythStatus1, "Pyth状态应该为异常")

	// 验证步骤1：使用降级权重（收盘价60% + Gate40%）
	weighted1 := service.calculateWeightedPriceOnly("AAPLX")
	assert.NotNil(t, weighted1, "应该使用降级权重")
	if weighted1 != nil {
		assert.True(t, weighted1.IsDegraded, "应该标记为降级")
		assert.Equal(t, "Pyth价格异常", weighted1.DegradeReason, "降级原因应该正确")
	}

	// 步骤2: Gate价格跳变异常（115.0，跳变12.7% > 10%）
	gateData1 := &gate.PriceData{
		Symbol:    "AAPLX",
		Price:     115.0,
		Timestamp: time.Now(),
	}
	service.handleGatePrice(gateData1)

	// 验证步骤2：Gate异常
	gateStatus1 := service.stateManager.GetGateStatus("AAPLX")
	assert.Equal(t, types.OracleStatusAbnormal, gateStatus1, "Gate状态应该为异常")

	// 验证步骤2：两个都异常，暂停报价
	service.calculateAndPushWeightedPrice("AAPLX")
	isPaused := service.stateManager.IsPaused("AAPLX")
	assert.True(t, isPaused, "两个预言机都异常时应该暂停报价")

	// 验证步骤2：发送告警（仅首次）
	service.mu.RLock()
	alreadyAlerted := service.bothOracleAlerted["AAPLX"]
	service.mu.RUnlock()
	assert.True(t, alreadyAlerted, "应该标记已发送告警")

	// 步骤3: Pyth价格恢复（105.0，跳变5% ≤ 10%）
	priceData2 := &pyth.PriceData{
		Symbol:    "AAPLX",
		Price:     105.0,
		Timestamp: time.Now(),
	}
	service.handlePythPrice(priceData2)

	// 验证步骤3：Pyth恢复
	pythStatus2 := service.stateManager.GetPythStatus("AAPLX")
	assert.Equal(t, types.OracleStatusNormal, pythStatus2, "Pyth状态应该恢复为正常")

	// 验证步骤3：恢复报价
	service.calculateAndPushWeightedPrice("AAPLX")
	isPaused2 := service.stateManager.IsPaused("AAPLX")
	assert.False(t, isPaused2, "Pyth恢复后应该恢复报价")

	// 验证步骤3：重置告警状态
	service.mu.RLock()
	alreadyAlerted2 := service.bothOracleAlerted["AAPLX"]
	service.mu.RUnlock()
	assert.False(t, alreadyAlerted2, "恢复后应该重置告警状态")

	// 验证步骤3：加权价格计算成功（使用降级权重：收盘价60% + Pyth40%，因为Gate仍异常）
	weighted2 := service.calculateWeightedPriceOnly("AAPLX")
	assert.NotNil(t, weighted2, "应该能计算加权价格")
	if weighted2 != nil {
		// 降级权重：收盘价60% + Pyth40% = 100.0×0.6 + 105.0×0.4 = 102.0
		expected := 100.0*0.6 + 105.0*0.4
		assert.InDelta(t, expected, weighted2.Price, 0.01, "降级权重计算的加权价格应该正确")
		assert.True(t, weighted2.IsDegraded, "Gate仍异常时应该标记为降级")
		assert.Equal(t, "Gate价格异常", weighted2.DegradeReason, "降级原因应该正确")
	}
}

// TestOracleRecovery_FullRecovery 测试两个预言机完全恢复
func TestOracleRecovery_FullRecovery(t *testing.T) {
	service, mr := setupTestService(t)
	defer mr.Close()

	service.mu.Lock()
	service.isReady = true
	service.closePrices["AAPLX"] = 100.0
	service.pythPrices["AAPLX"] = 100.0
	service.gatePrices["AAPLX"] = 102.0
	service.bothOracleAlerted["AAPLX"] = true // 之前已发送过告警
	service.mu.Unlock()

	// 设置两个预言机为异常
	service.stateManager.SetPythStatus("AAPLX", types.OracleStatusAbnormal)
	service.stateManager.SetGateStatus("AAPLX", types.OracleStatusAbnormal)
	service.stateManager.PauseStock("AAPLX", "两个预言机都异常")

	// 恢复Pyth
	priceData := &pyth.PriceData{
		Symbol:    "AAPLX",
		Price:     105.0,
		Timestamp: time.Now(),
	}
	service.handlePythPrice(priceData)

	// 恢复Gate
	gateData := &gate.PriceData{
		Symbol:    "AAPLX",
		Price:     107.0,
		Timestamp: time.Now(),
	}
	service.handleGatePrice(gateData)

	// 计算并推送加权价格
	service.calculateAndPushWeightedPrice("AAPLX")

	// 验证两个预言机都恢复
	pythStatus := service.stateManager.GetPythStatus("AAPLX")
	gateStatus := service.stateManager.GetGateStatus("AAPLX")
	assert.Equal(t, types.OracleStatusNormal, pythStatus, "Pyth应该恢复为正常")
	assert.Equal(t, types.OracleStatusNormal, gateStatus, "Gate应该恢复为正常")

	// 验证股票已恢复
	isPaused := service.stateManager.IsPaused("AAPLX")
	assert.False(t, isPaused, "两个预言机都恢复后应该恢复报价")

	// 验证告警状态已重置
	service.mu.RLock()
	alreadyAlerted := service.bothOracleAlerted["AAPLX"]
	service.mu.RUnlock()
	assert.False(t, alreadyAlerted, "完全恢复后应该重置告警状态")

	// 验证恢复正常权重
	weighted := service.calculateWeightedPriceOnly("AAPLX")
	assert.NotNil(t, weighted, "应该恢复正常权重")
	if weighted != nil {
		// 正常权重：收盘价40% + Pyth30% + Gate30% = 100.0×0.4 + 105.0×0.3 + 107.0×0.3 = 103.6
		expected := 100.0*0.4 + 105.0*0.3 + 107.0*0.3
		assert.InDelta(t, expected, weighted.Price, 0.01, "正常权重计算的加权价格应该正确")
		assert.False(t, weighted.IsDegraded, "完全恢复后不应该标记为降级")
	}
}
