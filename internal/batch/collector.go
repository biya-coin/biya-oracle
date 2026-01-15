// Package batch 批量收集推送模块
package batch

import (
	"log"
	"sync"
	"time"
)

// PriceWithSource 带来源信息的价格数据
type PriceWithSource struct {
	Price      float64   // 价格
	SourceInfo string    // 数据来源信息
	Timestamp  time.Time // 时间戳
}

// PriceBatchCollector 价格批量收集器
// 用于收集多个股票的价格更新，定期批量推送到链上
type PriceBatchCollector struct {
	mu          sync.RWMutex
	prices      map[string]PriceWithSource // symbol -> 最新价格（自动去重）
	ticker      *time.Ticker
	flushSignal chan struct{} // 手动触发刷新信号（无缓冲，确保不丢失）
	done        chan struct{}
	flushWg     sync.WaitGroup // 跟踪正在进行的 flush 操作
	
	// 配置
	maxFlushInterval time.Duration // 最大刷新间隔（1秒）
	maxBufferSize    int           // 缓冲区最大大小（防止积压过多）
	
	// 回调
	onFlush func(prices map[string]PriceWithSource)
}

// NewPriceBatchCollector 创建价格批量收集器
// maxFlushInterval: 最大刷新间隔（如 1秒）
// maxBufferSize: 缓冲区最大大小（如 100）
// onFlush: 刷新回调函数
func NewPriceBatchCollector(maxFlushInterval time.Duration, maxBufferSize int, onFlush func(map[string]PriceWithSource)) *PriceBatchCollector {
	return &PriceBatchCollector{
		prices:           make(map[string]PriceWithSource),
		flushSignal:      make(chan struct{}), // 无缓冲 channel，确保不丢失信号
		done:             make(chan struct{}),
		maxFlushInterval: maxFlushInterval,
		maxBufferSize:    maxBufferSize,
		onFlush:          onFlush,
	}
}

// Start 启动收集器
func (c *PriceBatchCollector) Start() {
	c.ticker = time.NewTicker(c.maxFlushInterval)
	
	go func() {
		for {
			select {
			case <-c.ticker.C:
				// 定时刷新
				c.flush()
			case <-c.flushSignal:
				// 手动触发刷新
				c.flush()
			case <-c.done:
				return
			}
		}
	}()
}

// Stop 停止收集器
func (c *PriceBatchCollector) Stop() {
	// 先停止 ticker，避免新的定时刷新
	if c.ticker != nil {
		c.ticker.Stop()
	}
	
	// 停止前刷新一次，确保所有数据都被推送
	c.flush()
	
	// 等待所有 flush 操作完成（包括异步的 onFlush 回调）
	// 使用超时避免长时间阻塞（链上推送最多30秒，这里给35秒缓冲）
	done := make(chan struct{})
	go func() {
		c.flushWg.Wait()
		close(done)
	}()
	
	select {
	case <-done:
		// 所有操作已完成
	case <-time.After(35 * time.Second):
		// 超时（可能链上推送阻塞），记录日志但不阻塞
		log.Printf("[批量收集] 警告: 停止时等待 flush 完成超时（35秒），强制退出")
	}
	
	// 关闭 done channel，通知 goroutine 退出
	close(c.done)
}

// AddPrice 添加价格更新
// symbol: 股票代币符号
// price: 价格
// sourceInfo: 数据来源信息
func (c *PriceBatchCollector) AddPrice(symbol string, price float64, sourceInfo string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	// 存储或更新价格（自动去重，同一股票只保留最新值）
	c.prices[symbol] = PriceWithSource{
		Price:      price,
		SourceInfo: sourceInfo,
		Timestamp:  time.Now(),
	}
	
	// 如果缓冲区超过最大大小，触发强制刷新
	if len(c.prices) >= c.maxBufferSize {
		log.Printf("[批量收集] 缓冲区已满 (%d/%d)，触发强制刷新", len(c.prices), c.maxBufferSize)
		go c.forceFlush()
	}
}

// ForceFlush 强制刷新缓冲区（数据源切换等特殊情况使用）
func (c *PriceBatchCollector) ForceFlush() {
	c.forceFlush()
}

// forceFlush 内部强制刷新方法
func (c *PriceBatchCollector) forceFlush() {
	// 检查是否已停止
	select {
	case <-c.done:
		// 已停止，不再发送信号
		return
	default:
	}
	
	// 使用无缓冲 channel，阻塞等待直到信号被接收（确保不丢失）
	// 但如果 goroutine 已退出，这里会阻塞，所以需要超时保护
	select {
	case c.flushSignal <- struct{}{}:
		// 信号已发送
	case <-time.After(100 * time.Millisecond):
		// 超时（可能 goroutine 已退出），记录日志但不阻塞
		log.Printf("[批量收集] 警告: 强制刷新信号发送超时")
	case <-c.done:
		// 在等待期间收到停止信号，退出
		return
	}
}

// flush 刷新缓冲区（内部方法）
func (c *PriceBatchCollector) flush() {
	c.mu.Lock()
	
	if len(c.prices) == 0 {
		c.mu.Unlock()
		return // 空缓冲区，无需推送
	}
	
	// 复制当前缓冲区（避免持有锁太久）
	prices := make(map[string]PriceWithSource)
	for k, v := range c.prices {
		prices[k] = v
	}
	
	// 清空缓冲区
	c.prices = make(map[string]PriceWithSource)
	c.mu.Unlock()
	
	// 调用回调（异步推送）
	if c.onFlush != nil {
		// 增加 WaitGroup 计数，跟踪异步操作
		c.flushWg.Add(1)
		go func() {
			defer c.flushWg.Done()
			c.onFlush(prices)
		}()
	}
}

// GetBufferedCount 获取当前缓冲区中的价格数量
func (c *PriceBatchCollector) GetBufferedCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.prices)
}

// Clear 清空缓冲区（仅用于测试或特殊场景）
func (c *PriceBatchCollector) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.prices = make(map[string]PriceWithSource)
}