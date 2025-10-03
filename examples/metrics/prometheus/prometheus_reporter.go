package prometheusmetrics

import (
	"time"

	"github.com/rushairer/batchflow"
)

// Reporter 实现 batchflow.MetricsReporter，写入 Prometheus 指标
type Reporter struct {
	m *Metrics

	// 绑定维度
	Database string
	TestName string
}

// NewReporter 创建 Reporter；database 为必须，testName 可按需要传入
func NewReporter(m *Metrics, database, testName string) *Reporter {
	return &Reporter{m: m, Database: database, TestName: testName}
}

// ObserveEnqueueLatency 提交到入队延迟
func (r *Reporter) ObserveEnqueueLatency(d time.Duration) {
	if r.m == nil {
		return
	}
	r.m.observeEnqueue(r.Database, r.TestName, d)
}

// ObserveBatchAssemble 攒批耗时
func (r *Reporter) ObserveBatchAssemble(d time.Duration) {
	if r.m == nil {
		return
	}
	r.m.observeAssemble(r.Database, r.TestName, d)
}

// ObserveExecuteDuration 执行耗时（含重试与退避）
func (r *Reporter) ObserveExecuteDuration(table string, n int, d time.Duration, status string) {
	if r.m == nil {
		return
	}
	r.m.observeExecute(r.Database, r.TestName, table, n, d, status)
}

// ObserveBatchSize 单独记录批大小
func (r *Reporter) ObserveBatchSize(n int) {
	if r.m == nil {
		return
	}
	// 复用 observeExecute 内部逻辑：这里仅记录 batchSize
	r.m.batchSize.WithLabelValues(r.Database, r.TestName).Observe(float64(n))
}

// IncError 错误计数（支持 retry:/final: 前缀）
func (r *Reporter) IncError(_ string, reason string) {
	if r.m == nil {
		return
	}
	// 与核心库一致：reason 包含 "retry:xxx" 或 "final:yyy"
	r.m.incError(r.Database, r.TestName, reason)
}

// SetConcurrency 执行并发度
func (r *Reporter) SetConcurrency(n int) {
	if r.m == nil {
		return
	}
	r.m.setConcurrency(r.Database, r.TestName, n)
}

// SetQueueLength 队列长度
func (r *Reporter) SetQueueLength(n int) {
	if r.m == nil {
		return
	}
	r.m.setQueueLen(r.Database, r.TestName, n)
}

// IncInflight 在途+1
func (r *Reporter) IncInflight() {
	if r.m == nil {
		return
	}
	r.m.incInflight(r.Database, r.TestName)
}

// DecInflight 在途-1
func (r *Reporter) DecInflight() {
	if r.m == nil {
		return
	}
	r.m.decInflight(r.Database, r.TestName)
}

// 确保实现接口
var _ batchflow.MetricsReporter = (*Reporter)(nil)
