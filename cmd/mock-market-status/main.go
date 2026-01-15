package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
)

// MarketStatusData 市场状态数据（与真实API格式一致）
type MarketStatusData struct {
	Market        string `json:"market"`         // 市场: US/HK
	StatusCN      string `json:"status_cn"`      // 中文状态
	OpenTime      string `json:"open_time"`      // 开盘时间
	TradingStatus string `json:"trading_status"` // 交易状态: TRADING/CLOSING等
	Status        string `json:"status"`         // 状态: Trading/Closed
}

// MarketStatusResponse 市场状态接口响应（与真实API格式一致）
type MarketStatusResponse struct {
	Code   int                `json:"code"`
	Msg    string             `json:"msg"`
	Data   []MarketStatusData `json:"data"`
	Result bool               `json:"result"`
}

// MarketStatusServer 市场状态模拟服务器
type MarketStatusServer struct {
	mu            sync.RWMutex
	currentStatus MarketStatusData
	port          string
	stateFile     string
}

// NewMarketStatusServer 创建市场状态模拟服务器
func NewMarketStatusServer(port, stateFile string) *MarketStatusServer {
	server := &MarketStatusServer{
		port:      port,
		stateFile: stateFile,
		currentStatus: MarketStatusData{
			Market:        "US",
			StatusCN:      "交易中",
			OpenTime:      "09:30",
			TradingStatus: "TRADING", // 默认交易中
			Status:        "Trading",
		},
	}

	// 尝试从文件加载状态
	server.loadState()

	return server
}

// loadState 从文件加载状态
func (s *MarketStatusServer) loadState() {
	if s.stateFile == "" {
		return
	}

	data, err := os.ReadFile(s.stateFile)
	if err != nil {
		log.Printf("[模拟服务] 无法加载状态文件（将使用默认状态）: %v", err)
		return
	}

	var status MarketStatusData
	if err := json.Unmarshal(data, &status); err != nil {
		log.Printf("[模拟服务] 状态文件格式错误（将使用默认状态）: %v", err)
		return
	}

	s.mu.Lock()
	s.currentStatus = status
	s.mu.Unlock()

	log.Printf("[模拟服务] 已从文件加载状态: %s (%s)", status.TradingStatus, status.StatusCN)
}

// saveState 保存状态到文件
func (s *MarketStatusServer) saveState() {
	if s.stateFile == "" {
		return
	}

	s.mu.RLock()
	data, err := json.MarshalIndent(s.currentStatus, "", "  ")
	s.mu.RUnlock()

	if err != nil {
		log.Printf("[模拟服务] 无法序列化状态: %v", err)
		return
	}

	if err := os.WriteFile(s.stateFile, data, 0644); err != nil {
		log.Printf("[模拟服务] 无法保存状态文件: %v", err)
		return
	}

	log.Printf("[模拟服务] 状态已保存到文件: %s", s.stateFile)
}

// handleGetMarketStatus 处理GET请求：返回市场状态（与真实API格式一致）
func (s *MarketStatusServer) handleGetMarketStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	status := s.currentStatus
	s.mu.RUnlock()

	response := MarketStatusResponse{
		Code:   200,
		Msg:    "success",
		Data:   []MarketStatusData{status},
		Result: true,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)

	log.Printf("[模拟服务] GET /market/get -> %s (%s)", status.TradingStatus, status.StatusCN)
}

