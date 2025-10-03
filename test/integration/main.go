package main

import (
	"log"
	"os"
	"runtime"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	log.Println("🚀 启动 BatchFlow 集成测试...")

	// 加载配置
	config := loadConfig()

	// 初始化 Prometheus 指标收集器
	var prometheusMetrics *PrometheusMetrics
	if config.PrometheusEnabled {
		prometheusMetrics = NewPrometheusMetrics()
		if err := prometheusMetrics.StartServer(config.PrometheusPort); err != nil {
			log.Printf("⚠️  启动 Prometheus 服务器失败: %v", err)
		} else {
			log.Printf("📊 Prometheus 指标服务器已启动: http://localhost:%d/metrics", config.PrometheusPort)
			// 确保在程序结束时停止服务器
			defer func() {
				if err := prometheusMetrics.StopServer(); err != nil {
					log.Printf("⚠️  停止 Prometheus 服务器失败: %v", err)
				}
			}()
		}
	}

	// 创建测试报告
	report := &TestReport{
		Timestamp:   time.Now(),
		Environment: "Docker 集成环境",
		GoVersion:   runtime.Version(),
		TestConfig:  config,
		Results:     []TestResult{},
	}

	startTime := time.Now()

	// 运行 MySQL 测试
	if mysqlDSN := os.Getenv("MYSQL_DSN"); mysqlDSN != "" {
		log.Println("📊 正在运行 MySQL 集成测试...")
		mysqlResults := runDatabaseTests("mysql", mysqlDSN, config, prometheusMetrics)
		report.Results = append(report.Results, mysqlResults...)
	}

	// 运行 PostgreSQL 测试
	if postgresDSN := os.Getenv("POSTGRES_DSN"); postgresDSN != "" {
		log.Println("📊 正在运行 PostgreSQL 集成测试...")
		postgresResults := runDatabaseTests("postgres", postgresDSN, config, prometheusMetrics)
		report.Results = append(report.Results, postgresResults...)
	}

	// 运行 SQLite 测试
	if sqliteDSN := os.Getenv("SQLITE_DSN"); sqliteDSN != "" {
		log.Println("📊 正在运行 SQLite 集成测试...")
		sqliteResults := runDatabaseTests("sqlite3", sqliteDSN, config, prometheusMetrics)
		report.Results = append(report.Results, sqliteResults...)
	}

	// 运行 Redis 测试
	if redisDSN := os.Getenv("REDIS_DSN"); redisDSN != "" {
		log.Println("📊 正在运行 Redis 集成测试...")
		redisResults := runRedisTests(redisDSN, config, prometheusMetrics)
		report.Results = append(report.Results, redisResults...)
	}

	// 生成摘要
	report.Summary = generateSummary(report.Results, time.Since(startTime))

	// 保存报告
	saveReport(report)

	// 输出结果
	printSummary(report)

	// 如果启用了 Prometheus，提供访问信息并等待指标被抓取
	if config.PrometheusEnabled && prometheusMetrics != nil {
		log.Printf("📊 Prometheus 指标可通过以下方式访问:")
		log.Printf("   指标端点: http://localhost:%d/metrics", config.PrometheusPort)
		log.Printf("   健康检查: http://localhost:%d/health", config.PrometheusPort)
		log.Printf("   💡 提示: 可以使用 Grafana 连接此端点来可视化性能曲线")

		// 等待 Prometheus 抓取指标数据
		waitTime := 60 * time.Second // 等待 60 秒让 Prometheus 抓取数据
		log.Printf("⏰ 等待 %v 让 Prometheus 抓取指标数据...", waitTime)
		log.Printf("   在此期间，Prometheus 将每 10 秒抓取一次指标")
		log.Printf("   Grafana 仪表板: http://localhost:3000 (admin/admin)")

		// 显示倒计时
		for i := int(waitTime.Seconds()); i > 0; i-- {
			if i%10 == 0 || i <= 10 {
				log.Printf("   ⏳ 还有 %d 秒...", i)
			}
			time.Sleep(1 * time.Second)
		}

		log.Printf("✅ 等待完成，Prometheus 应该已经抓取到指标数据")
	}

	// 如果有失败的测试，退出码为 1
	if report.Summary.FailedTests > 0 {
		log.Printf("❌ 测试完成，但有 %d 个测试失败", report.Summary.FailedTests)
		os.Exit(1)
	}

	log.Printf("🎉 所有测试成功完成！")
}
