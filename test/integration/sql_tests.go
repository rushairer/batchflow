package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"runtime"
	"sync"
	"time"

	"github.com/rushairer/batchflow/v2"
)

func runDatabaseTests(dbType, dsn string, config TestConfig, prometheusMetrics *PrometheusMetrics) []TestResult {
	var results []TestResult

	// 连接数据库
	db, err := sql.Open(dbType, dsn)
	if err != nil {
		log.Printf("❌ 连接 %s 失败：%v", dbType, err)
		return results
	}
	defer db.Close()

	// 设置连接池
	db.SetMaxOpenConns(100)
	db.SetMaxIdleConns(50)
	db.SetConnMaxLifetime(time.Hour)

	// 记录活跃连接数
	if prometheusMetrics != nil {
		prometheusMetrics.UpdateActiveConnections(dbType, 100) // 最大连接数
	}

	// 测试连接
	if err := db.Ping(); err != nil {
		log.Printf("❌ Ping %s 失败：%v", dbType, err)
		return results
	}

	// 创建测试表
	if err := createTestTables(db, dbType); err != nil {
		log.Printf("❌ 为 %s 创建测试表失败：%v", dbType, err)
		return results
	}

	// 运行不同的测试场景
	testCases := []struct {
		name     string
		testFunc func(*sql.DB, string, TestConfig, *PrometheusMetrics) TestResult
	}{
		{"重复主键 Upsert 语义测试", runDuplicateKeyUpsertTest},
		{"高吞吐量测试", runHighThroughputTest},
		{"并发工作线程测试", runConcurrentWorkersTest},
		{"大批次测试", runLargeBatchTest},
		{"内存压力测试", runMemoryPressureTest},
		{"长时间运行测试", runLongDurationTest},
	}

	for _, tc := range testCases {
		// 每个测试前清理表数据，确保测试独立性
		log.Printf("  🧹 在运行 %s 前清理表数据...", tc.name)
		if err := clearTestTable(db, dbType); err != nil {
			log.Printf("❌ Failed to clear table before %s: %v", tc.name, err)
			// 继续执行测试，但记录错误
		}

		log.Printf("  🔄 在 %s 上运行 %s...", dbType, tc.name)
		result := tc.testFunc(db, dbType, config, prometheusMetrics)
		result.TestName = tc.name
		result.Database = dbType
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

func runDuplicateKeyUpsertTest(db *sql.DB, dbType string, config TestConfig, prometheusMetrics *PrometheusMetrics) TestResult {
	_ = prometheusMetrics
	ctx := context.Background()
	startTime := time.Now()
	var errors []string

	var driver batchflow.SQLDriver
	switch dbType {
	case "mysql":
		driver = batchflow.DefaultMySQLDriver
	case "postgres":
		driver = batchflow.DefaultPostgreSQLDriver
	case "sqlite3":
		driver = batchflow.DefaultSQLiteDriver
	default:
		errors = append(errors, fmt.Sprintf("unsupported sql db type: %s", dbType))
	}

	if len(errors) == 0 {
		executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, driver)
		cfg := batchflow.ConflictUpdateOperationConfig.
			WithConflictColumns("id").
			WithUpdateColumns("name", "email", "data", "value", "is_active", "created_at")
		schema := batchflow.NewSQLSchema("integration_test", cfg,
			"id", "name", "email", "data", "value", "is_active", "created_at")

		rows := []map[string]any{
			{"id": int64(1), "name": "first", "email": "first@example.com", "data": "before", "value": 1.0, "is_active": true, "created_at": time.Now().UTC()},
			{"id": int64(1), "name": "second", "data": "after"},
			{"id": int64(2), "name": "other", "email": "other@example.com", "data": "other", "value": 2.0, "is_active": false, "created_at": time.Now().UTC()},
		}
		if err := executor.ExecuteBatch(ctx, schema, rows); err != nil {
			errors = append(errors, fmt.Sprintf("duplicate key upsert failed: %v", err))
		}
	}

	actualRecords, countErr := getSQLRecordCount(db)
	if countErr != nil {
		errors = append(errors, fmt.Sprintf("failed to count records: %v", countErr))
	}

	var name, email, dataValue string
	if len(errors) == 0 {
		query := "SELECT name, email, data FROM integration_test WHERE id = ?"
		if dbType == "postgres" {
			query = "SELECT name, email, data FROM integration_test WHERE id = $1"
		}
		if err := db.QueryRow(query, 1).Scan(&name, &email, &dataValue); err != nil {
			errors = append(errors, fmt.Sprintf("failed to read merged row: %v", err))
		}
	}
	if len(errors) == 0 && (name != "second" || email != "first@example.com" || dataValue != "after") {
		errors = append(errors, fmt.Sprintf("unexpected merged row: name=%q email=%q data=%q", name, email, dataValue))
	}

	expectedRecords := int64(2)
	dataIntegrityRate := 0.0
	if expectedRecords > 0 {
		dataIntegrityRate = float64(actualRecords) / float64(expectedRecords) * 100
	}
	rps := 0.0
	duration := time.Since(startTime)
	if duration > 0 {
		rps = float64(expectedRecords) / duration.Seconds()
	}

	return TestResult{
		Database:            dbType,
		Duration:            duration,
		TotalRecords:        expectedRecords,
		ActualRecords:       actualRecords,
		DataIntegrityRate:   dataIntegrityRate,
		DataIntegrityStatus: fmt.Sprintf("期望 %d 条，实际 %d 条", expectedRecords, actualRecords),
		RecordsPerSecond:    rps,
		RPSValid:            len(errors) == 0,
		ConcurrentWorkers:   1,
		TestParameters: TestParams{
			BatchSize:       config.BatchSize,
			BufferSize:      config.BufferSize,
			FlushInterval:   config.FlushInterval,
			ExpectedRecords: expectedRecords,
			TestDuration:    config.TestDuration,
		},
		Errors:  errors,
		Success: len(errors) == 0 && actualRecords == expectedRecords,
	}
}

func createTestTables(db *sql.DB, dbType string) error {
	var createSQL string

	switch dbType {
	case "mysql":
		createSQL = `
		DROP TABLE IF EXISTS integration_test;
		CREATE TABLE integration_test (
			id BIGINT PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			email VARCHAR(255) NOT NULL,
			data TEXT,
			value DECIMAL(10,2),
			is_active BOOLEAN DEFAULT TRUE,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			INDEX idx_name (name),
			INDEX idx_email (email)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
		`
	case "postgres":
		createSQL = `
		DROP TABLE IF EXISTS integration_test;
		CREATE TABLE integration_test (
			id BIGINT PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			email VARCHAR(255) NOT NULL,
			data TEXT,
			value DECIMAL(10,2),
			is_active BOOLEAN DEFAULT TRUE,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		
		CREATE INDEX IF NOT EXISTS idx_name ON integration_test(name);
		CREATE INDEX IF NOT EXISTS idx_email ON integration_test(email);
		`
	case "sqlite3":
		createSQL = `
		DROP TABLE IF EXISTS integration_test;
		CREATE TABLE integration_test (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			email TEXT NOT NULL,
			data TEXT,
			value REAL,
			is_active INTEGER DEFAULT 1,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		
		CREATE INDEX IF NOT EXISTS idx_name ON integration_test(name);
		CREATE INDEX IF NOT EXISTS idx_email ON integration_test(email);
		`
	}

	_, err := db.Exec(createSQL)
	return err
}

// 验证数据库中的实际记录数
func getSQLRecordCount(db *sql.DB) (int64, error) {
	var count int64
	err := db.QueryRow("SELECT COUNT(*) FROM integration_test").Scan(&count)
	return count, err
}

// 清理测试表数据 - 使用高性能的清理方式
func clearTestTable(db *sql.DB, dbType string) error {
	switch dbType {
	case "mysql":
		// MySQL 使用 TRUNCATE，性能最佳
		_, err := db.Exec("TRUNCATE TABLE integration_test")
		return err
	case "postgres":
		// PostgreSQL 使用 TRUNCATE，支持级联
		_, err := db.Exec("TRUNCATE TABLE integration_test RESTART IDENTITY")
		return err
	case "sqlite3":
		// SQLite 使用重建表方式，避免锁定问题
		return clearSQLiteTableByRecreate(db)
	default:
		// 兜底方案
		_, err := db.Exec("DELETE FROM integration_test")
		return err
	}
}

// clearSQLiteTableByRecreate SQLite专用的重建表清理方式
func clearSQLiteTableByRecreate(db *sql.DB) error {
	// 1. 删除表
	if _, err := db.Exec("DROP TABLE IF EXISTS integration_test"); err != nil {
		return fmt.Errorf("failed to drop table: %v", err)
	}

	// 2. 重新创建表
	createSQL := `
	CREATE TABLE integration_test (
		id INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		email TEXT NOT NULL,
		data TEXT,
		value REAL,
		is_active INTEGER DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`

	if _, err := db.Exec(createSQL); err != nil {
		return fmt.Errorf("failed to create table: %v", err)
	}

	// 3. 重新创建索引
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_name ON integration_test(name)",
		"CREATE INDEX IF NOT EXISTS idx_email ON integration_test(email)",
	}

	for _, indexSQL := range indexes {
		if _, err := db.Exec(indexSQL); err != nil {
			return fmt.Errorf("failed to create index: %v", err)
		}
	}

	log.Printf("  ✅ 已成功重建 SQLite 表")
	return nil
}

