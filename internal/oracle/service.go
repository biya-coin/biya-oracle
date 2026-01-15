// Package oracle 核心报价服务
package oracle

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"biya-oracle/internal/alert"
	"biya-oracle/internal/config"
	"biya-oracle/internal/datasource/cex"
	"biya-oracle/internal/datasource/gate"
	"biya-oracle/internal/datasource/pyth"
	"biya-oracle/internal/price"
	"biya-oracle/internal/pusher"
	"biya-oracle/internal/state"
	"biya-oracle/internal/storage"
	"biya-oracle/internal/throttle"
	"biya-oracle/internal/types"
)

// Service 报价服务
type Service struct {
	cfg           *config.Config
	storage       *storage.RedisStorage
	alertManager  *alert.LarkAlert
	stateManager  *state.Manager
	calculator    *price.Calculator
	validator     *price.Validator
	onChainPusher *pusher.OnChainPusher
	throttler     *throttle.PriceThrottler // 价格瘦身器

	// 数据源客户端
	cexClient  *cex.Client
	cexStatus  *cex.StatusClient
	pythClient *pyth.Client
	gateClient *gate.Client

	// 状态
	mu                 sync.RWMutex
	isReady            bool                         // 系统是否准备就绪（所有数据源连接成功后才为true）
	lastPrices         map[string]float64           // 每个股票代币的最后推送价格（瘦身后的）
	cexQuotes          map[string]*cex.QuoteMessage // 最新CEX报价缓存
	pythPrices         map[string]float64           // Pyth价格缓存
	gatePrices         map[string]float64           // Gate价格缓存
	closePrices        map[string]float64           // 收盘价缓存
	bothOracleAlerted  map[string]bool              // 记录是否已发送"两个预言机都异常"告警（去重）
	pythJumpAlerted    map[string]bool              // 记录Pyth是否已降权（日志去重）
	gateJumpAlerted    map[string]bool              // 记录Gate是否已降权（日志去重）
	closePriceAbnormal map[string]bool              // 记录收盘价是否异常（日志去重）

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewService 创建报价服务
func NewService(cfg *config.Config, storage *storage.RedisStorage) *Service {
	alertManager := alert.NewLarkAlert(cfg.Lark.WebhookURL)
	cexStatusClient := cex.NewStatusClient(cfg.CEX.MarketStatusURL, cfg.CEX.StockStatusURL)

	// 构建符号映射（CEXSymbol <-> BaseSymbol）
	var symbolMappings []state.SymbolMapping
	for _, token := range cfg.StockTokens {
		symbolMappings = append(symbolMappings, state.SymbolMapping{
			CEXSymbol:  token.CEXSymbol,
			BaseSymbol: token.BaseSymbol,
		})
	}

	// 创建价格瘦身器
	priceThrottler := throttle.NewPriceThrottler(
		cfg.Throttle.MinPushInterval,
		cfg.Throttle.MaxPushInterval,
		cfg.Throttle.PriceChangeThreshold,
	)

	// 创建链上推送器
	onChainPusher, err := pusher.NewOnChainPusher(cfg.OnChain)
	if err != nil {
		log.Printf("[服务] ⚠️ 链上推送器初始化失败: %v，将使用仅打印模式", err)
		// 创建一个禁用的推送器作为 fallback
		fallbackCfg := cfg.OnChain
		fallbackCfg.Enabled = false
		onChainPusher, _ = pusher.NewOnChainPusher(fallbackCfg)
	}

	svc := &Service{
		cfg:                cfg,
		storage:            storage,
		alertManager:       alertManager,
		calculator:         price.NewCalculator(),
		validator:          price.NewValidator(),
		onChainPusher:      onChainPusher,
		throttler:          priceThrottler,
		cexStatus:          cexStatusClient,
		lastPrices:         make(map[string]float64),
		cexQuotes:          make(map[string]*cex.QuoteMessage),
		pythPrices:         make(map[string]float64),
		gatePrices:         make(map[string]float64),
		closePrices:        make(map[string]float64),
		bothOracleAlerted:  make(map[string]bool),
		pythJumpAlerted:    make(map[string]bool),
		gateJumpAlerted:    make(map[string]bool),
		closePriceAbnormal: make(map[string]bool),
	}

	// 创建状态管理器（传入符号映射和致命错误回调）
	svc.stateManager = state.NewManager(cexStatusClient, alertManager, symbolMappings, svc.handleFatalError)

	return svc
}

// handleFatalError 处理致命错误（发送告警并终止程序）
func (s *Service) handleFatalError(source string, err error) {
	log.Printf("🔴 [致命错误] 来源: %s, 错误: %v", source, err)

	// 发送告警
	s.alertManager.SendAlert(types.AlertTypeReconnectFailed, "", map[string]string{
		"错误来源": source,
		"错误详情": err.Error(),
		"说明":   "致命错误，程序即将终止",
	})

	// 等待告警发送完成
	time.Sleep(2 * time.Second)

	// 终止程序
	log.Fatal("程序因致命错误终止")
}

// Start 启动服务
func (s *Service) Start(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)

	log.Println("========================================")
	log.Println("  股票代币报价系统 启动中...")
	log.Println("========================================")

	// 设置数据源切换回调
	s.stateManager.SetOnDataSourceChange(s.onDataSourceChange)

	// 第一步：初始化并启动数据源客户端
	log.Println("[启动] 第1步: 连接数据源...")
	if err := s.initDataSources(); err != nil {
		return err
	}

	// 第二步：等待所有数据源连接就绪
	log.Println("[启动] 第2步: 等待数据源就绪...")
	s.waitForDataSourcesReady()

	// 第三步：启动状态管理（开始轮询市场状态）
	log.Println("[启动] 第3步: 启动状态管理...")
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.stateManager.Start(s.ctx)
	}()

	// 等待首次状态查询完成
	time.Sleep(500 * time.Millisecond)

	// 第四步：清空预热期间收到的数据，设置系统就绪
	log.Println("[启动] 第4步: 清空预热数据，系统就绪...")
	s.clearPreheatData()
	s.mu.Lock()
	s.isReady = true
	s.mu.Unlock()

	log.Println("========================================")
	log.Println("  系统启动完成，开始接收并推送价格")
	log.Printf("  瘦身配置: 最小间隔=%ds, 最大间隔=%ds, 变动阈值=%.2f%%",
		s.cfg.Throttle.MinPushInterval,
		s.cfg.Throttle.MaxPushInterval,
		s.cfg.Throttle.PriceChangeThreshold*100)
	log.Println("========================================")

	return nil
}

