// Package pusher 链上推送模块单元测试（核心逻辑部分）
package pusher

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"cosmossdk.io/math"
	"biya-oracle/internal/config"
)

// TestPriceFormatting 测试价格格式化（float64 -> math.LegacyDec）
func TestPriceFormatting(t *testing.T) {
	testCases := []struct {
		name     string
		input    float64
		expected string
	}{
		{
			name:     "正常价格",
			input:    100.5,
			expected: "100.500000",
		},
		{
			name:     "小数价格",
			input:    0.123456,
			expected: "0.123456",
		},
		{
			name:     "大价格",
			input:    10000.123456,
			expected: "10000.123456",
		},
		{
			name:     "整数价格",
			input:    100.0,
			expected: "100.000000",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 格式化价格（与onchain.go中的逻辑一致）
			priceStr := fmt.Sprintf("%.6f", tc.input)
			priceDec := math.LegacyMustNewDecFromStr(priceStr)

			// 验证格式化结果
			assert.Equal(t, tc.expected, priceStr, "价格字符串应该正确")
			assert.NotNil(t, priceDec, "LegacyDec应该不为nil")

			// 验证可以转换回来
			decStr := priceDec.String()
			assert.NotEmpty(t, decStr, "LegacyDec字符串应该不为空")
		})
	}
}

// TestPushBatchPricesWithSources_EmptyPrices 测试空价格列表
func TestPushBatchPricesWithSources_EmptyPrices(t *testing.T) {
	cfg := config.OnChainConfig{
		Enabled: false, // 禁用链上推送
	}

	pusher, err := NewOnChainPusher(cfg)
	assert.NoError(t, err, "应该成功创建推送器")
	assert.NotNil(t, pusher, "推送器应该不为nil")

	// 测试空价格列表
	err = pusher.PushBatchPricesWithSources(nil)
	assert.NoError(t, err, "空价格列表应该返回nil")

	err = pusher.PushBatchPricesWithSources(map[string]PriceWithSource{})
	assert.NoError(t, err, "空价格列表应该返回nil")
}

// TestPushBatchPricesWithSources_Disabled 测试未启用时的处理
func TestPushBatchPricesWithSources_Disabled(t *testing.T) {
	cfg := config.OnChainConfig{
		Enabled:     false,
		ChainID:     "test",
		QuoteSymbol: "USDT",
	}

	pusher, err := NewOnChainPusher(cfg)
	assert.NoError(t, err, "应该成功创建推送器")

	// 测试未启用时的推送（应该只打印，不推送）
	prices := map[string]PriceWithSource{
		"AAPLX": {
			Price:      100.5,
			SourceInfo: "CEX",
		},
	}

	err = pusher.PushBatchPricesWithSources(prices)
	assert.NoError(t, err, "未启用时应该返回nil（仅打印）")
}

// TestBuildBatchData 测试批量数据构建逻辑
func TestBuildBatchData(t *testing.T) {
	// 模拟批量数据构建（与PushBatchPricesWithSources中的逻辑一致）
	prices := map[string]PriceWithSource{
		"AAPLX": {
			Price:      100.5,
			SourceInfo: "CEX",
		},
		"NVDAX": {
			Price:      200.5,
			SourceInfo: "CEX",
		},
		"TSLAX": {
			Price:      300.5,
			SourceInfo: "CEX",
		},
	}

	quoteSymbol := "USDT"
	var bases, quotes []string
	var priceDecimals []math.LegacyDec

	for symbol, priceData := range prices {
		bases = append(bases, symbol)
		quotes = append(quotes, quoteSymbol)

		// 将 float64 转换为 LegacyDec
		priceStr := fmt.Sprintf("%.6f", priceData.Price)
		priceDec := math.LegacyMustNewDecFromStr(priceStr)
		priceDecimals = append(priceDecimals, priceDec)
	}

	// 验证数据构建正确
	assert.Equal(t, 3, len(bases), "bases数组长度应该正确")
	assert.Equal(t, 3, len(quotes), "quotes数组长度应该正确")
	assert.Equal(t, 3, len(priceDecimals), "priceDecimals数组长度应该正确")

	// 验证数据内容
	assert.Contains(t, bases, "AAPLX", "bases应该包含AAPLX")
	assert.Contains(t, bases, "NVDAX", "bases应该包含NVDAX")
	assert.Contains(t, bases, "TSLAX", "bases应该包含TSLAX")

	for i, quote := range quotes {
		assert.Equal(t, quoteSymbol, quote, "所有quotes应该相同")
		assert.NotNil(t, priceDecimals[i], "价格应该不为nil")
	}
}

// TestPricePrecision 测试价格精度处理
func TestPricePrecision(t *testing.T) {
	testCases := []struct {
		name     string
		input    float64
		expected string
	}{
		{
			name:     "6位小数",
			input:    100.123456,
			expected: "100.123456",
		},
		{
			name:     "超过6位小数（应该截断）",
			input:    100.123456789,
			expected: "100.123457", // 注意：%.6f会四舍五入，不是截断
		},
		{
			name:     "少于6位小数（应该补零）",
			input:    100.1,
			expected: "100.100000",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			priceStr := fmt.Sprintf("%.6f", tc.input)
			assert.Equal(t, tc.expected, priceStr, "价格格式化应该正确")

			// 验证可以转换为LegacyDec
			priceDec := math.LegacyMustNewDecFromStr(priceStr)
			assert.NotNil(t, priceDec, "LegacyDec应该不为nil")
		})
	}
}
