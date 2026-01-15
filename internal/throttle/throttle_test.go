// Package throttle 价格瘦身模块单元测试
package throttle

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestShouldPush_FirstPush 测试首次推送
func TestShouldPush_FirstPush(t *testing.T) {
	throttler := NewPriceThrottler(1, 3, 0.003) // 最小1秒，最大3秒，阈值0.3%

	decision := throttler.ShouldPush("AAPLX", 100.0, "CEX", false)

	assert.True(t, decision.ShouldPush, "首次推送应该返回true")
	assert.Equal(t, 100.0, decision.Price, "价格应该正确")
	assert.Equal(t, "CEX", decision.Source, "数据来源应该正确")
	assert.Equal(t, "首次推送", decision.Reason, "原因应该正确")
}

// TestShouldPush_ForceImmediate 测试强制立即推送（数据源切换）
func TestShouldPush_ForceImmediate(t *testing.T) {
	throttler := NewPriceThrottler(1, 3, 0.003)

	// 先推送一次，建立状态
	throttler.ShouldPush("AAPLX", 100.0, "CEX", false)
	throttler.ConfirmPush("AAPLX", 100.0)

	// 等待一小段时间（小于最小间隔）
	time.Sleep(100 * time.Millisecond)

	// 强制立即推送
	decision := throttler.ShouldPush("AAPLX", 100.1, "CEX", true)

	assert.True(t, decision.ShouldPush, "强制推送应该返回true")
	assert.Equal(t, 100.1, decision.Price, "价格应该正确")
	assert.Equal(t, "数据源切换，强制推送", decision.Reason, "原因应该正确")
}

// TestShouldPush_MaxInterval 测试超过最大间隔（心跳推送）
func TestShouldPush_MaxInterval(t *testing.T) {
	throttler := NewPriceThrottler(1, 3, 0.003) // 最大间隔3秒

	// 先推送一次
	throttler.ShouldPush("AAPLX", 100.0, "CEX", false)
	throttler.ConfirmPush("AAPLX", 100.0)

	// 等待超过最大间隔
	time.Sleep(3100 * time.Millisecond)

	// 价格未变，但超过最大间隔
	decision := throttler.ShouldPush("AAPLX", 100.0, "CEX", false)

	assert.True(t, decision.ShouldPush, "超过最大间隔应该推送")
	assert.Equal(t, "超过最大间隔，心跳推送", decision.Reason, "原因应该正确")
}

// TestShouldPush_PriceChangeAboveThreshold 测试超过最小间隔且价格变动 > 0.3%
func TestShouldPush_PriceChangeAboveThreshold(t *testing.T) {
	throttler := NewPriceThrottler(1, 3, 0.003) // 阈值0.3%

	// 先推送一次
	throttler.ShouldPush("AAPLX", 100.0, "CEX", false)
	throttler.ConfirmPush("AAPLX", 100.0)

	// 等待超过最小间隔
	time.Sleep(1100 * time.Millisecond)

	// 价格变动0.5% > 0.3%
	decision := throttler.ShouldPush("AAPLX", 100.5, "CEX", false)

	assert.True(t, decision.ShouldPush, "价格变动超过阈值应该推送")
	assert.Equal(t, 100.5, decision.Price, "价格应该正确")
	assert.Equal(t, "价格变动超过阈值", decision.Reason, "原因应该正确")
}

// TestShouldPush_PriceChangeBelowThreshold 测试超过最小间隔但价格变动 < 0.3%
func TestShouldPush_PriceChangeBelowThreshold(t *testing.T) {
	throttler := NewPriceThrottler(1, 3, 0.003) // 阈值0.3%

	// 先推送一次
	throttler.ShouldPush("AAPLX", 100.0, "CEX", false)
	throttler.ConfirmPush("AAPLX", 100.0)

	// 等待超过最小间隔
	time.Sleep(1100 * time.Millisecond)

	// 价格变动0.2% < 0.3%
	decision := throttler.ShouldPush("AAPLX", 100.2, "CEX", false)

	assert.False(t, decision.ShouldPush, "价格变动过小不应该推送")
	assert.Equal(t, "价格变动过小，跳过", decision.Reason, "原因应该正确")
}

// TestShouldPush_BelowMinInterval 测试未超过最小间隔
func TestShouldPush_BelowMinInterval(t *testing.T) {
	throttler := NewPriceThrottler(1, 3, 0.003) // 最小间隔1秒

	// 先推送一次
	throttler.ShouldPush("AAPLX", 100.0, "CEX", false)
	throttler.ConfirmPush("AAPLX", 100.0)

	// 等待小于最小间隔
	time.Sleep(500 * time.Millisecond)

	// 即使价格变动很大（5%），但未超过最小间隔
	decision := throttler.ShouldPush("AAPLX", 105.0, "CEX", false)

	assert.False(t, decision.ShouldPush, "未超过最小间隔不应该推送")
	assert.Equal(t, "未超过最小推送间隔，跳过", decision.Reason, "原因应该正确")
}

// TestShouldPush_PriceDecrease 测试价格下跌变动
func TestShouldPush_PriceDecrease(t *testing.T) {
	throttler := NewPriceThrottler(1, 3, 0.003) // 阈值0.3%

	// 先推送一次
	throttler.ShouldPush("AAPLX", 100.0, "CEX", false)
	throttler.ConfirmPush("AAPLX", 100.0)

	// 等待超过最小间隔
	time.Sleep(1100 * time.Millisecond)

	// 价格下跌0.5%（绝对值）
	decision := throttler.ShouldPush("AAPLX", 99.5, "CEX", false)

	assert.True(t, decision.ShouldPush, "价格下跌超过阈值应该推送")
	assert.Equal(t, 99.5, decision.Price, "价格应该正确")
	assert.Equal(t, "价格变动超过阈值", decision.Reason, "原因应该正确")
}