func runHighThroughputTest(db *sql.DB, dbType string, config TestConfig, prometheusMetrics *PrometheusMetrics) TestResult {
	ctx := context.Background()

	var batchFlow *batchflow.BatchFlow
	switch dbType {
	case "mysql":
		executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultMySQLDriver)
		if prometheusMetrics != nil {
			metricsReporter := NewPrometheusMetricsReporter(prometheusMetrics, dbType, "高吞吐量测试")
			executor = executor.WithMetricsReporter(metricsReporter)
		}
		batchFlow = batchflow.NewBatchFlow(ctx, config.BufferSize, config.BatchSize, config.FlushInterval, executor)
	case "postgres":
		executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultPostgreSQLDriver)
		if prometheusMetrics != nil {
			metricsReporter := NewPrometheusMetricsReporter(prometheusMetrics, dbType, "高吞吐量测试")
			executor = executor.WithMetricsReporter(metricsReporter)
		}
		batchFlow = batchflow.NewBatchFlow(ctx, config.BufferSize, config.BatchSize, config.FlushInterval, executor)
	case "sqlite3":
		executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultSQLiteDriver)
		if prometheusMetrics != nil {
			metricsReporter := NewPrometheusMetricsReporter(prometheusMetrics, dbType, "高吞吐量测试")
			executor = executor.WithMetricsReporter(metricsReporter)
		}
		batchFlow = batchflow.NewBatchFlow(ctx, config.BufferSize, config.BatchSize, config.FlushInterval, executor)
	}

	schema := batchflow.NewSQLSchema("integration_test", batchflow.ConflictIgnoreOperationConfig,
		"id", "name", "email", "data", "value", "is_active", "created_at")

	startTime := time.Now()
	var recordCount int64
	var errors []string

	// 设置并发工作线程数和活跃连接数指标
	if prometheusMetrics != nil {
		prometheusMetrics.concurrentWorkers.WithLabelValues(dbType, "高吞吐量测试").Set(1) // 单线程测试
		prometheusMetrics.activeConnections.WithLabelValues(dbType).Set(1)           // 单个连接
	}

	// 记录初始内存
	var m1 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	// 高吞吐量测试 - 限制记录数量避免内存泄漏
	testCtx, cancel := context.WithTimeout(ctx, config.TestDuration)
	defer cancel()

	maxRecords := int64(config.ConcurrentWorkers * config.RecordsPerWorker) // 限制最大记录数

	for i := int64(0); i < maxRecords; i++ {
		select {
		case <-testCtx.Done():
			goto TestComplete
		default:
			batchStartTime := time.Now()

			request := batchflow.NewRequest(schema).
				SetInt64("id", i).
				SetString("name", fmt.Sprintf("User_%d", i)).
				SetString("email", fmt.Sprintf("user_%d@example.com", i)).
				SetString("data", fmt.Sprintf("Data_%d", i)). // 减少字符串长度
				SetFloat64("value", float64(i%10000)/100.0).
				SetBool("is_active", i%2 == 0).
				SetTime("created_at", time.Now().UTC()) // 写入 UTC

			if err := batchFlow.Submit(testCtx, request); err != nil {
				errors = append(errors, err.Error())
				if len(errors) > 100 { // 限制错误数量
					break
				}
			} else {
				recordCount++

				// 记录批处理时间和响应时间
				if prometheusMetrics != nil {
					batchDuration := time.Since(batchStartTime)
					prometheusMetrics.RecordBatchProcessTime(dbType, config.BatchSize, batchDuration)
					prometheusMetrics.RecordResponseTime(dbType, "insert", batchDuration)
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
						prometheusMetrics.UpdateCurrentRPS(dbType, "高吞吐量测试", currentRPS)
					}

					// 更新内存使用
					var m runtime.MemStats
					runtime.ReadMemStats(&m)
					prometheusMetrics.UpdateMemoryUsage(dbType, "高吞吐量测试",
						float64(m.Alloc)/1024/1024,
						float64(m.TotalAlloc)/1024/1024,
						float64(m.Sys)/1024/1024)
				}
			}
		}
	}

