// 股票代币报价系统 - 主程序入口
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"biya-oracle/internal/config"
	"biya-oracle/internal/oracle"
	"biya-oracle/internal/storage"
)

func main() {
	// 解析命令行参数
	configPath := flag.String("config", "config.yaml", "配置文件路径")
	flag.Parse()

	// 设置日志格式
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)

	log.Println("========================================")
	log.Println("  股票代币报价系统")
	log.Println("========================================")

	// 加载配置
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	log.Printf("配置加载完成: %d 个股票代币", len(cfg.StockTokens))

	// 初始化Redis存储
	redisStorage, err := storage.NewRedisStorage(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
	if err != nil {
		log.Fatalf("初始化Redis失败: %v", err)
	}
	defer redisStorage.Close()
	log.Println("Redis连接成功")

	// 创建服务
	svc := oracle.NewService(cfg, redisStorage)

	// 设置信号处理
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 启动服务
	if err := svc.Start(ctx); err != nil {
		log.Fatalf("启动服务失败: %v", err)
	}

	// 等待退出信号
	sig := <-sigChan
	log.Printf("收到信号 %v，正在关闭...", sig)

	// 停止服务
	svc.Stop()

	log.Println("程序退出")
}