package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// PrometheusMetrics Prometheus 指标收集器
//
// 更新历史：
// - 2025-10-03: 修复 RPS 计算指标，移除有问题的 Histogram，新增 Counter 类型指标
// - 2025-10-03: 修复内存使用量单位显示问题，确保 MB 单位正确性
// - 2025-10-03: 修复响应时间分位数不匹配问题，统一使用 0.5, 0.9, 0.99 分位数
// - 2025-10-03: 修复测试结果统计指标，移除错误的 increase() 函数依赖
type PrometheusMetrics struct {
	// 计数器指标 - 用于累计统计和速率计算
	totalRecordsProcessed *prometheus.CounterVec // 记录处理总数，支持按状态分类
	recordsProcessedRate  *prometheus.CounterVec // RPS 计算专用计数器 [修复于 2025-10-03]
	totalTestsRun         *prometheus.CounterVec // 测试执行总数，支持成功/失败分类
	totalErrors           *prometheus.CounterVec // 错误总数，支持按错误类型分类

	// 直方图指标 - 用于分布统计和分位数计算
	testDuration     *prometheus.HistogramVec // 测试持续时间分布
	batchProcessTime *prometheus.HistogramVec // 批处理时间分布

	// 仪表指标 - 用于实时状态监控
	currentRPS        *prometheus.GaugeVec // 当前 RPS，从测试结果直接设置
	memoryUsage       *prometheus.GaugeVec // 内存使用量(MB) [单位修复于 2025-10-03]
	dataIntegrityRate *prometheus.GaugeVec // 数据完整性比率 (0-1 范围)
	concurrentWorkers *prometheus.GaugeVec // 并发工作线程数
	activeConnections *prometheus.GaugeVec // 活跃数据库连接数

	// 核心库对齐指标 - 与主库 executor/processor 指标保持一致
	// 标签维度：database, instance_id（替代原有的 test_name，语义更清晰，支持多实例）
	executorConcurrency *prometheus.GaugeVec // 执行器并发度
	queueLength         *prometheus.GaugeVec // 队列长度
	inflightBatches     *prometheus.GaugeVec // 在途批次数

	// go-pipeline 对齐新增指标
	pipelineProcessDuration *prometheus.HistogramVec // 管道级处理耗时（按状态）
	pipelineDequeueLatency  *prometheus.HistogramVec // 出队等待时延
	pipelineDroppedTotal    *prometheus.CounterVec   // 丢弃计数（错误通道饱和等）

	// 核心库对齐的直方图指标
	enqueueLatency   *prometheus.HistogramVec // 入队延迟
	assembleDuration *prometheus.HistogramVec // 组装耗时
	executeDuration  *prometheus.HistogramVec // 执行耗时（按状态区分：success/fail）
	batchSize        *prometheus.HistogramVec // 批次大小分布

	// 摘要指标 - 用于响应时间分位数统计 [分位数修复于 2025-10-03]
	responseTime *prometheus.SummaryVec // 支持 0.5, 0.9, 0.99 分位数

	registry *prometheus.Registry
	server   *http.Server
	mutex    sync.RWMutex
}

