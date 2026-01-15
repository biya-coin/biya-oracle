// Package oracle Gate价格处理流程单元测试
package oracle

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"biya-oracle/internal/datasource/gate"
	"biya-oracle/internal/types"
)

// TestHandleGatePrice_Normal 测试正常Gate价格处理
func TestHandleGatePrice_Normal(t *testing.T) {
	service, mr := setupTestService(t)
	defer mr.Close()

	service.mu.Lock()
	service.isReady = true
	service.gatePrices["AAPLX"] = 100.0 // 上次正常价格
	service.mu.Unlock()

	// 设置数据源为加权合成
	service.stateManager.SetGateStatus("AAPLX", types.OracleStatusNormal)

	// 创建Gate价格数据（价格变动1% < 10%，正常）
	priceData := &gate.PriceData{
		Symbol:    "AAPLX",
		Price:     101.0,
		Timestamp: time.Now(),
	}

	// 验证价格校验
	err := service.validator.ValidatePrice(priceData.Price)
	assert.NoError(t, err, "正常价格应该通过校验")

	// 验证时间戳校验
	err = service.validator.ValidateTimestamp(priceData.Timestamp)
	assert.NoError(t, err, "正常时间戳应该通过校验")

	// 验证价格跳变检测（1% < 10%，正常）
	service.mu.RLock()
	lastPrice := service.gatePrices["AAPLX"]
	service.mu.RUnlock()

	jump, isAbnormal := service.calculator.CheckPriceJump(lastPrice, priceData.Price)
	assert.False(t, isAbnormal, "1%跳变应该认为正常")
	assert.InDelta(t, 0.01, jump, 0.0001, "跳变百分比应该正确")
}

// TestHandleGatePrice_PriceJumpAbnormal 测试Gate价格跳变异常（> 10%）
func TestHandleGatePrice_PriceJumpAbnormal(t *testing.T) {
	service, mr := setupTestService(t)
	defer mr.Close()

	service.mu.Lock()
	service.isReady = true
	service.gatePrices["AAPLX"] = 100.0 // 上次正常价格
	service.mu.Unlock()

	// 创建Gate价格数据（价格跳变15% > 10%，异常）
	priceData := &gate.PriceData{
		Symbol:    "AAPLX",
		Price:     115.0,
		Timestamp: time.Now(),
	}

	// 验证价格跳变检测
	service.mu.RLock()
	lastPrice := service.gatePrices["AAPLX"]
	service.mu.RUnlock()

	jump, isAbnormal := service.calculator.CheckPriceJump(lastPrice, priceData.Price)
	assert.True(t, isAbnormal, "15%跳变应该认为异常")
	assert.InDelta(t, 0.15, jump, 0.0001, "跳变百分比应该正确")

	// 验证状态应该被设置为异常
	service.stateManager.SetGateStatus("AAPLX", types.OracleStatusAbnormal)
	status := service.stateManager.GetGateStatus("AAPLX")
	assert.Equal(t, types.OracleStatusAbnormal, status, "Gate状态应该被设置为异常")
}

// TestHandleGatePrice_PriceRecovery 测试Gate价格恢复（跳变 ≤ 10%）
func TestHandleGatePrice_PriceRecovery(t *testing.T) {
	service, mr := setupTestService(t)
	defer mr.Close()

	service.mu.Lock()
	service.isReady = true
	service.gatePrices["AAPLX"] = 100.0 // 上次正常价格
	service.gateJumpAlerted["AAPLX"] = true // 已降权
	service.mu.Unlock()

	// 设置状态为异常
	service.stateManager.SetGateStatus("AAPLX", types.OracleStatusAbnormal)

	// 创建Gate价格数据（价格跳变5% ≤ 10%，恢复）
	priceData := &gate.PriceData{
		Symbol:    "AAPLX",
		Price:     105.0,
		Timestamp: time.Now(),
	}

	// 验证价格跳变检测
	service.mu.RLock()
	lastPrice := service.gatePrices["AAPLX"]
	service.mu.RUnlock()

	jump, isAbnormal := service.calculator.CheckPriceJump(lastPrice, priceData.Price)
	assert.False(t, isAbnormal, "5%跳变应该认为正常（恢复）")
	assert.InDelta(t, 0.05, jump, 0.0001, "跳变百分比应该正确")

	// 验证状态应该被恢复为正常
	service.stateManager.SetGateStatus("AAPLX", types.OracleStatusNormal)
	status := service.stateManager.GetGateStatus("AAPLX")
	assert.Equal(t, types.OracleStatusNormal, status, "Gate状态应该被恢复为正常")
}

// TestHandleGatePrice_FirstPrice 测试首次Gate价格（无上次价格）
func TestHandleGatePrice_FirstPrice(t *testing.T) {
	service, mr := setupTestService(t)
	defer mr.Close()

	service.mu.Lock()
	service.isReady = true
	// 不设置gatePrices，模拟首次价格
	service.mu.Unlock()

	// 创建Gate价格数据
	priceData := &gate.PriceData{
		Symbol:    "AAPLX",
		Price:     100.0,
		Timestamp: time.Now(),
	}

	// 验证价格跳变检测（无上次价格，应该认为正常）
	service.mu.RLock()
	lastPrice := service.gatePrices["AAPLX"]
	service.mu.RUnlock()

	jump, isAbnormal := service.calculator.CheckPriceJump(lastPrice, priceData.Price)
	assert.False(t, isAbnormal, "首次价格应该认为正常")
	assert.Equal(t, 0.0, jump, "首次价格跳变应该为0")
}