// TestConfirmPush 测试确认推送后更新状态
func TestConfirmPush(t *testing.T) {
	throttler := NewPriceThrottler(1, 3, 0.003)

	// 首次推送
	decision := throttler.ShouldPush("AAPLX", 100.0, "CEX", false)
	assert.True(t, decision.ShouldPush, "首次推送应该返回true")

	// 确认推送
	throttler.ConfirmPush("AAPLX", 100.0)

	// 验证状态已更新
	lastPrice := throttler.GetLastPushPrice("AAPLX")
	lastTime := throttler.GetLastPushTime("AAPLX")

	assert.Equal(t, 100.0, lastPrice, "上次推送价格应该正确")
	assert.True(t, time.Since(lastTime) < 1*time.Second, "上次推送时间应该接近当前时间")
}

// TestConfirmPush_UpdateState 测试确认推送后状态更新，影响下次判断
func TestConfirmPush_UpdateState(t *testing.T) {
	throttler := NewPriceThrottler(1, 3, 0.003)

	// 首次推送并确认
	throttler.ShouldPush("AAPLX", 100.0, "CEX", false)
	throttler.ConfirmPush("AAPLX", 100.0)

	// 等待超过最小间隔
	time.Sleep(1100 * time.Millisecond)

	// 下次推送应该基于上次确认的价格和时间
	decision := throttler.ShouldPush("AAPLX", 100.5, "CEX", false)
	assert.True(t, decision.ShouldPush, "应该基于上次确认的价格和时间判断")
}

// TestReset 测试重置状态
func TestReset(t *testing.T) {
	throttler := NewPriceThrottler(1, 3, 0.003)

	// 先推送并确认
	throttler.ShouldPush("AAPLX", 100.0, "CEX", false)
	throttler.ConfirmPush("AAPLX", 100.0)

	// 验证状态存在
	lastPrice := throttler.GetLastPushPrice("AAPLX")
	assert.Equal(t, 100.0, lastPrice, "应该有推送记录")

	// 重置状态
	throttler.Reset("AAPLX")

	// 验证状态已清除
	lastPrice = throttler.GetLastPushPrice("AAPLX")
	assert.Equal(t, 0.0, lastPrice, "重置后价格应该为0")

	// 下次推送应该视为首次推送
	decision := throttler.ShouldPush("AAPLX", 200.0, "CEX", false)
	assert.True(t, decision.ShouldPush, "重置后应该视为首次推送")
	assert.Equal(t, "首次推送", decision.Reason, "原因应该正确")
}

// TestResetAll 测试重置所有状态
func TestResetAll(t *testing.T) {
	throttler := NewPriceThrottler(1, 3, 0.003)

	// 推送多个股票
	throttler.ShouldPush("AAPLX", 100.0, "CEX", false)
	throttler.ConfirmPush("AAPLX", 100.0)
	throttler.ShouldPush("NVDAX", 200.0, "CEX", false)
	throttler.ConfirmPush("NVDAX", 200.0)

	// 验证状态存在
	assert.Equal(t, 100.0, throttler.GetLastPushPrice("AAPLX"))
	assert.Equal(t, 200.0, throttler.GetLastPushPrice("NVDAX"))

	// 重置所有
	throttler.ResetAll()

	// 验证所有状态已清除
	assert.Equal(t, 0.0, throttler.GetLastPushPrice("AAPLX"))
	assert.Equal(t, 0.0, throttler.GetLastPushPrice("NVDAX"))
}

// TestGetLastPushPrice_NotExists 测试获取不存在的股票状态
func TestGetLastPushPrice_NotExists(t *testing.T) {
	throttler := NewPriceThrottler(1, 3, 0.003)

	price := throttler.GetLastPushPrice("UNKNOWN")
	time := throttler.GetLastPushTime("UNKNOWN")

	assert.Equal(t, 0.0, price, "不存在的股票价格应该为0")
	assert.True(t, time.IsZero(), "不存在股票的时间应该为零值")
}

// TestShouldPush_MultipleSymbols 测试多个股票独立管理
func TestShouldPush_MultipleSymbols(t *testing.T) {
	throttler := NewPriceThrottler(1, 3, 0.003)

	// AAPLX 首次推送
	decision1 := throttler.ShouldPush("AAPLX", 100.0, "CEX", false)
	assert.True(t, decision1.ShouldPush, "AAPLX首次推送应该返回true")
	throttler.ConfirmPush("AAPLX", 100.0)

	// NVDAX 首次推送（独立于AAPLX）
	decision2 := throttler.ShouldPush("NVDAX", 200.0, "CEX", false)
	assert.True(t, decision2.ShouldPush, "NVDAX首次推送应该返回true")
	throttler.ConfirmPush("NVDAX", 200.0)

	// 等待超过最小间隔
	time.Sleep(1100 * time.Millisecond)

	// AAPLX 价格变动小，不推送
	decision3 := throttler.ShouldPush("AAPLX", 100.2, "CEX", false)
	assert.False(t, decision3.ShouldPush, "AAPLX价格变动小不应该推送")

	// NVDAX 价格变动大，推送
	decision4 := throttler.ShouldPush("NVDAX", 201.0, "CEX", false)
	assert.True(t, decision4.ShouldPush, "NVDAX价格变动大应该推送")

	// 两个股票的状态应该独立
	assert.Equal(t, 100.0, throttler.GetLastPushPrice("AAPLX"), "AAPLX状态应该独立")
	assert.Equal(t, 200.0, throttler.GetLastPushPrice("NVDAX"), "NVDAX状态应该独立")
}
