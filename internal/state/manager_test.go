// Package state 状态管理模块单元测试（同包测试，可访问私有方法）
package state

import (
	"testing"
	"time"

	"biya-oracle/internal/types"

	"github.com/stretchr/testify/assert"
)

// TestDecideDataSource_TradingMarketNormalStock 测试：市场交易时段 + 股票正常 → CEX报价
func TestDecideDataSource_TradingMarketNormalStock(t *testing.T) {
	symbolMappings := []SymbolMapping{
		{CEXSymbol: "AAPL", BaseSymbol: "AAPLX"},
	}
	manager := NewManager(nil, nil, symbolMappings, nil)

	testCases := []struct {
		name         string
		marketStatus types.MarketStatus
		stockStatus  types.StockStatus
		expectedDS   types.DataSource
		shouldPause  bool
	}{
		{
			name:         "盘前 + 正常",
			marketStatus: types.MarketStatusPreHourTrading,
			stockStatus:  types.StockStatusNormal,
			expectedDS:   types.DataSourceCEX,
			shouldPause:  false,
		},
		{
			name:         "盘中 + 正常",
			marketStatus: types.MarketStatusTrading,
			stockStatus:  types.StockStatusNormal,
			expectedDS:   types.DataSourceCEX,
			shouldPause:  false,
		},
		{
			name:         "盘后 + 正常",
			marketStatus: types.MarketStatusPostHourTrading,
			stockStatus:  types.StockStatusNormal,
			expectedDS:   types.DataSourceCEX,
			shouldPause:  false,
		},
		{
			name:         "夜盘 + 正常",
			marketStatus: types.MarketStatusOvernight,
			stockStatus:  types.StockStatusNormal,
			expectedDS:   types.DataSourceCEX,
			shouldPause:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ds, pause, reason := manager.decideDataSource(tc.marketStatus, tc.stockStatus)
			assert.Equal(t, tc.expectedDS, ds, "数据源应该正确")
			assert.Equal(t, tc.shouldPause, pause, "暂停标志应该正确")
			if !tc.shouldPause {
				assert.Empty(t, reason, "正常情况不应该有暂停原因")
			}
		})
	}
}

// TestDecideDataSource_TradingMarketCircuitBreaker 测试：市场交易时段 + 股票熔断 → 加权合成报价
func TestDecideDataSource_TradingMarketCircuitBreaker(t *testing.T) {
	symbolMappings := []SymbolMapping{
		{CEXSymbol: "AAPL", BaseSymbol: "AAPLX"},
	}
	manager := NewManager(nil, nil, symbolMappings, nil)

	testCases := []struct {
		name         string
		marketStatus types.MarketStatus
		expectedDS   types.DataSource
		shouldPause  bool
	}{
		{
			name:         "盘前 + 熔断",
			marketStatus: types.MarketStatusPreHourTrading,
			expectedDS:   types.DataSourceWeighted,
			shouldPause:  false,
		},
		{
			name:         "盘中 + 熔断",
			marketStatus: types.MarketStatusTrading,
			expectedDS:   types.DataSourceWeighted,
			shouldPause:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ds, pause, reason := manager.decideDataSource(tc.marketStatus, types.StockStatusCircuitBreaker)
			assert.Equal(t, tc.expectedDS, ds, "数据源应该正确")
			assert.Equal(t, tc.shouldPause, pause, "不应该暂停")
			assert.Empty(t, reason, "不应该有暂停原因")
		})
	}
}

// TestDecideDataSource_TradingMarketOtherStockStatus 测试：市场交易时段 + 股票其他状态 → 暂停报价
func TestDecideDataSource_TradingMarketOtherStockStatus(t *testing.T) {
	symbolMappings := []SymbolMapping{
		{CEXSymbol: "AAPL", BaseSymbol: "AAPLX"},
	}
	manager := NewManager(nil, nil, symbolMappings, nil)

	testCases := []struct {
		name         string
		marketStatus types.MarketStatus
		stockStatus  types.StockStatus
		expectedDS   types.DataSource
		shouldPause  bool
		reasonPrefix string
	}{
		{
			name:         "盘前 + 停牌",
			marketStatus: types.MarketStatusPreHourTrading,
			stockStatus:  types.StockStatusHalted,
			expectedDS:   types.DataSourceCEX,
			shouldPause:  true,
			reasonPrefix: "股票状态异常: halted",
		},
		{
			name:         "盘中 + 暂停交易",
			marketStatus: types.MarketStatusTrading,
			stockStatus:  types.StockStatusSuspended,
			expectedDS:   types.DataSourceCEX,
			shouldPause:  true,
			reasonPrefix: "股票状态异常: suspended",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ds, pause, reason := manager.decideDataSource(tc.marketStatus, tc.stockStatus)
			assert.Equal(t, tc.expectedDS, ds, "数据源应该正确")
			assert.True(t, pause, "应该暂停")
			assert.Contains(t, reason, tc.reasonPrefix, "暂停原因应该正确")
		})
	}
}