// NewPrometheusMetrics 创建 Prometheus 指标收集器
func NewPrometheusMetrics() *PrometheusMetrics {
	registry := prometheus.NewRegistry()

	pm := &PrometheusMetrics{
		// 计数器指标
		totalRecordsProcessed: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "batchflow_records_processed_total",
				Help: "Total number of records processed by BatchFlow",
			},
			[]string{"database", "test_name", "status"},
		),

		recordsProcessedRate: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "batchflow_records_rate_total",
				Help: "Total records processed for RPS calculation (use with rate() function). 修复于 2025-10-03: 替换有问题的 Histogram 指标",
			},
			[]string{"database", "test_name"},
		),

		totalTestsRun: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "batchflow_tests_run_total",
				Help: "Total number of tests run",
			},
			[]string{"database", "test_name", "result"},
		),

		totalErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "batchflow_errors_total",
				Help: "Total number of errors encountered",
			},
			[]string{"database", "test_name", "error_type"},
		),

		// 直方图指标
		testDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "batchflow_test_duration_seconds",
				Help:    "Duration of BatchFlow tests in seconds",
				Buckets: prometheus.ExponentialBuckets(0.1, 2, 10), // 0.1s to ~100s
			},
			[]string{"database", "test_name"},
		),

		batchProcessTime: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "batchflow_batch_process_duration_seconds",
				Help:    "Time taken to process a batch",
				Buckets: prometheus.ExponentialBuckets(0.001, 2, 15), // 1ms to ~32s
			},
			[]string{"database", "batch_size"},
		),

		// 仪表盘指标
		currentRPS: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "batchflow_current_rps",
				Help: "Current records per second",
			},
			[]string{"database", "test_name"},
		),

		memoryUsage: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "batchflow_memory_usage_mb",
				Help: "Memory usage in MB. 修复于 2025-10-03: 确认单位正确性，代码已正确转换字节到MB，Grafana面板已修复单位显示",
			},
			[]string{"database", "test_name", "type"}, // type: alloc, total_alloc, sys
		),

		dataIntegrityRate: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "batchflow_data_integrity_rate",
				Help: "Data integrity rate (0-1 range, multiply by 100 for percentage display)",
			},
			[]string{"database", "test_name"},
		),

		concurrentWorkers: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "batchflow_concurrent_workers",
				Help: "Number of concurrent workers",
			},
			[]string{"database", "test_name"},
		),

		activeConnections: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "batchflow_active_connections",
				Help: "Number of active database connections",
			},
			[]string{"database"},
		),

		// 新增：核心库对齐的 Gauge
		// 标签：database, instance_id（支持多实例隔离）
		executorConcurrency: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "batchflow_executor_concurrency",
				Help: "Current executor concurrency",
			},
			[]string{"database", "instance_id"},
		),
		queueLength: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "batchflow_pipeline_queue_length",
				Help: "Current pipeline queue length",
			},
			[]string{"database", "instance_id"},
		),
		inflightBatches: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "batchflow_inflight_batches",
				Help: "Current in-flight batch count (executing now)",
			},
			[]string{"database", "instance_id"},
		),

		// go-pipeline 对齐新增指标初始化
		pipelineProcessDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "batchflow_pipeline_process_duration_seconds",
				Help:    "Pipeline-level process duration per flush",
				Buckets: prometheus.ExponentialBuckets(0.0005, 2, 18),
			},
			[]string{"database", "instance_id", "status"},
		),
		pipelineDequeueLatency: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "batchflow_pipeline_dequeue_latency_seconds",
				Help:    "Time waiting in pipeline queue before processing",
				Buckets: prometheus.ExponentialBuckets(0.0005, 2, 18),
			},
			[]string{"database", "instance_id"},
		),
		pipelineDroppedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "batchflow_pipeline_dropped_total",
				Help: "Total number of dropped events (e.g., error channel full)",
			},
			[]string{"database", "instance_id", "reason"},
		),

		// 新增：核心库对齐的 Histogram
		enqueueLatency: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "batchflow_enqueue_latency_seconds",
				Help:    "Latency from submit to enqueue",
				Buckets: prometheus.ExponentialBuckets(0.0005, 2, 18),
			},
			[]string{"database", "instance_id"},
		),
		assembleDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "batchflow_batch_assemble_duration_seconds",
				Help:    "Duration to assemble a batch",
				Buckets: prometheus.ExponentialBuckets(0.0005, 2, 18),
			},
			[]string{"database", "instance_id"},
		),
		executeDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "batchflow_execute_duration_seconds",
				Help:    "Execute duration for a batch (includes retry/backoff)",
				Buckets: prometheus.ExponentialBuckets(0.0005, 2, 18),
			},
			[]string{"database", "instance_id", "status"}, // status: success/fail
		),
		batchSize: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "batchflow_batch_size",
				Help:    "Batch size distribution",
				Buckets: prometheus.ExponentialBuckets(1, 2, 12),
			},
			[]string{"database", "instance_id"},
		),

		// 摘要指标
		responseTime: prometheus.NewSummaryVec(
			prometheus.SummaryOpts{
				Name:       "batchflow_response_time_seconds",
				Help:       "Response time for batch operations. 修复于 2025-10-03: 确保分位数 0.5, 0.9, 0.99 与 Grafana 查询匹配",
				Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
			},
			[]string{"database", "operation"},
		),

		registry: registry,
	}

	// 注册所有指标
	registry.MustRegister(
		pm.totalRecordsProcessed,
		pm.recordsProcessedRate,
		pm.totalTestsRun,
		pm.totalErrors,
		pm.testDuration,
		pm.batchProcessTime,
		// 注册新增直方图
		pm.enqueueLatency,
		pm.assembleDuration,
		pm.executeDuration,
		pm.batchSize,
		// 注册 go-pipeline 对齐新增指标
		pm.pipelineProcessDuration,
		pm.pipelineDequeueLatency,
		pm.pipelineDroppedTotal,
		// 既有与新增 Gauge
		pm.currentRPS,
		pm.memoryUsage,
		pm.dataIntegrityRate,
		pm.concurrentWorkers,
		pm.activeConnections,
		pm.executorConcurrency,
		pm.queueLength,
		pm.inflightBatches,
		pm.responseTime,
	)

	// 初始化基础指标，确保端点始终返回有效数据
	pm.initializeBaseMetrics()

	return pm
}

