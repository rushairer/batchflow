package main

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rushairer/batchflow/v2"
)

// runRedisTests 运行 Redis 数据库测试
func runRedisTests(dsn string, config TestConfig, prometheusMetrics *PrometheusMetrics) []TestResult {
	var results []TestResult

	// 解析 Redis DSN
	opt, err := redis.ParseURL(dsn)
	if err != nil {
		log.Printf("❌ 解析 Redis DSN 失败：%v", err)
		return results
	}

	// 连接 Redis
	rdb := redis.NewClient(opt)
	defer rdb.Close()

	// 测试连接
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Printf("❌ Ping Redis 失败：%v", err)
		return results
	}

	// 记录活跃连接数
	if prometheusMetrics != nil {
		prometheusMetrics.UpdateActiveConnections("redis", 1) // Redis 单连接
	}

	// 运行不同的测试场景
	testCases := []struct {
		name     string
		testFunc func(*redis.Client, TestConfig, *PrometheusMetrics) TestResult
	}{
		{"高吞吐量测试", runRedisHighThroughputTest},
		{"并发工作线程测试", runRedisConcurrentWorkersTest},
		{"大批次测试", runRedisLargeBatchTest},
		{"内存压力测试", runRedisMemoryPressureTest},
		{"长时间运行测试", runRedisLongDurationTest},
	}

	for _, tc := range testCases {
		// 每个测试前清理 Redis 数据
		log.Printf("  🧹 在运行 %s 前清理 Redis 数据...", tc.name)
		if err := rdb.FlushDB(ctx).Err(); err != nil {
			log.Printf("❌ Failed to flush Redis DB before %s: %v", tc.name, err)
		}

		log.Printf("  🔄 在 Redis 上运行 %s...", tc.name)
		result := tc.testFunc(rdb, config, prometheusMetrics)
		result.TestName = tc.name
		result.Database = "redis"
		results = append(results, result)

		// 实时记录测试结果到 Prometheus
		if prometheusMetrics != nil {
			prometheusMetrics.RecordTestResult(result)
		}

		// 测试间隔，让系统恢复
		time.Sleep(5 * time.Second)
	}

	return results
}