// TestDecideDataSource_NonTradingMarketNormalStock 测试：市场非交易时段 + 股票正常 → 加权合成报价
func TestDecideDataSource_NonTradingMarketNormalStock(t *testing.T) {
	symbolMappings := []SymbolMapping{
		{CEXSymbol: "AAPL", BaseSymbol: "AAPLX"},
	}
	manager := NewManager(nil, nil, symbolMappings, nil)

	testCases := []struct {
		name         string
		marketStatus types.MarketStatus
		expectedDS   types.DataSource
		shouldPause  bool
	}{
		{
			name:         "未开盘 + 正常",
			marketStatus: types.MarketStatusNotYetOpen,
			expectedDS:   types.DataSourceWeighted,
			shouldPause:  false,
		},
		{
			name:         "休市 + 正常",
			marketStatus: types.MarketStatusMarketClosed,
			expectedDS:   types.DataSourceWeighted,
			shouldPause:  false,
		},
		{
			name:         "收盘 + 正常",
			marketStatus: types.MarketStatusClosing,
			expectedDS:   types.DataSourceWeighted,
			shouldPause:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ds, pause, reason := manager.decideDataSource(tc.marketStatus, types.StockStatusNormal)
			assert.Equal(t, tc.expectedDS, ds, "数据源应该正确")
			assert.Equal(t, tc.shouldPause, pause, "不应该暂停")
			assert.Empty(t, reason, "不应该有暂停原因")
		})
	}
}

// TestDecideDataSource_NonTradingMarketCircuitBreaker 测试：市场非交易时段 + 股票熔断 → 加权合成报价
func TestDecideDataSource_NonTradingMarketCircuitBreaker(t *testing.T) {
	symbolMappings := []SymbolMapping{
		{CEXSymbol: "AAPL", BaseSymbol: "AAPLX"},
	}
	manager := NewManager(nil, nil, symbolMappings, nil)

	testCases := []struct {
		name         string
		marketStatus types.MarketStatus
		expectedDS   types.DataSource
		shouldPause  bool
	}{
		{
			name:         "未开盘 + 熔断",
			marketStatus: types.MarketStatusNotYetOpen,
			expectedDS:   types.DataSourceWeighted,
			shouldPause:  false,
		},
		{
			name:         "休市 + 熔断",
			marketStatus: types.MarketStatusMarketClosed,
			expectedDS:   types.DataSourceWeighted,
			shouldPause:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ds, pause, reason := manager.decideDataSource(tc.marketStatus, types.StockStatusCircuitBreaker)
			assert.Equal(t, tc.expectedDS, ds, "数据源应该正确")
			assert.Equal(t, tc.shouldPause, pause, "不应该暂停")
			assert.Empty(t, reason, "不应该有暂停原因")
		})
	}
}

// TestDecideDataSource_NonTradingMarketOtherStockStatus 测试：市场非交易时段 + 股票其他状态 → 暂停报价
func TestDecideDataSource_NonTradingMarketOtherStockStatus(t *testing.T) {
	symbolMappings := []SymbolMapping{
		{CEXSymbol: "AAPL", BaseSymbol: "AAPLX"},
	}
	manager := NewManager(nil, nil, symbolMappings, nil)

	testCases := []struct {
		name         string
		marketStatus types.MarketStatus
		stockStatus  types.StockStatus
		expectedDS   types.DataSource
		shouldPause  bool
		reasonPrefix string
	}{
		{
			name:         "未开盘 + 停牌",
			marketStatus: types.MarketStatusNotYetOpen,
			stockStatus:  types.StockStatusHalted,
			expectedDS:   types.DataSourceWeighted,
			shouldPause:  true,
			reasonPrefix: "股票状态异常: halted",
		},
		{
			name:         "休市 + 暂停交易",
			marketStatus: types.MarketStatusMarketClosed,
			stockStatus:  types.StockStatusSuspended,
			expectedDS:   types.DataSourceWeighted,
			shouldPause:  true,
			reasonPrefix: "股票状态异常: suspended",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ds, pause, reason := manager.decideDataSource(tc.marketStatus, tc.stockStatus)
			assert.Equal(t, tc.expectedDS, ds, "数据源应该正确")
			assert.True(t, pause, "应该暂停")
			assert.Contains(t, reason, tc.reasonPrefix, "暂停原因应该正确")
		})
	}
}