// Stop 停止服务
func (s *Service) Stop() {
	log.Println("[服务] 正在停止...")

	s.cancel()

	// 停止数据源
	if s.cexClient != nil {
		s.cexClient.Stop()
	}
	if s.pythClient != nil {
		s.pythClient.Stop()
	}
	if s.gateClient != nil {
		s.gateClient.Stop()
	}

	s.stateManager.Stop()

	// 关闭链上推送器连接
	if s.onChainPusher != nil {
		if err := s.onChainPusher.Close(); err != nil {
			log.Printf("[服务] 关闭链上推送器失败: %v", err)
		}
	}

	s.wg.Wait()

	log.Println("[服务] 已停止")
}

// waitForDataSourcesReady 等待数据源连接就绪
func (s *Service) waitForDataSourcesReady() {
	maxWait := 30 * time.Second
	checkInterval := 500 * time.Millisecond
	start := time.Now()

	for time.Since(start) < maxWait {
		cexReady := s.cexClient != nil && s.cexClient.IsConnected()
		pythReady := s.pythClient != nil && s.pythClient.IsConnected()
		gateReady := s.gateClient != nil && s.gateClient.IsConnected()

		if cexReady && pythReady && gateReady {
			log.Println("[启动] 所有数据源已连接就绪")
			return
		}

		time.Sleep(checkInterval)
	}

	// 超时后打印警告，但不阻止启动
	log.Println("[启动] 警告: 部分数据源可能未连接，继续启动...")
}

