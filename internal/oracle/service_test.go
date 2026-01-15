// Package oracle 核心服务模块单元测试
package oracle

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"biya-oracle/internal/config"
	"biya-oracle/internal/datasource/cex"
	"biya-oracle/internal/price"
	"biya-oracle/internal/pusher"
	"biya-oracle/internal/storage"
	"biya-oracle/internal/types"
)

// MockOnChainPusher Mock链上推送器
type MockOnChainPusher struct {
	mock.Mock
}

func (m *MockOnChainPusher) PushPrice(symbol string, price float64, sourceInfo string) error {
	args := m.Called(symbol, price, sourceInfo)
	return args.Error(0)
}

func (m *MockOnChainPusher) PushBatchPricesWithSources(prices map[string]pusher.PriceWithSource) error {
	args := m.Called(prices)
	return args.Error(0)
}

// MockLarkAlert Mock告警服务
type MockLarkAlert struct {
	mock.Mock
}

func (m *MockLarkAlert) SendAlert(alertType types.AlertType, symbol string, data map[string]string) {
	m.Called(alertType, symbol, data)
}

// setupTestService 创建测试用的服务实例（使用真实依赖，但禁用链上推送）
func setupTestService(t *testing.T) (*Service, *miniredis.Miniredis) {
	// 创建内存Redis
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("创建内存Redis失败: %v", err)
	}

	// 创建Redis存储
	redisStorage, err := storage.NewRedisStorage(mr.Addr(), "", 0)
	if err != nil {
		t.Fatalf("创建Redis存储失败: %v", err)
	}

	// 创建配置
	cfg := &config.Config{
		Throttle: config.ThrottleConfig{
			MinPushInterval:      1,
			MaxPushInterval:      3,
			PriceChangeThreshold: 0.003,
		},
		StockTokens: []config.StockTokenConfig{
			{
				CEXSymbol:  "AAPL",
				BaseSymbol: "AAPLX",
				PythSymbol: "AAPLX",
				GatePair:   "AAPLX_USDT",
			},
			{
				CEXSymbol:  "NVDA",
				BaseSymbol: "NVDAX",
				PythSymbol: "NVDAX",
				GatePair:   "NVDAX_USDT",
			},
		},
		OnChain: config.OnChainConfig{
			Enabled: false, // 测试时禁用链上推送
		},
		Lark: config.LarkConfig{
			WebhookURL: "http://test.webhook",
		},
		CEX: config.CEXConfig{
			MarketStatusURL: "http://test.market",
			StockStatusURL:  "http://test.stock",
		},
	}

	// 创建服务（使用真实依赖，但链上推送被禁用）
	service := NewService(cfg, redisStorage)

	return service, mr
}

// TestHandleCEXQuote_Normal 测试正常CEX报价处理
func TestHandleCEXQuote_Normal(t *testing.T) {
	service, mr := setupTestService(t)
	defer mr.Close()

	// 设置系统为就绪状态
	service.mu.Lock()
	service.isReady = true
	service.mu.Unlock()

	// 由于Service的依赖关系复杂，我们采用更实用的测试方法
	// 测试核心业务逻辑：价格校验、格式化、收集

	// 创建CEX报价消息
	quote := &cex.QuoteMessage{
		Symbol:       "AAPL",
		LatestPrice:  cex.FlexibleFloat(100.0),
		MarketStatus: "Regular",
		Timestamp:    time.Now().UnixMilli(),
	}

	// 由于Service的依赖关系复杂，我们采用更实用的测试方法
	// 测试核心业务逻辑：价格校验、格式化、收集

	// 测试1: 价格校验
	err := service.validator.ValidatePrice(float64(quote.LatestPrice))
	assert.NoError(t, err, "正常价格应该通过校验")

	// 测试2: 时间戳校验
	timestamp := time.UnixMilli(quote.Timestamp)
	err = service.validator.ValidateTimestamp(timestamp)
	assert.NoError(t, err, "正常时间戳应该通过校验")

	// 测试3: 价格格式化
	formattedPrice := price.FormatPrice(float64(quote.LatestPrice))
	assert.Equal(t, 100.0, formattedPrice, "价格格式化应该正确")

	// 测试4: GetCurrentPrice方法
	currentPrice := quote.GetCurrentPrice()
	assert.Equal(t, 100.0, currentPrice, "GetCurrentPrice应该返回正确价格")
}