// TestUpdateStatus_DataSourceSwitch 测试数据源切换
func TestUpdateStatus_DataSourceSwitch(t *testing.T) {
	symbolMappings := []SymbolMapping{
		{CEXSymbol: "AAPL", BaseSymbol: "AAPLX"},
	}
	manager := NewManager(nil, nil, symbolMappings, nil)

	var switchCalled bool
	var oldDS, newDS types.DataSource
	var symbol string

	// 使用channel来同步回调
	callbackChan := make(chan struct{}, 1)

	manager.SetOnDataSourceChange(func(old, new types.DataSource, sym string) {
		switchCalled = true
		oldDS = old
		newDS = new
		symbol = sym
		callbackChan <- struct{}{}
	})

	// 初始状态：CEX
	stockStatuses := map[string]types.StockStatus{
		"AAPLX": types.StockStatusNormal,
	}
	manager.updateStatus(types.MarketStatusTrading, stockStatuses)
	assert.Equal(t, types.DataSourceCEX, manager.GetCurrentDataSource("AAPLX"), "初始应该为CEX")

	// 切换到加权合成
	manager.updateStatus(types.MarketStatusMarketClosed, stockStatuses)
	assert.Equal(t, types.DataSourceWeighted, manager.GetCurrentDataSource("AAPLX"), "应该切换为WEIGHTED")

	// 等待回调完成（最多等待100ms）
	select {
	case <-callbackChan:
		// 回调已执行
	case <-time.After(100 * time.Millisecond):
		t.Fatal("切换回调未在预期时间内执行")
	}

	assert.True(t, switchCalled, "应该触发切换回调")
	assert.Equal(t, types.DataSourceCEX, oldDS, "旧数据源应该正确")
	assert.Equal(t, types.DataSourceWeighted, newDS, "新数据源应该正确")
	assert.Equal(t, "AAPLX", symbol, "股票符号应该正确")
}

// TestUpdateStatus_PauseAndResume 测试暂停和恢复
func TestUpdateStatus_PauseAndResume(t *testing.T) {
	symbolMappings := []SymbolMapping{
		{CEXSymbol: "AAPL", BaseSymbol: "AAPLX"},
	}
	manager := NewManager(nil, nil, symbolMappings, nil)

	// 暂停报价
	stockStatuses := map[string]types.StockStatus{
		"AAPLX": types.StockStatusHalted,
	}
	manager.updateStatus(types.MarketStatusTrading, stockStatuses)
	assert.True(t, manager.IsPaused("AAPLX"), "应该被暂停")
	assert.Contains(t, manager.GetPauseReason("AAPLX"), "股票状态异常: halted", "暂停原因应该正确")

	// 恢复报价
	stockStatuses["AAPLX"] = types.StockStatusNormal
	manager.updateStatus(types.MarketStatusTrading, stockStatuses)
	assert.False(t, manager.IsPaused("AAPLX"), "应该已恢复")
}

// TestPauseStock 测试暂停报价
func TestPauseStock(t *testing.T) {
	symbolMappings := []SymbolMapping{
		{CEXSymbol: "AAPL", BaseSymbol: "AAPLX"},
	}
	manager := NewManager(nil, nil, symbolMappings, nil)

	manager.PauseStock("AAPLX", "测试暂停")

	assert.True(t, manager.IsPaused("AAPLX"), "股票应该被暂停")
	assert.Equal(t, "测试暂停", manager.GetPauseReason("AAPLX"), "暂停原因应该正确")
}

// TestResumeStock 测试恢复报价
func TestResumeStock(t *testing.T) {
	symbolMappings := []SymbolMapping{
		{CEXSymbol: "AAPL", BaseSymbol: "AAPLX"},
	}
	manager := NewManager(nil, nil, symbolMappings, nil)

	// 先暂停
	manager.PauseStock("AAPLX", "测试暂停")
	assert.True(t, manager.IsPaused("AAPLX"), "股票应该被暂停")

	// 恢复
	manager.ResumeStock("AAPLX")
	assert.False(t, manager.IsPaused("AAPLX"), "股票应该已恢复")
	assert.Empty(t, manager.GetPauseReason("AAPLX"), "暂停原因应该为空")
}

