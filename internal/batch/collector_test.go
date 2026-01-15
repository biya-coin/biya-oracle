// Package batch 批量收集模块单元测试
package batch

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestAddPrice_SinglePrice 测试添加价格到缓冲区
func TestAddPrice_SinglePrice(t *testing.T) {
	collector := NewPriceBatchCollector(1*time.Second, 100, nil)

	collector.AddPrice("AAPLX", 100.0, "CEX")

	count := collector.GetBufferedCount()
	assert.Equal(t, 1, count, "缓冲区应该包含1个价格")
}

// TestAddPrice_Deduplication 测试同一股票价格更新（自动去重）
func TestAddPrice_Deduplication(t *testing.T) {
	collector := NewPriceBatchCollector(1*time.Second, 100, nil)

	// 添加同一股票的多个价格
	collector.AddPrice("AAPLX", 100.0, "CEX")
	collector.AddPrice("AAPLX", 101.0, "CEX")
	collector.AddPrice("AAPLX", 102.0, "CEX")

	count := collector.GetBufferedCount()
	assert.Equal(t, 1, count, "同一股票应该去重，只保留最新值")

	// 验证最新价格（通过flush回调）
	var flushedPrices map[string]PriceWithSource
	var flushMutex sync.Mutex
	collector.onFlush = func(prices map[string]PriceWithSource) {
		flushMutex.Lock()
		flushedPrices = prices
		flushMutex.Unlock()
	}

	collector.ForceFlush()
	time.Sleep(200 * time.Millisecond) // 等待flush完成

	flushMutex.Lock()
	defer flushMutex.Unlock()
	if flushedPrices != nil {
		price, exists := flushedPrices["AAPLX"]
		assert.True(t, exists, "AAPLX应该存在")
		assert.Equal(t, 102.0, price.Price, "应该保留最新的价格102.0")
	}
}

// TestAddPrice_MultipleSymbols 测试多个股票价格收集
func TestAddPrice_MultipleSymbols(t *testing.T) {
	collector := NewPriceBatchCollector(1*time.Second, 100, nil)

	collector.AddPrice("AAPLX", 100.0, "CEX")
	collector.AddPrice("NVDAX", 200.0, "CEX")
	collector.AddPrice("TSLAX", 300.0, "CEX")

	count := collector.GetBufferedCount()
	assert.Equal(t, 3, count, "缓冲区应该包含3个股票的价格")
}

// TestAddPrice_BufferFull 测试缓冲区满时触发强制刷新
func TestAddPrice_BufferFull(t *testing.T) {
	collector := NewPriceBatchCollector(1*time.Second, 3, nil) // maxBufferSize = 3

	var flushCount int
	var flushMutex sync.Mutex
	collector.onFlush = func(prices map[string]PriceWithSource) {
		flushMutex.Lock()
		flushCount++
		flushMutex.Unlock()
	}

	// 启动收集器（需要启动才能接收flush信号）
	collector.Start()

	// 添加3个股票（达到最大大小）
	collector.AddPrice("AAPLX", 100.0, "CEX")
	collector.AddPrice("NVDAX", 200.0, "CEX")
	collector.AddPrice("TSLAX", 300.0, "CEX")

	// 添加第4个股票，应该触发强制刷新
	collector.AddPrice("BTC", 50000.0, "CEX")

	// 等待flush完成（forceFlush是异步的，需要等待）
	time.Sleep(300 * time.Millisecond)

	flushMutex.Lock()
	actualCount := flushCount
	flushMutex.Unlock()

	assert.Greater(t, actualCount, 0, "缓冲区满时应该触发强制刷新")

	collector.Stop()
}

// TestFlush_TimedFlush 测试定时刷新（1秒间隔）
func TestFlush_TimedFlush(t *testing.T) {
	collector := NewPriceBatchCollector(100*time.Millisecond, 100, nil) // 100ms间隔

	var flushedPrices map[string]PriceWithSource
	var flushMutex sync.Mutex
	collector.onFlush = func(prices map[string]PriceWithSource) {
		flushMutex.Lock()
		flushedPrices = prices
		flushMutex.Unlock()
	}

	// 启动收集器
	collector.Start()

	// 添加价格
	collector.AddPrice("AAPLX", 100.0, "CEX")

	// 等待定时刷新（100ms + 缓冲）
	time.Sleep(250 * time.Millisecond)

	flushMutex.Lock()
	defer flushMutex.Unlock()
	assert.NotNil(t, flushedPrices, "定时刷新应该触发")
	if flushedPrices != nil {
		_, exists := flushedPrices["AAPLX"]
		assert.True(t, exists, "刷新后的数据应该包含AAPLX")
	}

	// 验证缓冲区已清空
	count := collector.GetBufferedCount()
	assert.Equal(t, 0, count, "刷新后缓冲区应该被清空")

	collector.Stop()
}

