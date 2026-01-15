// Pyth Network SSE 实时报价订阅 Demo
// 订阅 AAPLX、NVDAX、TSLAX 三个股票代币的实时价格
// 参考文档: https://docs.pyth.network/price-feeds/core/fetch-price-updates#streaming
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	// Pyth Hermes SSE 端点
	HermesSSEURL = "https://hermes.pyth.network/v2/updates/price/stream"
)

// Feed ID 映射表
var feedIDs = map[string]string{
	"AAPLX": "978e6cc68a119ce066aa830017318563a9ed04ec3a0a6439010fc11296a58675",
	"NVDAX": "4244d07890e4610f46bbde67de8f43a4bf8b569eebe904f136b469f148503b7f",
	"TSLAX": "47a156470288850a440df3a6ce85a55917b813a19bb5b31128a33a986566a362",
}

// Feed ID 反向映射（用于从 feed_id 查找 symbol）
var feedIDToSymbol = map[string]string{
	"978e6cc68a119ce066aa830017318563a9ed04ec3a0a6439010fc11296a58675": "AAPLX",
	"4244d07890e4610f46bbde67de8f43a4bf8b569eebe904f136b469f148503b7f": "NVDAX",
	"47a156470288850a440df3a6ce85a55917b813a19bb5b31128a33a986566a362": "TSLAX",
}

// SSEResponse Pyth SSE 响应结构
type SSEResponse struct {
	Binary struct {
		Encoding string   `json:"encoding"`
		Data     []string `json:"data"`
	} `json:"binary"`
	Parsed []ParsedPrice `json:"parsed"`
}

// ParsedPrice 解析后的价格数据
type ParsedPrice struct {
	ID    string `json:"id"`
	Price struct {
		Price       string `json:"price"`
		Conf        string `json:"conf"`
		Expo        int    `json:"expo"`
		PublishTime int64  `json:"publish_time"`
	} `json:"price"`
	EmaPrice struct {
		Price       string `json:"price"`
		Conf        string `json:"conf"`
		Expo        int    `json:"expo"`
		PublishTime int64  `json:"publish_time"`
	} `json:"ema_price"`
	Metadata struct {
		Slot               int64 `json:"slot"`
		ProofAvailableTime int64 `json:"proof_available_time"`
		PrevPublishTime    int64 `json:"prev_publish_time"`
	} `json:"metadata"`
}

// PythClient SSE客户端，支持断线重连
type PythClient struct {
	feedIDs    []string
	done       chan struct{}
	reconnect  chan struct{}
	mu         sync.Mutex
	isRunning  bool
	isStopping bool
	resp       *http.Response
}

// NewPythClient 创建新的Pyth客户端
func NewPythClient(feedIDs []string) *PythClient {
	return &PythClient{
		feedIDs:   feedIDs,
		done:      make(chan struct{}),
		reconnect: make(chan struct{}, 1),
	}
}

// buildURL 构建SSE请求URL
func (c *PythClient) buildURL() string {
	var params []string
	for _, id := range c.feedIDs {
		params = append(params, fmt.Sprintf("ids[]=0x%s", id))
	}
	return fmt.Sprintf("%s?%s", HermesSSEURL, strings.Join(params, "&"))
}

// connect 建立SSE连接
func (c *PythClient) connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	url := c.buildURL()
	log.Printf("连接地址: %s", url)

	// 创建HTTP请求
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("创建请求失败: %v", err)
	}

	// 设置SSE相关头部
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Connection", "keep-alive")

	// 发送请求
	client := &http.Client{
		Timeout: 0, // SSE不设置超时
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("连接失败: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return fmt.Errorf("HTTP状态码错误: %d", resp.StatusCode)
	}

	c.resp = resp
	log.Println("SSE连接成功!")
	log.Printf("订阅Feed IDs: %v", c.feedIDs)

	return nil
}

// reconnectWithBackoff 带退避策略的重连
func (c *PythClient) reconnectWithBackoff() {
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

// calculatePrice 根据 price 和 expo 计算实际价格
func calculatePrice(priceStr string, expo int) float64 {
	var price float64
	fmt.Sscanf(priceStr, "%f", &price)
	return price * math.Pow(10, float64(expo))
}

// readMessages 读取SSE消息循环
func (c *PythClient) readMessages() {
	for {
		select {
		case <-c.done:
			return
		default:
		}

		c.mu.Lock()
		resp := c.resp
		c.mu.Unlock()

		if resp == nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		scanner := bufio.NewScanner(resp.Body)
		// 增加缓冲区大小，因为SSE消息可能很大
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024)

		for scanner.Scan() {
			select {
			case <-c.done:
				return
			default:
			}

			line := scanner.Text()

			// SSE消息格式: "data:JSON数据"
			if !strings.HasPrefix(line, "data:") {
				continue
			}

			// 提取JSON数据
			jsonData := strings.TrimPrefix(line, "data:")

			// 解析响应
			var response SSEResponse
			if err := json.Unmarshal([]byte(jsonData), &response); err != nil {
				log.Printf("解析JSON失败: %v", err)
				continue
			}

			// 处理每个价格更新
			for _, parsed := range response.Parsed {
				symbol, ok := feedIDToSymbol[parsed.ID]
				if !ok {
					symbol = parsed.ID[:8] + "..." // 如果找不到映射，显示截断的ID
				}

				// 计算实际价格
				price := calculatePrice(parsed.Price.Price, parsed.Price.Expo)

				// 格式化输出
				log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
				log.Printf("股票代币: %s", symbol)
				log.Printf("最新价格: %.4f", price)
				log.Printf("时间戳: %d (%s)", parsed.Price.PublishTime,
					time.Unix(parsed.Price.PublishTime, 0).Format("2006-01-02 15:04:05"))
			}
		}

		// 检查扫描器错误
		if err := scanner.Err(); err != nil {
			c.mu.Lock()
			isStopping := c.isStopping
			c.mu.Unlock()

			// 如果是正在停止，不打印错误也不重连
			if isStopping {
				return
			}

			log.Printf("读取消息错误: %v，准备重连...", err)
		} else {
			c.mu.Lock()
			isStopping := c.isStopping
			c.mu.Unlock()

			if isStopping {
				return
			}

			log.Println("SSE连接已断开，准备重连...")
		}

		// 触发重连
		select {
		case c.reconnect <- struct{}{}:
		default:
		}
		return
	}
}

// Start 启动客户端
func (c *PythClient) Start() error {
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
				if c.resp != nil {
					c.resp.Body.Close()
					c.resp = nil
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
func (c *PythClient) Stop() {
	c.mu.Lock()
	c.isRunning = false
	c.isStopping = true // 标记正在停止，避免打印错误日志
	c.mu.Unlock()

	close(c.done)

	c.mu.Lock()
	if c.resp != nil {
		c.resp.Body.Close()
		c.resp = nil
	}
	c.mu.Unlock()
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	log.Println("=== Pyth Network SSE 实时报价订阅 Demo（支持断线重连）===")
	log.Println("注意: SSE连接会在24小时后自动断开，客户端会自动重连")

	// 获取所有feed IDs
	var ids []string
	for symbol, id := range feedIDs {
		ids = append(ids, id)
		log.Printf("订阅 %s: %s", symbol, id)
	}

	// 创建Pyth客户端
	client := NewPythClient(ids)

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
