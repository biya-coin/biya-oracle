// Package cex CEX数据源模块单元测试（数据解析部分）
package cex

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestFlexibleFloat_UnmarshalJSON 测试FlexibleFloat的JSON解析
func TestFlexibleFloat_UnmarshalJSON(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected float64
	}{
		{
			name:     "字符串格式",
			input:    `"100.5"`,
			expected: 100.5,
		},
		{
			name:     "数字格式",
			input:    `100.5`,
			expected: 100.5,
		},
		{
			name:     "空字符串",
			input:    `""`,
			expected: 0,
		},
		{
			name:     "整数",
			input:    `100`,
			expected: 100.0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var ff FlexibleFloat
			err := json.Unmarshal([]byte(tc.input), &ff)
			assert.NoError(t, err, "应该成功解析")
			assert.Equal(t, FlexibleFloat(tc.expected), ff, "解析结果应该正确")
		})
	}
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
		{
			name:         "未知状态（默认使用latest_price）",
			marketStatus: "Unknown",
			latestPrice:  100.0,
			bidPrice:     99.0,
			bluePrice:    100.5,
			expected:     100.0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			quote := &QuoteMessage{
				MarketStatus: tc.marketStatus,
				LatestPrice:  FlexibleFloat(tc.latestPrice),
				BidPrice:     FlexibleFloat(tc.bidPrice),
				BluePrice:    FlexibleFloat(tc.bluePrice),
			}

			result := quote.GetCurrentPrice()
			assert.Equal(t, tc.expected, result, "GetCurrentPrice应该根据市场状态返回正确价格")
		})
	}
}

// TestGetClosePrice 测试获取收盘价
func TestGetClosePrice(t *testing.T) {
	quote := &QuoteMessage{
		MarketStatus: "Regular",
		LatestPrice:  FlexibleFloat(100.5),
		BidPrice:     FlexibleFloat(99.0),
		BluePrice:    FlexibleFloat(100.0),
	}

	// GetClosePrice应该调用GetCurrentPrice
	result := quote.GetClosePrice()
	expected := quote.GetCurrentPrice()
	assert.Equal(t, expected, result, "GetClosePrice应该返回GetCurrentPrice的结果")
}

// TestQuoteMessage_JSONUnmarshal 测试QuoteMessage的JSON解析
func TestQuoteMessage_JSONUnmarshal(t *testing.T) {
	jsonStr := `{
		"symbol": "AAPL",
		"latest_price": "100.5",
		"bid_price": "99.0",
		"blue_price": "100.0",
		"marketStatus": "Regular",
		"timestamp": 1234567890
	}`

	var quote QuoteMessage
	err := json.Unmarshal([]byte(jsonStr), &quote)
	assert.NoError(t, err, "应该成功解析JSON")
	assert.Equal(t, "AAPL", quote.Symbol, "Symbol应该正确")
	assert.Equal(t, FlexibleFloat(100.5), quote.LatestPrice, "LatestPrice应该正确")
	assert.Equal(t, FlexibleFloat(99.0), quote.BidPrice, "BidPrice应该正确")
	assert.Equal(t, FlexibleFloat(100.0), quote.BluePrice, "BluePrice应该正确")
	assert.Equal(t, "Regular", quote.MarketStatus, "MarketStatus应该正确")
	assert.Equal(t, int64(1234567890), quote.Timestamp, "Timestamp应该正确")
}

// TestQuoteMessage_JSONUnmarshal_NumberFormat 测试数字格式的JSON解析
func TestQuoteMessage_JSONUnmarshal_NumberFormat(t *testing.T) {
	jsonStr := `{
		"symbol": "AAPL",
		"latest_price": 100.5,
		"bid_price": 99.0,
		"blue_price": 100.0,
		"marketStatus": "Regular",
		"timestamp": 1234567890
	}`

	var quote QuoteMessage
	err := json.Unmarshal([]byte(jsonStr), &quote)
	assert.NoError(t, err, "应该成功解析JSON（数字格式）")
	assert.Equal(t, FlexibleFloat(100.5), quote.LatestPrice, "LatestPrice应该正确")
	assert.Equal(t, FlexibleFloat(99.0), quote.BidPrice, "BidPrice应该正确")
}
