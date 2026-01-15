// Package price 价格处理模块
package price

import (
	"fmt"
	"math"
	"time"
)

// ValidationError 校验错误
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// Validator 价格校验器
type Validator struct {
	maxTimeDiff time.Duration // 最大时间差
	maxSwitchDiff float64     // 最大切换差异
}

// NewValidator 创建价格校验器
func NewValidator() *Validator {
	return &Validator{
		maxTimeDiff:   5 * time.Minute, // 时间戳与系统时间相差 < 5分钟
		maxSwitchDiff: 0.05,            // 切换价格差异 < 5%
	}
}

// ValidatePrice 校验价格
// 规则: 价格 > 0
func (v *Validator) ValidatePrice(price float64) error {
	if price <= 0 {
		return &ValidationError{
			Field:   "price",
			Message: fmt.Sprintf("价格必须大于0，当前值: %.6f", price),
		}
	}
	return nil
}

// ValidateTimestamp 校验时间戳
// 规则: 与系统时间相差 < 5分钟
func (v *Validator) ValidateTimestamp(timestamp time.Time) error {
	diff := time.Since(timestamp)
	if diff < 0 {
		diff = -diff
	}
	
	if diff > v.maxTimeDiff {
		return &ValidationError{
			Field:   "timestamp",
			Message: fmt.Sprintf("时间戳与系统时间相差过大: %.1f秒 (允许: %.1f秒)", diff.Seconds(), v.maxTimeDiff.Seconds()),
		}
	}
	return nil
}

// ValidateSwitchDiff 校验切换价格差异
// 规则: 差异 < 5%
// 返回: 差异百分比, 是否通过, error
func (v *Validator) ValidateSwitchDiff(oldPrice, newPrice float64) (float64, bool, error) {
	if oldPrice <= 0 {
		// 没有旧价格，不需要校验差异
		return 0, true, nil
	}

	diff := math.Abs((newPrice - oldPrice) / oldPrice)
	
	if diff >= v.maxSwitchDiff {
		return diff, false, &ValidationError{
			Field:   "switch_diff",
			Message: fmt.Sprintf("切换价格差异过大: %.2f%% (允许: %.2f%%)", diff*100, v.maxSwitchDiff*100),
		}
	}

	return diff, true, nil
}

// ValidatePriceJump 校验价格跳变
// 规则: 跳变 < 10%
// 返回: 跳变百分比, 是否正常（未跳变）
func (v *Validator) ValidatePriceJump(oldPrice, newPrice float64) (float64, bool) {
	if oldPrice <= 0 {
		// 没有旧价格，认为正常
		return 0, true
	}

	jump := math.Abs((newPrice - oldPrice) / oldPrice)
	return jump, jump <= 0.1 // 跳变 <= 10% 为正常
}

// ValidateAll 执行所有校验
func (v *Validator) ValidateAll(price float64, timestamp time.Time, lastPrice float64) []error {
	var errors []error

	if err := v.ValidatePrice(price); err != nil {
		errors = append(errors, err)
	}

	if err := v.ValidateTimestamp(timestamp); err != nil {
		errors = append(errors, err)
	}

	if lastPrice > 0 {
		if _, ok, err := v.ValidateSwitchDiff(lastPrice, price); !ok && err != nil {
			errors = append(errors, err)
		}
	}

	return errors
}
