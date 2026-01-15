// Package state 状态管理模块
package state

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"biya-oracle/internal/alert"
	"biya-oracle/internal/datasource/cex"
	"biya-oracle/internal/types"
)

const (
	// 状态轮询间隔
	pollInterval = 3 * time.Second
	// 最大连续查询失败次数
	maxQueryFailures = 10
)

// SymbolMapping 符号映射（CEX符号 <-> BaseSymbol）
type SymbolMapping struct {
	CEXSymbol  string // CEX使用的股票代码（如 "AAPL"）
	BaseSymbol string // 系统内部统一标识符（如 "AAPLX"）
}

// DataSourceChangeHandler 数据源切换回调
type DataSourceChangeHandler func(oldSource, newSource types.DataSource, symbol string)

// FatalErrorHandler 致命错误回调
type FatalErrorHandler func(source string, err error)

// Manager 状态管理器
type Manager struct {
	statusClient    *cex.StatusClient
	alertManager    *alert.LarkAlert
	symbolMappings  []SymbolMapping // 符号映射列表
	cexToBase       map[string]string // CEXSymbol -> BaseSymbol 映射
	baseToCEX       map[string]string // BaseSymbol -> CEXSymbol 映射
	done            chan struct{}
	mu              sync.RWMutex
	
	// 状态（按 BaseSymbol 独立存储）
	dataSources   map[string]types.DataSource   // 每个股票的数据源（key: BaseSymbol）
	marketStatus  types.MarketStatus
	stockStatuses map[string]types.StockStatus  // key: BaseSymbol
	pausedStocks  map[string]string             // 暂停报价的股票及原因（key: BaseSymbol）
	
	// 预言机状态（按股票独立存储，key: BaseSymbol）
	pythStatuses map[string]types.OracleStatus  // 每个股票的Pyth状态
	gateStatuses map[string]types.OracleStatus  // 每个股票的Gate状态
	
	// 回调
	onDataSourceChange DataSourceChangeHandler
	fatalErrorHandler  FatalErrorHandler
	
	// 连续查询失败计数
	queryFailureCount int
}

// NewManager 创建状态管理器
// symbolMappings: 符号映射列表，包含 CEXSymbol 和 BaseSymbol 的对应关系
func NewManager(statusClient *cex.StatusClient, alertManager *alert.LarkAlert, symbolMappings []SymbolMapping, fatalHandler FatalErrorHandler) *Manager {
	// 构建符号映射
	cexToBase := make(map[string]string)
	baseToCEX := make(map[string]string)
	dataSources := make(map[string]types.DataSource)
	
	for _, mapping := range symbolMappings {
		cexToBase[mapping.CEXSymbol] = mapping.BaseSymbol
		baseToCEX[mapping.BaseSymbol] = mapping.CEXSymbol
		// 使用 BaseSymbol 作为 key 初始化数据源
		dataSources[mapping.BaseSymbol] = types.DataSourceCEX
	}
	
	// 初始化每个股票的预言机状态为正常
	pythStatuses := make(map[string]types.OracleStatus)
	gateStatuses := make(map[string]types.OracleStatus)
	for _, mapping := range symbolMappings {
		pythStatuses[mapping.BaseSymbol] = types.OracleStatusNormal
		gateStatuses[mapping.BaseSymbol] = types.OracleStatusNormal
	}
	
	return &Manager{
		statusClient:      statusClient,
		alertManager:      alertManager,
		symbolMappings:    symbolMappings,
		cexToBase:         cexToBase,
		baseToCEX:         baseToCEX,
		done:              make(chan struct{}),
		dataSources:       dataSources,
		stockStatuses:     make(map[string]types.StockStatus),
		pausedStocks:      make(map[string]string),
		pythStatuses:      pythStatuses,
		gateStatuses:      gateStatuses,
		fatalErrorHandler: fatalHandler,
	}
}

// SetOnDataSourceChange 设置数据源切换回调
func (m *Manager) SetOnDataSourceChange(handler DataSourceChangeHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onDataSourceChange = handler
}

// Start 启动状态管理
func (m *Manager) Start(ctx context.Context) {
	log.Println("[状态管理] 启动，轮询间隔:", pollInterval)
	
	// 立即执行一次状态查询
	m.pollStatus(ctx)
	
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("[状态管理] 收到停止信号")
			return
		case <-m.done:
			log.Println("[状态管理] 停止")
			return
		case <-ticker.C:
			m.pollStatus(ctx)
		}
	}
}

// Stop 停止状态管理
func (m *Manager) Stop() {
	close(m.done)
}

