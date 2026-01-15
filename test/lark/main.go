// Lark 告警通知 Demo
// 通过 Lark 群机器人 Webhook 发送告警消息
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

const (
	// Lark 机器人 Webhook 地址
	LarkWebhookURL = "https://open.larksuite.com/open-apis/bot/v2/hook/5d5f6b52-cc10-4d37-a278-4af224288a9a"
)

// AlertType 告警类型
type AlertType string

const (
	AlertTypePriceAnomaly    AlertType = "PRICE_ANOMALY"     // 价格异常
	AlertTypeConnectionLost  AlertType = "CONNECTION_LOST"   // 连接断开
	AlertTypeDataSourceError AlertType = "DATA_SOURCE_ERROR" // 数据源错误
	AlertTypeSwitchDiff      AlertType = "SWITCH_DIFF"       // 切换差异过大
	AlertTypeOracleError     AlertType = "ORACLE_ERROR"      // 预言机异常
	AlertTypeTest            AlertType = "TEST"              // 测试告警
)

// LarkMessage Lark 消息结构
type LarkMessage struct {
	MsgType string      `json:"msg_type"`
	Content interface{} `json:"content,omitempty"`
	Card    interface{} `json:"card,omitempty"` // 卡片消息使用 card 字段
}

// TextContent 文本消息内容
type TextContent struct {
	Text string `json:"text"`
}

// CardContent 卡片消息内容
type CardContent struct {
	Config   CardConfig    `json:"config"`
	Header   CardHeader    `json:"header"`
	Elements []CardElement `json:"elements"`
}

// CardConfig 卡片配置
type CardConfig struct {
	WideScreenMode bool `json:"wide_screen_mode"`
}

// CardHeader 卡片头部
type CardHeader struct {
	Title    CardText `json:"title"`
	Template string   `json:"template"` // blue, wathet, turquoise, green, yellow, orange, red, carmine, violet, purple, indigo, grey
}

// CardText 卡片文本
type CardText struct {
	Tag     string `json:"tag"`
	Content string `json:"content"`
}

// CardElement 卡片元素
type CardElement struct {
	Tag    string    `json:"tag"`
	Text   *CardText `json:"text,omitempty"`
	Fields []Field   `json:"fields,omitempty"`
}

// Field 字段
type Field struct {
	IsShort bool     `json:"is_short"`
	Text    CardText `json:"text"`
}

// LarkResponse Lark API 响应
type LarkResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
	} `json:"data"`
}

// AlertManager 告警管理器
type AlertManager struct {
	webhookURL string
	httpClient *http.Client
}

// NewAlertManager 创建告警管理器
func NewAlertManager(webhookURL string) *AlertManager {
	return &AlertManager{
		webhookURL: webhookURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SendTextAlert 发送简单文本告警
func (m *AlertManager) SendTextAlert(message string) error {
	msg := LarkMessage{
		MsgType: "text",
		Content: TextContent{
			Text: message,
		},
	}
	return m.send(msg)
}

// SendCardAlert 发送卡片告警（更美观）
func (m *AlertManager) SendCardAlert(alertType AlertType, symbol string, details map[string]string) error {
	// 根据告警类型设置颜色
	var template string
	var title string
	switch alertType {
	case AlertTypePriceAnomaly:
		template = "orange"
		title = "⚠️ 价格异常告警"
	case AlertTypeConnectionLost:
		template = "red"
		title = "🔴 连接断开告警"
	case AlertTypeDataSourceError:
		template = "red"
		title = "❌ 数据源错误"
	case AlertTypeSwitchDiff:
		template = "orange"
		title = "⚠️ 切换差异过大"
	case AlertTypeOracleError:
		template = "red"
		title = "❌ 预言机异常"
	case AlertTypeTest:
		template = "blue"
		title = "🔔 测试告警"
	default:
		template = "grey"
		title = "📢 系统告警"
	}

	// 构建字段
	var fields []Field
	if symbol != "" {
		fields = append(fields, Field{
			IsShort: true,
			Text: CardText{
				Tag:     "lark_md",
				Content: fmt.Sprintf("**股票代币**\n%s", symbol),
			},
		})
	}
	fields = append(fields, Field{
		IsShort: true,
		Text: CardText{
			Tag:     "lark_md",
			Content: fmt.Sprintf("**告警时间**\n%s", time.Now().Format("2006-01-02 15:04:05")),
		},
	})

	for key, value := range details {
		fields = append(fields, Field{
			IsShort: false,
			Text: CardText{
				Tag:     "lark_md",
				Content: fmt.Sprintf("**%s**\n%s", key, value),
			},
		})
	}

	msg := LarkMessage{
		MsgType: "interactive",
		Card: map[string]interface{}{
			"config": CardConfig{
				WideScreenMode: true,
			},
			"header": CardHeader{
				Title: CardText{
					Tag:     "plain_text",
					Content: title,
				},
				Template: template,
			},
			"elements": []CardElement{
				{
					Tag:    "div",
					Fields: fields,
				},
			},
		},
	}

	return m.send(msg)
}

// send 发送消息到 Lark
func (m *AlertManager) send(msg LarkMessage) error {
	jsonData, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("序列化消息失败: %v", err)
	}

	resp, err := m.httpClient.Post(m.webhookURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("发送请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %v", err)
	}

	var larkResp LarkResponse
	if err := json.Unmarshal(body, &larkResp); err != nil {
		return fmt.Errorf("解析响应失败: %v, body: %s", err, string(body))
	}

	if larkResp.Code != 0 {
		return fmt.Errorf("Lark API 错误: code=%d, msg=%s", larkResp.Code, larkResp.Msg)
	}

	return nil
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	log.Println("=== Lark 告警通知 Demo ===")

	// 创建告警管理器
	alertManager := NewAlertManager(LarkWebhookURL)

	// 测试1: 发送简单文本告警
	log.Println("发送文本告警...")
	err := alertManager.SendTextAlert("【测试告警】这是一条来自股票代币报价系统的测试消息\n时间: " + time.Now().Format("2006-01-02 15:04:05"))
	if err != nil {
		log.Printf("发送文本告警失败: %v", err)
	} else {
		log.Println("文本告警发送成功!")
	}

	// 等待1秒，避免发送过快
	time.Sleep(1 * time.Second)

	// 测试2: 发送卡片告警
	log.Println("发送卡片告警...")
	err = alertManager.SendCardAlert(AlertTypeTest, "AAPLX", map[string]string{
		"告警详情": "这是一条测试告警，用于验证 Lark 机器人告警功能是否正常",
		"数据源":   "CEX / Pyth / Gate.io",
		"系统状态": "正常运行中",
	})
	if err != nil {
		log.Printf("发送卡片告警失败: %v", err)
	} else {
		log.Println("卡片告警发送成功!")
	}

	// 等待1秒
	time.Sleep(1 * time.Second)

	// 测试3: 模拟真实告警场景
	log.Println("发送模拟真实告警...")
	err = alertManager.SendCardAlert(AlertTypePriceAnomaly, "NVDAX", map[string]string{
		"告警详情":   "Pyth 价格跳变超过 10%",
		"当前价格":   "185.50",
		"上次价格":   "168.23",
		"价格跳变":   "10.26%",
		"处理方式":   "已启用降级权重: 收盘价60% + Gate40%",
	})
	if err != nil {
		log.Printf("发送模拟告警失败: %v", err)
	} else {
		log.Println("模拟告警发送成功!")
	}

	log.Println("告警测试完成!")
}
