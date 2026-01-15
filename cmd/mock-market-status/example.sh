#!/bin/bash

# 市场状态模拟服务使用示例

BASE_URL="http://localhost:8888"

echo "=== 市场状态模拟服务使用示例 ==="
echo ""

# 1. 查看当前状态
echo "1. 查看当前市场状态："
curl -s "$BASE_URL/status" | jq '.'
echo ""

# 2. 设置为休市（触发切换到加权合成）
echo "2. 设置为休市（触发切换到加权合成）："
curl -X POST "$BASE_URL/status/set" \
  -H "Content-Type: application/json" \
  -d '{"trading_status": "MARKET_CLOSED"}' | jq '.'
echo ""

sleep 2

# 3. 再次查看状态
echo "3. 再次查看状态："
curl -s "$BASE_URL/status" | jq '.'
echo ""

# 4. 设置为交易中（触发切换到CEX）
echo "4. 设置为交易中（触发切换到CEX）："
curl -X POST "$BASE_URL/status/set" \
  -H "Content-Type: application/json" \
  -d '{"trading_status": "TRADING"}' | jq '.'
echo ""

sleep 2

# 5. 查看程序使用的接口（与真实API格式一致）
echo "5. 查看程序使用的接口（/market/get）："
curl -s "$BASE_URL/market/get" | jq '.'
echo ""

# 6. 设置为盘前交易（触发切换到加权合成）
echo "6. 设置为盘前交易（触发切换到加权合成）："
curl -X POST "$BASE_URL/status/set" \
  -H "Content-Type: application/json" \
  -d '{"trading_status": "PRE_HOUR_TRADING"}' | jq '.'
echo ""

echo "=== 示例完成 ==="
