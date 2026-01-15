// Gate.io WebSocket 实时报价订阅 Demo
// 订阅 AAPLX_USDT、NVDAX_USDT、TSLAX_USDT 三个交易对的实时价格
// 参考文档: https://www.gate.com/docs/developers/apiv4/ws/zh_CN/#%E7%8E%B0%E8%B4%A7-websocket-v4
package main

import (
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// Gate.io WebSocket 地址
	GateWSURL = "wss://api.gateio.ws/ws/v4/"
	// 订阅频道
	ChannelTickers = "spot.tickers"
)

// SubscribeRequest Gate.io 订阅请求结构
type SubscribeRequest struct {
	Time    int64    `json:"time"`
	Channel string   `json:"channel"`
	Event   string   `json:"event"`
	Payload []string `json:"payload"`
}

// GateResponse Gate.io WebSocket 响应结构
type GateResponse struct {
	Time    int64       `json:"time"`
	Channel string      `json:"channel"`
	Event   string      `json:"event"`
	Result  *TickerData `json:"result"`
}

// TickerData Tickers 频道推送的数据
type TickerData struct {
	CurrencyPair     string `json:"currency_pair"`      // 交易对
	Last             string `json:"last"`               // 最新成交价
	LowestAsk        string `json:"lowest_ask"`         // 卖一价
	HighestBid       string `json:"highest_bid"`        // 买一价
	ChangePercentage string `json:"change_percentage"`  // 24小时涨跌幅
	BaseVolume       string `json:"base_volume"`        // 基础货币交易量
	QuoteVolume      string `json:"quote_volume"`       // 计价货币交易量
	High24h          string `json:"high_24h"`           // 24小时最高价
	Low24h           string `json:"low_24h"`            // 24小时最低价
}

// GateClient WebSocket客户端，支持断线重连
type GateClient struct {
	conn       *websocket.Conn
	symbols    []string
	done       chan struct{}
	reconnect  chan struct{}
	mu         sync.Mutex
	isRunning  bool
	isStopping bool // 标记是否正在停止，避免退出时打印错误日志
}

// NewGateClient 创建新的Gate客户端
func NewGateClient(symbols []string) *GateClient {
	return &GateClient{
		symbols:   symbols,
		done:      make(chan struct{}),
		reconnect: make(chan struct{}, 1),
	}
}

// connect 建立WebSocket连接
func (c *GateClient) connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	log.Printf("连接地址: %s", GateWSURL)

	conn, _, err := websocket.DefaultDialer.Dial(GateWSURL, nil)
	if err != nil {
		return err
	}
	c.conn = conn
	log.Println("WebSocket连接成功!")

	// 发送订阅请求
	subscribeReq := SubscribeRequest{
		Time:    time.Now().Unix(),
		Channel: ChannelTickers,
		Event:   "subscribe",
		Payload: c.symbols,
	}

	if err := conn.WriteJSON(subscribeReq); err != nil {
		conn.Close()
		return err
	}
	log.Printf("订阅请求已发送: channel=%s, symbols=%v", subscribeReq.Channel, subscribeReq.Payload)

	return nil
}

// reconnectWithBackoff 带退避策略的重连
func (c *GateClient) reconnectWithBackoff() {
	backoff := time.Second      // 初始重连间隔1秒
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

// sendPing 发送应用层ping消息
func (c *GateClient) sendPing() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return nil
	}

	pingMsg := map[string]interface{}{
		"time":    time.Now().Unix(),
		"channel": "spot.ping",
	}
	return c.conn.WriteJSON(pingMsg)
}

// startPingRoutine 启动ping心跳协程
func (c *GateClient) startPingRoutine() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			if err := c.sendPing(); err != nil {
				log.Printf("发送ping失败: %v", err)
			}
		}
	}
}

// readMessages 读取消息循环
func (c *GateClient) readMessages() {
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

		// 解析响应
		var response GateResponse
		if err := json.Unmarshal(message, &response); err != nil {
			// 可能是其他类型的消息（如pong），忽略
			continue
		}

		// 只处理 tickers 频道的 update 事件
		if response.Channel != ChannelTickers || response.Event != "update" {
			// 处理订阅确认消息
			if response.Event == "subscribe" {
				log.Printf("订阅确认: channel=%s", response.Channel)
			}
			continue
		}

		// 跳过空数据
		if response.Result == nil || response.Result.CurrencyPair == "" {
			continue
		}

		ticker := response.Result

		// 格式化输出
		log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		log.Printf("交易对: %s", ticker.CurrencyPair)
		log.Printf("最新价格: %s", ticker.Last)
		log.Printf("时间戳: %d (%s)", response.Time,
			time.Unix(response.Time, 0).Format("2006-01-02 15:04:05"))
	}
}

// Start 启动客户端
func (c *GateClient) Start() error {
	if err := c.connect(); err != nil {
		return err
	}

	c.isRunning = true

	// 启动消息读取协程
	go c.readMessages()

	// 启动ping心跳协程
	go c.startPingRoutine()

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
func (c *GateClient) Stop() {
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
	log.Println("=== Gate.io WebSocket 实时报价订阅 Demo（支持断线重连）===")

	// 创建Gate客户端，订阅三个交易对
	client := NewGateClient([]string{"AAPLX_USDT", "NVDAX_USDT", "TSLAX_USDT"})

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
