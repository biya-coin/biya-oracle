// Package storage Redis存储模块
package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// Key前缀
	keyPrefixClosePrice = "oracle:close_price:" // CEX收盘价快照

	// TTL
	ttlClosePrice = 7 * 24 * time.Hour // 收盘价保留7天
)

// StoredPrice 存储的价格数据
type StoredPrice struct {
	Price     float64   `json:"price"`
	Timestamp time.Time `json:"timestamp"`
	Source    string    `json:"source"`
}

// RedisStorage Redis存储
type RedisStorage struct {
	client *redis.Client
}

// NewRedisStorage 创建Redis存储
func NewRedisStorage(addr, password string, db int) (*RedisStorage, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("Redis连接失败: %w", err)
	}

	return &RedisStorage{client: client}, nil
}

// Close 关闭连接
func (s *RedisStorage) Close() error {
	return s.client.Close()
}

// SetClosePrice 存储CEX收盘价（保留7天，每个股票只保留最新一条，覆盖更新）
func (s *RedisStorage) SetClosePrice(ctx context.Context, symbol string, price float64, timestamp time.Time) error {
	// Key格式: oracle:close_price:{symbol}，每个股票只有一个key，覆盖更新
	key := keyPrefixClosePrice + symbol
	data := StoredPrice{
		Price:     price,
		Timestamp: timestamp,
		Source:    "cex",
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("序列化收盘价失败: %w", err)
	}

	return s.client.Set(ctx, key, jsonData, ttlClosePrice).Err()
}

// GetClosePrice 获取CEX收盘价（每个股票只有一条）
func (s *RedisStorage) GetClosePrice(ctx context.Context, symbol string) (*StoredPrice, error) {
	key := keyPrefixClosePrice + symbol

	data, err := s.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // 不存在
		}
		return nil, fmt.Errorf("获取收盘价失败: %w", err)
	}

	var price StoredPrice
	if err := json.Unmarshal(data, &price); err != nil {
		return nil, fmt.Errorf("解析收盘价失败: %w", err)
	}

	return &price, nil
}

// GetLatestClosePrice 获取最新的CEX收盘价（每个股票只有一条，直接获取）
func (s *RedisStorage) GetLatestClosePrice(ctx context.Context, symbol string) (*StoredPrice, error) {
	return s.GetClosePrice(ctx, symbol)
}
