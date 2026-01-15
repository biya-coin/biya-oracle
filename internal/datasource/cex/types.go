// Package cex CEX数据源模块
package cex

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// FlexibleFloat 可以处理string或float64的浮点数类型
type FlexibleFloat float64

func (ff *FlexibleFloat) UnmarshalJSON(data []byte) error {
	// 先尝试解析为字符串
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		if s == "" {
			*ff = 0
			return nil
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return fmt.Errorf("解析字符串为float失败: %w", err)
		}
		*ff = FlexibleFloat(f)
		return nil
	}

	// 尝试解析为float64
	var f float64
	if err := json.Unmarshal(data, &f); err == nil {
		*ff = FlexibleFloat(f)
		return nil
	}

	return fmt.Errorf("无法解析FlexibleFloat: 既不是字符串也不是数字")
}

// QuoteMessage CEX WebSocket报价消息
type QuoteMessage struct {
	Symbol            string        `json:"symbol"`
	Amount            FlexibleFloat `json:"amount"`
	BidHighPrice      FlexibleFloat `json:"bid_high_price"`
	BluePrice         FlexibleFloat `json:"blue_price"`
	BlueUpDownAmount  FlexibleFloat `json:"blue_up_down_amount"`
	BlueUpDownPercent FlexibleFloat `json:"blue_up_down_percent"`
	BidUpDownAmount   FlexibleFloat `json:"bid_up_down_amount"`
	BidLowPrice       FlexibleFloat `json:"bid_low_price"`
	BidVolume         int           `json:"bid_volume"`
	LatestPrice       FlexibleFloat `json:"latest_price"`
	Type              string        `json:"type"`
	UpDownPercent     FlexibleFloat `json:"up_down_percent"`
	Market            string        `json:"market"`
	Volume            int           `json:"volume"`
	BidTimestamp      int64         `json:"bid_timestamp"`
	High              FlexibleFloat `json:"high"`
	BidAmount         FlexibleFloat `json:"bid_amount"`
	UpDownAmount      FlexibleFloat `json:"up_down_amount"`
	Low               FlexibleFloat `json:"low"`
	DataType          string        `json:"data_type"`
	SymbolType        int           `json:"symbol_type"`
	Close             FlexibleFloat `json:"close"`
	BidPrice          FlexibleFloat `json:"bid_price"`
	Open              FlexibleFloat `json:"open"`
	MarketStatus      string        `json:"marketStatus"`
	Timestamp         int64         `json:"timestamp"`
}

// GetCurrentPrice 根据市场状态获取当前价格
func (q *QuoteMessage) GetCurrentPrice() float64 {
	switch q.MarketStatus {
	case "Regular":
		return float64(q.LatestPrice)
	case "PreMarket", "AfterHours":
		return float64(q.BidPrice)
	case "OverNight":
		return float64(q.BluePrice)
	default:
		return float64(q.LatestPrice)
	}
}

// GetClosePrice 获取收盘价（用于切换到加权合成时）
func (q *QuoteMessage) GetClosePrice() float64 {
	return q.GetCurrentPrice()
}

// SubscribeRequest WebSocket订阅请求
type SubscribeRequest struct {
	Type       string   `json:"type"`
	HandleType string   `json:"handleType"`
	Quote      []string `json:"quote"`
}

// MarketStatusResponse 市场状态接口响应
type MarketStatusResponse struct {
	Code   int                `json:"code"`
	Msg    string             `json:"msg"`
	Data   []MarketStatusData `json:"data"`
	Result bool               `json:"result"`
}

// MarketStatusData 市场状态数据
type MarketStatusData struct {
	Market        string `json:"market"`         // 市场: US/HK
	StatusCN      string `json:"status_cn"`      // 中文状态
	OpenTime      string `json:"open_time"`      // 开盘时间
	TradingStatus string `json:"trading_status"` // 交易状态: TRADING/CLOSING等
	Status        string `json:"status"`         // 状态: Trading/Closed
}

// StockStatusResponse 股票状态接口响应
type StockStatusResponse struct {
	Code   int               `json:"code"`
	Msg    string            `json:"msg"`
	Data   []StockStatusData `json:"data"`
	Result bool              `json:"result"`
}

// StockStatusData 股票状态数据
type StockStatusData struct {
	Symbol          string  `json:"symbol"`            // 股票代码
	Status          string  `json:"status"`            // 股票状态: NORMAL, HALTED等
	HourTradingTag  string  `json:"hour_trading_tag"`  // 交易时段: Regular, PreMarket, AfterHours, OverNight
	LatestPrice     float64 `json:"latest_price"`      // 最新价格
	IsOverNight     int     `json:"isOverNight"`       // 是否夜盘
}
