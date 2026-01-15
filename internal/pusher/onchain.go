// Package pusher 链上推送模块
package pusher

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"cosmossdk.io/math"
	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"

	oracletypes "github.com/InjectiveLabs/sdk-go/chain/oracle/types"
	chainclient "github.com/InjectiveLabs/sdk-go/client/chain"
	"github.com/InjectiveLabs/sdk-go/client/common"

	"biya-oracle/internal/config"
)

// OnChainPusher 链上推送器
type OnChainPusher struct {
	config      config.OnChainConfig
	chainClient chainclient.ChainClientV2
	senderAddr  string
	network     common.Network

	mu sync.Mutex
}

// NewOnChainPusher 创建链上推送器
func NewOnChainPusher(cfg config.OnChainConfig) (*OnChainPusher, error) {
	pusher := &OnChainPusher{
		config: cfg,
	}

	// 如果未启用，直接返回（仅用于控制台打印模式）
	if !cfg.Enabled {
		log.Printf("[链上推送] 链上推送未启用，仅控制台打印模式")
		return pusher, nil
	}

	// 配置自定义网络（需要设置所有必要的 gRPC 端点和 CookieAssistant）
	pusher.network = common.Network{
		ChainId:                 cfg.ChainID,
		LcdEndpoint:             cfg.LCDEndpoint,
		TmEndpoint:              cfg.TMEndpoint,
		ChainGrpcEndpoint:       cfg.GRPCEndpoint,
		ChainStreamGrpcEndpoint: cfg.GRPCEndpoint, // 使用相同的 gRPC 端点
		FeeDenom:                "inj",
		Name:                    "custom",
		ChainCookieAssistant:    &common.DisabledCookieAssistant{},
		ExchangeCookieAssistant: &common.DisabledCookieAssistant{},
		ExplorerCookieAssistant: &common.DisabledCookieAssistant{},
	}

	// 创建 Tendermint 客户端
	tmClient, err := rpchttp.New(cfg.TMEndpoint)
	if err != nil {
		return nil, fmt.Errorf("创建 Tendermint 客户端失败: %w", err)
	}

	// 初始化 Keyring
	senderAddress, cosmosKeyring, err := chainclient.InitCosmosKeyring(
		cfg.KeyringHome,
		"injectived",
		cfg.KeyringBackend,
		cfg.AccountName,
		cfg.Password,
		cfg.PrivateKey, // 如果提供私钥，优先使用私钥
		false,
	)
	if err != nil {
		return nil, fmt.Errorf("初始化 Keyring 失败: %w", err)
	}

	pusher.senderAddr = senderAddress.String()
	log.Printf("[链上推送] Relayer 地址: %s", pusher.senderAddr)

	// 创建客户端上下文
	clientCtx, err := chainclient.NewClientContext(
		cfg.ChainID,
		senderAddress.String(),
		cosmosKeyring,
	)
	if err != nil {
		return nil, fmt.Errorf("创建客户端上下文失败: %w", err)
	}

	clientCtx = clientCtx.WithNodeURI(cfg.TMEndpoint).WithClient(tmClient)

	// 解析配置的 gas 价格
	gasPriceStr := strings.TrimSuffix(cfg.GasPrice, "inj")
	gasPrice, _ := strconv.ParseInt(gasPriceStr, 10, 64)
	if gasPrice == 0 {
		gasPrice = 500000000 // 默认 gas 价格
	}

	// 构建 gas 价格字符串（带 denom）
	gasPriceWithDenom := fmt.Sprintf("%dinj", gasPrice)

	// 创建链客户端 V2（使用配置的 gas 价格）
	chainClient, err := chainclient.NewChainClientV2(
		clientCtx,
		pusher.network,
		common.OptionGasPrices(gasPriceWithDenom),
	)
	if err != nil {
		return nil, fmt.Errorf("创建链客户端失败: %w", err)
	}

	pusher.chainClient = chainClient

	log.Printf("[链上推送] 初始化完成 - ChainID: %s, GasPrice: %s", cfg.ChainID, gasPriceWithDenom)

	return pusher, nil
}

// PushPrice 推送单个价格
func (p *OnChainPusher) PushPrice(symbol string, price float64, sourceInfo string) error {
	// 先打印控制台日志
	p.printConsoleLog(symbol, price, sourceInfo)

	// 如果未启用链上推送，仅打印
	if !p.config.Enabled {
		return nil
	}

	// 执行链上推送
	return p.pushPriceToChain([]string{symbol}, []string{p.config.QuoteSymbol}, []float64{price}, sourceInfo)
}

