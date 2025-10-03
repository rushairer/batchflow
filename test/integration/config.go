package main

import (
	"log"
	"os"
	"strconv"
	"time"
)

// 环境变量解析辅助函数
func parseIntEnv(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func parseBoolEnv(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func parseDurationEnv(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func loadConfig() TestConfig {
	// 统一从环境变量读取配置，docker-compose为唯一配置源
	config := TestConfig{
		TestDuration:      parseDurationEnv("TEST_DURATION", 1800*time.Second), // 30分钟默认
		ConcurrentWorkers: parseIntEnv("CONCURRENT_WORKERS", 10),
		RecordsPerWorker:  parseIntEnv("RECORDS_PER_WORKER", 2000),
		BatchSize:         uint32(parseIntEnv("BATCH_SIZE", 200)),
		BufferSize:        uint32(parseIntEnv("BUFFER_SIZE", 5000)),
		FlushInterval:     parseDurationEnv("FLUSH_INTERVAL", 100*time.Millisecond),
		// Prometheus 配置
		PrometheusEnabled: parseBoolEnv("PROMETHEUS_ENABLED", true), // 默认启用
		PrometheusPort:    parseIntEnv("PROMETHEUS_PORT", 8080),     // 默认端口 8080
	}

	log.Printf("📋 已加载测试配置：")
	log.Printf("   测试时长：%v", config.TestDuration)
	log.Printf("   并发工作线程：%d", config.ConcurrentWorkers)
	log.Printf("   每线程记录数：%d", config.RecordsPerWorker)
	log.Printf("   批大小：%d", config.BatchSize)
	log.Printf("   缓冲区大小：%d", config.BufferSize)
	log.Printf("   刷新间隔：%v", config.FlushInterval)
	log.Printf("   Prometheus启用：%v", config.PrometheusEnabled)
	if config.PrometheusEnabled {
		log.Printf("   Prometheus端口：%d", config.PrometheusPort)
	}

	return config
}