TestComplete:
	duration := time.Since(startTime)

	// 等待处理完成
	time.Sleep(5 * time.Second)

	// 记录最终内存
	var m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m2)

	// 查询数据库中的实际记录数
	actualRecords, countErr := getSQLRecordCount(db)
	if countErr != nil {
		errors = append(errors, fmt.Sprintf("统计实际记录数失败：%v", countErr))
		actualRecords = -1 // 标记为无法获取
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
		Success: len(errors) == 0 && rpsValid, // 只有数据完整性100%才算成功
	}
}

func runConcurrentWorkersTest(db *sql.DB, dbType string, config TestConfig, prometheusMetrics *PrometheusMetrics) TestResult {
	ctx := context.Background()

	// 设置并发工作线程数和活跃连接数指标
	if prometheusMetrics != nil {
		prometheusMetrics.concurrentWorkers.WithLabelValues(dbType, "并发工作线程测试").Set(float64(config.ConcurrentWorkers))
		prometheusMetrics.activeConnections.WithLabelValues(dbType).Set(float64(config.ConcurrentWorkers)) // 每个工作线程一个连接
	}

	var batchFlow *batchflow.BatchFlow
	switch dbType {
	case "mysql":
		executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultMySQLDriver)
		if prometheusMetrics != nil {
			metricsReporter := NewPrometheusMetricsReporter(prometheusMetrics, dbType, "并发工作线程测试")
			executor = executor.WithMetricsReporter(metricsReporter)
		}
		batchFlow = batchflow.NewBatchFlow(ctx, config.BufferSize, config.BatchSize, config.FlushInterval, executor)
	case "postgres":
		executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultPostgreSQLDriver)
		if prometheusMetrics != nil {
			metricsReporter := NewPrometheusMetricsReporter(prometheusMetrics, dbType, "并发工作线程测试")
			executor = executor.WithMetricsReporter(metricsReporter)
		}
		batchFlow = batchflow.NewBatchFlow(ctx, config.BufferSize, config.BatchSize, config.FlushInterval, executor)
	case "sqlite3":
		executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultSQLiteDriver)
		if prometheusMetrics != nil {
			metricsReporter := NewPrometheusMetricsReporter(prometheusMetrics, dbType, "并发工作线程测试")
			executor = executor.WithMetricsReporter(metricsReporter)
		}
		batchFlow = batchflow.NewBatchFlow(ctx, config.BufferSize, config.BatchSize, config.FlushInterval, executor)
	}

	schema := batchflow.NewSQLSchema("integration_test", batchflow.ConflictIgnoreOperationConfig,
		"id", "name", "email", "data", "value", "is_active", "created_at")

	startTime := time.Now()
	var totalRecords int64
	var mu sync.Mutex
	var errors []string

	// 记录初始内存
	var m1 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	// 并发工作者测试 - 批次处理避免内存峰值
	var wg sync.WaitGroup
	batchSize := 100 // 每批处理100条记录

	for workerID := 0; workerID < config.ConcurrentWorkers; workerID++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			workerRecords := 0
			baseID := int64(id * config.RecordsPerWorker)

			// 分批处理，避免内存峰值
			for batch := 0; batch < config.RecordsPerWorker; batch += batchSize {
				endIdx := batch + batchSize
				if endIdx > config.RecordsPerWorker {
					endIdx = config.RecordsPerWorker
				}

				for i := batch; i < endIdx; i++ {
					request := batchflow.NewRequest(schema).
						SetInt64("id", baseID+int64(i)).
						SetString("name", fmt.Sprintf("W%d_U%d", id, i)).          // 缩短字符串
						SetString("email", fmt.Sprintf("u%d_%d@test.com", id, i)). // 缩短字符串
						SetString("data", fmt.Sprintf("D%d_%d", id, i)).           // 大幅缩短数据
						SetFloat64("value", float64((id*100+i)%1000)/10.0).
						SetBool("is_active", (id+i)%2 == 0).
						SetTime("created_at", time.Now().UTC()) // 写入 UTC

					if err := batchFlow.Submit(ctx, request); err != nil {
						mu.Lock()
						errors = append(errors, fmt.Sprintf("Worker %d: %v", id, err))
						mu.Unlock()
					} else {
						workerRecords++
					}
				}

				// 每批处理完后强制GC
				runtime.GC()

				// 添加小延迟，避免过度竞争
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

	// 查询数据库中的实际记录数
	actualRecords, countErr := getSQLRecordCount(db)
	if countErr != nil {
		mu.Lock()
		errors = append(errors, fmt.Sprintf("统计实际记录数失败：%v", countErr))
		mu.Unlock()
		actualRecords = -1 // 标记为无法获取
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
		Success: len(errors) == 0 && rpsValid, // 只有数据完整性100%才算成功
	}
}