// TestHandleCEXQuote_InvalidPrice 测试CEX价格无效（≤ 0）
func TestHandleCEXQuote_InvalidPrice(t *testing.T) {
	service, mr := setupTestService(t)
	defer mr.Close()

	service.mu.Lock()
	service.isReady = true
	service.mu.Unlock()

	// 创建无效价格的CEX报价
	quote := &cex.QuoteMessage{
		Symbol:       "AAPL",
		LatestPrice:  cex.FlexibleFloat(0),
		MarketStatus: "Regular",
		Timestamp:    time.Now().UnixMilli(),
	}

	// 价格校验应该失败
	err := service.validator.ValidatePrice(float64(quote.LatestPrice))
	assert.Error(t, err, "无效价格应该返回错误")
	assert.Contains(t, err.Error(), "价格必须大于0", "错误信息应该包含提示")
}

// TestHandleCEXQuote_ExpiredTimestamp 测试CEX时间戳过期
func TestHandleCEXQuote_ExpiredTimestamp(t *testing.T) {
	service, mr := setupTestService(t)
	defer mr.Close()

	service.mu.Lock()
	service.isReady = true
	service.mu.Unlock()

	// 创建过期时间戳的CEX报价
	quote := &cex.QuoteMessage{
		Symbol:       "AAPL",
		LatestPrice:  cex.FlexibleFloat(100.0),
		MarketStatus: "Regular",
		Timestamp:    time.Now().Add(-6 * time.Minute).UnixMilli(),
	}

	// 时间戳校验应该失败
	timestamp := time.UnixMilli(quote.Timestamp)
	err := service.validator.ValidateTimestamp(timestamp)
	assert.Error(t, err, "过期时间戳应该返回错误")
	assert.Contains(t, err.Error(), "时间戳与系统时间相差过大", "错误信息应该包含提示")
}

// TestHandleCEXQuote_NotReady 测试系统未就绪时丢弃数据
func TestHandleCEXQuote_NotReady(t *testing.T) {
	service, mr := setupTestService(t)
	defer mr.Close()

	// 系统未就绪
	service.mu.Lock()
	service.isReady = false
	service.mu.Unlock()

	// 创建CEX报价
	quote := &cex.QuoteMessage{
		Symbol:       "AAPL",
		LatestPrice:  cex.FlexibleFloat(100.0),
		MarketStatus: "Regular",
		Timestamp:    time.Now().UnixMilli(),
	}

	// 处理报价（应该被丢弃）
	service.handleCEXQuote(quote)

	// 验证报价未被缓存
	service.mu.RLock()
	cachedQuote := service.cexQuotes["AAPL"]
	service.mu.RUnlock()

	assert.Nil(t, cachedQuote, "系统未就绪时不应该缓存报价")
}

// TestGetCurrentPrice_DifferentMarketStatus 测试不同市场状态的价格字段选择
func TestGetCurrentPrice_DifferentMarketStatus(t *testing.T) {
	testCases := []struct {
		name         string
		marketStatus string
		latestPrice  float64
		bidPrice     float64
		bluePrice    float64
		expected     float64
	}{
		{
			name:         "Regular（盘中）",
			marketStatus: "Regular",
			latestPrice:  100.0,
			bidPrice:     99.0,
			bluePrice:    100.5,
			expected:     100.0,
		},
		{
			name:         "PreMarket（盘前）",
			marketStatus: "PreMarket",
			latestPrice:  100.0,
			bidPrice:     99.0,
			bluePrice:    100.5,
			expected:     99.0,
		},
		{
			name:         "AfterHours（盘后）",
			marketStatus: "AfterHours",
			latestPrice:  100.0,
			bidPrice:     101.0,
			bluePrice:    100.5,
			expected:     101.0,
		},
		{
			name:         "OverNight（夜盘）",
			marketStatus: "OverNight",
			latestPrice:  100.0,
			bidPrice:     99.0,
			bluePrice:    100.5,
			expected:     100.5,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			quote := &cex.QuoteMessage{
				MarketStatus: tc.marketStatus,
				LatestPrice:  cex.FlexibleFloat(tc.latestPrice),
				BidPrice:     cex.FlexibleFloat(tc.bidPrice),
				BluePrice:    cex.FlexibleFloat(tc.bluePrice),
			}

			result := quote.GetCurrentPrice()
			assert.Equal(t, tc.expected, result, "GetCurrentPrice应该根据市场状态返回正确价格")
		})
	}
}

// 由于Service模块的依赖关系复杂，我们需要采用不同的测试策略
// 让我们先测试一些可以独立测试的方法，或者创建测试辅助函数