// runRedisHighThroughputTest Redis 高吞吐量测试
func runRedisHighThroughputTest(rdb *redis.Client, config TestConfig, prometheusMetrics *PrometheusMetrics) TestResult {
	ctx := context.Background()

	executor := batchflow.NewRedisThrottledBatchExecutor(rdb)
	if prometheusMetrics != nil {
		metricsReporter := NewPrometheusMetricsReporter(prometheusMetrics, "redis", "高吞吐量测试")
		executor.WithMetricsReporter(metricsReporter)
	}
	batchFlow := batchflow.NewBatchFlow(ctx, config.BufferSize, config.BatchSize, config.FlushInterval, executor)

	// Redis 使用简单的 key-value schema
	schema := batchflow.NewSchema("redis_test",
		"cmd", "key", "value", "ex_flag", "ttl")

	startTime := time.Now()
	var recordCount int64
	var errors []string

	// 记录初始内存
	var m1 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	// 高吞吐量测试
	testCtx, cancel := context.WithTimeout(ctx, config.TestDuration)
	defer cancel()

	maxRecords := int64(config.ConcurrentWorkers * config.RecordsPerWorker)

	for i := int64(0); i < maxRecords; i++ {
		select {
		case <-testCtx.Done():
			goto TestComplete
		default:
			request := batchflow.NewRequest(schema).
				SetString("cmd", "SET").
				SetString("key", fmt.Sprintf("test:user:%d", i)).
				SetString("value", fmt.Sprintf(`{"id":%d,"name":"User_%d","email":"user_%d@example.com","active":%t}`, i, i, i, i%2 == 0)).
				SetString("ex_flag", "EX").
				SetInt64("ttl", 3600000) // 1小时 TTL (毫秒)

			batchStartTime := time.Now()

			if err := batchFlow.Submit(testCtx, request); err != nil {
				errors = append(errors, err.Error())
				if len(errors) > 100 {
					break
				}
			} else {
				recordCount++

				// 记录批处理时间和响应时间
				if prometheusMetrics != nil {
					batchDuration := time.Since(batchStartTime)
					prometheusMetrics.RecordBatchProcessTime("redis", config.BatchSize, batchDuration)
					prometheusMetrics.RecordResponseTime("redis", "set", batchDuration)
				}
			}

			// 定期更新实时指标
			if i%1000 == 0 {
				runtime.GC()

				// 更新实时 RPS 和内存使用
				if prometheusMetrics != nil {
					elapsed := time.Since(startTime).Seconds()
					if elapsed > 0 {
						currentRPS := float64(recordCount) / elapsed
						prometheusMetrics.UpdateCurrentRPS("redis", "高吞吐量测试", currentRPS)
					}

					// 更新内存使用
					var m runtime.MemStats
					runtime.ReadMemStats(&m)
					prometheusMetrics.UpdateMemoryUsage("redis", "高吞吐量测试",
						float64(m.Alloc)/1024/1024,
						float64(m.TotalAlloc)/1024/1024,
						float64(m.Sys)/1024/1024)
				}
			}
		}
	}

TestComplete:
	duration := time.Since(startTime)

	log.Printf("🔍 测试完成，提交了 %d 条记录，耗时 %v", recordCount, duration)
	log.Printf("🔍 等待Redis处理完成...")

	// 等待处理完成
	time.Sleep(5 * time.Second)

	// 记录最终内存
	var m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m2)

	log.Printf("🔍 开始统计Redis中的实际记录数...")

	// 查询 Redis 中的实际记录数
	actualRecords, countErr := getRedisRecordCount(rdb, ctx)
	if countErr != nil {
		log.Printf("❌ 统计实际记录数失败：%v", countErr)
		errors = append(errors, fmt.Sprintf("统计实际记录数失败：%v", countErr))
		actualRecords = -1
	} else {
		log.Printf("🔍 统计完成 - 提交: %d, 实际: %d", recordCount, actualRecords)
	}

	// 计算数据完整性
	dataIntegrityRate, integrityStatus, rpsValid, rpsNote := calculateDataIntegrity(recordCount, actualRecords)

	// 只有在数据完整性100%时才计算有效的RPS
	rps := 0.0
	if rpsValid && duration.Seconds() > 0 {
		rps = float64(recordCount) / duration.Seconds()
	}

	return TestResult{
		Duration:            duration,
		TotalRecords:        recordCount,
		ActualRecords:       actualRecords,
		DataIntegrityRate:   dataIntegrityRate,
		DataIntegrityStatus: integrityStatus,
		RecordsPerSecond:    rps,
		RPSValid:            rpsValid,
		RPSNote:             rpsNote,
		ConcurrentWorkers:   1,
		TestParameters: TestParams{
			BatchSize:       config.BatchSize,
			BufferSize:      config.BufferSize,
			FlushInterval:   config.FlushInterval,
			ExpectedRecords: int64(config.ConcurrentWorkers * config.RecordsPerWorker),
			TestDuration:    config.TestDuration,
		},
		MemoryUsage: MemoryStats{
			AllocMB:      calculateMemoryDiffMB(m2.Alloc, m1.Alloc),
			TotalAllocMB: calculateMemoryDiffMB(m2.TotalAlloc, m1.TotalAlloc),
			SysMB:        calculateMemoryDiffMB(m2.Sys, m1.Sys),
			NumGC:        m2.NumGC - m1.NumGC,
		},
		Errors:  errors,
		Success: len(errors) == 0 && rpsValid,
	}
}