// clearPreheatData 清空预热期间收到的数据
func (s *Service) clearPreheatData() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 清空所有缓存数据
	s.cexQuotes = make(map[string]*cex.QuoteMessage)
	s.pythPrices = make(map[string]float64)
	s.gatePrices = make(map[string]float64)
	s.lastPrices = make(map[string]float64)
	s.bothOracleAlerted = make(map[string]bool)
	s.pythJumpAlerted = make(map[string]bool)
	s.gateJumpAlerted = make(map[string]bool)
	// closePrices 保留，因为它是从Redis加载的

	log.Println("[启动] 预热数据已清空")
}

// initDataSources 初始化数据源
func (s *Service) initDataSources() error {
	// 初始化CEX客户端
	var cexSymbols []string
	for _, token := range s.cfg.StockTokens {
		cexSymbols = append(cexSymbols, token.CEXSymbol)
	}

	s.cexClient = cex.NewClient(s.cfg.CEX.WSBaseURL, cexSymbols, s.handleCEXQuote, s.handleFatalError)
	if err := s.cexClient.Start(); err != nil {
		log.Printf("[服务] CEX客户端启动失败: %v", err)
		// CEX失败不阻止启动，会使用加权合成
	}

	// 初始化Pyth客户端
	var pythMappings []pyth.FeedIDMapping
	for _, token := range s.cfg.StockTokens {
		pythMappings = append(pythMappings, pyth.FeedIDMapping{
			FeedID: token.PythFeedID,
			Symbol: token.BaseSymbol,
		})
	}

	s.pythClient = pyth.NewClient(s.cfg.Pyth.SSEURL, pythMappings, s.handlePythPrice, s.handleFatalError)
	if err := s.pythClient.Start(); err != nil {
		log.Printf("[服务] Pyth客户端启动失败: %v", err)
	}

	// 初始化Gate客户端
	var gateMappings []gate.PairMapping
	for _, token := range s.cfg.StockTokens {
		gateMappings = append(gateMappings, gate.PairMapping{
			Pair:   token.GatePair,
			Symbol: token.BaseSymbol,
		})
	}

	s.gateClient = gate.NewClient(s.cfg.Gate.WSURL, gateMappings, s.handleGatePrice, s.handleFatalError)
	if err := s.gateClient.Start(); err != nil {
		log.Printf("[服务] Gate客户端启动失败: %v", err)
	}

	return nil
}

// handleCEXQuote 处理CEX报价
func (s *Service) handleCEXQuote(quote *cex.QuoteMessage) {
	// 检查系统是否准备就绪
	s.mu.RLock()
	ready := s.isReady
	s.mu.RUnlock()
	if !ready {
		return // 系统未就绪，丢弃数据
	}

	s.mu.Lock()
	// 缓存CEX报价
	s.cexQuotes[quote.Symbol] = quote
	s.mu.Unlock()

	// 查找对应的股票代币配置
	tokenCfg := s.cfg.GetStockTokenBySymbol(quote.Symbol)
	if tokenCfg == nil {
		return
	}
	symbol := tokenCfg.BaseSymbol

	// 获取该股票的当前数据源
	dataSource := s.stateManager.GetCurrentDataSource(symbol)

	// 如果该股票当前使用CEX数据源
	if dataSource == types.DataSourceCEX {
		currentPrice := quote.GetCurrentPrice()

		// 价格校验
		if err := s.validator.ValidatePrice(currentPrice); err != nil {
			log.Printf("[CEX] %s 价格无效: %v", symbol, err)
			s.alertManager.SendAlert(types.AlertTypePriceInvalid, symbol, map[string]string{
				"价格": formatFloat(currentPrice),
				"原因": err.Error(),
			})
			return
		}

		// 时间戳校验
		timestamp := time.UnixMilli(quote.Timestamp)
		if err := s.validator.ValidateTimestamp(timestamp); err != nil {
			log.Printf("[CEX] %s 时间戳过期: %v", symbol, err)
			s.alertManager.SendAlert(types.AlertTypeTimestampExpired, symbol, map[string]string{
				"时间戳": timestamp.Format("2006-01-02 15:04:05"),
				"原因":  err.Error(),
			})
			return
		}

		// 格式化价格
		formattedPrice := price.FormatPrice(currentPrice)

		// 推送价格（来源: CEX + 市场状态）
		// 注：正常价格推送不检查5%阈值，5%阈值只在数据源切换时检查
		sourceInfo := types.FormatCEXSourceInfo(quote.MarketStatus)
		s.pushPrice(symbol, formattedPrice, sourceInfo)
	}

	// 注：CEX报价只在内存中缓存（s.cexQuotes），不频繁写Redis
	// 只有在切换数据源时才会将收盘价写入Redis（见 captureClosePrice）
}

