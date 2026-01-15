// Package types 公共类型定义
package types

import (
	"fmt"
	"strings"
	"time"
)

// DataSource 数据源类型
type DataSource string

const (
	DataSourceCEX      DataSource = "CEX"      // CEX数据源（纳斯达克/BlueOcean）
	DataSourceWeighted DataSource = "WEIGHTED" // 加权合成报价
)

// MarketStatus CEX市场状态
type MarketStatus string

const (
	MarketStatusPreHourTrading  MarketStatus = "pre_hour_trading"  // 盘前
	MarketStatusTrading         MarketStatus = "trading"           // 盘中
	MarketStatusPostHourTrading MarketStatus = "post_hour_trading" // 盘后
	MarketStatusOvernight       MarketStatus = "overnight_trading" // 夜盘
	MarketStatusNotYetOpen      MarketStatus = "not_yet_open"      // 未开盘
	MarketStatusMiddleClose     MarketStatus = "middle_close"      // 午间休市
	MarketStatusClosing         MarketStatus = "closing"           // 收盘
	MarketStatusEarlyClosed     MarketStatus = "early_closed"      // 提前休市
	MarketStatusMarketClosed    MarketStatus = "market_closed"     // 休市
)

// IsTrading 是否为交易时段
func (m MarketStatus) IsTrading() bool {
	switch m {
	case MarketStatusPreHourTrading, MarketStatusTrading, MarketStatusPostHourTrading, MarketStatusOvernight:
		return true
	default:
		return false
	}
}

// StockStatus 股票状态
type StockStatus string

const (
	StockStatusNormal         StockStatus = "normal"          // 正常
	StockStatusCircuitBreaker StockStatus = "circuit_breaker" // 熔断
	StockStatusHalted         StockStatus = "halted"          // 停牌
	StockStatusSuspended      StockStatus = "suspended"       // 暂停交易
)

// CEXMarketStatus CEX WebSocket推送的市场状态
type CEXMarketStatus string

const (
	CEXMarketStatusPreMarket  CEXMarketStatus = "PreMarket"  // 盘前
	CEXMarketStatusRegular    CEXMarketStatus = "Regular"    // 盘中
	CEXMarketStatusAfterHours CEXMarketStatus = "AfterHours" // 盘后
	CEXMarketStatusOverNight  CEXMarketStatus = "OverNight"  // 夜盘
)

// FormatCEXSourceInfo 格式化CEX数据来源信息
// marketStatus: WebSocket推送中的marketStatus字段值（Regular/PreMarket/AfterHours/OverNight）
// 返回格式如：CEX（盘中）、CEX（盘前）、CEX（盘后）、CEX（夜盘）
func FormatCEXSourceInfo(marketStatus string) string {
	var statusCN string
	switch marketStatus {
	case "Regular":
		statusCN = "盘中"
	case "PreMarket":
		statusCN = "盘前"
	case "AfterHours":
		statusCN = "盘后"
	case "OverNight":
		statusCN = "夜盘"
	default:
		statusCN = marketStatus // 未知状态直接显示原值
	}
	return fmt.Sprintf("CEX（%s）", statusCN)
}

// PriceData 价格数据
type PriceData struct {
	Symbol    string    `json:"symbol"`    // 股票代币符号
	Price     float64   `json:"price"`     // 价格
	Timestamp time.Time `json:"timestamp"` // 时间戳
	Source    string    `json:"source"`    // 数据来源
}

// CEXQuote CEX报价数据
type CEXQuote struct {
	Symbol       string          `json:"symbol"`        // 股票代码
	LatestPrice  float64         `json:"latest_price"`  // 盘中价
	BidPrice     float64         `json:"bid_price"`     // 盘前/盘后价
	BluePrice    float64         `json:"blue_price"`    // 夜盘价
	Close        float64         `json:"close"`         // 收盘价
	MarketStatus CEXMarketStatus `json:"market_status"` // 市场状态
	Timestamp    int64           `json:"timestamp"`     // 时间戳（毫秒）
}

// GetCurrentPrice 根据市场状态获取当前价格
func (q *CEXQuote) GetCurrentPrice() float64 {
	switch q.MarketStatus {
	case CEXMarketStatusRegular:
		return q.LatestPrice
	case CEXMarketStatusPreMarket, CEXMarketStatusAfterHours:
		return q.BidPrice
	case CEXMarketStatusOverNight:
		return q.BluePrice
	default:
		return q.LatestPrice
	}
}

// GetClosePrice 获取收盘价（用于切换到加权合成时）
func (q *CEXQuote) GetClosePrice() float64 {
	return q.GetCurrentPrice()
}

