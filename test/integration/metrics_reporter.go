package main

import (
	"time"

	"github.com/rushairer/batchflow"
)

// PrometheusMetricsReporter 实现 batchflow.MetricsReporter，将批处理指标上报至 Prometheus
type PrometheusMetricsReporter struct {
	prometheusMetrics *PrometheusMetrics
	database          string
	testName          string
}

// NewPrometheusMetricsReporter 创建 Prometheus 指标报告器
func NewPrometheusMetricsReporter(prometheusMetrics *PrometheusMetrics, database, testName string) batchflow.MetricsReporter {
	return &PrometheusMetricsReporter{
		prometheusMetrics: prometheusMetrics,
		database:          database,
		testName:          testName,
	}
}

// 以下为新接口适配

func (r *PrometheusMetricsReporter) ObserveEnqueueLatency(d time.Duration) {
	if r.prometheusMetrics == nil {
		return
	}
	r.prometheusMetrics.RecordEnqueueLatency(r.database, d)
}

func (r *PrometheusMetricsReporter) ObserveBatchAssemble(d time.Duration) {
	if r.prometheusMetrics == nil {
		return
	}
	r.prometheusMetrics.RecordAssembleDuration(r.database, d)
}

func (r *PrometheusMetricsReporter) ObserveExecuteDuration(table string, n int, d time.Duration, status string) {
	if r.prometheusMetrics == nil {
		return
	}
	r.prometheusMetrics.RecordExecuteDuration(r.database, table, status, d)
	r.prometheusMetrics.RecordBatchSize(r.database, n)
}

func (r *PrometheusMetricsReporter) ObserveBatchSize(n int) {
	if r.prometheusMetrics == nil {
		return
	}
	r.prometheusMetrics.RecordBatchSize(r.database, n)
}

func (r *PrometheusMetricsReporter) IncError(table string, reason string) {
	if r.prometheusMetrics == nil {
		return
	}
	// 与核心库约定对齐：error_type 必须以 "retry:" 或 "final:" 等前缀开头，便于 PromQL 正确匹配
	// 此处不再拼接 table，避免将标签值从 "retry:xxx" 变成 "table:retry:xxx" 导致查询失配
	r.prometheusMetrics.totalErrors.WithLabelValues(r.database, r.testName, reason).Inc()
}

func (r *PrometheusMetricsReporter) SetConcurrency(n int) {
	if r.prometheusMetrics == nil {
		return
	}
	r.prometheusMetrics.SetExecutorConcurrency(r.database, n)
}

func (r *PrometheusMetricsReporter) SetQueueLength(n int) {
	if r.prometheusMetrics == nil {
		return
	}
	r.prometheusMetrics.SetQueueLength(r.database, n)
}

func (r *PrometheusMetricsReporter) IncInflight() {
	if r.prometheusMetrics == nil {
		return
	}
	r.prometheusMetrics.IncInflight(r.database)
}

func (r *PrometheusMetricsReporter) DecInflight() {
	if r.prometheusMetrics == nil {
		return
	}
	r.prometheusMetrics.DecInflight(r.database)
}