// TestIsPaused_NotPaused 测试查询未暂停的股票
func TestIsPaused_NotPaused(t *testing.T) {
	symbolMappings := []SymbolMapping{
		{CEXSymbol: "TSLA", BaseSymbol: "TSLAX"},
	}
	manager := NewManager(nil, nil, symbolMappings, nil)

	assert.False(t, manager.IsPaused("TSLAX"), "未暂停的股票应该返回false")
	assert.Empty(t, manager.GetPauseReason("TSLAX"), "未暂停的股票原因应该为空")
}

// TestGetCurrentDataSource 测试获取当前数据源
func TestGetCurrentDataSource(t *testing.T) {
	symbolMappings := []SymbolMapping{
		{CEXSymbol: "AAPL", BaseSymbol: "AAPLX"},
	}
	manager := NewManager(nil, nil, symbolMappings, nil)

	// 默认应该返回CEX（初始化时设置）
	ds := manager.GetCurrentDataSource("AAPLX")
	assert.Equal(t, types.DataSourceCEX, ds, "默认数据源应该为CEX")

	// 不存在的股票应该返回CEX（默认值）
	ds = manager.GetCurrentDataSource("UNKNOWN")
	assert.Equal(t, types.DataSourceCEX, ds, "未知股票应该返回默认值CEX")
}

// TestSetPythStatus 测试设置Pyth状态
func TestSetPythStatus(t *testing.T) {
	symbolMappings := []SymbolMapping{
		{CEXSymbol: "AAPL", BaseSymbol: "AAPLX"},
	}
	manager := NewManager(nil, nil, symbolMappings, nil)

	manager.SetPythStatus("AAPLX", types.OracleStatusAbnormal)
	assert.Equal(t, types.OracleStatusAbnormal, manager.GetPythStatus("AAPLX"), "Pyth状态应该正确")

	manager.SetPythStatus("AAPLX", types.OracleStatusNormal)
	assert.Equal(t, types.OracleStatusNormal, manager.GetPythStatus("AAPLX"), "Pyth状态应该恢复为正常")
}

// TestSetGateStatus 测试设置Gate状态
func TestSetGateStatus(t *testing.T) {
	symbolMappings := []SymbolMapping{
		{CEXSymbol: "AAPL", BaseSymbol: "AAPLX"},
	}
	manager := NewManager(nil, nil, symbolMappings, nil)

	manager.SetGateStatus("AAPLX", types.OracleStatusAbnormal)
	assert.Equal(t, types.OracleStatusAbnormal, manager.GetGateStatus("AAPLX"), "Gate状态应该正确")

	manager.SetGateStatus("AAPLX", types.OracleStatusNormal)
	assert.Equal(t, types.OracleStatusNormal, manager.GetGateStatus("AAPLX"), "Gate状态应该恢复为正常")
}

// TestRollbackDataSource 测试回滚数据源
func TestRollbackDataSource(t *testing.T) {
	symbolMappings := []SymbolMapping{
		{CEXSymbol: "AAPL", BaseSymbol: "AAPLX"},
	}
	manager := NewManager(nil, nil, symbolMappings, nil)

	// 设置数据源为WEIGHTED
	manager.RollbackDataSource("AAPLX", types.DataSourceWeighted)

	ds := manager.GetCurrentDataSource("AAPLX")
	assert.Equal(t, types.DataSourceWeighted, ds, "数据源应该被回滚")
}

// TestGetState 测试获取完整状态
func TestGetState(t *testing.T) {
	symbolMappings := []SymbolMapping{
		{CEXSymbol: "AAPL", BaseSymbol: "AAPLX"},
	}
	manager := NewManager(nil, nil, symbolMappings, nil)

	state := manager.GetState()
	assert.NotNil(t, state, "状态应该不为nil")
	assert.NotNil(t, state.DataSources, "数据源map应该不为nil")
	assert.NotNil(t, state.PausedStocks, "暂停股票map应该不为nil")
	assert.NotNil(t, state.PythStatuses, "Pyth状态map应该不为nil")
	assert.NotNil(t, state.GateStatuses, "Gate状态map应该不为nil")
}
