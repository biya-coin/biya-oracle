// Package price 价格校验模块单元测试
package price

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestValidatePrice_NormalPrice 测试正常价格校验（> 0）
func TestValidatePrice_NormalPrice(t *testing.T) {
	testCases := []struct {
		name  string
		price float64
		valid bool
	}{
		{
			name:  "正常价格 100.5",
			price: 100.5,
			valid: true,
		},
		{
			name:  "正常价格 0.0001",
			price: 0.0001,
			valid: true,
		},
		{
			name:  "正常价格 1.0",
			price: 1.0,
			valid: true,
		},
		{
			name:  "价格 = 0",
			price: 0,
			valid: false,
		},
		{
			name:  "价格 < 0",
			price: -10.0,
			valid: false,
		},
		{
			name:  "价格 = -0.0001",
			price: -0.0001,
			valid: false,
		},
	}

	validator := NewValidator()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.ValidatePrice(tc.price)
			if tc.valid {
				assert.NoError(t, err, "正常价格应该通过校验")
			} else {
				assert.Error(t, err, "无效价格应该返回错误")
				if err != nil {
					assert.Contains(t, err.Error(), "价格必须大于0", "错误信息应该包含提示")
				}
			}
		})
	}
}

// TestValidateTimestamp_NormalTimestamp 测试正常时间戳校验（< 5分钟）
func TestValidateTimestamp_NormalTimestamp(t *testing.T) {
	validator := NewValidator()
	now := time.Now()

	testCases := []struct {
		name      string
		timestamp time.Time
		valid     bool
	}{
		{
			name:      "2分钟前（正常）",
			timestamp: now.Add(-2 * time.Minute),
			valid:     true,
		},
		{
			name:      "当前时间（正常）",
			timestamp: now,
			valid:     true,
		},
		{
			name:      "1分钟前（正常）",
			timestamp: now.Add(-1 * time.Minute),
			valid:     true,
		},
		{
			name:      "4分59秒前（正常，边界值）",
			timestamp: now.Add(-4*time.Minute - 59*time.Second),
			valid:     true,
		},
		{
			name:      "6分钟前（过期）",
			timestamp: now.Add(-6 * time.Minute),
			valid:     false,
		},
		{
			name:      "10分钟前（过期）",
			timestamp: now.Add(-10 * time.Minute),
			valid:     false,
		},
		{
			name:      "5分钟前（过期，边界值）",
			timestamp: now.Add(-5 * time.Minute),
			valid:     false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.ValidateTimestamp(tc.timestamp)
			if tc.valid {
				assert.NoError(t, err, "正常时间戳应该通过校验")
			} else {
				assert.Error(t, err, "过期时间戳应该返回错误")
				if err != nil {
					assert.Contains(t, err.Error(), "时间戳与系统时间相差过大", "错误信息应该包含提示")
				}
			}
		})
	}
}

// TestValidateTimestamp_FutureTimestamp 测试未来时间戳
func TestValidateTimestamp_FutureTimestamp(t *testing.T) {
	validator := NewValidator()
	now := time.Now()

	testCases := []struct {
		name      string
		timestamp time.Time
		valid     bool
	}{
		{
			name:      "2分钟后（正常，取绝对值）",
			timestamp: now.Add(2 * time.Minute),
			valid:     true,
		},
		{
			name:      "6分钟后（过期，取绝对值）",
			timestamp: now.Add(6 * time.Minute),
			valid:     false,
		},
		{
			name:      "5分钟后（过期，边界值）",
			timestamp: now.Add(5*time.Minute + 1*time.Second), // 超过5分钟才算过期
			valid:     false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.ValidateTimestamp(tc.timestamp)
			if tc.valid {
				assert.NoError(t, err, "正常时间戳应该通过校验")
			} else {
				assert.Error(t, err, "过期时间戳应该返回错误")
			}
		})
	}
}