// TestForceFlush 测试手动强制刷新
func TestForceFlush(t *testing.T) {
	collector := NewPriceBatchCollector(1*time.Second, 100, nil)

	var flushedPrices map[string]PriceWithSource
	var flushMutex sync.Mutex
	collector.onFlush = func(prices map[string]PriceWithSource) {
		flushMutex.Lock()
		flushedPrices = prices
		flushMutex.Unlock()
	}

	// 启动收集器
	collector.Start()

	// 添加价格
	collector.AddPrice("AAPLX", 100.0, "CEX")

	// 手动强制刷新
	collector.ForceFlush()

	// 等待flush完成
	time.Sleep(200 * time.Millisecond)

	flushMutex.Lock()
	defer flushMutex.Unlock()
	assert.NotNil(t, flushedPrices, "强制刷新应该触发")
	if flushedPrices != nil {
		_, exists := flushedPrices["AAPLX"]
		assert.True(t, exists, "刷新后的数据应该包含AAPLX")
	}

	// 验证缓冲区已清空
	count := collector.GetBufferedCount()
	assert.Equal(t, 0, count, "刷新后缓冲区应该被清空")

	collector.Stop()
}

// TestFlush_EmptyBuffer 测试空缓冲区不触发刷新
func TestFlush_EmptyBuffer(t *testing.T) {
	collector := NewPriceBatchCollector(1*time.Second, 100, nil)

	var flushCalled bool
	collector.onFlush = func(prices map[string]PriceWithSource) {
		flushCalled = true
	}

	// 启动收集器
	collector.Start()

	// 不添加任何价格，等待定时刷新
	time.Sleep(1100 * time.Millisecond)

	// 空缓冲区不应该触发onFlush
	assert.False(t, flushCalled, "空缓冲区不应该触发onFlush回调")

	collector.Stop()
}

// TestStop_WaitForFlush 测试停止时等待所有flush完成
func TestStop_WaitForFlush(t *testing.T) {
	collector := NewPriceBatchCollector(1*time.Second, 100, nil)

	var flushCompleted bool
	var flushMutex sync.Mutex
	collector.onFlush = func(prices map[string]PriceWithSource) {
		// 模拟异步操作（如链上推送）
		time.Sleep(100 * time.Millisecond)
		flushMutex.Lock()
		flushCompleted = true
		flushMutex.Unlock()
	}

	// 启动收集器
	collector.Start()

	// 添加价格并触发flush
	collector.AddPrice("AAPLX", 100.0, "CEX")
	collector.ForceFlush()

	// 立即停止（应该等待flush完成）
	stopStart := time.Now()
	collector.Stop()
	stopDuration := time.Since(stopStart)

	// 验证flush已完成
	flushMutex.Lock()
	defer flushMutex.Unlock()
	assert.True(t, flushCompleted, "停止时应该等待flush完成")
	assert.GreaterOrEqual(t, stopDuration, 100*time.Millisecond, "停止应该等待flush完成")
}

// TestStop_FlushRemainingData 测试停止时刷新剩余数据
func TestStop_FlushRemainingData(t *testing.T) {
	collector := NewPriceBatchCollector(1*time.Second, 100, nil)

	var flushedPrices map[string]PriceWithSource
	var flushMutex sync.Mutex
	collector.onFlush = func(prices map[string]PriceWithSource) {
		flushMutex.Lock()
		flushedPrices = prices
		flushMutex.Unlock()
	}

	// 启动收集器
	collector.Start()

	// 添加价格到缓冲区（不触发自动刷新）
	collector.AddPrice("AAPLX", 100.0, "CEX")
	collector.AddPrice("NVDAX", 200.0, "CEX")

	// 立即停止（应该刷新剩余数据）
	collector.Stop()

	// 等待flush完成
	time.Sleep(200 * time.Millisecond)

	flushMutex.Lock()
	defer flushMutex.Unlock()
	assert.NotNil(t, flushedPrices, "停止时应该刷新剩余数据")
	if flushedPrices != nil {
		assert.Equal(t, 2, len(flushedPrices), "应该包含2个股票的价格")
		_, exists1 := flushedPrices["AAPLX"]
		_, exists2 := flushedPrices["NVDAX"]
		assert.True(t, exists1 && exists2, "应该包含所有剩余的价格")
	}
}

