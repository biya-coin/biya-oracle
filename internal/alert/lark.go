// Package alert Lark告警模块
package alert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"biya-oracle/internal/types"
)

// LarkAlert Lark告警管理器
type LarkAlert struct {
	webhookURL string
	httpClient *http.Client
}

// NewLarkAlert 创建Lark告警管理器
func NewLarkAlert(webhookURL string) *LarkAlert {
	return &LarkAlert{
		webhookURL: webhookURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// larkMessage Lark消息结构
type larkMessage struct {
	MsgType string      `json:"msg_type"`
	Content interface{} `json:"content,omitempty"`
	Card    interface{} `json:"card,omitempty"`
}

// larkTextContent 文本消息内容
type larkTextContent struct {
	Text string `json:"text"`
}

// larkResponse Lark API响应
type larkResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

// SendAlert 发送告警
func (l *LarkAlert) SendAlert(alertType types.AlertType, symbol string, details map[string]string) {
	// 异步发送，不阻塞主流程
	go func() {
		if err := l.sendCardAlert(alertType, symbol, details); err != nil {
			log.Printf("[告警] 发送Lark告警失败: %v", err)
			// 降级为文本告警
			if err := l.sendTextAlert(alertType, symbol, details); err != nil {
				log.Printf("[告警] 发送Lark文本告警也失败: %v", err)
			}
		}
	}()
}

// sendTextAlert 发送文本告警
func (l *LarkAlert) sendTextAlert(alertType types.AlertType, symbol string, details map[string]string) error {
	text := fmt.Sprintf("【%s】股票代币: %s\n时间: %s\n",
		getAlertTitle(alertType),
		symbol,
		time.Now().Format("2006-01-02 15:04:05"))

	for key, value := range details {
		text += fmt.Sprintf("%s: %s\n", key, value)
	}

	msg := larkMessage{
		MsgType: "text",
		Content: larkTextContent{Text: text},
	}

	return l.send(msg)
}

// sendCardAlert 发送卡片告警
func (l *LarkAlert) sendCardAlert(alertType types.AlertType, symbol string, details map[string]string) error {
	template, title := getAlertStyle(alertType)

	// 构建字段
	fields := []map[string]interface{}{}
	
	if symbol != "" {
		fields = append(fields, map[string]interface{}{
			"is_short": true,
			"text": map[string]interface{}{
				"tag":     "lark_md",
				"content": fmt.Sprintf("**股票代币**\n%s", symbol),
			},
		})
	}

	fields = append(fields, map[string]interface{}{
		"is_short": true,
		"text": map[string]interface{}{
			"tag":     "lark_md",
			"content": fmt.Sprintf("**告警时间**\n%s", time.Now().Format("2006-01-02 15:04:05")),
		},
	})

	for key, value := range details {
		fields = append(fields, map[string]interface{}{
			"is_short": false,
			"text": map[string]interface{}{
				"tag":     "lark_md",
				"content": fmt.Sprintf("**%s**\n%s", key, value),
			},
		})
	}

	msg := larkMessage{
		MsgType: "interactive",
		Card: map[string]interface{}{
			"config": map[string]interface{}{
				"wide_screen_mode": true,
			},
			"header": map[string]interface{}{
				"title": map[string]interface{}{
					"tag":     "plain_text",
					"content": title,
				},
				"template": template,
			},
			"elements": []map[string]interface{}{
				{
					"tag":    "div",
					"fields": fields,
				},
			},
		},
	}

	return l.send(msg)
}

// send 发送消息到Lark
func (l *LarkAlert) send(msg larkMessage) error {
	jsonData, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("序列化消息失败: %w", err)
	}

	resp, err := l.httpClient.Post(l.webhookURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}

	var larkResp larkResponse
	if err := json.Unmarshal(body, &larkResp); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}

	if larkResp.Code != 0 {
		return fmt.Errorf("Lark API错误: code=%d, msg=%s", larkResp.Code, larkResp.Msg)
	}

	return nil
}

// getAlertStyle 获取告警样式
func getAlertStyle(alertType types.AlertType) (template, title string) {
	switch alertType {
	case types.AlertTypePriceAnomaly:
		return "orange", "⚠️ 价格异常告警"
	case types.AlertTypeConnectionLost:
		return "red", "🔴 连接断开告警"
	case types.AlertTypeDataSourceError:
		return "red", "❌ 数据源错误"
	case types.AlertTypeSwitchDiff:
		return "orange", "⚠️ 切换差异过大"
	case types.AlertTypeOracleError:
		return "orange", "⚠️ 预言机异常"
	case types.AlertTypePriceInvalid:
		return "red", "❌ 价格无效"
	case types.AlertTypeTimestampExpired:
		return "orange", "⚠️ 时间戳过期"
	case types.AlertTypeBothOracleError:
		return "red", "🔴 两个预言机都异常"
	case types.AlertTypeStockStatusError:
		return "red", "🔴 股票状态异常"
	case types.AlertTypeReconnectFailed:
		return "red", "🔴 数据源重连失败"
	case types.AlertTypeStatusQueryFailed:
		return "red", "🔴 状态查询失败"
	default:
		return "grey", "📢 系统告警"
	}
}

// getAlertTitle 获取告警标题（用于文本消息）
func getAlertTitle(alertType types.AlertType) string {
	_, title := getAlertStyle(alertType)
	return title
}