// PushPrices 批量推送价格
func (p *OnChainPusher) PushPrices(prices map[string]float64, sourceInfo string) error {
	// 先打印控制台日志
	p.printBatchConsoleLog(prices, sourceInfo)

	// 如果未启用链上推送，仅打印
	if !p.config.Enabled {
		return nil
	}

	// 构建批量数据
	var bases, quotes []string
	var priceVals []float64
	for symbol, price := range prices {
		bases = append(bases, symbol)
		quotes = append(quotes, p.config.QuoteSymbol)
		priceVals = append(priceVals, price)
	}

	return p.pushPriceToChain(bases, quotes, priceVals, sourceInfo)
}

// pushPriceToChain 执行链上推送
func (p *OnChainPusher) pushPriceToChain(bases, quotes []string, prices []float64, sourceInfo string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 构建价格数组（使用 LegacyDec）
	var priceDecimals []math.LegacyDec
	for _, price := range prices {
		// 将 float64 转换为 LegacyDec
		// 使用 6 位小数精度，避免 float64 的精度问题
		// （float64 无法精确表示大多数十进制小数，如 259.83 实际存储为 259.829999...）
		priceStr := fmt.Sprintf("%.6f", price)
		priceDec := math.LegacyMustNewDecFromStr(priceStr)
		priceDecimals = append(priceDecimals, priceDec)
	}

	// 构建 MsgRelayPriceFeedPrice 消息
	msg := &oracletypes.MsgRelayPriceFeedPrice{
		Sender: p.senderAddr,
		Base:   bases,
		Quote:  quotes,
		Price:  priceDecimals,
	}

	// 广播交易
	_, result, err := p.chainClient.BroadcastMsg(ctx, txtypes.BroadcastMode_BROADCAST_MODE_SYNC, msg)
	if err != nil {
		return fmt.Errorf("广播交易失败: %w", err)
	}

	// 获取交易哈希
	txHash := ""
	if result != nil && result.TxResponse != nil {
		txHash = result.TxResponse.TxHash
		// 检查交易是否成功
		if result.TxResponse.Code != 0 {
			return fmt.Errorf("交易失败 (code=%d): %s", result.TxResponse.Code, result.TxResponse.RawLog)
		}
	}
	log.Printf("[链上推送] ✅ 交易成功 - TxHash: %s, 来源: %s", txHash, sourceInfo)
	return nil
}

// printConsoleLog 打印控制台日志
func (p *OnChainPusher) printConsoleLog(symbol string, price float64, sourceInfo string) {
	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Printf("📡 [链上推送] 价格更新")
	log.Printf("   股票代币: %s", symbol)
	log.Printf("   价格: %.6f %s", price, p.config.QuoteSymbol)
	log.Printf("   来源: %s", sourceInfo)
	log.Printf("   时间: %s", time.Now().Format("2006-01-02 15:04:05.000"))
	log.Printf("   链ID: %s", p.config.ChainID)
	if p.config.Enabled {
		log.Printf("   模式: 链上推送")
	} else {
		log.Printf("   模式: 仅控制台打印")
	}
	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}

// printBatchConsoleLog 打印批量推送的控制台日志
func (p *OnChainPusher) printBatchConsoleLog(prices map[string]float64, sourceInfo string) {
	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Printf("📡 [链上推送] 批量价格更新")
	log.Printf("   时间: %s", time.Now().Format("2006-01-02 15:04:05.000"))
	log.Printf("   来源: %s", sourceInfo)
	log.Printf("   链ID: %s", p.config.ChainID)
	if p.config.Enabled {
		log.Printf("   模式: 链上推送")
	} else {
		log.Printf("   模式: 仅控制台打印")
	}
	for symbol, price := range prices {
		log.Printf("   • %s: %.6f %s", symbol, price, p.config.QuoteSymbol)
	}
	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}

// Close 关闭连接
func (p *OnChainPusher) Close() error {
	if p.chainClient != nil {
		p.chainClient.Close()
	}
	return nil
}

// Stop 停止推送器
func (p *OnChainPusher) Stop() {
	p.Close()
}

// String 格式化推送器信息
func (p *OnChainPusher) String() string {
	if p.config.Enabled {
		return fmt.Sprintf("OnChainPusher[enabled=true, chainID=%s, relayer=%s]",
			p.config.ChainID, p.senderAddr)
	}
	return fmt.Sprintf("OnChainPusher[enabled=false, chainID=%s]", p.config.ChainID)
}

// IsEnabled 返回是否启用链上推送
func (p *OnChainPusher) IsEnabled() bool {
	return p.config.Enabled
}
