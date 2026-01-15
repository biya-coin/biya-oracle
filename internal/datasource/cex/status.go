// Package cex CEX数据源模块
package cex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"biya-oracle/internal/types"
)

// StatusClient CEX状态查询客户端
type StatusClient struct {
	marketStatusURL string
	stockStatusURL  string
	httpClient      *http.Client
}

// NewStatusClient 创建状态查询客户端
func NewStatusClient(marketStatusURL, stockStatusURL string) *StatusClient {
	return &StatusClient{
		marketStatusURL: marketStatusURL,
		stockStatusURL:  stockStatusURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// GetMarketStatus 查询市场状态（美股市场）
func (c *StatusClient) GetMarketStatus(ctx context.Context) (types.MarketStatus, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.marketStatusURL, nil)
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	var result MarketStatusResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}

	if result.Code != 200 {
		return "", fmt.Errorf("API错误: code=%d, msg=%s", result.Code, result.Msg)
	}

	// 从数组中找到US市场的状态
	for _, market := range result.Data {
		if market.Market == "US" {
			return c.mapMarketStatus(market.TradingStatus), nil
		}
	}

	return "", fmt.Errorf("未找到US市场状态")
}

// GetStockStatus 查询股票状态（逐个查询）
func (c *StatusClient) GetStockStatus(ctx context.Context, symbols []string) (map[string]types.StockStatus, error) {
	statuses := make(map[string]types.StockStatus)

	for _, symbol := range symbols {
		status, err := c.getOneStockStatus(ctx, symbol)
		if err != nil {
			// 单个股票查询失败，记录日志但继续查询其他股票
			// 默认为normal
			statuses[symbol] = types.StockStatusNormal
			continue
		}
		statuses[symbol] = status
	}

	return statuses, nil
}

// getOneStockStatus 查询单个股票状态
func (c *StatusClient) getOneStockStatus(ctx context.Context, symbol string) (types.StockStatus, error) {
	// 接口参数是 symbol（单数），不是 symbols
	url := fmt.Sprintf("%s?symbol=%s", c.stockStatusURL, symbol)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return types.StockStatusNormal, fmt.Errorf("创建请求失败: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return types.StockStatusNormal, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return types.StockStatusNormal, fmt.Errorf("读取响应失败: %w", err)
	}

	var result StockStatusResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return types.StockStatusNormal, fmt.Errorf("解析响应失败: %w", err)
	}

	if result.Code != 200 {
		return types.StockStatusNormal, fmt.Errorf("API错误: code=%d, msg=%s", result.Code, result.Msg)
	}

	// 从返回数据中获取状态
	if len(result.Data) > 0 {
		return c.mapStockStatus(result.Data[0].Status), nil
	}

	// 没有数据，默认为normal
	return types.StockStatusNormal, nil
}

// mapMarketStatus 映射市场状态
func (c *StatusClient) mapMarketStatus(status string) types.MarketStatus {
	switch strings.ToUpper(status) {
	case "PRE_HOUR_TRADING", "PREHOURTRADING", "PRE_MARKET", "PREMARKET":
		return types.MarketStatusPreHourTrading
	case "TRADING":
		return types.MarketStatusTrading
	case "POST_HOUR_TRADING", "POSTHOURTRADING", "AFTER_HOURS", "AFTERHOURS":
		return types.MarketStatusPostHourTrading
	case "OVERNIGHT", "OVER_NIGHT":
		return types.MarketStatusOvernight
	case "NOT_YET_OPEN", "NOTYETOPEN":
		return types.MarketStatusNotYetOpen
	case "MIDDLE_CLOSE", "MIDDLECLOSE":
		return types.MarketStatusMiddleClose
	case "CLOSING", "CLOSED":
		return types.MarketStatusClosing
	case "EARLY_CLOSED", "EARLYCLOSED":
		return types.MarketStatusEarlyClosed
	case "MARKET_CLOSED", "MARKETCLOSED":
		return types.MarketStatusMarketClosed
	default:
		return types.MarketStatus(strings.ToLower(status))
	}
}

// mapStockStatus 映射股票状态
func (c *StatusClient) mapStockStatus(status string) types.StockStatus {
	switch strings.ToUpper(status) {
	case "NORMAL":
		return types.StockStatusNormal
	case "CIRCUIT_BREAKER", "CIRCUITBREAKER":
		return types.StockStatusCircuitBreaker
	case "HALTED":
		return types.StockStatusHalted
	case "SUSPENDED":
		return types.StockStatusSuspended
	default:
		return types.StockStatus(strings.ToLower(status))
	}
}