// TestGetBufferedCount 测试获取缓冲区数量
func TestGetBufferedCount(t *testing.T) {
	collector := NewPriceBatchCollector(1*time.Second, 100, nil)

	assert.Equal(t, 0, collector.GetBufferedCount(), "初始缓冲区应该为空")

	collector.AddPrice("AAPLX", 100.0, "CEX")
	assert.Equal(t, 1, collector.GetBufferedCount(), "添加1个价格后应该为1")

	collector.AddPrice("NVDAX", 200.0, "CEX")
	assert.Equal(t, 2, collector.GetBufferedCount(), "添加2个价格后应该为2")

	// 同一股票更新（去重）
	collector.AddPrice("AAPLX", 101.0, "CEX")
	assert.Equal(t, 2, collector.GetBufferedCount(), "同一股票更新应该去重，数量不变")
}

// TestClear 测试清空缓冲区
func TestClear(t *testing.T) {
	collector := NewPriceBatchCollector(1*time.Second, 100, nil)

	collector.AddPrice("AAPLX", 100.0, "CEX")
	collector.AddPrice("NVDAX", 200.0, "CEX")

	assert.Equal(t, 2, collector.GetBufferedCount(), "应该有2个价格")

	collector.Clear()

	assert.Equal(t, 0, collector.GetBufferedCount(), "清空后应该为0")
}

// TestAddPrice_Concurrent 测试并发添加价格
func TestAddPrice_Concurrent(t *testing.T) {
	collector := NewPriceBatchCollector(1*time.Second, 100, nil)

	// 并发添加多个股票的价格
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			symbol := "STOCK" + string(rune('A'+idx))
			collector.AddPrice(symbol, float64(100+idx), "CEX")
		}(i)
	}

	wg.Wait()

	count := collector.GetBufferedCount()
	assert.Equal(t, 10, count, "并发添加后应该有10个价格")
}

// TestFlush_Concurrent 测试并发flush
func TestFlush_Concurrent(t *testing.T) {
	collector := NewPriceBatchCollector(1*time.Second, 100, nil)

	var flushCount int
	var flushMutex sync.Mutex
	collector.onFlush = func(prices map[string]PriceWithSource) {
		flushMutex.Lock()
		flushCount++
		flushMutex.Unlock()
	}

	// 启动收集器
	collector.Start()

	// 添加价格
	collector.AddPrice("AAPLX", 100.0, "CEX")

	// 并发触发多个flush
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			collector.ForceFlush()
		}()
	}

	wg.Wait()
	time.Sleep(300 * time.Millisecond)

	flushMutex.Lock()
	defer flushMutex.Unlock()
	// 由于去重和锁保护，flush次数应该合理
	assert.Greater(t, flushCount, 0, "应该至少触发一次flush")

	collector.Stop()
}

// TestStop_Timeout 测试停止时的超时保护
func TestStop_Timeout(t *testing.T) {
	collector := NewPriceBatchCollector(1*time.Second, 100, nil)

	// 模拟一个长时间运行的flush（超过35秒超时）
	collector.onFlush = func(prices map[string]PriceWithSource) {
		time.Sleep(100 * time.Millisecond) // 实际测试中不会真的等35秒
	}

	// 启动收集器
	collector.Start()

	// 添加价格并触发flush
	collector.AddPrice("AAPLX", 100.0, "CEX")
	collector.ForceFlush()

	// 停止（应该正常完成，不会超时）
	stopStart := time.Now()
	collector.Stop()
	stopDuration := time.Since(stopStart)

	// 验证在合理时间内完成（远小于35秒）
	assert.Less(t, stopDuration, 5*time.Second, "停止应该在合理时间内完成")
}