// TestValidateSwitchDiff_NormalDiff 测试正常切换差异（< 5%）
func TestValidateSwitchDiff_NormalDiff(t *testing.T) {
	validator := NewValidator()

	testCases := []struct {
		name      string
		oldPrice  float64
		newPrice  float64
		expected  float64
		shouldPass bool
	}{
		{
			name:       "正常切换差异 3%",
			oldPrice:   100.0,
			newPrice:   103.0,
			expected:   0.03,
			shouldPass: true,
		},
		{
			name:       "正常切换差异 1%",
			oldPrice:   100.0,
			newPrice:   101.0,
			expected:   0.01,
			shouldPass: true,
		},
		{
			name:       "正常切换差异 4.99%",
			oldPrice:   100.0,
			newPrice:   104.99,
			expected:   0.0499,
			shouldPass: true,
		},
		{
			name:       "切换差异过大 6%",
			oldPrice:   100.0,
			newPrice:   106.0,
			expected:   0.06,
			shouldPass: false,
		},
		{
			name:       "切换差异过大 10%",
			oldPrice:   100.0,
			newPrice:   110.0,
			expected:   0.10,
			shouldPass: false,
		},
		{
			name:       "切换差异 = 5%（边界值，不通过）",
			oldPrice:   100.0,
			newPrice:   105.0,
			expected:   0.05,
			shouldPass: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			diff, ok, err := validator.ValidateSwitchDiff(tc.oldPrice, tc.newPrice)
			assert.InDelta(t, tc.expected, diff, 0.0001, "差异百分比应该正确")
			assert.Equal(t, tc.shouldPass, ok, "校验结果应该正确")
			if !tc.shouldPass {
				assert.Error(t, err, "差异过大应该返回错误")
				if err != nil {
					assert.Contains(t, err.Error(), "切换价格差异过大", "错误信息应该包含提示")
				}
			}
		})
	}
}

// TestValidateSwitchDiff_FirstSwitch 测试首次切换（无上次价格）
func TestValidateSwitchDiff_FirstSwitch(t *testing.T) {
	validator := NewValidator()

	testCases := []struct {
		name      string
		oldPrice  float64
		newPrice  float64
		shouldPass bool
	}{
		{
			name:       "首次切换（oldPrice = 0）",
			oldPrice:   0,
			newPrice:   100.0,
			shouldPass: true,
		},
		{
			name:       "首次切换（oldPrice < 0）",
			oldPrice:   -1.0,
			newPrice:   100.0,
			shouldPass: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			diff, ok, err := validator.ValidateSwitchDiff(tc.oldPrice, tc.newPrice)
			assert.Equal(t, 0.0, diff, "首次切换差异应该为0")
			assert.True(t, ok, "首次切换应该通过")
			assert.NoError(t, err, "首次切换不应该有错误")
		})
	}
}

// TestValidateSwitchDiff_PriceDecrease 测试价格下跌差异
func TestValidateSwitchDiff_PriceDecrease(t *testing.T) {
	validator := NewValidator()

	testCases := []struct {
		name      string
		oldPrice  float64
		newPrice  float64
		expected  float64
		shouldPass bool
	}{
		{
			name:       "价格下跌 3%",
			oldPrice:   100.0,
			newPrice:   97.0,
			expected:   0.03,
			shouldPass: true,
		},
		{
			name:       "价格下跌 6%",
			oldPrice:   100.0,
			newPrice:   94.0,
			expected:   0.06,
			shouldPass: false,
		},
		{
			name:       "价格下跌 10%",
			oldPrice:   100.0,
			newPrice:   90.0,
			expected:   0.10,
			shouldPass: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			diff, ok, err := validator.ValidateSwitchDiff(tc.oldPrice, tc.newPrice)
			assert.InDelta(t, tc.expected, diff, 0.0001, "差异百分比应该正确（取绝对值）")
			assert.Equal(t, tc.shouldPass, ok, "校验结果应该正确")
			if !tc.shouldPass {
				assert.Error(t, err, "差异过大应该返回错误")
			}
		})
	}
}