// handlePythPrice 处理Pyth价格
func (s *Service) handlePythPrice(priceData *pyth.PriceData) {
	// 检查系统是否准备就绪
	s.mu.RLock()
	ready := s.isReady
	s.mu.RUnlock()
	if !ready {
		return // 系统未就绪，丢弃数据
	}

	symbol := priceData.Symbol
	currentPrice := priceData.Price

	// 价格校验
	if err := s.validator.ValidatePrice(currentPrice); err != nil {
		log.Printf("[Pyth] %s 价格无效: %v", symbol, err)
		s.alertManager.SendAlert(types.AlertTypePriceInvalid, symbol, map[string]string{
			"数据源": "Pyth",
			"价格":  formatFloat(currentPrice),
			"原因":  err.Error(),
		})
		return
	}

	// 时间戳校验
	if err := s.validator.ValidateTimestamp(priceData.Timestamp); err != nil {
		log.Printf("[Pyth] %s 时间戳过期: %v", symbol, err)
		s.alertManager.SendAlert(types.AlertTypeTimestampExpired, symbol, map[string]string{
			"数据源": "Pyth",
			"时间戳": priceData.Timestamp.Format("2006-01-02 15:04:05"),
			"原因":  err.Error(),
		})
		return
	}

	// 检查价格跳变（只在内存中进行，不用Redis）
	// pythPrices 存储的是上一次正常价格，用于跳变检测和加权计算
	s.mu.RLock()
	lastValidPrice := s.pythPrices[symbol]
	s.mu.RUnlock()

	if lastValidPrice > 0 {
		// 有上一次正常价格，检查跳变
		jump, isAbnormal := s.calculator.CheckPriceJump(lastValidPrice, currentPrice)
		if isAbnormal {
			// 跳变 >= 10%，降权，不更新缓存
			s.stateManager.SetPythStatus(symbol, types.OracleStatusAbnormal)

			// 只在第一次降权时记录日志（连续异常不重复记录）
			s.mu.RLock()
			alreadyDegraded := s.pythJumpAlerted[symbol]
			s.mu.RUnlock()
			if !alreadyDegraded {
				log.Printf("[Pyth] %s 价格跳变已降权: 降权前价格=%.4f, 当前价格=%.4f, 跳变幅度=%.2f%%",
					symbol, lastValidPrice, currentPrice, jump*100)
				s.mu.Lock()
				s.pythJumpAlerted[symbol] = true
				s.mu.Unlock()
			}
			// 注：降权后继续监听，等恢复到10%以内
		} else {
			// 跳变 < 10%，正常/恢复权重，更新缓存
			s.stateManager.SetPythStatus(symbol, types.OracleStatusNormal)
			s.mu.Lock()
			s.pythPrices[symbol] = currentPrice
			// 重置降权状态
			if s.pythJumpAlerted[symbol] {
				delete(s.pythJumpAlerted, symbol)
				log.Printf("[Pyth] %s 价格已恢复正常 (当前价格=%.4f)", symbol, currentPrice)
			}
			s.mu.Unlock()
		}
	} else {
		// 没有上一次价格（程序刚启动），直接存储，状态正常
		s.stateManager.SetPythStatus(symbol, types.OracleStatusNormal)
		s.mu.Lock()
		s.pythPrices[symbol] = currentPrice
		s.mu.Unlock()
	}

	// 如果该股票当前使用加权合成
	if s.stateManager.GetCurrentDataSource(symbol) == types.DataSourceWeighted {
		s.calculateAndPushWeightedPrice(symbol)
	}
}

