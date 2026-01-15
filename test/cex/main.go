// CEX WebSocket 实时报价订阅 Demo
// 订阅 NVDA、AAPL、TSLA 三只股票的实时价格
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

// SubscribeRequest 订阅请求结构
type SubscribeRequest struct {
	Type       string   `json:"type"`
	HandleType string   `json:"handleType"`
	Quote      []string `json:"quote"`
}

// FlexibleFloat 灵活的浮点数类型，可以从字符串或数字解析
type FlexibleFloat float64

// UnmarshalJSON 自定义JSON解析，支持字符串和数字两种格式
func (f *FlexibleFloat) UnmarshalJSON(data []byte) error {
	// 先尝试解析为数字
	var num float64
	if err := json.Unmarshal(data, &num); err == nil {
		*f = FlexibleFloat(num)
		return nil
	}
	// 再尝试解析为字符串
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		if str == "" {
			*f = 0
			return nil
		}
		num, err := strconv.ParseFloat(str, 64)
		if err != nil {
			return err
		}
		*f = FlexibleFloat(num)
		return nil
	}
	return fmt.Errorf("cannot unmarshal %s into FlexibleFloat", string(data))
}

// Float64 返回 float64 值
func (f FlexibleFloat) Float64() float64 {
	return float64(f)
}

// QuoteMessage CEX 报价消息结构
// 注意：某些字段可能返回字符串或数字，使用 FlexibleFloat 处理
type QuoteMessage struct {
	Symbol            string        `json:"symbol"`               // 交易对
	Amount            FlexibleFloat `json:"amount"`               // 成交额
	BidHighPrice      FlexibleFloat `json:"bid_high_price"`       // 竞价最高价
	BluePrice         FlexibleFloat `json:"blue_price"`           // 夜盘最新价
	BlueUpDownAmount  FlexibleFloat `json:"blue_up_down_amount"`  // 夜盘价格涨跌额
	BlueUpDownPercent FlexibleFloat `json:"blue_up_down_percent"` // 夜盘价格涨跌幅百分比
	BidUpDownAmount   FlexibleFloat `json:"bid_up_down_amount"`   // 竞价涨跌额
	BidLowPrice       FlexibleFloat `json:"bid_low_price"`        // 竞价最低价
	BidVolume         int64         `json:"bid_volume"`           // 竞价成交量
	LatestPrice       FlexibleFloat `json:"latest_price"`         // 最新价（盘中价格）
	Type              string        `json:"type"`                 // 订阅类型 stock/option
	UpDownPercent     FlexibleFloat `json:"up_down_percent"`      // 竞价涨跌幅
	Market            string        `json:"market"`               // 市场 US/HK
	Volume            int64         `json:"volume"`               // 成交量
	BidTimestamp      int64         `json:"bid_timestamp"`        // 竞价时间
	High              FlexibleFloat `json:"high"`                 // 最高价
	BidAmount         FlexibleFloat `json:"bid_amount"`           // 竞价成交额
	UpDownAmount      FlexibleFloat `json:"up_down_amount"`       // 涨跌额
	Low               FlexibleFloat `json:"low"`                  // 最低价
	DataType          string        `json:"data_type"`            // 数据类型
	SymbolType        int           `json:"symbol_type"`          // 证券类型
	Close             FlexibleFloat `json:"close"`                // 收盘价
	BidPrice          FlexibleFloat `json:"bid_price"`            // 竞价价格（盘前、盘后价）
	Open              FlexibleFloat `json:"open"`                 // 开盘价
	MarketStatus      string        `json:"marketStatus"`         // 市场状态: Regular/PreMarket/AfterHours/OverNight
	Timestamp         int64         `json:"timestamp"`            // 行情数据时间戳
}

// MarketStatus 市场状态常量
const (
	MarketStatusRegular    = "Regular"    // 盘中
	MarketStatusPreMarket  = "PreMarket"  // 盘前
	MarketStatusAfterHours = "AfterHours" // 盘后
	MarketStatusOverNight  = "OverNight"  // 夜盘
)

// generateAppId 生成AppId: RWA + 当前时间戳
func generateAppId() string {
	return fmt.Sprintf("RWA%d", time.Now().UnixMilli())
}

// getWebSocketURL 获取WebSocket连接URL
func getWebSocketURL() string {
	appId := generateAppId()
	return fmt.Sprintf("wss://market-ws.biya.in/market/subscribe/%s", appId)
}

// getPriceByMarketStatus 根据市场状态获取对应的价格
// Regular（盘中）→ latest_price
// PreMarket（盘前）→ bid_price
// AfterHours（盘后）→ bid_price
// OverNight（夜盘）→ blue_price
func getPriceByMarketStatus(msg *QuoteMessage) (float64, string) {
	switch msg.MarketStatus {
	case MarketStatusRegular:
		return msg.LatestPrice.Float64(), "latest_price"
	case MarketStatusPreMarket:
		return msg.BidPrice.Float64(), "bid_price"
	case MarketStatusAfterHours:
		return msg.BidPrice.Float64(), "bid_price"
	case MarketStatusOverNight:
		return msg.BluePrice.Float64(), "blue_price"
	default:
		// 未知状态，默认返回 latest_price
		return msg.LatestPrice.Float64(), "latest_price(default)"
	}
}

// formatMarketStatus 格式化市场状态显示
func formatMarketStatus(status string) string {
	switch status {
	case MarketStatusRegular:
		return "盘中(Regular)"
	case MarketStatusPreMarket:
		return "盘前(PreMarket)"
	case MarketStatusAfterHours:
		return "盘后(AfterHours)"
	case MarketStatusOverNight:
		return "夜盘(OverNight)"
	default:
		return fmt.Sprintf("未知(%s)", status)
	}
}

