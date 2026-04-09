package main

import (
	"time"

	"github.com/rushairer/batchflow"
)

// PrometheusMetricsReporter 实现 batchflow.MetricsReporter，将批处理指标上报至 Prometheus
//
// 标签设计说明：
// - database: 数据库类型（mysql/postgres/sqlite/redis）
// - instance_id: BatchFlow 实例标识，支持多实例隔离
//   - 在集成测试中使用 testName 作为 instance_id
//   - 在生产环境中建议使用有意义的业务标识（如 "order_writer", "log_collector"）
type PrometheusMetricsReporter struct {
	prometheusMetrics *PrometheusMetrics
	database          string // 数据库类型：mysql/postgres/sqlite/redis
	instanceID        string // 实例标识：多个 BatchFlow 实例的唯一标识
}

// NewPrometheusMetricsReporter 创建 Prometheus 指标报告器
// - database: 数据库类型（mysql/postgres/sqlite/redis）
// - instanceID: 实例标识，用于区分多个 BatchFlow 实例（集成测试中使用 testName）
func NewPrometheusMetricsReporter(prometheusMetrics *PrometheusMetrics, database, instanceID string) batchflow.MetricsReporter {
	return &PrometheusMetricsReporter{
		prometheusMetrics: prometheusMetrics,
		database:          database,
		instanceID:        instanceID,
	}
}

// ========== batchflow.MetricsReporter 接口实现 ==========

// ObserveEnqueueLatency 记录提交到入队的延迟
func (r *PrometheusMetricsReporter) ObserveEnqueueLatency(d time.Duration) {
	if r.prometheusMetrics == nil {
		return
	}
	r.prometheusMetrics.RecordEnqueueLatency(r.database, r.instanceID, d)
}

// ObserveBatchAssemble 记录批次组装耗时
func (r *PrometheusMetricsReporter) ObserveBatchAssemble(d time.Duration) {
	if r.prometheusMetrics == nil {
		return
	}
	r.prometheusMetrics.RecordAssembleDuration(r.database, r.instanceID, d)
}

// ObserveExecuteDuration 记录批次执行耗时（含重试）
// - table: 表名（可选，当前实现忽略以避免高基数）
// - n: 批次大小
// - d: 执行耗时
// - status: 状态（success/fail）
func (r *PrometheusMetricsReporter) ObserveExecuteDuration(table string, n int, d time.Duration, status string) {
	if r.prometheusMetrics == nil {
		return
	}
	// 记录执行耗时（按状态区分）
	r.prometheusMetrics.RecordExecuteDuration(r.database, r.instanceID, status, d)
	_ = table
	_ = n
}

// ObserveBatchSize 单独记录批次大小（某些场景可能独立调用）
func (r *PrometheusMetricsReporter) ObserveBatchSize(n int) {
	if r.prometheusMetrics == nil {
		return
	}
	r.prometheusMetrics.RecordBatchSize(r.database, r.instanceID, n)
}

// IncError 记录错误计数
// - table: 表名（当前实现忽略）
// - reason: 错误原因，必须以 "retry:" 或 "final:" 开头（与核心库约定一致）
func (r *PrometheusMetricsReporter) IncError(table string, reason string) {
	if r.prometheusMetrics == nil {
		return
	}
	// 与核心库约定对齐：error_type 必须以 "retry:" 或 "final:" 等前缀开头
	r.prometheusMetrics.totalErrors.WithLabelValues(r.database, r.instanceID, reason).Inc()
}

// SetConcurrency 设置执行器并发度（Gauge 类型）
func (r *PrometheusMetricsReporter) SetConcurrency(n int) {
	if r.prometheusMetrics == nil {
		return
	}
	r.prometheusMetrics.SetExecutorConcurrency(r.database, r.instanceID, n)
}

// SetQueueLength 设置队列长度（Gauge 类型）
func (r *PrometheusMetricsReporter) SetQueueLength(n int) {
	if r.prometheusMetrics == nil {
		return
	}
	r.prometheusMetrics.SetQueueLength(r.database, r.instanceID, n)
}

// IncInflight 在途批次数+1
func (r *PrometheusMetricsReporter) IncInflight() {
	if r.prometheusMetrics == nil {
		return
	}
	r.prometheusMetrics.IncInflight(r.database, r.instanceID)
}

// DecInflight 在途批次数-1
func (r *PrometheusMetricsReporter) DecInflight() {
	if r.prometheusMetrics == nil {
		return
	}
	r.prometheusMetrics.DecInflight(r.database, r.instanceID)
}

// ========== batchflow.PipelineMetricsReporter 接口实现（可选扩展）==========

// ObserveDequeueLatency 记录出队等待延迟（元素在队列中的等待时间）
func (r *PrometheusMetricsReporter) ObserveDequeueLatency(d time.Duration) {
	if r.prometheusMetrics == nil {
		return
	}
	r.prometheusMetrics.RecordPipelineDequeueLatency(r.database, r.instanceID, d)
}

// ObserveProcessDuration 记录管道级处理耗时（与执行器级指标区分）
func (r *PrometheusMetricsReporter) ObserveProcessDuration(d time.Duration, status string) {
	if r.prometheusMetrics == nil {
		return
	}
	r.prometheusMetrics.RecordPipelineProcessDuration(r.database, r.instanceID, status, d)
}

// IncDropped 记录丢弃事件（如错误通道饱和）
func (r *PrometheusMetricsReporter) IncDropped(reason string) {
	if r.prometheusMetrics == nil {
		return
	}
	r.prometheusMetrics.IncPipelineDropped(r.database, r.instanceID, reason)
}