// handleGatePrice 处理Gate价格
func (s *Service) handleGatePrice(priceData *gate.PriceData) {
	// 检查系统是否准备就绪
	s.mu.RLock()
	ready := s.isReady
	s.mu.RUnlock()
	if !ready {
		return // 系统未就绪，丢弃数据
	}

	symbol := priceData.Symbol
	currentPrice := priceData.Price

	// 价格校验
	if err := s.validator.ValidatePrice(currentPrice); err != nil {
		log.Printf("[Gate] %s 价格无效: %v", symbol, err)
		s.alertManager.SendAlert(types.AlertTypePriceInvalid, symbol, map[string]string{
			"数据源": "Gate",
			"价格":  formatFloat(currentPrice),
			"原因":  err.Error(),
		})
		return
	}

	// 时间戳校验
	if err := s.validator.ValidateTimestamp(priceData.Timestamp); err != nil {
		log.Printf("[Gate] %s 时间戳过期: %v", symbol, err)
		s.alertManager.SendAlert(types.AlertTypeTimestampExpired, symbol, map[string]string{
			"数据源": "Gate",
			"时间戳": priceData.Timestamp.Format("2006-01-02 15:04:05"),
			"原因":  err.Error(),
		})
		return
	}

	// 检查价格跳变（只在内存中进行，不用Redis）
	// gatePrices 存储的是上一次正常价格，用于跳变检测和加权计算
	s.mu.RLock()
	lastValidPrice := s.gatePrices[symbol]
	s.mu.RUnlock()

	if lastValidPrice > 0 {
		// 有上一次正常价格，检查跳变
		jump, isAbnormal := s.calculator.CheckPriceJump(lastValidPrice, currentPrice)
		if isAbnormal {
			// 跳变 >= 10%，降权，不更新缓存
			s.stateManager.SetGateStatus(symbol, types.OracleStatusAbnormal)

			// 只在第一次降权时记录日志（连续异常不重复记录）
			s.mu.RLock()
			alreadyDegraded := s.gateJumpAlerted[symbol]
			s.mu.RUnlock()
			if !alreadyDegraded {
				log.Printf("[Gate] %s 价格跳变已降权: 降权前价格=%.4f, 当前价格=%.4f, 跳变幅度=%.2f%%",
					symbol, lastValidPrice, currentPrice, jump*100)
				s.mu.Lock()
				s.gateJumpAlerted[symbol] = true
				s.mu.Unlock()
			}
			// 注：降权后继续监听，等恢复到10%以内
		} else {
			// 跳变 < 10%，正常/恢复权重，更新缓存
			s.stateManager.SetGateStatus(symbol, types.OracleStatusNormal)
			s.mu.Lock()
			s.gatePrices[symbol] = currentPrice
			// 重置降权状态
			if s.gateJumpAlerted[symbol] {
				delete(s.gateJumpAlerted, symbol)
				log.Printf("[Gate] %s 价格已恢复正常 (当前价格=%.4f)", symbol, currentPrice)
			}
			s.mu.Unlock()
		}
	} else {
		// 没有上一次价格（程序刚启动），直接存储，状态正常
		s.stateManager.SetGateStatus(symbol, types.OracleStatusNormal)
		s.mu.Lock()
		s.gatePrices[symbol] = currentPrice
		s.mu.Unlock()
	}

	// 如果该股票当前使用加权合成
	if s.stateManager.GetCurrentDataSource(symbol) == types.DataSourceWeighted {
		s.calculateAndPushWeightedPrice(symbol)
	}
}