// pollStatus 轮询状态
func (m *Manager) pollStatus(ctx context.Context) {
	// 查询市场状态
	marketStatus, err := m.statusClient.GetMarketStatus(ctx)
	if err != nil {
		m.handleQueryFailure("市场状态", err)
		return
	}

	// 收集 CEX 符号列表
	var cexSymbols []string
	for _, mapping := range m.symbolMappings {
		cexSymbols = append(cexSymbols, mapping.CEXSymbol)
	}

	// 查询股票状态（使用 CEX 符号）
	cexStockStatuses, err := m.statusClient.GetStockStatus(ctx, cexSymbols)
	if err != nil {
		m.handleQueryFailure("股票状态", err)
		return
	}

	// 将 CEX 符号的结果映射为 BaseSymbol
	stockStatuses := make(map[string]types.StockStatus)
	for cexSymbol, status := range cexStockStatuses {
		if baseSymbol, ok := m.cexToBase[cexSymbol]; ok {
			stockStatuses[baseSymbol] = status
		}
	}

	// 查询成功，重置失败计数
	m.mu.Lock()
	m.queryFailureCount = 0
	m.mu.Unlock()

	// 更新状态并判断数据源（使用 BaseSymbol）
	m.updateStatus(marketStatus, stockStatuses)
}

// handleQueryFailure 处理查询失败
func (m *Manager) handleQueryFailure(queryType string, err error) {
	m.mu.Lock()
	m.queryFailureCount++
	currentCount := m.queryFailureCount
	m.mu.Unlock()

	log.Printf("[状态管理] 查询%s失败 (第%d次/%d次): %v", queryType, currentCount, maxQueryFailures, err)

	// 检查是否达到最大失败次数
	if currentCount >= maxQueryFailures {
		log.Printf("[状态管理] 连续查询失败次数达到上限 (%d次)，触发致命错误", maxQueryFailures)
		
		// 发送告警
		if m.alertManager != nil {
			m.alertManager.SendAlert(types.AlertTypeStatusQueryFailed, "", map[string]string{
				"查询类型":   queryType,
				"失败次数":   fmt.Sprintf("%d", currentCount),
				"最后错误":   err.Error(),
				"说明":     "连续查询失败次数达到上限，程序即将终止",
			})
		}
		
		// 触发致命错误回调
		if m.fatalErrorHandler != nil {
			m.fatalErrorHandler("状态管理", fmt.Errorf("连续查询%s失败次数达到上限 (%d次): %v", queryType, maxQueryFailures, err))
		}
	}
}

// updateStatus 更新状态并决定数据源（每个股票独立判断）
func (m *Manager) updateStatus(marketStatus types.MarketStatus, stockStatuses map[string]types.StockStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.marketStatus = marketStatus
	m.stockStatuses = stockStatuses

	// 遍历每个股票，独立判断数据源
	for symbol, stockStatus := range stockStatuses {
		oldDataSource := m.dataSources[symbol]
		newDataSource, shouldPause, pauseReason := m.decideDataSource(marketStatus, stockStatus)
		
		// 处理暂停报价
		_, wasPaused := m.pausedStocks[symbol]
		if shouldPause {
			if !wasPaused {
				m.pausedStocks[symbol] = pauseReason
				log.Printf("[状态管理] %s 暂停报价: %s", symbol, pauseReason)
				
				// 发送告警
				if m.alertManager != nil {
					m.alertManager.SendAlert(types.AlertTypeStockStatusError, symbol, map[string]string{
						"市场状态": string(marketStatus),
						"股票状态": string(stockStatus),
						"原因":   pauseReason,
					})
				}
			}
			continue
		}

		// 恢复报价
		if wasPaused {
			delete(m.pausedStocks, symbol)
			log.Printf("[状态管理] %s 恢复报价", symbol)
		}

		// 数据源切换（每个股票独立）
		if newDataSource != oldDataSource {
			m.dataSources[symbol] = newDataSource
			log.Printf("[状态管理] %s 数据源切换: %s -> %s (市场=%s, 股票=%s)",
				symbol, oldDataSource, newDataSource, marketStatus, stockStatus)
			
			// 触发回调
			if m.onDataSourceChange != nil {
				go m.onDataSourceChange(oldDataSource, newDataSource, symbol)
			}
		}
	}
}

