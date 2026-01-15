// Package gate Gate.io数据源模块
package gate

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// 重连参数
	initialReconnectDelay = 1 * time.Second
	maxReconnectDelay     = 30 * time.Second
	maxReconnectAttempts  = 10 // 最大重连次数
	// 心跳间隔
	pingInterval = 10 * time.Second
	// 频道
	channelTickers = "spot.tickers"
)

// PriceData Gate价格数据
type PriceData struct {
	Pair      string    // 交易对
	Symbol    string    // 股票代币符号
	Price     float64   // 最新价格
	Timestamp time.Time // 时间戳
}

// PriceHandler 价格处理回调
type PriceHandler func(price *PriceData)

// FatalErrorHandler 致命错误回调（重连失败达到最大次数）
type FatalErrorHandler func(source string, err error)

// PairMapping 交易对到符号的映射
type PairMapping struct {
	Pair   string
	Symbol string
}

// Client Gate.io WebSocket客户端
type Client struct {
	wsURL             string
	pairMappings      []PairMapping
	pairToSymbol      map[string]string
	conn              *websocket.Conn
	done              chan struct{}
	reconnect         chan struct{}
	mu                sync.Mutex
	isRunning         bool
	isStopping        bool
	priceHandler      PriceHandler
	fatalErrorHandler FatalErrorHandler
	reconnectCount    int // 当前连续重连次数
}

// NewClient 创建Gate客户端
func NewClient(wsURL string, pairMappings []PairMapping, handler PriceHandler, fatalHandler FatalErrorHandler) *Client {
	// 构建交易对到符号的映射
	pairToSymbol := make(map[string]string)
	for _, m := range pairMappings {
		pairToSymbol[m.Pair] = m.Symbol
	}

	return &Client{
		wsURL:             wsURL,
		pairMappings:      pairMappings,
		pairToSymbol:      pairToSymbol,
		done:              make(chan struct{}),
		reconnect:         make(chan struct{}, 1),
		priceHandler:      handler,
		fatalErrorHandler: fatalHandler,
	}
}

// Start 启动客户端
func (c *Client) Start() error {
	c.mu.Lock()
	if c.isRunning {
		c.mu.Unlock()
		return nil
	}
	c.isRunning = true
	c.mu.Unlock()

	// 初始连接
	if err := c.connect(); err != nil {
		return err
	}

	// 启动重连监听
	go c.reconnectLoop()

	return nil
}