// onDataSourceChange 数据源切换回调（针对单个股票）
func (s *Service) onDataSourceChange(oldSource, newSource types.DataSource, symbol string) {
	log.Printf("[服务] %s 数据源切换: %s -> %s", symbol, oldSource, newSource)

	// 如果切换到加权合成，需要获取该股票的收盘价
	if newSource == types.DataSourceWeighted {
		s.captureClosePrice(symbol)
	}

	// 获取对应的股票代币配置
	tokenCfg := s.cfg.GetStockTokenBySymbol(symbol)
	if tokenCfg == nil {
		log.Printf("[服务] 未找到股票配置: %s", symbol)
		return
	}

	// 获取上次推送价格
	s.mu.RLock()
	lastPrice := s.lastPrices[symbol]
	s.mu.RUnlock()

	var newPrice float64
	var sourceInfo string

	if newSource == types.DataSourceCEX {
		// 从缓存获取CEX价格
		s.mu.RLock()
		quote := s.cexQuotes[tokenCfg.CEXSymbol]
		s.mu.RUnlock()

		if quote == nil {
			log.Printf("[服务] %s 没有CEX报价缓存", symbol)
			return
		}
		newPrice = price.FormatPrice(quote.GetCurrentPrice())
		sourceInfo = types.FormatCEXSourceInfo(quote.MarketStatus)
	} else {
		// 计算加权价格
		weighted := s.calculateWeightedPriceOnly(symbol)
		if weighted == nil || weighted.Price <= 0 {
			log.Printf("[服务] %s 加权价格计算失败", symbol)
			return
		}
		newPrice = weighted.Price
		sourceInfo = weighted.SourceInfo()
	}

	// 数据源切换时的5%阈值检测
	if lastPrice > 0 {
		diff, ok, err := s.validator.ValidateSwitchDiff(lastPrice, newPrice)
		if !ok {
			log.Printf("[服务] %s 数据源切换后价格差异过大: %.2f%%，暂停报价并回滚数据源", symbol, diff*100)
			// 回滚数据源到原来的状态，这样下次轮询会再次尝试切换
			s.stateManager.RollbackDataSource(symbol, oldSource)
			// 将该股票标记为暂停状态
			s.stateManager.PauseStock(symbol, fmt.Sprintf("数据源切换价格差异过大: %.2f%%", diff*100))
			s.alertManager.SendAlert(types.AlertTypeSwitchDiff, symbol, map[string]string{
				"触发场景": fmt.Sprintf("数据源切换 %s -> %s", oldSource, newSource),
				"上次价格": formatFloat(lastPrice),
				"当前价格": formatFloat(newPrice),
				"差异":   formatFloat(diff*100) + "%",
				"原因":   err.Error(),
			})
			return
		}
	}

	// 通过5%检测，强制立即推送（数据源切换时忽略瘦身限制）
	s.tryPushPrice(symbol, newPrice, sourceInfo, true)
}