// StartServer 启动 Prometheus HTTP 服务器
func (pm *PrometheusMetrics) StartServer(port int) error {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	if pm.server != nil {
		return fmt.Errorf("prometheus server already running")
	}

	// 设置 Gin 为发布模式，减少日志输出
	gin.SetMode(gin.ReleaseMode)

	// 创建 Gin 路由器
	router := gin.Default()

	// 添加 Go 运行时指标到我们的自定义 registry
	pm.registry.MustRegister(collectors.NewBuildInfoCollector())
	pm.registry.MustRegister(collectors.NewGoCollector())
	pm.registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	// 创建使用我们自定义 registry 的 handler
	metricsHandler := promhttp.HandlerFor(pm.registry, promhttp.HandlerOpts{
		EnableOpenMetrics: false,
	})

	// 添加 /metrics 端点
	router.GET("/metrics", gin.WrapH(metricsHandler))

	// 添加健康检查端点
	router.GET("/health", func(c *gin.Context) {
		c.String(http.StatusOK, "OK")
	})

	pm.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: router,
	}

	go func() {
		log.Printf("📊 Prometheus metrics server starting on port %d", port)
		log.Printf("📊 Metrics endpoint: http://localhost:%d/metrics", port)
		if err := pm.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("❌ Prometheus server error: %v", err)
		}
	}()

	// 等待服务器启动
	time.Sleep(100 * time.Millisecond)
	return nil
}

// StopServer 停止 Prometheus HTTP 服务器
func (pm *PrometheusMetrics) StopServer() error {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	if pm.server == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := pm.server.Shutdown(ctx)
	pm.server = nil

	if err == nil {
		log.Println("📊 Prometheus metrics server stopped")
	}

	return err
}

// RecordTestResult 记录测试结果
// RecordTestResult 记录测试结果到 Prometheus 指标
//
// 更新历史：
// - 2025-10-03: 修复 RPS 记录逻辑，同时更新 recordsProcessedRate Counter
// - 2025-10-03: 修复内存使用量记录，确保 MB 单位正确性
// - 2025-10-03: 修复数据完整性比率计算，确保 0-1 范围
//
// 参数：
//   - result: 包含测试执行结果的完整数据结构
//
// 记录的指标包括：
//   - 测试执行计数（成功/失败）
//   - 记录处理总数和速率计算用数据
//   - 错误统计
//   - 测试持续时间
//   - 实时 RPS 值
//   - 内存使用情况（alloc/total_alloc/sys，单位：MB）
//   - 数据完整性比率（0-1 范围）
//   - 并发工作线程数
func (pm *PrometheusMetrics) RecordTestResult(result TestResult) {
	database := result.Database
	testName := result.TestName

	// 记录测试运行
	if result.Success {
		pm.totalTestsRun.WithLabelValues(database, testName, "success").Inc()
	} else {
		pm.totalTestsRun.WithLabelValues(database, testName, "failure").Inc()
	}

	// 记录处理的记录数 (确保值为正数)
	if result.TotalRecords > 0 {
		pm.totalRecordsProcessed.WithLabelValues(database, testName, "processed").Add(float64(result.TotalRecords))
		// 同时记录到 RPS 计算用的 Counter
		pm.recordsProcessedRate.WithLabelValues(database, testName).Add(float64(result.TotalRecords))
	}
	if result.ActualRecords > 0 {
		pm.totalRecordsProcessed.WithLabelValues(database, testName, "actual").Add(float64(result.ActualRecords))
	}

	// 记录错误
	if len(result.Errors) > 0 {
		for _, err := range result.Errors {
			pm.totalErrors.WithLabelValues(database, testName, "general").Inc()
			log.Printf("📊 Recorded error for %s/%s: %s", database, testName, err)
		}
	}

	// 记录测试持续时间
	pm.testDuration.WithLabelValues(database, testName).Observe(result.Duration.Seconds())

	// 记录 RPS（如果有效）
	if result.RPSValid && result.RecordsPerSecond > 0 {
		pm.currentRPS.WithLabelValues(database, testName).Set(result.RecordsPerSecond)
	}

	// 记录内存使用情况
	pm.memoryUsage.WithLabelValues(database, testName, "alloc").Set(result.MemoryUsage.AllocMB)
	pm.memoryUsage.WithLabelValues(database, testName, "total_alloc").Set(result.MemoryUsage.TotalAllocMB)
	pm.memoryUsage.WithLabelValues(database, testName, "sys").Set(result.MemoryUsage.SysMB)

	// 记录数据完整性 (确保值在 0-1 范围内)
	integrityRate := result.DataIntegrityRate / 100.0 // 将百分比转换为 0-1 范围
	pm.dataIntegrityRate.WithLabelValues(database, testName).Set(integrityRate)

	// 记录并发工作线程数
	pm.concurrentWorkers.WithLabelValues(database, testName).Set(float64(result.ConcurrentWorkers))

	log.Printf("📊 Recorded metrics for %s/%s: RPS=%.2f, Integrity=%.2f%%, Duration=%v",
		database, testName, result.RecordsPerSecond, result.DataIntegrityRate, result.Duration)
}

