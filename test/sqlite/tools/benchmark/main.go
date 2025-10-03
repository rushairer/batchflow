package main

import (
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	fmt.Println("🔍 SQLite 性能基准测试")
	fmt.Println("========================================")

	// 创建数据库
	db, err := sql.Open("sqlite3", "../../data/benchmark.db?cache=shared&mode=rwc&_busy_timeout=30000&_journal_mode=WAL")
	if err != nil {
		log.Fatal("Failed to open database:", err)
	}
	defer db.Close()

	// 设置连接池 - SQLite 优化
	db.SetMaxOpenConns(1) // SQLite 单连接
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)

	// 创建测试表
	createSQL := `
	DROP TABLE IF EXISTS benchmark_test;
	CREATE TABLE benchmark_test (
		id INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		email TEXT NOT NULL,
		data TEXT,
		value REAL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_name ON benchmark_test(name);
	CREATE INDEX IF NOT EXISTS idx_email ON benchmark_test(email);`

	if _, err := db.Exec(createSQL); err != nil {
		log.Fatal("Failed to create table:", err)
	}

	// 测试场景
	scenarios := []struct {
		name        string
		workers     int
		recordsEach int
		batchSize   int
	}{
		{"单线程小批次", 1, 1000, 50},
		{"单线程中批次", 1, 5000, 100},
		{"单线程大批次", 1, 10000, 200},
		{"低并发小批次", 2, 1000, 50},
		{"低并发中批次", 3, 2000, 100},
		{"中等并发", 5, 1000, 100},
		{"SQLite配置测试", 5, 5000, 100}, // 对应 .env.sqlite.test
	}

	for _, scenario := range scenarios {
		fmt.Printf("\n🧪 测试场景: %s\n", scenario.name)
		fmt.Printf("   工作者: %d, 每工作者记录: %d, 批次大小: %d\n",
			scenario.workers, scenario.recordsEach, scenario.batchSize)

		// 清理表
		db.Exec("DELETE FROM benchmark_test")

		// 运行测试
		start := time.Now()
		totalRecords, err := runBenchmark(db, scenario.workers, scenario.recordsEach, scenario.batchSize)
		duration := time.Since(start)

		if err != nil {
			fmt.Printf("   ❌ 测试失败: %v\n", err)
			continue
		}

		// 验证记录数
		var actualCount int64
		db.QueryRow("SELECT COUNT(*) FROM benchmark_test").Scan(&actualCount)

		rps := float64(totalRecords) / duration.Seconds()
		fmt.Printf("   ✅ 耗时: %v\n", duration)
		fmt.Printf("   📊 提交记录: %d, 实际记录: %d\n", totalRecords, actualCount)
		fmt.Printf("   ⚡ RPS: %.2f\n", rps)

		if actualCount != totalRecords {
			fmt.Printf("   ⚠️  数据不一致!\n")
		}
	}

	fmt.Println("\n🎯 SQLite 性能建议:")
	fmt.Println("   - 推荐并发工作者: 1-5 个")
	fmt.Println("   - 推荐批次大小: 50-200")
	fmt.Println("   - 预期 RPS: 1,000-10,000")
	fmt.Println("   - 重点关注: 数据一致性 > 高性能")
}

func runBenchmark(db *sql.DB, workers, recordsEach, batchSize int) (int64, error) {
	var wg sync.WaitGroup
	var totalRecords int64
	var mu sync.Mutex
	errChan := make(chan error, workers)

	for workerID := 0; workerID < workers; workerID++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			workerRecords := 0
			baseID := int64(id * recordsEach)

			// 分批处理
			for batch := 0; batch < recordsEach; batch += batchSize {
				endIdx := batch + batchSize
				if endIdx > recordsEach {
					endIdx = recordsEach
				}

				// 构建批量插入SQL
				values := make([]string, 0, endIdx-batch)
				args := make([]interface{}, 0, (endIdx-batch)*4)

				for i := batch; i < endIdx; i++ {
					values = append(values, "(?, ?, ?, ?)")
					args = append(args,
						baseID+int64(i),
						fmt.Sprintf("User_%d_%d", id, i),
						fmt.Sprintf("user_%d_%d@test.com", id, i),
						fmt.Sprintf("Data_%d_%d", id, i),
					)
				}

				sql := fmt.Sprintf("INSERT INTO benchmark_test (id, name, email, data) VALUES %s",
					fmt.Sprintf("%s", values[0]))
				for i := 1; i < len(values); i++ {
					sql += ", " + values[i]
				}

				if _, err := db.Exec(sql, args...); err != nil {
					errChan <- fmt.Errorf("worker %d batch error: %v", id, err)
					return
				}

				workerRecords += endIdx - batch

				// 小延迟，减少锁竞争
				time.Sleep(1 * time.Millisecond)
			}

			mu.Lock()
			totalRecords += int64(workerRecords)
			mu.Unlock()
		}(workerID)
	}

	wg.Wait()
	close(errChan)

	// 检查错误
	if len(errChan) > 0 {
		return totalRecords, <-errChan
	}

	return totalRecords, nil
}