// CEXClient WebSocket客户端，支持断线重连
type CEXClient struct {
	conn       *websocket.Conn
	url        string
	symbols    []string
	done       chan struct{}
	reconnect  chan struct{}
	mu         sync.Mutex
	isRunning  bool
	isStopping bool // 标记是否正在停止，避免退出时打印错误日志
}

// NewCEXClient 创建新的CEX客户端
func NewCEXClient(symbols []string) *CEXClient {
	return &CEXClient{
		symbols:   symbols,
		done:      make(chan struct{}),
		reconnect: make(chan struct{}, 1),
	}
}

// connect 建立WebSocket连接
func (c *CEXClient) connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 每次连接生成新的URL（包含新的AppId）
	c.url = getWebSocketURL()
	log.Printf("连接地址: %s", c.url)

	conn, _, err := websocket.DefaultDialer.Dial(c.url, nil)
	if err != nil {
		return fmt.Errorf("WebSocket连接失败: %v", err)
	}
	c.conn = conn
	log.Println("WebSocket连接成功!")

	// 发送订阅请求
	subscribeReq := SubscribeRequest{
		Type:       "stock",
		HandleType: "STOCK",
		Quote:      c.symbols,
	}

	if err := conn.WriteJSON(subscribeReq); err != nil {
		conn.Close()
		return fmt.Errorf("发送订阅请求失败: %v", err)
	}
	log.Printf("订阅请求已发送: %+v", subscribeReq)

	return nil
}

// reconnectWithBackoff 带退避策略的重连
func (c *CEXClient) reconnectWithBackoff() {
	backoff := time.Second         // 初始重连间隔1秒
	maxBackoff := 30 * time.Second // 最大重连间隔30秒

	for {
		select {
		case <-c.done:
			return
		default:
		}

		log.Printf("尝试重连，等待 %v...", backoff)
		time.Sleep(backoff)

		if err := c.connect(); err != nil {
			log.Printf("重连失败: %v", err)
			// 指数退避，但不超过最大值
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}

		// 重连成功，重置退避时间
		log.Println("重连成功!")
		return
	}
}

// readMessages 读取消息循环
func (c *CEXClient) readMessages() {
	for {
		select {
		case <-c.done:
			return
		default:
		}

		c.mu.Lock()
		conn := c.conn
		c.mu.Unlock()

		if conn == nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		_, message, err := conn.ReadMessage()
		if err != nil {
			c.mu.Lock()
			isStopping := c.isStopping
			c.mu.Unlock()

			// 如果是正在停止，不打印错误也不重连
			if isStopping {
				return
			}

			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Println("WebSocket连接已正常关闭")
			} else {
				log.Printf("读取消息错误: %v，准备重连...", err)
			}

			// 触发重连
			select {
			case c.reconnect <- struct{}{}:
			default:
			}
			return
		}

		// 解析报价消息
		var quoteMsg QuoteMessage
		if err := json.Unmarshal(message, &quoteMsg); err != nil {
			// 忽略解析失败的消息（可能是心跳或其他类型消息）
			continue
		}

		// 跳过空消息
		if quoteMsg.Symbol == "" {
			continue
		}

		// 根据市场状态获取价格
		price, priceField := getPriceByMarketStatus(&quoteMsg)

		// 格式化输出
		log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		log.Printf("股票: %s | 市场: %s", quoteMsg.Symbol, quoteMsg.Market)
		log.Printf("市场状态: %s", formatMarketStatus(quoteMsg.MarketStatus))
		log.Printf("当前价格: %.4f (来源字段: %s)", price, priceField)
		log.Printf("时间戳: %d (%s)", quoteMsg.Timestamp,
			time.UnixMilli(quoteMsg.Timestamp).Format("2006-01-02 15:04:05"))
	}
}

// Start 启动客户端
func (c *CEXClient) Start() error {
	if err := c.connect(); err != nil {
		return err
	}

	c.isRunning = true

	// 启动消息读取协程
	go c.readMessages()

	// 启动重连监听协程
	go func() {
		for {
			select {
			case <-c.done:
				return
			case <-c.reconnect:
				c.mu.Lock()
				if c.conn != nil {
					c.conn.Close()
					c.conn = nil
				}
				c.mu.Unlock()

				c.reconnectWithBackoff()

				// 重连成功后重新启动消息读取
				go c.readMessages()
			}
		}
	}()

	return nil
}

// Stop 停止客户端
func (c *CEXClient) Stop() {
	c.mu.Lock()
	c.isRunning = false
	c.isStopping = true // 标记正在停止，避免打印错误日志
	c.mu.Unlock()

	close(c.done)

	c.mu.Lock()
	if c.conn != nil {
		c.conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		c.conn.Close()
		c.conn = nil
	}
	c.mu.Unlock()
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	log.Println("=== CEX WebSocket 实时报价订阅 Demo（支持断线重连）===")

	// 创建CEX客户端
	client := NewCEXClient([]string{"NVDA", "AAPL", "TSLA"})

	// 启动客户端
	if err := client.Start(); err != nil {
		log.Fatalf("启动客户端失败: %v", err)
	}

	// 设置信号处理，优雅退出
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 等待退出信号
	<-sigChan
	log.Println("收到退出信号，正在关闭连接...")

	// 停止客户端
	client.Stop()

	log.Println("程序退出")
}