// RecordBatchProcessTime 记录批处理时间
func (pm *PrometheusMetrics) RecordBatchProcessTime(database string, batchSize uint32, duration time.Duration) {
	pm.batchProcessTime.WithLabelValues(database, fmt.Sprintf("%d", batchSize)).Observe(duration.Seconds())
}

// RecordResponseTime 记录响应时间
func (pm *PrometheusMetrics) RecordResponseTime(database, operation string, duration time.Duration) {
	pm.responseTime.WithLabelValues(database, operation).Observe(duration.Seconds())
}

// 新增：与 MetricsReporter 对齐的方法
// 所有方法添加 instanceID 参数，支持多实例隔离
func (pm *PrometheusMetrics) RecordEnqueueLatency(database, instanceID string, d time.Duration) {
	pm.enqueueLatency.WithLabelValues(database, instanceID).Observe(d.Seconds())
}

func (pm *PrometheusMetrics) RecordAssembleDuration(database, instanceID string, d time.Duration) {
	pm.assembleDuration.WithLabelValues(database, instanceID).Observe(d.Seconds())
}

func (pm *PrometheusMetrics) RecordExecuteDuration(database, instanceID, status string, d time.Duration) {
	pm.executeDuration.WithLabelValues(database, instanceID, status).Observe(d.Seconds())
}

func (pm *PrometheusMetrics) RecordBatchSize(database, instanceID string, n int) {
	pm.batchSize.WithLabelValues(database, instanceID).Observe(float64(n))
}

func (pm *PrometheusMetrics) SetExecutorConcurrency(database, instanceID string, n int) {
	pm.executorConcurrency.WithLabelValues(database, instanceID).Set(float64(n))
}

func (pm *PrometheusMetrics) SetQueueLength(database, instanceID string, n int) {
	pm.queueLength.WithLabelValues(database, instanceID).Set(float64(n))
}

func (pm *PrometheusMetrics) IncInflight(database, instanceID string) {
	pm.inflightBatches.WithLabelValues(database, instanceID).Inc()
}

func (pm *PrometheusMetrics) DecInflight(database, instanceID string) {
	pm.inflightBatches.WithLabelValues(database, instanceID).Dec()
}

// 新增：go-pipeline 对齐指标录入
func (pm *PrometheusMetrics) RecordPipelineProcessDuration(database, instanceID, status string, d time.Duration) {
	pm.pipelineProcessDuration.WithLabelValues(database, instanceID, status).Observe(d.Seconds())
}

func (pm *PrometheusMetrics) RecordPipelineDequeueLatency(database, instanceID string, d time.Duration) {
	pm.pipelineDequeueLatency.WithLabelValues(database, instanceID).Observe(d.Seconds())
}

func (pm *PrometheusMetrics) IncPipelineDropped(database, instanceID, reason string) {
	pm.pipelineDroppedTotal.WithLabelValues(database, instanceID, reason).Inc()
}