// captureClosePrice 捕获单个股票的收盘价（从CEX切换到加权时）
// 此时才将内存中的CEX报价写入Redis，作为收盘价
func (s *Service) captureClosePrice(symbol string) {
	ctx := context.Background()

	// 获取对应的股票代币配置
	tokenCfg := s.cfg.GetStockTokenBySymbol(symbol)
	if tokenCfg == nil {
		log.Printf("[服务] 未找到股票配置: %s", symbol)
		return
	}

	// 从内存获取最后一条CEX报价（平时只在内存中缓存，不写Redis）
	s.mu.RLock()
	quote := s.cexQuotes[tokenCfg.CEXSymbol]
	s.mu.RUnlock()

	if quote == nil {
		log.Printf("[服务] %s 没有CEX报价缓存", tokenCfg.CEXSymbol)
		return
	}

	// 根据marketStatus获取收盘价
	var closePrice float64
	switch quote.MarketStatus {
	case "PreMarket", "AfterHours":
		closePrice = float64(quote.BidPrice)
	case "Regular":
		closePrice = float64(quote.LatestPrice)
	case "OverNight":
		closePrice = float64(quote.BluePrice)
	default:
		closePrice = float64(quote.LatestPrice)
	}

	// 切换数据源时才将收盘价写入Redis（用于程序重启后恢复）
	if err := s.storage.SetClosePrice(ctx, symbol, closePrice, time.UnixMilli(quote.Timestamp)); err != nil {
		log.Printf("[服务] 存储收盘价到Redis失败: %v", err)
	}

	// 同时更新内存缓存
	s.mu.Lock()
	s.closePrices[symbol] = closePrice
	s.mu.Unlock()

	log.Printf("[服务] %s 收盘价已捕获并存入Redis: %.4f (marketStatus=%s)", symbol, closePrice, quote.MarketStatus)
}

// calculateWeightedPriceOnly 仅计算加权价格（不推送）
// 用于数据源切换时的价格计算
func (s *Service) calculateWeightedPriceOnly(symbol string) *types.WeightedPrice {
	s.mu.RLock()
	closePrice := s.closePrices[symbol]
	pythPrice := s.pythPrices[symbol]
	gatePrice := s.gatePrices[symbol]
	s.mu.RUnlock()

	// 如果没有收盘价，尝试从Redis获取
	if closePrice == 0 {
		ctx := context.Background()
		storedPrice, err := s.storage.GetLatestClosePrice(ctx, symbol)
		if err == nil && storedPrice != nil {
			closePrice = storedPrice.Price
			s.mu.Lock()
			s.closePrices[symbol] = closePrice
			s.mu.Unlock()
		}
	}

	// 获取该股票的预言机状态（按股票独立）
	pythStatus := s.stateManager.GetPythStatus(symbol)
	gateStatus := s.stateManager.GetGateStatus(symbol)

	// 检查两个预言机是否都异常
	if s.calculator.IsBothOracleAbnormal(pythStatus, gateStatus) {
		return nil
	}

	// 计算加权价格
	return s.calculator.CalculateWeightedPrice(symbol, closePrice, pythPrice, gatePrice, pythStatus, gateStatus)
}

