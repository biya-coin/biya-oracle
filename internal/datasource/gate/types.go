// Package gate Gate.io数据源模块
package gate

// SubscribeRequest Gate.io订阅请求
type SubscribeRequest struct {
	Time    int64    `json:"time"`
	Channel string   `json:"channel"`
	Event   string   `json:"event"`
	Payload []string `json:"payload"`
}

// Response Gate.io WebSocket响应
type Response struct {
	Time    int64       `json:"time"`
	Channel string      `json:"channel"`
	Event   string      `json:"event"`
	Result  *TickerData `json:"result,omitempty"`
	Error   *ErrorData  `json:"error,omitempty"`
}

// TickerData Ticker数据
type TickerData struct {
	CurrencyPair     string `json:"currency_pair"`      // 交易对
	Last             string `json:"last"`               // 最新价格
	LowestAsk        string `json:"lowest_ask"`         // 最低卖价
	HighestBid       string `json:"highest_bid"`        // 最高买价
	ChangePercentage string `json:"change_percentage"`  // 24h涨跌幅
	BaseVolume       string `json:"base_volume"`        // 24h成交量
	QuoteVolume      string `json:"quote_volume"`       // 24h成交额
	High24h          string `json:"high_24h"`           // 24h最高价
	Low24h           string `json:"low_24h"`            // 24h最低价
}

// ErrorData 错误数据
type ErrorData struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// PingMessage Ping消息
type PingMessage struct {
	Time    int64  `json:"time"`
	Channel string `json:"channel"`
}

// PongMessage Pong消息
type PongMessage struct {
	Time    int64  `json:"time"`
	Channel string `json:"channel"`
	Event   string `json:"event"`
}