// decideDataSource 根据市场状态和股票状态决定数据源
// 返回: 数据源, 是否暂停, 暂停原因
func (m *Manager) decideDataSource(marketStatus types.MarketStatus, stockStatus types.StockStatus) (types.DataSource, bool, string) {
	// 市场在交易时段
	if marketStatus.IsTrading() {
		switch stockStatus {
		case types.StockStatusNormal:
			// 市场交易时段 + 股票正常 → CEX报价
			return types.DataSourceCEX, false, ""
		case types.StockStatusCircuitBreaker:
			// 市场交易时段 + 股票熔断 → 加权合成报价
			return types.DataSourceWeighted, false, ""
		default:
			// 市场交易时段 + 股票其他状态 → 暂停报价 + 告警
			return types.DataSourceCEX, true, "股票状态异常: " + string(stockStatus)
		}
	}

	// 市场非交易时段
	switch stockStatus {
	case types.StockStatusNormal, types.StockStatusCircuitBreaker:
		// 市场非交易时段 + 股票正常或熔断 → 加权合成报价
		return types.DataSourceWeighted, false, ""
	default:
		// 市场非交易时段 + 股票其他状态 → 暂停报价 + 告警
		return types.DataSourceWeighted, true, "股票状态异常: " + string(stockStatus)
	}
}

// GetCurrentDataSource 获取指定股票的当前数据源
func (m *Manager) GetCurrentDataSource(symbol string) types.DataSource {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if ds, ok := m.dataSources[symbol]; ok {
		return ds
	}
	return types.DataSourceCEX // 默认返回CEX
}

// GetMarketStatus 获取市场状态
func (m *Manager) GetMarketStatus() types.MarketStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.marketStatus
}

// GetStockStatus 获取股票状态
func (m *Manager) GetStockStatus(symbol string) types.StockStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.stockStatuses[symbol]
}

// IsPaused 指定股票是否暂停报价
func (m *Manager) IsPaused(symbol string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, paused := m.pausedStocks[symbol]
	return paused
}

// GetPauseReason 获取指定股票的暂停原因
func (m *Manager) GetPauseReason(symbol string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.pausedStocks[symbol]
}

// PauseStock 手动暂停某个股票的报价（例如：5%差异过大时）
func (m *Manager) PauseStock(symbol string, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pausedStocks[symbol] = reason
}

// ResumeStock 恢复某个股票的报价
func (m *Manager) ResumeStock(symbol string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.pausedStocks, symbol)
}

// RollbackDataSource 回滚指定股票的数据源到指定状态
// 当数据源切换失败（如5%差异过大）时使用，这样下一次轮询会再次尝试切换
func (m *Manager) RollbackDataSource(symbol string, dataSource types.DataSource) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dataSources[symbol] = dataSource
	log.Printf("[状态管理] %s 数据源回滚到: %s", symbol, dataSource)
}

// SetPythStatus 设置指定股票的Pyth状态
func (m *Manager) SetPythStatus(symbol string, status types.OracleStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pythStatuses[symbol] = status
}

// SetGateStatus 设置指定股票的Gate状态
func (m *Manager) SetGateStatus(symbol string, status types.OracleStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.gateStatuses[symbol] = status
}

// GetPythStatus 获取指定股票的Pyth状态
func (m *Manager) GetPythStatus(symbol string) types.OracleStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if status, ok := m.pythStatuses[symbol]; ok {
		return status
	}
	return types.OracleStatusNormal // 默认正常
}

// GetGateStatus 获取指定股票的Gate状态
func (m *Manager) GetGateStatus(symbol string) types.OracleStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if status, ok := m.gateStatuses[symbol]; ok {
		return status
	}
	return types.OracleStatusNormal // 默认正常
}

// GetState 获取完整状态
func (m *Manager) GetState() *types.SystemState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	// 复制数据源map
	dataSources := make(map[string]types.DataSource)
	for k, v := range m.dataSources {
		dataSources[k] = v
	}
	
	// 复制暂停股票map
	pausedStocks := make(map[string]string)
	for k, v := range m.pausedStocks {
		pausedStocks[k] = v
	}
	
	// 复制预言机状态map
	pythStatuses := make(map[string]types.OracleStatus)
	for k, v := range m.pythStatuses {
		pythStatuses[k] = v
	}
	gateStatuses := make(map[string]types.OracleStatus)
	for k, v := range m.gateStatuses {
		gateStatuses[k] = v
	}
	
	return &types.SystemState{
		DataSources:   dataSources,
		MarketStatus:  m.marketStatus,
		StockStatuses: m.stockStatuses,
		PythStatuses:  pythStatuses,
		GateStatuses:  gateStatuses,
		PausedStocks:  pausedStocks,
	}
}