// calculateAndPushWeightedPrice 计算并推送加权价格
func (s *Service) calculateAndPushWeightedPrice(symbol string) {
	// 获取该股票的预言机状态（按股票独立）
	pythStatus := s.stateManager.GetPythStatus(symbol)
	gateStatus := s.stateManager.GetGateStatus(symbol)

	// 检查两个预言机是否都异常
	if s.calculator.IsBothOracleAbnormal(pythStatus, gateStatus) {
		// 检查是否已经发送过告警（去重）
		s.mu.RLock()
		alreadyAlerted := s.bothOracleAlerted[symbol]
		s.mu.RUnlock()

		if !alreadyAlerted {
			log.Printf("[服务] %s 两个预言机都异常，暂停报价", symbol)
			// 正式将该股票标记为暂停状态
			s.stateManager.PauseStock(symbol, "两个预言机都异常")
			s.alertManager.SendAlert(types.AlertTypeBothOracleError, symbol, map[string]string{
				"Pyth状态": string(pythStatus),
				"Gate状态": string(gateStatus),
				"说明":     "两个预言机都异常，已暂停报价",
			})
			// 标记已告警
			s.mu.Lock()
			s.bothOracleAlerted[symbol] = true
			s.mu.Unlock()
		}
		return
	}

	// 预言机恢复正常，重置告警状态并恢复报价
	s.mu.Lock()
	if s.bothOracleAlerted[symbol] {
		delete(s.bothOracleAlerted, symbol)
		s.mu.Unlock()
		// 恢复该股票的报价
		s.stateManager.ResumeStock(symbol)
		log.Printf("[服务] %s 预言机已恢复，恢复报价", symbol)
	} else {
		s.mu.Unlock()
	}

	// 计算加权价格
	weighted := s.calculateWeightedPriceOnly(symbol)
	if weighted == nil {
		return
	}

	if weighted.Price <= 0 {
		log.Printf("[服务] %s 加权价格计算失败: %s", symbol, weighted.DegradeReason)
		return
	}

	// 处理收盘价异常的日志去重（Pyth/Gate的降权日志已在各自handler中处理）
	if weighted.IsDegraded && weighted.DegradeReason == "CEX收盘价异常" {
		s.mu.RLock()
		alreadyLogged := s.closePriceAbnormal[symbol]
		s.mu.RUnlock()
		if !alreadyLogged {
			log.Printf("[服务] %s 收盘价异常，使用降级权重: Pyth50%%+Gate50%%", symbol)
			s.mu.Lock()
			s.closePriceAbnormal[symbol] = true
			s.mu.Unlock()
		}
	} else if !weighted.IsDegraded || weighted.DegradeReason != "CEX收盘价异常" {
		// 收盘价恢复正常（或其他原因导致的降级），重置状态
		s.mu.Lock()
		if s.closePriceAbnormal[symbol] {
			delete(s.closePriceAbnormal, symbol)
			s.mu.Unlock()
			log.Printf("[服务] %s 收盘价已恢复正常", symbol)
		} else {
			s.mu.Unlock()
		}
	}

	// 推送价格（来源: 加权合成）
	// 注：正常价格推送不检查5%阈值，5%阈值只在数据源切换时检查
	s.pushPrice(symbol, weighted.Price, weighted.SourceInfo())
}

// tryPushPrice 尝试推送价格（经过瘦身器判断）
// forceImmediate: 是否强制立即推送（如数据源切换时）
func (s *Service) tryPushPrice(symbol string, priceValue float64, sourceInfo string, forceImmediate bool) {
	// 检查该股票是否被暂停报价
	if s.stateManager.IsPaused(symbol) {
		// 股票被暂停，不推送
		return
	}

	// 通过瘦身器判断是否应该推送
	decision := s.throttler.ShouldPush(symbol, priceValue, sourceInfo, forceImmediate)

	if !decision.ShouldPush {
		// 不推送，跳过
		return
	}

	// 执行实际推送
	s.doPushPrice(symbol, priceValue, sourceInfo)

	// 确认推送完成，更新瘦身器状态
	s.throttler.ConfirmPush(symbol, priceValue)
}

// doPushPrice 实际执行价格推送
// sourceInfo: 数据来源信息，如 "CEX（盘中）" 或 "加权合成（CEX40%+Pyth30%+Gate30%）"
func (s *Service) doPushPrice(symbol string, priceValue float64, sourceInfo string) {
	// 更新最后价格（瘦身后的有效价格）
	s.mu.Lock()
	s.lastPrices[symbol] = priceValue
	s.mu.Unlock()

	// 推送到链上
	if err := s.onChainPusher.PushPrice(symbol, priceValue, sourceInfo); err != nil {
		log.Printf("[服务] 推送价格失败: %v", err)
		s.alertManager.SendAlert(types.AlertTypeDataSourceError, symbol, map[string]string{
			"操作": "链上推送",
			"原因": err.Error(),
		})
	}
}

// pushPrice 推送价格（带瘦身，非强制）
// 保留此方法以保持向后兼容
func (s *Service) pushPrice(symbol string, priceValue float64, sourceInfo string) {
	s.tryPushPrice(symbol, priceValue, sourceInfo, false)
}

// formatFloat 格式化浮点数
func formatFloat(f float64) string {
	if f < 1 {
		return fmt.Sprintf("%.4f", f)
	}
	return fmt.Sprintf("%.2f", f)
}