// OracleStatus 预言机状态
type OracleStatus string

const (
	OracleStatusNormal   OracleStatus = "NORMAL"   // 正常
	OracleStatusAbnormal OracleStatus = "ABNORMAL" // 异常（价格跳变>10%）
	OracleStatusError    OracleStatus = "ERROR"    // 错误（连接失败等）
)

// OraclePrice 预言机价格数据
type OraclePrice struct {
	Symbol    string       `json:"symbol"`    // 股票代币符号
	Price     float64      `json:"price"`     // 价格
	Timestamp time.Time    `json:"timestamp"` // 时间戳
	Source    string       `json:"source"`    // 数据来源（pyth/gate）
	Status    OracleStatus `json:"status"`    // 状态
}

// WeightedPrice 加权合成价格
type WeightedPrice struct {
	Symbol           string    `json:"symbol"`             // 股票代币符号
	Price            float64   `json:"price"`              // 最终价格
	ClosePrice       float64   `json:"close_price"`        // CEX收盘价
	ClosePriceWeight float64   `json:"close_price_weight"` // 收盘价权重
	PythPrice        float64   `json:"pyth_price"`         // Pyth价格
	PythWeight       float64   `json:"pyth_weight"`        // Pyth权重
	GatePrice        float64   `json:"gate_price"`         // Gate价格
	GateWeight       float64   `json:"gate_weight"`        // Gate权重
	Timestamp        time.Time `json:"timestamp"`          // 计算时间戳
	IsDegraded       bool      `json:"is_degraded"`        // 是否降级
	DegradeReason    string    `json:"degrade_reason"`     // 降级原因
}

// SourceInfo 获取数据来源描述
// 返回格式如：
// - "加权合成（CEX40%+Pyth30%+Gate30%）"
// - "加权合成（CEX60%+Gate40%）" - Pyth异常时
// - "加权合成（CEX60%+Pyth40%）" - Gate异常时
// - "加权合成（Pyth50%+Gate50%）" - CEX收盘价异常时
func (w *WeightedPrice) SourceInfo() string {
	var parts []string

	if w.ClosePriceWeight > 0 {
		parts = append(parts, fmt.Sprintf("CEX%.0f%%", w.ClosePriceWeight*100))
	}
	if w.PythWeight > 0 {
		parts = append(parts, fmt.Sprintf("Pyth%.0f%%", w.PythWeight*100))
	}
	if w.GateWeight > 0 {
		parts = append(parts, fmt.Sprintf("Gate%.0f%%", w.GateWeight*100))
	}

	if len(parts) == 0 {
		return "加权合成（无数据）"
	}

	return fmt.Sprintf("加权合成（%s）", strings.Join(parts, "+"))
}

// SystemState 系统状态
type SystemState struct {
	DataSources   map[string]DataSource   `json:"data_sources"`   // 每个股票的数据源
	MarketStatus  MarketStatus            `json:"market_status"`  // 市场状态
	StockStatuses map[string]StockStatus  `json:"stock_statuses"` // 股票状态
	PythStatuses  map[string]OracleStatus `json:"pyth_statuses"`  // 每个股票的Pyth状态
	GateStatuses  map[string]OracleStatus `json:"gate_statuses"`  // 每个股票的Gate状态
	PausedStocks  map[string]string       `json:"paused_stocks"`  // 暂停报价的股票及原因
}

// AlertType 告警类型
type AlertType string

const (
	AlertTypePriceAnomaly      AlertType = "PRICE_ANOMALY"       // 价格异常
	AlertTypeConnectionLost    AlertType = "CONNECTION_LOST"     // 连接断开
	AlertTypeDataSourceError   AlertType = "DATA_SOURCE_ERROR"   // 数据源错误
	AlertTypeSwitchDiff        AlertType = "SWITCH_DIFF"         // 切换差异过大
	AlertTypeOracleError       AlertType = "ORACLE_ERROR"        // 预言机异常
	AlertTypePriceInvalid      AlertType = "PRICE_INVALID"       // 价格无效
	AlertTypeTimestampExpired  AlertType = "TIMESTAMP_EXPIRED"   // 时间戳过期
	AlertTypeBothOracleError   AlertType = "BOTH_ORACLE_ERROR"   // 两个预言机都异常
	AlertTypeStockStatusError  AlertType = "STOCK_STATUS_ERROR"  // 股票状态异常
	AlertTypeReconnectFailed   AlertType = "RECONNECT_FAILED"    // 重连失败（达到最大次数）
	AlertTypeStatusQueryFailed AlertType = "STATUS_QUERY_FAILED" // 状态查询失败（达到最大次数）
)