// runRedisConcurrentWorkersTest Redis 并发工作线程测试
func runRedisConcurrentWorkersTest(rdb *redis.Client, config TestConfig, prometheusMetrics *PrometheusMetrics) TestResult {
	ctx := context.Background()

	batchFlow := batchflow.NewRedisBatchFlow(ctx, rdb, batchflow.PipelineConfig{
		BufferSize:    config.BufferSize,
		FlushSize:     config.BatchSize,
		FlushInterval: config.FlushInterval,
	})

	schema := batchflow.NewSchema("redis_test",
		"cmd", "key", "value", "ex_flag", "ttl")

	startTime := time.Now()
	var totalRecords int64
	var mu sync.Mutex
	var errors []string

	// 记录初始内存
	var m1 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	// 并发工作者测试
	var wg sync.WaitGroup
	batchSize := 100

	for workerID := 0; workerID < config.ConcurrentWorkers; workerID++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			workerRecords := 0
			baseID := int64(id * config.RecordsPerWorker)

			for batch := 0; batch < config.RecordsPerWorker; batch += batchSize {
				endIdx := batch + batchSize
				if endIdx > config.RecordsPerWorker {
					endIdx = config.RecordsPerWorker
				}

				for i := batch; i < endIdx; i++ {
					request := batchflow.NewRequest(schema).
						SetString("cmd", "SET").
						SetString("key", fmt.Sprintf("test:worker:%d:user:%d", id, baseID+int64(i))).
						SetString("value", fmt.Sprintf(`{"worker_id":%d,"user_id":%d,"name":"W%d_U%d","active":%t}`, id, baseID+int64(i), id, i, (id+i)%2 == 0)).
						SetString("ex_flag", "EX").
						SetInt64("ttl", 3600000) // 1小时 TTL

					if err := batchFlow.Submit(ctx, request); err != nil {
						mu.Lock()
						errors = append(errors, fmt.Sprintf("Worker %d: %v", id, err))
						mu.Unlock()
					} else {
						workerRecords++
					}
				}

				runtime.GC()
				time.Sleep(10 * time.Millisecond)
			}

			mu.Lock()
			totalRecords += int64(workerRecords)
			mu.Unlock()
		}(workerID)
	}

	wg.Wait()
	duration := time.Since(startTime)

	// 等待处理完成
	time.Sleep(5 * time.Second)

	// 记录最终内存
	var m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m2)

	// 查询 Redis 中的实际记录数
	actualRecords, countErr := getRedisRecordCount(rdb, ctx)
	if countErr != nil {
		mu.Lock()
		errors = append(errors, fmt.Sprintf("统计实际记录数失败：%v", countErr))
		mu.Unlock()
		actualRecords = -1
	}

	// 计算数据完整性
	dataIntegrityRate, integrityStatus, rpsValid, rpsNote := calculateDataIntegrity(totalRecords, actualRecords)

	// 只有在数据完整性100%时才计算有效的RPS
	rps := 0.0
	if rpsValid && duration.Seconds() > 0 {
		rps = float64(totalRecords) / duration.Seconds()
	}

	return TestResult{
		Duration:            duration,
		TotalRecords:        totalRecords,
		ActualRecords:       actualRecords,
		DataIntegrityRate:   dataIntegrityRate,
		DataIntegrityStatus: integrityStatus,
		RecordsPerSecond:    rps,
		RPSValid:            rpsValid,
		RPSNote:             rpsNote,
		ConcurrentWorkers:   config.ConcurrentWorkers,
		TestParameters: TestParams{
			BatchSize:       config.BatchSize,
			BufferSize:      config.BufferSize,
			FlushInterval:   config.FlushInterval,
			ExpectedRecords: int64(config.ConcurrentWorkers * config.RecordsPerWorker),
			TestDuration:    config.TestDuration,
		},
		MemoryUsage: MemoryStats{
			AllocMB:      calculateMemoryDiffMB(m2.Alloc, m1.Alloc),
			TotalAllocMB: calculateMemoryDiffMB(m2.TotalAlloc, m1.TotalAlloc),
			SysMB:        calculateMemoryDiffMB(m2.Sys, m1.Sys),
			NumGC:        m2.NumGC - m1.NumGC,
		},
		Errors:  errors,
		Success: len(errors) == 0 && rpsValid,
	}
}

// runRedisLargeBatchTest Redis 大批次测试
func runRedisLargeBatchTest(rdb *redis.Client, config TestConfig, prometheusMetrics *PrometheusMetrics) TestResult {
	largeConfig := config
	largeConfig.BatchSize = 5000
	largeConfig.BufferSize = 50000

	result := runRedisHighThroughputTest(rdb, largeConfig, prometheusMetrics)
	result.TestName = "Large Batch Test"
	return result
}

// runRedisMemoryPressureTest Redis 内存压力测试
func runRedisMemoryPressureTest(rdb *redis.Client, config TestConfig, prometheusMetrics *PrometheusMetrics) TestResult {
	memConfig := config
	memConfig.BatchSize = 100
	memConfig.BufferSize = 1000
	memConfig.RecordsPerWorker = 30000

	result := runRedisConcurrentWorkersTest(rdb, memConfig, prometheusMetrics)
	result.TestName = "Memory Pressure Test"
	return result
}

// runRedisLongDurationTest Redis 长时间运行测试
func runRedisLongDurationTest(rdb *redis.Client, config TestConfig, prometheusMetrics *PrometheusMetrics) TestResult {
	longConfig := config
	longConfig.TestDuration = 10 * time.Minute

	result := runRedisHighThroughputTest(rdb, longConfig, prometheusMetrics)
	result.TestName = "Long Duration Test"
	return result
}

// getRedisRecordCount 获取 Redis 中的记录数量
func getRedisRecordCount(rdb *redis.Client, ctx context.Context) (int64, error) {
	log.Printf("🔍 开始统计Redis记录数...")

	// 使用 DBSIZE 命令获取当前数据库中的 key 数量
	count, err := rdb.DBSize(ctx).Result()
	if err != nil {
		log.Printf("❌ DBSIZE命令执行失败: %v", err)
		return 0, err
	}

	log.Printf("🔍 DBSIZE返回结果: %d", count)

	// 额外验证：使用KEYS命令采样检查（仅用于调试，生产环境不推荐）
	keys, err := rdb.Keys(ctx, "test:*").Result()
	if err != nil {
		log.Printf("⚠️ KEYS命令执行失败: %v", err)
	} else {
		log.Printf("🔍 KEYS test:* 返回数量: %d", len(keys))
		if len(keys) > 0 && len(keys) <= 5 {
			log.Printf("🔍 前几个key示例: %v", keys)
		}
	}

	return count, nil
}
