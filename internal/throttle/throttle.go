// Package throttle 价格数据瘦身模块
package throttle

import (
	"math"
	"sync"
	"time"
)

// PriceThrottler 价格瘦身器
// 用于控制价格推送频率，避免过多的链上交易
type PriceThrottler struct {
	minPushInterval      time.Duration // 最小推送间隔
	maxPushInterval      time.Duration // 最大推送间隔（心跳）
	priceChangeThreshold float64       // 价格变动阈值

	mu          sync.RWMutex
	symbolState map[string]*symbolPushState // 每个股票的推送状态
}

// symbolPushState 单个股票的推送状态
type symbolPushState struct {
	lastPushTime   time.Time // 上次推送时间
	lastPushPrice  float64   // 上次推送价格
	pendingPrice   float64   // 待推送价格
	pendingSource  string    // 待推送数据来源
}

// NewPriceThrottler 创建价格瘦身器
// minInterval: 最小推送间隔（秒）
// maxInterval: 最大推送间隔（秒）
// priceThreshold: 价格变动阈值（如0.003表示0.3%）
func NewPriceThrottler(minInterval, maxInterval int, priceThreshold float64) *PriceThrottler {
	return &PriceThrottler{
		minPushInterval:      time.Duration(minInterval) * time.Second,
		maxPushInterval:      time.Duration(maxInterval) * time.Second,
		priceChangeThreshold: priceThreshold,
		symbolState:          make(map[string]*symbolPushState),
	}
}

// PushDecision 推送决策结果
type PushDecision struct {
	ShouldPush bool    // 是否应该推送
	Price      float64 // 推送价格
	Source     string  // 数据来源
	Reason     string  // 决策原因（用于日志）
}

// ShouldPush 判断是否应该推送价格
// symbol: 股票代币符号
// newPrice: 新价格
// sourceInfo: 数据来源信息
// forceImmediate: 是否强制立即推送（如数据源切换时）
func (t *PriceThrottler) ShouldPush(symbol string, newPrice float64, sourceInfo string, forceImmediate bool) *PushDecision {
	t.mu.Lock()
	defer t.mu.Unlock()

	// 获取或创建该股票的状态
	state, exists := t.symbolState[symbol]
	if !exists {
		state = &symbolPushState{}
		t.symbolState[symbol] = state
	}

	// 更新待推送价格
	state.pendingPrice = newPrice
	state.pendingSource = sourceInfo

	now := time.Now()
	timeSinceLastPush := now.Sub(state.lastPushTime)

	// 首次推送（没有历史价格）
	if state.lastPushPrice == 0 {
		return &PushDecision{
			ShouldPush: true,
			Price:      newPrice,
			Source:     sourceInfo,
			Reason:     "首次推送",
		}
	}

	// 强制立即推送（数据源切换时）
	if forceImmediate {
		return &PushDecision{
			ShouldPush: true,
			Price:      newPrice,
			Source:     sourceInfo,
			Reason:     "数据源切换，强制推送",
		}
	}

	// 计算价格变动幅度
	priceChange := math.Abs((newPrice - state.lastPushPrice) / state.lastPushPrice)

	// 规则1: 超过最大间隔 -> 强制推送（心跳）
	if timeSinceLastPush >= t.maxPushInterval {
		return &PushDecision{
			ShouldPush: true,
			Price:      newPrice,
			Source:     sourceInfo,
			Reason:     "超过最大间隔，心跳推送",
		}
	}

	// 规则2: 超过最小间隔且价格变动超过阈值 -> 推送
	if timeSinceLastPush >= t.minPushInterval {
		if priceChange >= t.priceChangeThreshold {
			return &PushDecision{
				ShouldPush: true,
				Price:      newPrice,
				Source:     sourceInfo,
				Reason:     "价格变动超过阈值",
			}
		}
		// 价格变动小，不推送
		return &PushDecision{
			ShouldPush: false,
			Reason:     "价格变动过小，跳过",
		}
	}

	// 规则3: 未超过最小间隔 -> 不推送
	return &PushDecision{
		ShouldPush: false,
		Reason:     "未超过最小推送间隔，跳过",
	}
}

// ConfirmPush 确认推送完成，更新状态
// 只有在实际推送成功后才调用此方法
func (t *PriceThrottler) ConfirmPush(symbol string, price float64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	state, exists := t.symbolState[symbol]
	if !exists {
		state = &symbolPushState{}
		t.symbolState[symbol] = state
	}

	state.lastPushTime = time.Now()
	state.lastPushPrice = price
}

// GetLastPushPrice 获取上次推送价格
func (t *PriceThrottler) GetLastPushPrice(symbol string) float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if state, exists := t.symbolState[symbol]; exists {
		return state.lastPushPrice
	}
	return 0
}

// GetLastPushTime 获取上次推送时间
func (t *PriceThrottler) GetLastPushTime(symbol string) time.Time {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if state, exists := t.symbolState[symbol]; exists {
		return state.lastPushTime
	}
	return time.Time{}
}

// Reset 重置某个股票的状态（用于数据源切换等场景）
func (t *PriceThrottler) Reset(symbol string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.symbolState, symbol)
}

// ResetAll 重置所有状态
func (t *PriceThrottler) ResetAll() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.symbolState = make(map[string]*symbolPushState)
}
