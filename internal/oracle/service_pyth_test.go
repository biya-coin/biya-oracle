// Package oracle Pyth价格处理流程单元测试
package oracle

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"biya-oracle/internal/datasource/pyth"
	"biya-oracle/internal/types"
)

// TestHandlePythPrice_Normal 测试正常Pyth价格处理
func TestHandlePythPrice_Normal(t *testing.T) {
	service, mr := setupTestService(t)
	defer mr.Close()

	service.mu.Lock()
	service.isReady = true
	service.pythPrices["AAPLX"] = 100.0 // 上次正常价格
	service.mu.Unlock()

	// 通过状态管理器设置状态
	service.stateManager.SetPythStatus("AAPLX", types.OracleStatusNormal)

	// 创建Pyth价格数据（价格变动1% < 10%，正常）
	priceData := &pyth.PriceData{
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
	lastPrice := service.pythPrices["AAPLX"]
	service.mu.RUnlock()

	jump, isAbnormal := service.calculator.CheckPriceJump(lastPrice, priceData.Price)
	assert.False(t, isAbnormal, "1%跳变应该认为正常")
	assert.InDelta(t, 0.01, jump, 0.0001, "跳变百分比应该正确")
}

// TestHandlePythPrice_PriceJumpAbnormal 测试Pyth价格跳变异常（> 10%）
func TestHandlePythPrice_PriceJumpAbnormal(t *testing.T) {
	service, mr := setupTestService(t)
	defer mr.Close()

	service.mu.Lock()
	service.isReady = true
	service.pythPrices["AAPLX"] = 100.0 // 上次正常价格
	service.mu.Unlock()

	// 创建Pyth价格数据（价格跳变15% > 10%，异常）
	priceData := &pyth.PriceData{
		Symbol:    "AAPLX",
		Price:     115.0,
		Timestamp: time.Now(),
	}

	// 验证价格跳变检测
	service.mu.RLock()
	lastPrice := service.pythPrices["AAPLX"]
	service.mu.RUnlock()

	jump, isAbnormal := service.calculator.CheckPriceJump(lastPrice, priceData.Price)
	assert.True(t, isAbnormal, "15%跳变应该认为异常")
	assert.InDelta(t, 0.15, jump, 0.0001, "跳变百分比应该正确")

	// 验证状态应该被设置为异常
	service.stateManager.SetPythStatus("AAPLX", types.OracleStatusAbnormal)
	status := service.stateManager.GetPythStatus("AAPLX")
	assert.Equal(t, types.OracleStatusAbnormal, status, "Pyth状态应该被设置为异常")
}

// TestHandlePythPrice_PriceRecovery 测试Pyth价格恢复（跳变 ≤ 10%）
func TestHandlePythPrice_PriceRecovery(t *testing.T) {
	service, mr := setupTestService(t)
	defer mr.Close()

	service.mu.Lock()
	service.isReady = true
	service.pythPrices["AAPLX"] = 100.0 // 上次正常价格
	service.pythJumpAlerted["AAPLX"] = true // 已降权
	service.mu.Unlock()

	// 设置状态为异常
	service.stateManager.SetPythStatus("AAPLX", types.OracleStatusAbnormal)

	// 创建Pyth价格数据（价格跳变5% ≤ 10%，恢复）
	priceData := &pyth.PriceData{
		Symbol:    "AAPLX",
		Price:     105.0,
		Timestamp: time.Now(),
	}

	// 验证价格跳变检测
	service.mu.RLock()
	lastPrice := service.pythPrices["AAPLX"]
	service.mu.RUnlock()

	jump, isAbnormal := service.calculator.CheckPriceJump(lastPrice, priceData.Price)
	assert.False(t, isAbnormal, "5%跳变应该认为正常（恢复）")
	assert.InDelta(t, 0.05, jump, 0.0001, "跳变百分比应该正确")

	// 验证状态应该被恢复为正常
	service.stateManager.SetPythStatus("AAPLX", types.OracleStatusNormal)
	status := service.stateManager.GetPythStatus("AAPLX")
	assert.Equal(t, types.OracleStatusNormal, status, "Pyth状态应该被恢复为正常")
}

// TestHandlePythPrice_FirstPrice 测试首次Pyth价格（无上次价格）
func TestHandlePythPrice_FirstPrice(t *testing.T) {
	service, mr := setupTestService(t)
	defer mr.Close()

	service.mu.Lock()
	service.isReady = true
	// 不设置pythPrices，模拟首次价格
	service.mu.Unlock()

	// 创建Pyth价格数据
	priceData := &pyth.PriceData{
		Symbol:    "AAPLX",
		Price:     100.0,
		Timestamp: time.Now(),
	}

	// 验证价格跳变检测（无上次价格，应该认为正常）
	service.mu.RLock()
	lastPrice := service.pythPrices["AAPLX"]
	service.mu.RUnlock()

	jump, isAbnormal := service.calculator.CheckPriceJump(lastPrice, priceData.Price)
	assert.False(t, isAbnormal, "首次价格应该认为正常")
	assert.Equal(t, 0.0, jump, "首次价格跳变应该为0")
}

// TestHandlePythPrice_InvalidPrice 测试Pyth价格无效
func TestHandlePythPrice_InvalidPrice(t *testing.T) {
	service, mr := setupTestService(t)
	defer mr.Close()

	service.mu.Lock()
	service.isReady = true
	service.mu.Unlock()

	// 创建无效价格的Pyth数据
	priceData := &pyth.PriceData{
		Symbol:    "AAPLX",
		Price:     0,
		Timestamp: time.Now(),
	}

	// 价格校验应该失败
	err := service.validator.ValidatePrice(priceData.Price)
	assert.Error(t, err, "无效价格应该返回错误")
}

// TestHandlePythPrice_ExpiredTimestamp 测试Pyth时间戳过期
func TestHandlePythPrice_ExpiredTimestamp(t *testing.T) {
	service, mr := setupTestService(t)
	defer mr.Close()

	service.mu.Lock()
	service.isReady = true
	service.mu.Unlock()

	// 创建过期时间戳的Pyth数据
	priceData := &pyth.PriceData{
		Symbol:    "AAPLX",
		Price:     100.0,
		Timestamp: time.Now().Add(-6 * time.Minute),
	}

	// 时间戳校验应该失败
	err := service.validator.ValidateTimestamp(priceData.Timestamp)
	assert.Error(t, err, "过期时间戳应该返回错误")
}
