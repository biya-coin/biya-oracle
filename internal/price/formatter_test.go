// Package price 价格格式化模块单元测试
package price

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestFormatPrice_PriceLessThanOne 测试价格 < 1 的格式化（保留4位小数，舍位）
func TestFormatPrice_PriceLessThanOne(t *testing.T) {
	testCases := []struct {
		name     string
		input    float64
		expected float64
	}{
		{
			name:     "0.12345 -> 0.1234",
			input:    0.12345,
			expected: 0.1234,
		},
		{
			name:     "0.9999 -> 0.9999",
			input:    0.9999,
			expected: 0.9999,
		},
		{
			name:     "0.123456 -> 0.1234",
			input:    0.123456,
			expected: 0.1234,
		},
		{
			name:     "0.0001 -> 0.0001",
			input:    0.0001,
			expected: 0.0001,
		},
		{
			name:     "0.00001 -> 0.0",
			input:    0.00001,
			expected: 0.0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := FormatPrice(tc.input)
			assert.Equal(t, tc.expected, result, "价格格式化结果应该正确")
		})
	}
}

// TestFormatPrice_PriceGreaterOrEqualToOne 测试价格 >= 1 的格式化（保留2位小数，舍位）
func TestFormatPrice_PriceGreaterOrEqualToOne(t *testing.T) {
	testCases := []struct {
		name     string
		input    float64
		expected float64
	}{
		{
			name:     "180.567 -> 180.56",
			input:    180.567,
			expected: 180.56,
		},
		{
			name:     "100.0 -> 100.0",
			input:    100.0,
			expected: 100.0,
		},
		{
			name:     "1.0 -> 1.0",
			input:    1.0,
			expected: 1.0,
		},
		{
			name:     "1.234 -> 1.23",
			input:    1.234,
			expected: 1.23,
		},
		{
			name:     "259.830000 -> 259.83",
			input:    259.830000,
			expected: 259.83,
		},
		{
			name:     "439.140000 -> 439.14",
			input:    439.140000,
			expected: 439.14,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := FormatPrice(tc.input)
			assert.Equal(t, tc.expected, result, "价格格式化结果应该正确")
		})
	}
}

// TestFormatPrice_BoundaryValues 测试边界值
func TestFormatPrice_BoundaryValues(t *testing.T) {
	testCases := []struct {
		name     string
		input    float64
		expected float64
	}{
		{
			name:     "价格 = 0.9999（< 1，保留4位）",
			input:    0.9999,
			expected: 0.9999,
		},
		{
			name:     "价格 = 1.0001（>= 1，保留2位）",
			input:    1.0001,
			expected: 1.0,
		},
		{
			name:     "价格 = 1.0（边界值）",
			input:    1.0,
			expected: 1.0,
		},
		{
			name:     "价格 = 0.0（特殊情况）",
			input:    0.0,
			expected: 0.0,
		},
		{
			name:     "价格 = 0.99999（接近1但<1）",
			input:    0.99999,
			expected: 0.9999,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := FormatPrice(tc.input)
			assert.Equal(t, tc.expected, result, "边界值格式化结果应该正确")
		})
	}
}

// TestFormatPrice_LargeNumbers 测试大数值
func TestFormatPrice_LargeNumbers(t *testing.T) {
	testCases := []struct {
		name     string
		input    float64
		expected float64
	}{
		{
			name:     "大整数",
			input:    1000.0,
			expected: 1000.0,
		},
		{
			name:     "大数带小数",
			input:    1234.567,
			expected: 1234.56,
		},
		{
			name:     "极大数",
			input:    999999.999,
			expected: 999999.99,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := FormatPrice(tc.input)
			assert.Equal(t, tc.expected, result, "大数值格式化结果应该正确")
		})
	}
}
