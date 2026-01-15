// Package pyth Pyth Network数据源模块
package pyth

// SSEResponse Pyth SSE响应结构
type SSEResponse struct {
	Binary struct {
		Encoding string   `json:"encoding"`
		Data     []string `json:"data"`
	} `json:"binary"`
	Parsed []ParsedPrice `json:"parsed"`
}

// ParsedPrice 解析后的价格数据
type ParsedPrice struct {
	ID    string    `json:"id"`     // Feed ID
	Price PriceInfo `json:"price"`  // 价格信息
	EMAPrice PriceInfo `json:"ema_price"` // EMA价格
}

// PriceInfo 价格详情
type PriceInfo struct {
	Price       string `json:"price"`        // 价格（需要根据expo计算）
	Conf        string `json:"conf"`         // 置信区间
	Expo        int    `json:"expo"`         // 指数
	PublishTime int64  `json:"publish_time"` // 发布时间戳
}
