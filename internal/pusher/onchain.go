// Package pusher 链上推送模块
package pusher

import (
	"fmt"
	"log"
	"time"
)

// OnChainPusher 链上推送器
type OnChainPusher struct {
	// 配置（占位，后续实现）
	chainID      string
	tmEndpoint   string
	grpcEndpoint string
	quoteSymbol  string
}

// NewOnChainPusher 创建链上推送器
func NewOnChainPusher(chainID, tmEndpoint, grpcEndpoint, quoteSymbol string) *OnChainPusher {
	return &OnChainPusher{
		chainID:      chainID,
		tmEndpoint:   tmEndpoint,
		grpcEndpoint: grpcEndpoint,
		quoteSymbol:  quoteSymbol,
	}
}

// PushPrice 推送价格到链上
// 当前为占位实现，使用控制台打印输出
// sourceInfo: 数据来源信息，如 "CEX" 或 "加权合成（CEX40%+Pyth30%+Gate30%）"
func (p *OnChainPusher) PushPrice(symbol string, price float64, sourceInfo string) error {
	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Printf("📡 [链上推送] 价格更新")
	log.Printf("   股票代币: %s", symbol)
	log.Printf("   价格: %.6f %s", price, p.quoteSymbol)
	log.Printf("   来源: %s", sourceInfo)
	log.Printf("   时间: %s", time.Now().Format("2006-01-02 15:04:05.000"))
	log.Printf("   链ID: %s", p.chainID)
	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// TODO: 实际链上推送逻辑
	// 1. 构建交易
	// 2. 签名
	// 3. 广播

	return nil
}

// PushPriceBatch 批量推送价格
func (p *OnChainPusher) PushPriceBatch(prices map[string]float64) error {
	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Printf("📡 [链上推送] 批量价格更新")
	log.Printf("   时间: %s", time.Now().Format("2006-01-02 15:04:05.000"))
	log.Printf("   链ID: %s", p.chainID)

	for symbol, price := range prices {
		log.Printf("   • %s: %.6f %s", symbol, price, p.quoteSymbol)
	}

	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	return nil
}

// String 格式化价格推送信息
func (p *OnChainPusher) String() string {
	return fmt.Sprintf("OnChainPusher[chainID=%s, quoteSymbol=%s]", p.chainID, p.quoteSymbol)
}
