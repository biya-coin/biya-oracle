// Package config 配置管理模块
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config 系统配置
type Config struct {
	Redis       RedisConfig        `yaml:"redis"`
	CEX         CEXConfig          `yaml:"cex"`
	Pyth        PythConfig         `yaml:"pyth"`
	Gate        GateConfig         `yaml:"gate"`
	Lark        LarkConfig         `yaml:"lark"`
	Throttle    ThrottleConfig     `yaml:"throttle"`
	OnChain     OnChainConfig      `yaml:"onchain"`
	StockTokens []StockTokenConfig `yaml:"stock_tokens"`
}

// ThrottleConfig 价格瘦身配置
type ThrottleConfig struct {
	MinPushInterval      int     `yaml:"min_push_interval"`       // 最小推送间隔（秒）
	MaxPushInterval      int     `yaml:"max_push_interval"`       // 最大推送间隔（秒）
	PriceChangeThreshold float64 `yaml:"price_change_threshold"`  // 价格变动阈值
}

// RedisConfig Redis配置
type RedisConfig struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

// CEXConfig CEX配置
type CEXConfig struct {
	MarketStatusURL string `yaml:"market_status_url"`
	StockStatusURL  string `yaml:"stock_status_url"`
	WSBaseURL       string `yaml:"ws_base_url"`
}

// PythConfig Pyth配置
type PythConfig struct {
	SSEURL string `yaml:"sse_url"`
}

// GateConfig Gate配置
type GateConfig struct {
	WSURL string `yaml:"ws_url"`
}

// LarkConfig Lark告警配置
type LarkConfig struct {
	WebhookURL string `yaml:"webhook_url"`
}

// OnChainConfig 链上推送配置
type OnChainConfig struct {
	Enabled        bool   `yaml:"enabled"`         // 是否启用链上推送（false时仅控制台打印）
	ChainID        string `yaml:"chain_id"`
	TMEndpoint     string `yaml:"tm_endpoint"`
	GRPCEndpoint   string `yaml:"grpc_endpoint"`
	LCDEndpoint    string `yaml:"lcd_endpoint"`
	KeyringHome    string `yaml:"keyring_home"`
	KeyringBackend string `yaml:"keyring_backend"`
	AccountName    string `yaml:"account_name"`
	Password       string `yaml:"password"`
	PrivateKey     string `yaml:"private_key"`
	QuoteSymbol    string `yaml:"quote_symbol"`
	GasLimit       uint64 `yaml:"gas_limit"`       // Gas限制
	GasPrice       string `yaml:"gas_price"`       // Gas价格（如 "500000000inj"）
	AccountPrefix  string `yaml:"account_prefix"`  // 账户前缀（如 "inj"）
}

// StockTokenConfig 股票代币配置
type StockTokenConfig struct {
	CEXSymbol   string `yaml:"cex_symbol"`    // CEX使用的股票代码
	PythSymbol  string `yaml:"pyth_symbol"`   // Pyth使用的股票代币代码
	PythFeedID  string `yaml:"pyth_feed_id"`  // Pyth Feed ID
	GatePair    string `yaml:"gate_pair"`     // Gate.io交易对
	BaseSymbol  string `yaml:"base_symbol"`   // 链上base符号
}

// Load 从文件加载配置
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 设置默认值
	cfg.setDefaults()

	return &cfg, nil
}

// setDefaults 设置默认值
func (c *Config) setDefaults() {
	if c.Pyth.SSEURL == "" {
		c.Pyth.SSEURL = "https://hermes.pyth.network/v2/updates/price/stream"
	}
	if c.Gate.WSURL == "" {
		c.Gate.WSURL = "wss://api.gateio.ws/ws/v4/"
	}
	// 瘦身参数默认值
	if c.Throttle.MinPushInterval <= 0 {
		c.Throttle.MinPushInterval = 1 // 默认1秒
	}
	if c.Throttle.MaxPushInterval <= 0 {
		c.Throttle.MaxPushInterval = 3 // 默认3秒
	}
	if c.Throttle.PriceChangeThreshold <= 0 {
		c.Throttle.PriceChangeThreshold = 0.003 // 默认0.3%
	}
}

// GetStockTokenBySymbol 根据符号获取股票代币配置
func (c *Config) GetStockTokenBySymbol(symbol string) *StockTokenConfig {
	for i := range c.StockTokens {
		if c.StockTokens[i].CEXSymbol == symbol ||
			c.StockTokens[i].PythSymbol == symbol ||
			c.StockTokens[i].BaseSymbol == symbol {
			return &c.StockTokens[i]
		}
	}
	return nil
}

// GetStockTokenByPythFeedID 根据Pyth Feed ID获取股票代币配置
func (c *Config) GetStockTokenByPythFeedID(feedID string) *StockTokenConfig {
	for i := range c.StockTokens {
		if c.StockTokens[i].PythFeedID == feedID {
			return &c.StockTokens[i]
		}
	}
	return nil
}

// GetStockTokenByGatePair 根据Gate交易对获取股票代币配置
func (c *Config) GetStockTokenByGatePair(pair string) *StockTokenConfig {
	for i := range c.StockTokens {
		if c.StockTokens[i].GatePair == pair {
			return &c.StockTokens[i]
		}
	}
	return nil
}