// Stop 停止客户端
func (c *Client) Stop() {
	c.mu.Lock()
	c.isRunning = false
	c.isStopping = true
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

// connect 建立连接
func (c *Client) connect() error {
	log.Printf("[Gate] 连接WebSocket: %s", c.wsURL)

	conn, _, err := websocket.DefaultDialer.Dial(c.wsURL, nil)
	if err != nil {
		return fmt.Errorf("WebSocket连接失败: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	log.Println("[Gate] WebSocket连接成功")

	// 发送订阅请求
	if err := c.subscribe(); err != nil {
		conn.Close()
		return fmt.Errorf("订阅失败: %w", err)
	}

	// 启动心跳
	go c.heartbeat()

	// 启动消息读取
	go c.readMessages()

	return nil
}

// subscribe 发送订阅请求
func (c *Client) subscribe() error {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()

	if conn == nil {
		return fmt.Errorf("连接未建立")
	}

	// 获取所有交易对
	var pairs []string
	for _, m := range c.pairMappings {
		pairs = append(pairs, m.Pair)
	}

	req := SubscribeRequest{
		Time:    time.Now().Unix(),
		Channel: channelTickers,
		Event:   "subscribe",
		Payload: pairs,
	}

	if err := conn.WriteJSON(req); err != nil {
		return fmt.Errorf("发送订阅请求失败: %w", err)
	}

	log.Printf("[Gate] 订阅请求已发送: %+v", req)
	return nil
}

// heartbeat 心跳保活
func (c *Client) heartbeat() {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			c.mu.Lock()
			conn := c.conn
			isStopping := c.isStopping
			c.mu.Unlock()

			if isStopping || conn == nil {
				return
			}

			ping := PingMessage{
				Time:    time.Now().Unix(),
				Channel: channelTickers,
			}

			if err := conn.WriteJSON(ping); err != nil {
				log.Printf("[Gate] 发送心跳失败: %v", err)
			}
		}
	}
}

// readMessages 读取消息
func (c *Client) readMessages() {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()

	if conn == nil {
		return
	}

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			c.mu.Lock()
			isStopping := c.isStopping
			c.mu.Unlock()

			if isStopping {
				return
			}

			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Println("[Gate] WebSocket连接已正常关闭")
			} else {
				log.Printf("[Gate] 读取消息错误: %v，准备重连...", err)
			}

			// 触发重连
			select {
			case c.reconnect <- struct{}{}:
			default:
			}
			return
		}

		// 解析消息
		var resp Response
		if err := json.Unmarshal(message, &resp); err != nil {
			log.Printf("[Gate] 解析消息失败: %v", err)
			continue
		}

		// 处理不同类型的消息
		switch resp.Event {
		case "subscribe":
			if resp.Error != nil {
				log.Printf("[Gate] 订阅失败: code=%d, message=%s", resp.Error.Code, resp.Error.Message)
			}
		case "update":
			if resp.Result != nil && resp.Channel == channelTickers {
				c.handleTickerUpdate(resp.Result, resp.Time)
			}
		case "pong":
			// 心跳响应，忽略
		}
	}
}

// handleTickerUpdate 处理Ticker更新
func (c *Client) handleTickerUpdate(ticker *TickerData, timestamp int64) {
	symbol, ok := c.pairToSymbol[ticker.CurrencyPair]
	if !ok {
		return
	}

	price, err := strconv.ParseFloat(ticker.Last, 64)
	if err != nil {
		log.Printf("[Gate] 解析价格失败: %v", err)
		return
	}

	priceData := &PriceData{
		Pair:      ticker.CurrencyPair,
		Symbol:    symbol,
		Price:     price,
		Timestamp: time.Unix(timestamp, 0),
	}

	if c.priceHandler != nil {
		c.priceHandler(priceData)
	}
}

// reconnectLoop 重连循环
func (c *Client) reconnectLoop() {
	delay := initialReconnectDelay

	for {
		select {
		case <-c.done:
			return
		case <-c.reconnect:
			c.mu.Lock()
			isRunning := c.isRunning
			c.reconnectCount++
			currentCount := c.reconnectCount
			c.mu.Unlock()

			if !isRunning {
				return
			}

			// 检查是否达到最大重连次数
			if currentCount > maxReconnectAttempts {
				log.Printf("[Gate] 重连失败次数达到上限 (%d次)，触发致命错误", maxReconnectAttempts)
				if c.fatalErrorHandler != nil {
					c.fatalErrorHandler("Gate", fmt.Errorf("重连失败次数达到上限 (%d次)", maxReconnectAttempts))
				}
				return
			}

			log.Printf("[Gate] %v后尝试重连... (第%d次/%d次)", delay, currentCount, maxReconnectAttempts)
			time.Sleep(delay)

			if err := c.connect(); err != nil {
				log.Printf("[Gate] 重连失败: %v", err)
				// 指数退避
				delay *= 2
				if delay > maxReconnectDelay {
					delay = maxReconnectDelay
				}
				// 继续重连
				select {
				case c.reconnect <- struct{}{}:
				default:
				}
			} else {
				// 重连成功，重置延迟和计数
				delay = initialReconnectDelay
				c.mu.Lock()
				c.reconnectCount = 0
				c.mu.Unlock()
				log.Println("[Gate] 重连成功")
			}
		}
	}
}

// IsConnected 检查是否已连接
func (c *Client) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn != nil && c.isRunning
}
