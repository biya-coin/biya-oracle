// Package pyth Pyth Network数据源模块
package pyth

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// 重连参数
	initialReconnectDelay = 1 * time.Second
	maxReconnectDelay     = 30 * time.Second
	maxReconnectAttempts  = 10 // 最大重连次数
)

// PriceData Pyth价格数据
type PriceData struct {
	FeedID    string    // Feed ID
	Symbol    string    // 股票代币符号
	Price     float64   // 实际价格
	Timestamp time.Time // 时间戳
}

// PriceHandler 价格处理回调
type PriceHandler func(price *PriceData)

// FatalErrorHandler 致命错误回调（重连失败达到最大次数）
type FatalErrorHandler func(source string, err error)

// FeedIDMapping Feed ID到符号的映射
type FeedIDMapping struct {
	FeedID string
	Symbol string
}

// Client Pyth SSE客户端
type Client struct {
	sseURL            string
	feedMappings      []FeedIDMapping
	feedIDToSymbol    map[string]string
	done              chan struct{}
	reconnect         chan struct{}
	mu                sync.Mutex
	isRunning         bool
	isStopping        bool
	priceHandler      PriceHandler
	fatalErrorHandler FatalErrorHandler
	cancelFunc        context.CancelFunc
	reconnectCount    int // 当前连续重连次数
}

// NewClient 创建Pyth客户端
func NewClient(sseURL string, feedMappings []FeedIDMapping, handler PriceHandler, fatalHandler FatalErrorHandler) *Client {
	// 构建Feed ID到符号的映射
	feedIDToSymbol := make(map[string]string)
	for _, m := range feedMappings {
		feedIDToSymbol[m.FeedID] = m.Symbol
	}

	return &Client{
		sseURL:            sseURL,
		feedMappings:      feedMappings,
		feedIDToSymbol:    feedIDToSymbol,
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
	if c.cancelFunc != nil {
		c.cancelFunc()
	}
	c.mu.Unlock()

	close(c.done)
}

// connect 建立SSE连接
func (c *Client) connect() error {
	// 构建URL，添加所有Feed ID
	var feedIDs []string
	for _, m := range c.feedMappings {
		feedIDs = append(feedIDs, "ids[]="+m.FeedID)
	}
	url := fmt.Sprintf("%s?%s&parsed=true", c.sseURL, strings.Join(feedIDs, "&"))

	log.Printf("[Pyth] 连接SSE: %s", url)

	ctx, cancel := context.WithCancel(context.Background())
	c.mu.Lock()
	c.cancelFunc = cancel
	c.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("SSE连接失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return fmt.Errorf("SSE连接失败: HTTP %d", resp.StatusCode)
	}

	log.Println("[Pyth] SSE连接成功")

	// 启动消息读取
	go c.readMessages(resp)

	return nil
}

// readMessages 读取SSE消息
func (c *Client) readMessages(resp *http.Response) {
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	// 增加scanner的buffer大小，Pyth的消息可能较大
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		c.mu.Lock()
		isStopping := c.isStopping
		c.mu.Unlock()

		if isStopping {
			return
		}

		line := scanner.Text()

		// 跳过空行和注释
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}

		// 处理data行
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimPrefix(line, "data:")
			data = strings.TrimSpace(data)
			c.parseAndHandleMessage(data)
		}
	}

	// 检查是否正在停止
	c.mu.Lock()
	isStopping := c.isStopping
	c.mu.Unlock()

	if isStopping {
		return
	}

	if err := scanner.Err(); err != nil {
		log.Printf("[Pyth] 读取消息错误: %v，准备重连...", err)
	} else {
		log.Println("[Pyth] SSE连接已断开，准备重连...")
	}

	// 触发重连
	select {
	case c.reconnect <- struct{}{}:
	default:
	}
}

// parseAndHandleMessage 解析并处理消息
func (c *Client) parseAndHandleMessage(data string) {
	var resp SSEResponse
	if err := json.Unmarshal([]byte(data), &resp); err != nil {
		log.Printf("[Pyth] 解析消息失败: %v", err)
		return
	}

	// 处理解析后的价格数据
	for _, parsed := range resp.Parsed {
		symbol, ok := c.feedIDToSymbol[parsed.ID]
		if !ok {
			continue
		}

		// 计算实际价格
		price, err := c.calculatePrice(parsed.Price)
		if err != nil {
			log.Printf("[Pyth] 计算价格失败: %v", err)
			continue
		}

		priceData := &PriceData{
			FeedID:    parsed.ID,
			Symbol:    symbol,
			Price:     price,
			Timestamp: time.Unix(parsed.Price.PublishTime, 0),
		}

		if c.priceHandler != nil {
			c.priceHandler(priceData)
		}
	}
}

// calculatePrice 根据价格字符串和指数计算实际价格
func (c *Client) calculatePrice(info PriceInfo) (float64, error) {
	priceInt, err := strconv.ParseInt(info.Price, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("解析价格失败: %w", err)
	}

	// 计算实际价格：price * 10^expo
	actualPrice := float64(priceInt) * math.Pow(10, float64(info.Expo))
	return actualPrice, nil
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
				log.Printf("[Pyth] 重连失败次数达到上限 (%d次)，触发致命错误", maxReconnectAttempts)
				if c.fatalErrorHandler != nil {
					c.fatalErrorHandler("Pyth", fmt.Errorf("重连失败次数达到上限 (%d次)", maxReconnectAttempts))
				}
				return
			}

			log.Printf("[Pyth] %v后尝试重连... (第%d次/%d次)", delay, currentCount, maxReconnectAttempts)
			time.Sleep(delay)

			if err := c.connect(); err != nil {
				log.Printf("[Pyth] 重连失败: %v", err)
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
				log.Println("[Pyth] 重连成功")
			}
		}
	}
}

// IsConnected 检查是否已连接
func (c *Client) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.isRunning && !c.isStopping
}