// handleSetMarketStatus 处理POST/PUT请求：设置市场状态
func (s *MarketStatusServer) handleSetMarketStatus(w http.ResponseWriter, r *http.Request) {
	var request struct {
		TradingStatus string `json:"trading_status"` // 交易状态: TRADING, CLOSING, MARKET_CLOSED等
		StatusCN      string `json:"status_cn"`      // 中文状态（可选）
		Status        string `json:"status"`         // 状态: Trading, Closed（可选）
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, fmt.Sprintf("无效的请求体: %v", err), http.StatusBadRequest)
		return
	}

	if request.TradingStatus == "" {
		http.Error(w, "trading_status 字段不能为空", http.StatusBadRequest)
		return
	}

	// 更新状态
	s.mu.Lock()
	oldStatus := s.currentStatus.TradingStatus
	s.currentStatus.TradingStatus = request.TradingStatus

	// 如果提供了中文状态，则使用；否则根据交易状态自动生成
	if request.StatusCN != "" {
		s.currentStatus.StatusCN = request.StatusCN
	} else {
		s.currentStatus.StatusCN = s.getStatusCN(request.TradingStatus)
	}

	// 如果提供了Status，则使用；否则根据交易状态自动生成
	if request.Status != "" {
		s.currentStatus.Status = request.Status
	} else {
		s.currentStatus.Status = s.getStatus(request.TradingStatus)
	}

	newStatus := s.currentStatus.TradingStatus
	s.mu.Unlock()

	// 保存到文件
	s.saveState()

	// 返回成功响应
	response := map[string]interface{}{
		"code":    200,
		"msg":     "success",
		"result":  true,
		"old":     oldStatus,
		"new":     newStatus,
		"message": fmt.Sprintf("市场状态已从 %s 更新为 %s", oldStatus, newStatus),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)

	log.Printf("[模拟服务] ✅ 市场状态已更新: %s -> %s (%s)", oldStatus, newStatus, s.currentStatus.StatusCN)
}

// getStatusCN 根据交易状态获取中文状态
func (s *MarketStatusServer) getStatusCN(tradingStatus string) string {
	switch tradingStatus {
	case "TRADING":
		return "交易中"
	case "PRE_HOUR_TRADING", "PREHOURTRADING", "PRE_MARKET", "PREMARKET":
		return "盘前交易"
	case "POST_HOUR_TRADING", "POSTHOURTRADING", "AFTER_HOURS", "AFTERHOURS":
		return "盘后交易"
	case "OVERNIGHT", "OVER_NIGHT":
		return "夜盘交易"
	case "CLOSING", "CLOSED", "MARKET_CLOSED", "MARKETCLOSED":
		return "市场休市"
	case "NOT_YET_OPEN", "NOTYETOPEN":
		return "尚未开盘"
	case "MIDDLE_CLOSE", "MIDDLECLOSE":
		return "午间休市"
	case "EARLY_CLOSED", "EARLYCLOSED":
		return "提前收盘"
	default:
		return "未知状态"
	}
}

// getStatus 根据交易状态获取状态
func (s *MarketStatusServer) getStatus(tradingStatus string) string {
	switch tradingStatus {
	case "TRADING", "PRE_HOUR_TRADING", "PREHOURTRADING", "PRE_MARKET", "PREMARKET",
		"POST_HOUR_TRADING", "POSTHOURTRADING", "AFTER_HOURS", "AFTERHOURS",
		"OVERNIGHT", "OVER_NIGHT":
		return "Trading"
	case "CLOSING", "CLOSED", "MARKET_CLOSED", "MARKETCLOSED",
		"NOT_YET_OPEN", "NOTYETOPEN", "MIDDLE_CLOSE", "MIDDLECLOSE",
		"EARLY_CLOSED", "EARLYCLOSED":
		return "Closed"
	default:
		return "Unknown"
	}
}

