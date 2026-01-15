// Package price 价格处理模块
package price

import "math"

// FormatPrice 格式化价格
// price < 1: 保留4位小数（向下舍位）
// price >= 1: 保留2位小数（向下舍位）
func FormatPrice(price float64) float64 {
	if price < 1 {
		return math.Floor(price*10000) / 10000
	}
	return math.Floor(price*100) / 100
}