// initializeBaseMetrics 初始化基础指标，确保端点始终返回有效数据
//
// 更新历史：
// - 2025-10-03: 修复测试名称标签不匹配问题，统一使用中文测试名称
// - 2025-12-02: 重构标签体系，使用 instance_id 替代 test_name，支持多实例隔离
//
// 功能说明：
//   - 为所有数据库和测试类型组合初始化指标为 0
//   - 确保 Prometheus 端点始终返回完整的指标集合
//   - 避免 Grafana 查询时出现缺失数据的情况
func (pm *PrometheusMetrics) initializeBaseMetrics() {
	// 初始化计数器指标为 0
	databases := []string{"mysql", "postgres", "sqlite", "redis"}
	// 测试实例标识（集成测试中使用测试名称作为 instance_id）
	instanceIDs := []string{"高吞吐量测试", "并发工作线程测试", "大批次测试", "内存压力测试", "长时间运行测试"}

	for _, db := range databases {
		for _, instanceID := range instanceIDs {
			// 初始化传统测试报告指标（用于 RecordTestResult）
			pm.totalRecordsProcessed.WithLabelValues(db, instanceID, "processed").Add(0)
			pm.totalRecordsProcessed.WithLabelValues(db, instanceID, "actual").Add(0)
			pm.recordsProcessedRate.WithLabelValues(db, instanceID).Add(0)
			pm.totalTestsRun.WithLabelValues(db, instanceID, "success").Add(0)
			pm.totalTestsRun.WithLabelValues(db, instanceID, "failure").Add(0)
			pm.totalErrors.WithLabelValues(db, instanceID, "general").Add(0)
			pm.currentRPS.WithLabelValues(db, instanceID).Set(0)
			pm.memoryUsage.WithLabelValues(db, instanceID, "alloc").Set(0)
			pm.memoryUsage.WithLabelValues(db, instanceID, "total_alloc").Set(0)
			pm.memoryUsage.WithLabelValues(db, instanceID, "sys").Set(0)
			pm.dataIntegrityRate.WithLabelValues(db, instanceID).Set(1.0)
			pm.concurrentWorkers.WithLabelValues(db, instanceID).Set(0)

			// 初始化核心库对齐指标（database, instance_id）
			pm.executorConcurrency.WithLabelValues(db, instanceID).Set(0)
			pm.queueLength.WithLabelValues(db, instanceID).Set(0)
			pm.inflightBatches.WithLabelValues(db, instanceID).Set(0)

			// 初始化常见错误类型
			pm.totalErrors.WithLabelValues(db, instanceID, "retry:deadlock").Add(0)
			pm.totalErrors.WithLabelValues(db, instanceID, "final:context").Add(0)
			pm.totalErrors.WithLabelValues(db, instanceID, "final:non_retryable").Add(0)
		}

		// activeConnections: 1个标签 (database)
		pm.activeConnections.WithLabelValues(db).Set(0)
	}
}

// UpdateActiveConnections 更新活跃连接数
func (pm *PrometheusMetrics) UpdateActiveConnections(database string, count int) {
	pm.activeConnections.WithLabelValues(database).Set(float64(count))
}

// UpdateCurrentRPS 更新当前 RPS
func (pm *PrometheusMetrics) UpdateCurrentRPS(database, testName string, rps float64) {
	pm.currentRPS.WithLabelValues(database, testName).Set(rps)
}

// RecordProcessedRecords 记录处理的记录数（用于实时 RPS 计算）
func (pm *PrometheusMetrics) RecordProcessedRecords(database, testName string, count int) {
	if count > 0 {
		pm.recordsProcessedRate.WithLabelValues(database, testName).Add(float64(count))
	}
}

// UpdateMemoryUsage 更新内存使用情况
//
// 更新历史：
// - 2025-10-03: 确认参数单位正确性，所有参数已经是 MB 单位
//
// 参数说明：
//   - database: 数据库类型标识
//   - testName: 测试名称标识
//   - allocMB: 当前分配的内存量（MB），来自 runtime.ReadMemStats().Alloc 转换
//   - totalAllocMB: 累计分配的内存量（MB），来自 runtime.ReadMemStats().TotalAlloc 转换
//   - sysMB: 系统内存占用（MB），来自 runtime.ReadMemStats().Sys 转换
//
// 注意：所有内存值通过 calculateMemoryDiffMB() 函数从字节转换为 MB，
// 转换公式：MB = bytes / 1024 / 1024
func (pm *PrometheusMetrics) UpdateMemoryUsage(database, testName string, allocMB, totalAllocMB, sysMB float64) {
	pm.memoryUsage.WithLabelValues(database, testName, "alloc").Set(allocMB)
	pm.memoryUsage.WithLabelValues(database, testName, "total_alloc").Set(totalAllocMB)
	pm.memoryUsage.WithLabelValues(database, testName, "sys").Set(sysMB)
}

// GetMetricsURL 获取指标 URL
func (pm *PrometheusMetrics) GetMetricsURL(port int) string {
	return fmt.Sprintf("http://localhost:%d/metrics", port)
}