func runLargeBatchTest(db *sql.DB, dbType string, config TestConfig, prometheusMetrics *PrometheusMetrics) TestResult {
	// 大批次测试 - 使用更大的批次大小
	largeConfig := config
	largeConfig.BatchSize = 5000
	largeConfig.BufferSize = 50000

	result := runHighThroughputTest(db, dbType, largeConfig, prometheusMetrics)
	result.TestName = "Large Batch Test"
	return result
}

func runMemoryPressureTest(db *sql.DB, dbType string, config TestConfig, prometheusMetrics *PrometheusMetrics) TestResult {
	// 内存压力测试 - 使用大数据量和小批次
	memConfig := config
	memConfig.BatchSize = 100
	memConfig.BufferSize = 1000
	memConfig.RecordsPerWorker = 50000

	result := runConcurrentWorkersTest(db, dbType, memConfig, prometheusMetrics)
	result.TestName = "Memory Pressure Test"
	return result
}

func runLongDurationTest(db *sql.DB, dbType string, config TestConfig, prometheusMetrics *PrometheusMetrics) TestResult {
	// 长时间运行测试
	longConfig := config
	longConfig.TestDuration = 10 * time.Minute

	result := runHighThroughputTest(db, dbType, longConfig, prometheusMetrics)
	result.TestName = "Long Duration Test"
	return result
}
