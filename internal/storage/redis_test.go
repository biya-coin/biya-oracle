// Package storage Redis存储模块单元测试
package storage

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
)

// setupTestRedis 创建测试用的Redis存储
func setupTestRedis(t *testing.T) (*RedisStorage, *miniredis.Miniredis) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("创建内存Redis失败: %v", err)
	}

	storage, err := NewRedisStorage(mr.Addr(), "", 0)
	if err != nil {
		t.Fatalf("创建Redis存储失败: %v", err)
	}

	return storage, mr
}

// TestNewRedisStorage_Success 测试成功创建Redis存储
func TestNewRedisStorage_Success(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("创建内存Redis失败: %v", err)
	}
	defer mr.Close()

	storage, err := NewRedisStorage(mr.Addr(), "", 0)
	assert.NoError(t, err, "应该成功创建Redis存储")
	assert.NotNil(t, storage, "存储对象应该不为nil")

	// 清理
	storage.Close()
}

// TestNewRedisStorage_ConnectionFailed 测试连接失败
func TestNewRedisStorage_ConnectionFailed(t *testing.T) {
	// 使用无效地址
	storage, err := NewRedisStorage("invalid:6379", "", 0)
	assert.Error(t, err, "连接失败应该返回错误")
	assert.Nil(t, storage, "连接失败时应该返回nil")
}

// TestSetClosePrice_Success 测试成功存储收盘价
func TestSetClosePrice_Success(t *testing.T) {
	storage, mr := setupTestRedis(t)
	defer mr.Close()
	defer storage.Close()

	ctx := context.Background()
	symbol := "AAPLX"
	price := 100.5
	timestamp := time.Now()

	err := storage.SetClosePrice(ctx, symbol, price, timestamp)
	assert.NoError(t, err, "应该成功存储收盘价")

	// 验证数据已存储
	stored, err := storage.GetClosePrice(ctx, symbol)
	assert.NoError(t, err, "应该能获取收盘价")
	assert.NotNil(t, stored, "收盘价应该存在")
	if stored != nil {
		assert.Equal(t, price, stored.Price, "价格应该正确")
		assert.True(t, stored.Timestamp.Equal(timestamp), "时间戳应该正确")
		assert.Equal(t, "cex", stored.Source, "来源应该正确")
	}
}

// TestSetClosePrice_Overwrite 测试覆盖更新
func TestSetClosePrice_Overwrite(t *testing.T) {
	storage, mr := setupTestRedis(t)
	defer mr.Close()
	defer storage.Close()

	ctx := context.Background()
	symbol := "AAPLX"

	// 第一次存储
	err := storage.SetClosePrice(ctx, symbol, 100.0, time.Now())
	assert.NoError(t, err, "第一次存储应该成功")

	// 第二次存储（覆盖）
	newPrice := 100.5
	newTimestamp := time.Now()
	err = storage.SetClosePrice(ctx, symbol, newPrice, newTimestamp)
	assert.NoError(t, err, "第二次存储应该成功")

	// 验证已覆盖
	stored, err := storage.GetClosePrice(ctx, symbol)
	assert.NoError(t, err, "应该能获取收盘价")
	assert.NotNil(t, stored, "收盘价应该存在")
	if stored != nil {
		assert.Equal(t, newPrice, stored.Price, "价格应该被覆盖")
		assert.True(t, stored.Timestamp.Equal(newTimestamp), "时间戳应该被覆盖")
	}
}

// TestGetClosePrice_NotExists 测试获取不存在的收盘价
func TestGetClosePrice_NotExists(t *testing.T) {
	storage, mr := setupTestRedis(t)
	defer mr.Close()
	defer storage.Close()

	ctx := context.Background()
	symbol := "UNKNOWN"

	stored, err := storage.GetClosePrice(ctx, symbol)
	assert.NoError(t, err, "不存在时不应该返回错误")
	assert.Nil(t, stored, "不存在时应该返回nil")
}

// TestGetLatestClosePrice_Success 测试获取最新收盘价
func TestGetLatestClosePrice_Success(t *testing.T) {
	storage, mr := setupTestRedis(t)
	defer mr.Close()
	defer storage.Close()

	ctx := context.Background()
	symbol := "AAPLX"
	price := 100.5
	timestamp := time.Now()

	// 存储收盘价
	err := storage.SetClosePrice(ctx, symbol, price, timestamp)
	assert.NoError(t, err, "应该成功存储收盘价")

	// 获取最新收盘价
	stored, err := storage.GetLatestClosePrice(ctx, symbol)
	assert.NoError(t, err, "应该能获取最新收盘价")
	assert.NotNil(t, stored, "收盘价应该存在")
	if stored != nil {
		assert.Equal(t, price, stored.Price, "价格应该正确")
	}
}

// TestSetClosePrice_TTL 测试TTL设置（7天）
func TestSetClosePrice_TTL(t *testing.T) {
	storage, mr := setupTestRedis(t)
	defer mr.Close()
	defer storage.Close()

	ctx := context.Background()
	symbol := "AAPLX"

	// 存储收盘价
	err := storage.SetClosePrice(ctx, symbol, 100.5, time.Now())
	assert.NoError(t, err, "应该成功存储收盘价")

	// 验证TTL已设置（应该接近7天，允许一些误差）
	ttl := mr.TTL(keyPrefixClosePrice + symbol)
	assert.Greater(t, ttl, 6*24*time.Hour, "TTL应该大于6天")
	assert.LessOrEqual(t, ttl, 7*24*time.Hour, "TTL应该小于等于7天")
}

// TestSetClosePrice_MultipleSymbols 测试多个股票独立存储
func TestSetClosePrice_MultipleSymbols(t *testing.T) {
	storage, mr := setupTestRedis(t)
	defer mr.Close()
	defer storage.Close()

	ctx := context.Background()

	// 存储多个股票的收盘价
	symbols := []string{"AAPLX", "NVDAX", "TSLAX"}
	prices := []float64{100.5, 200.5, 300.5}

	for i, symbol := range symbols {
		err := storage.SetClosePrice(ctx, symbol, prices[i], time.Now())
		assert.NoError(t, err, "应该成功存储收盘价")
	}

	// 验证每个股票的价格都正确
	for i, symbol := range symbols {
		stored, err := storage.GetClosePrice(ctx, symbol)
		assert.NoError(t, err, "应该能获取收盘价")
		assert.NotNil(t, stored, "收盘价应该存在")
		if stored != nil {
			assert.Equal(t, prices[i], stored.Price, "价格应该正确")
		}
	}
}

// TestGetClosePrice_JSONParseError 测试JSON解析错误（模拟损坏的数据）
func TestGetClosePrice_JSONParseError(t *testing.T) {
	storage, mr := setupTestRedis(t)
	defer mr.Close()
	defer storage.Close()

	ctx := context.Background()
	symbol := "AAPLX"
	key := keyPrefixClosePrice + symbol

	// 直接写入损坏的JSON数据
	mr.Set(key, "invalid json")

	// 尝试获取应该返回错误
	stored, err := storage.GetClosePrice(ctx, symbol)
	assert.Error(t, err, "JSON解析错误应该返回错误")
	assert.Nil(t, stored, "解析失败时应该返回nil")
	assert.Contains(t, err.Error(), "解析收盘价失败", "错误信息应该包含提示")
}

// TestClose 测试关闭连接
func TestClose(t *testing.T) {
	storage, mr := setupTestRedis(t)
	defer mr.Close()

	err := storage.Close()
	assert.NoError(t, err, "关闭连接应该成功")
}