// TestValidatePriceJump_NormalJump 测试正常价格跳变（跳变 ≤ 10%）
func TestValidatePriceJump_NormalJump(t *testing.T) {
	validator := NewValidator()

	testCases := []struct {
		name      string
		oldPrice  float64
		newPrice  float64
		expected  float64
		isNormal  bool
	}{
		{
			name:      "正常价格变动 5%",
			oldPrice:  100.0,
			newPrice:  105.0,
			expected:  0.05,
			isNormal:  true,
		},
		{
			name:      "正常价格变动 10%（边界值）",
			oldPrice:  100.0,
			newPrice:  110.0,
			expected:  0.10,
			isNormal:  true,
		},
		{
			name:      "正常价格变动 1%",
			oldPrice:  100.0,
			newPrice:  101.0,
			expected:  0.01,
			isNormal:  true,
		},
		{
			name:      "价格跳变异常 15%",
			oldPrice:  100.0,
			newPrice:  115.0,
			expected:  0.15,
			isNormal:  false,
		},
		{
			name:      "价格跳变异常 20%",
			oldPrice:  100.0,
			newPrice:  120.0,
			expected:  0.20,
			isNormal:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			jump, isNormal := validator.ValidatePriceJump(tc.oldPrice, tc.newPrice)
			assert.InDelta(t, tc.expected, jump, 0.0001, "跳变百分比应该正确")
			assert.Equal(t, tc.isNormal, isNormal, "跳变判断应该正确")
		})
	}
}

// TestValidatePriceJump_FirstPrice 测试首次价格（无上次价格）
func TestValidatePriceJump_FirstPrice(t *testing.T) {
	validator := NewValidator()

	testCases := []struct {
		name     string
		oldPrice float64
		newPrice float64
		isNormal bool
	}{
		{
			name:     "首次价格（oldPrice = 0）",
			oldPrice: 0,
			newPrice: 100.0,
			isNormal: true,
		},
		{
			name:     "首次价格（oldPrice < 0）",
			oldPrice: -1.0,
			newPrice: 100.0,
			isNormal: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			jump, isNormal := validator.ValidatePriceJump(tc.oldPrice, tc.newPrice)
			assert.Equal(t, 0.0, jump, "首次价格跳变应该为0")
			assert.True(t, isNormal, "首次价格应该认为正常")
		})
	}
}

// TestValidatePriceJump_PriceDecrease 测试价格下跌跳变
func TestValidatePriceJump_PriceDecrease(t *testing.T) {
	validator := NewValidator()

	testCases := []struct {
		name     string
		oldPrice float64
		newPrice float64
		expected float64
		isNormal bool
	}{
		{
			name:     "价格下跌 5%",
			oldPrice: 100.0,
			newPrice: 95.0,
			expected: 0.05,
			isNormal: true,
		},
		{
			name:     "价格下跌 15%",
			oldPrice: 100.0,
			newPrice: 85.0,
			expected: 0.15,
			isNormal: false,
		},
		{
			name:     "价格下跌 10%（边界值）",
			oldPrice: 100.0,
			newPrice: 90.0,
			expected: 0.10,
			isNormal: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			jump, isNormal := validator.ValidatePriceJump(tc.oldPrice, tc.newPrice)
			assert.InDelta(t, tc.expected, jump, 0.0001, "跳变百分比应该正确（取绝对值）")
			assert.Equal(t, tc.isNormal, isNormal, "跳变判断应该正确")
		})
	}
}

// TestValidateAll 测试所有校验规则
func TestValidateAll(t *testing.T) {
	validator := NewValidator()
	now := time.Now()

	testCases := []struct {
		name      string
		price     float64
		timestamp time.Time
		lastPrice float64
		hasError  bool
	}{
		{
			name:      "所有校验通过",
			price:     100.0,
			timestamp: now.Add(-2 * time.Minute),
			lastPrice: 99.0,
			hasError:  false,
		},
		{
			name:      "价格无效",
			price:     0,
			timestamp: now.Add(-2 * time.Minute),
			lastPrice: 99.0,
			hasError:  true,
		},
		{
			name:      "时间戳过期",
			price:     100.0,
			timestamp: now.Add(-6 * time.Minute),
			lastPrice: 99.0,
			hasError:  true,
		},
		{
			name:      "切换差异过大",
			price:     110.0,
			timestamp: now.Add(-2 * time.Minute),
			lastPrice: 100.0,
			hasError:  true,
		},
		{
			name:      "首次价格（无lastPrice）",
			price:     100.0,
			timestamp: now.Add(-2 * time.Minute),
			lastPrice: 0,
			hasError:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errors := validator.ValidateAll(tc.price, tc.timestamp, tc.lastPrice)
			if tc.hasError {
				assert.NotEmpty(t, errors, "应该返回错误")
			} else {
				assert.Empty(t, errors, "不应该有错误")
			}
		})
	}
}