// handleGetCurrentStatus 处理GET请求：获取当前状态（用于查看）
func (s *MarketStatusServer) handleGetCurrentStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	status := s.currentStatus
	s.mu.RUnlock()

	response := map[string]interface{}{
		"code":    200,
		"msg":     "success",
		"result":  true,
		"current": status,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Start 启动服务器
func (s *MarketStatusServer) Start() error {
	// 注册路由
	http.HandleFunc("/market/get", s.handleGetMarketStatus) // 与真实API路径一致
	http.HandleFunc("/status", s.handleGetCurrentStatus)    // 查看当前状态
	http.HandleFunc("/status/set", s.handleSetMarketStatus) // 设置状态（POST/PUT）
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// 简单的帮助页面
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>市场状态模拟服务</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; }
        .endpoint { background: #f5f5f5; padding: 15px; margin: 10px 0; border-radius: 5px; }
        .method { color: #fff; padding: 3px 8px; border-radius: 3px; font-weight: bold; }
        .get { background: #4CAF50; }
        .post { background: #2196F3; }
        code { background: #f0f0f0; padding: 2px 6px; border-radius: 3px; }
        pre { background: #f5f5f5; padding: 10px; border-radius: 5px; overflow-x: auto; }
    </style>
</head>
<body>
    <h1>市场状态模拟服务</h1>
    <p>这是一个用于测试的模拟服务，可以控制市场状态以触发数据源切换。</p>
    
    <h2>API端点</h2>
    
    <div class="endpoint">
        <span class="method get">GET</span> <code>/market/get</code>
        <p>返回市场状态（与真实API格式一致，程序使用此接口）</p>
    </div>
    
    <div class="endpoint">
        <span class="method get">GET</span> <code>/status</code>
        <p>查看当前市场状态</p>
    </div>
    
    <div class="endpoint">
        <span class="method post">POST</span> <code>/status/set</code>
        <p>设置市场状态（用于触发数据源切换）</p>
        <p>请求体示例：</p>
        <pre>{
  "trading_status": "MARKET_CLOSED",
  "status_cn": "市场休市"
}</pre>
    </div>
    
    <h2>常用状态值</h2>
    <ul>
        <li><code>TRADING</code> - 交易中（使用CEX数据源）</li>
        <li><code>MARKET_CLOSED</code> - 市场休市（使用加权合成数据源）</li>
        <li><code>OVERNIGHT</code> - 夜盘交易（使用CEX数据源）</li>
        <li><code>PRE_HOUR_TRADING</code> - 盘前交易（使用加权合成数据源）</li>
        <li><code>POST_HOUR_TRADING</code> - 盘后交易（使用加权合成数据源）</li>
    </ul>
    
    <h2>快速测试</h2>
    <p>使用curl命令设置市场状态：</p>
    <pre># 设置为休市（触发切换到加权合成）
curl -X POST http://localhost:%s/status/set \\
  -H "Content-Type: application/json" \\
  -d '{"trading_status": "MARKET_CLOSED"}'

# 设置为交易中（触发切换到CEX）
curl -X POST http://localhost:%s/status/set \\
  -H "Content-Type: application/json" \\
  -d '{"trading_status": "TRADING"}'
</pre>
</body>
</html>
`, s.port, s.port)
	})

	addr := fmt.Sprintf(":%s", s.port)
	log.Printf("[模拟服务] 🚀 市场状态模拟服务已启动")
	log.Printf("[模拟服务] 📍 监听地址: http://localhost%s", addr)
	log.Printf("[模拟服务] 📖 帮助页面: http://localhost%s/", addr)
	log.Printf("[模拟服务] 🔍 查看状态: http://localhost%s/status", addr)
	log.Printf("[模拟服务] ⚙️  设置状态: http://localhost%s/status/set", addr)
	log.Printf("[模拟服务] 📡 程序接口: http://localhost%s/market/get", addr)

	s.mu.RLock()
	currentStatus := s.currentStatus.TradingStatus
	s.mu.RUnlock()
	log.Printf("[模拟服务] 📊 当前市场状态: %s", currentStatus)

	return http.ListenAndServe(addr, nil)
}

func main() {
	port := flag.String("port", "8888", "服务器端口")
	stateFile := flag.String("state-file", "", "状态文件路径（可选，用于持久化状态）")
	flag.Parse()

	server := NewMarketStatusServer(*port, *stateFile)
	if err := server.Start(); err != nil {
		log.Fatalf("启动服务器失败: %v", err)
	}
}
