# 市场状态模拟服务

这是一个用于测试的本地市场状态模拟服务，可以随时控制市场状态，从而触发数据源切换（CEX ↔ 加权合成），方便测试5%差异检测和告警机制。

## 功能特性

- ✅ **与真实API格式一致**：返回格式与生产环境API完全一致，程序无需修改即可使用
- ✅ **随时控制状态**：通过HTTP接口随时修改市场状态
- ✅ **状态持久化**：支持将状态保存到文件，重启后自动恢复
- ✅ **简单易用**：提供Web界面和命令行工具

## 快速开始

### 1. 启动模拟服务

```bash
cd cmd/mock-market-status
go run main.go
```

或者指定端口和状态文件：

```bash
go run main.go -port 8888 -state-file ./market_status.json
```

### 2. 修改配置文件

在 `config.yaml` 中，将 `cex.market_status_url` 指向本地模拟服务：

```yaml
cex:
  market_status_url: "http://localhost:8888/market/get"  # 使用本地模拟服务
  stock_status_url: "https://japi-stock-quote.biya.in/quote/briefs"
  ws_base_url: "wss://market-ws.biya.in/market/subscribe"
```

### 3. 启动程序

正常启动报价程序，程序会从本地模拟服务获取市场状态。

### 4. 控制市场状态

#### 方式1：使用curl命令

```bash
# 设置为休市（触发切换到加权合成）
curl -X POST http://localhost:8888/status/set \
  -H "Content-Type: application/json" \
  -d '{"trading_status": "MARKET_CLOSED"}'

# 设置为交易中（触发切换到CEX）
curl -X POST http://localhost:8888/status/set \
  -H "Content-Type: application/json" \
  -d '{"trading_status": "TRADING"}'

# 设置为盘前交易（触发切换到加权合成）
curl -X POST http://localhost:8888/status/set \
  -H "Content-Type: application/json" \
  -d '{"trading_status": "PRE_HOUR_TRADING"}'
```

#### 方式2：使用Web界面

访问 `http://localhost:8888/` 查看帮助页面和API文档。

#### 方式3：查看当前状态

```bash
curl http://localhost:8888/status
```

## API接口

### 1. GET /market/get

返回市场状态（与真实API格式一致，程序使用此接口）

**响应示例：**
```json
{
  "code": 200,
  "msg": "success",
  "data": [
    {
      "market": "US",
      "status_cn": "市场休市",
      "open_time": "09:30",
      "trading_status": "MARKET_CLOSED",
      "status": "Closed"
    }
  ],
  "result": true
}
```

### 2. GET /status

查看当前市场状态（用于调试）

**响应示例：**
```json
{
  "code": 200,
  "msg": "success",
  "result": true,
  "current": {
    "market": "US",
    "status_cn": "市场休市",
    "open_time": "09:30",
    "trading_status": "MARKET_CLOSED",
    "status": "Closed"
  }
}
```

### 3. POST /status/set

设置市场状态（用于触发数据源切换）

**请求体：**
```json
{
  "trading_status": "MARKET_CLOSED",  // 必需：交易状态
  "status_cn": "市场休市",              // 可选：中文状态（不提供则自动生成）
  "status": "Closed"                   // 可选：状态（不提供则自动生成）
}
```

**响应示例：**
```json
{
  "code": 200,
  "msg": "success",
  "result": true,
  "old": "TRADING",
  "new": "MARKET_CLOSED",
  "message": "市场状态已从 TRADING 更新为 MARKET_CLOSED"
}
```

## 常用状态值

| 状态值 | 说明 | 数据源选择 |
|--------|------|-----------|
| `TRADING` | 交易中 | CEX |
| `MARKET_CLOSED` | 市场休市 | 加权合成 |
| `OVERNIGHT` | 夜盘交易 | CEX |
| `PRE_HOUR_TRADING` | 盘前交易 | 加权合成 |
| `POST_HOUR_TRADING` | 盘后交易 | 加权合成 |
| `CLOSING` | 收盘中 | 加权合成 |
| `NOT_YET_OPEN` | 尚未开盘 | 加权合成 |

## 测试场景

### 场景1：测试数据源切换

1. 启动模拟服务
2. 启动报价程序（配置指向本地模拟服务）
3. 等待程序稳定运行（使用CEX数据源）
4. 设置市场状态为 `MARKET_CLOSED`（触发切换到加权合成）
5. 观察日志，确认数据源已切换
6. 观察是否触发5%差异检测和告警

### 场景2：测试5%差异告警

1. 确保程序使用CEX数据源，记录当前价格
2. 设置市场状态为 `MARKET_CLOSED`（切换到加权合成）
3. 如果加权合成价格与CEX价格差异 > 5%，应该触发告警
4. 如果差异持续 > 5%，程序应该暂停报价

### 场景3：测试价格恢复

1. 在场景2的基础上，等待加权合成价格恢复（或手动调整）
2. 当价格差异 < 5% 时，程序应该自动恢复报价

## 状态持久化

如果指定了 `-state-file` 参数，状态会保存到文件中：

```bash
go run main.go -state-file ./market_status.json
```

- 启动时自动加载文件中的状态
- 每次修改状态时自动保存到文件
- 重启服务后状态保持不变

## 注意事项

1. **仅用于测试**：此服务仅用于测试环境，不要在生产环境使用
2. **端口冲突**：确保端口8888未被占用，或使用 `-port` 参数指定其他端口
3. **配置文件**：修改 `config.yaml` 后需要重启报价程序
4. **数据源切换**：切换市场状态后，程序会在下一次轮询（3秒）时检测并切换数据源

## 故障排查

### 问题1：程序无法连接到模拟服务

- 检查模拟服务是否已启动：`curl http://localhost:8888/status`
- 检查 `config.yaml` 中的 `market_status_url` 是否正确
- 检查防火墙是否阻止了连接

### 问题2：状态修改后程序没有切换

- 检查程序日志，确认是否收到了新的市场状态
- 程序每3秒轮询一次，等待3秒后再次检查
- 检查程序日志中的市场状态值是否正确

### 问题3：状态文件无法加载

- 检查文件路径是否正确
- 检查文件格式是否为有效的JSON
- 检查文件权限是否可读
