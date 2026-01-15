// Package cex CEX数据源模块
package cex

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// 重连参数
	initialReconnectDelay = 1 * time.Second
	maxReconnectDelay     = 30 * time.Second
	maxReconnectAttempts  = 10 // 最大重连次数
)

// QuoteHandler 报价处理回调
type QuoteHandler func(quote *QuoteMessage)

// FatalErrorHandler 致命错误回调（重连失败达到最大次数）
type FatalErrorHandler func(source string, err error)

// Client CEX WebSocket客户端
type Client struct {
	wsBaseURL         string
	symbols           []string
	conn              *websocket.Conn
	done              chan struct{}
	reconnect         chan struct{}
	mu                sync.Mutex
	isRunning         bool
	isStopping        bool
	quoteHandler      QuoteHandler
	fatalErrorHandler FatalErrorHandler
	reconnectCount    int // 当前连续重连次数
}

// NewClient 创建CEX客户端
func NewClient(wsBaseURL string, symbols []string, handler QuoteHandler, fatalHandler FatalErrorHandler) *Client {
	return &Client{
		wsBaseURL:         wsBaseURL,
		symbols:           symbols,
		done:              make(chan struct{}),
		reconnect:         make(chan struct{}, 1),
		quoteHandler:      handler,
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
	// 生成AppId
	appID := fmt.Sprintf("RWA%d", time.Now().UnixMilli())
	wsURL := fmt.Sprintf("%s/%s", c.wsBaseURL, appID)

	log.Printf("[CEX] 连接WebSocket: %s", wsURL)

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return fmt.Errorf("WebSocket连接失败: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	log.Println("[CEX] WebSocket连接成功")

	// 发送订阅请求
	if err := c.subscribe(); err != nil {
		conn.Close()
		return fmt.Errorf("订阅失败: %w", err)
	}

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

	req := SubscribeRequest{
		Type:       "stock",
		HandleType: "STOCK",
		Quote:      c.symbols,
	}

	if err := conn.WriteJSON(req); err != nil {
		return fmt.Errorf("发送订阅请求失败: %w", err)
	}

	log.Printf("[CEX] 订阅请求已发送: %+v", req)
	return nil
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
				log.Println("[CEX] WebSocket连接已正常关闭")
			} else {
				log.Printf("[CEX] 读取消息错误: %v，准备重连...", err)
			}

			// 触发重连
			select {
			case c.reconnect <- struct{}{}:
			default:
			}
			return
		}

		// 解析消息
		var quote QuoteMessage
		if err := json.Unmarshal(message, &quote); err != nil {
			log.Printf("[CEX] 解析消息失败: %v", err)
			continue
		}

		// 调用回调处理
		if c.quoteHandler != nil {
			c.quoteHandler(&quote)
		}
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
				log.Printf("[CEX] 重连失败次数达到上限 (%d次)，触发致命错误", maxReconnectAttempts)
				if c.fatalErrorHandler != nil {
					c.fatalErrorHandler("CEX", fmt.Errorf("重连失败次数达到上限 (%d次)", maxReconnectAttempts))
				}
				return
			}

			log.Printf("[CEX] %v后尝试重连... (第%d次/%d次)", delay, currentCount, maxReconnectAttempts)
			time.Sleep(delay)

			if err := c.connect(); err != nil {
				log.Printf("[CEX] 重连失败: %v", err)
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
				log.Println("[CEX] 重连成功")
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
