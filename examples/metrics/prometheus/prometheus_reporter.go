package prometheusmetrics

import (
	"time"

	"github.com/rushairer/batchflow"
)

// Reporter 实现 batchflow.MetricsReporter + batchflow.PipelineMetricsReporter
// + batchflow.BatchFlowMetricsReporter
// 将 BatchFlow 指标写入 Prometheus
type Reporter struct {
	m *Metrics

	// 绑定维度
	Database   string // 数据库类型（mysql/postgres/sqlite/redis）
	InstanceID string // 实例标识（支持多实例隔离，如 "order_writer", "log_collector"）
}

// NewReporter 创建 Reporter
// - database: 数据库类型（必须）
// - instanceID: 实例标识（生产环境推荐使用业务标识，测试环境可使用测试名称）
func NewReporter(m *Metrics, database, instanceID string) *Reporter {
	return &Reporter{m: m, Database: database, InstanceID: instanceID}
}

// ========== batchflow.MetricsReporter 接口实现 ==========

// ObserveEnqueueLatency 提交到入队延迟
func (r *Reporter) ObserveEnqueueLatency(d time.Duration) {
	if r.m == nil {
		return
	}
	r.m.observeEnqueue(r.Database, r.InstanceID, d)
}

// ObserveBatchAssemble 攒批耗时
func (r *Reporter) ObserveBatchAssemble(d time.Duration) {
	if r.m == nil {
		return
	}
	r.m.observeAssemble(r.Database, r.InstanceID, d)
}

// ObserveExecuteDuration 执行耗时（含重试与退避）
func (r *Reporter) ObserveExecuteDuration(table string, n int, d time.Duration, status string) {
	if r.m == nil {
		return
	}
	r.m.observeExecute(r.Database, r.InstanceID, table, n, d, status)
}

// ObserveBatchSize 单独记录批大小
func (r *Reporter) ObserveBatchSize(n int) {
	if r.m == nil {
		return
	}
	// 内部会根据标签维度配置自动选择
	labels := []string{r.Database}
	if hasLabel(r.m.batchSize, "instance_id") {
		labels = []string{r.Database, r.InstanceID}
	}
	r.m.batchSize.WithLabelValues(labels...).Observe(float64(n))
}

// IncError 错误计数（支持 retry:/final: 前缀）
func (r *Reporter) IncError(_ string, reason string) {
	if r.m == nil {
		return
	}
	// 与核心库一致：reason 包含 "retry:xxx" 或 "final:yyy"
	r.m.incError(r.Database, r.InstanceID, reason)
}

// SetConcurrency 执行并发度
func (r *Reporter) SetConcurrency(n int) {
	if r.m == nil {
		return
	}
	r.m.setConcurrency(r.Database, r.InstanceID, n)
}

// SetQueueLength 队列长度
func (r *Reporter) SetQueueLength(n int) {
	if r.m == nil {
		return
	}
	r.m.setQueueLen(r.Database, r.InstanceID, n)
}

// IncInflight 在途+1
func (r *Reporter) IncInflight() {
	if r.m == nil {
		return
	}
	r.m.incInflight(r.Database, r.InstanceID)
}

// DecInflight 在途-1
func (r *Reporter) DecInflight() {
	if r.m == nil {
		return
	}
	r.m.decInflight(r.Database, r.InstanceID)
}

// ========== batchflow.PipelineMetricsReporter 接口实现（可选扩展）==========

// ObserveDequeueLatency 出队等待延迟（元素在队列中的等待时间）
func (r *Reporter) ObserveDequeueLatency(d time.Duration) {
	if r.m == nil || r.m.pipelineDequeueLatency == nil {
		return
	}
	labels := []string{r.Database}
	if hasLabel(r.m.pipelineDequeueLatency, "instance_id") {
		labels = []string{r.Database, r.InstanceID}
	}
	r.m.pipelineDequeueLatency.WithLabelValues(labels...).Observe(d.Seconds())
}

// ObserveProcessDuration 管道级处理耗时（与执行器级指标区分）
func (r *Reporter) ObserveProcessDuration(d time.Duration, status string) {
	if r.m == nil || r.m.pipelineProcessDuration == nil {
		return
	}
	var labels []string
	if hasLabel(r.m.pipelineProcessDuration, "instance_id") {
		labels = []string{r.Database, r.InstanceID, status}
	} else {
		labels = []string{r.Database, status}
	}
	r.m.pipelineProcessDuration.WithLabelValues(labels...).Observe(d.Seconds())
}

// IncDropped 丢弃事件计数（如错误通道饱和）
func (r *Reporter) IncDropped(reason string) {
	if r.m == nil || r.m.pipelineDroppedTotal == nil {
		return
	}
	var labels []string
	if hasLabel(r.m.pipelineDroppedTotal, "instance_id") {
		labels = []string{r.Database, r.InstanceID, reason}
	} else {
		labels = []string{r.Database, reason}
	}
	r.m.pipelineDroppedTotal.WithLabelValues(labels...).Inc()
}

// ========== batchflow.BatchFlowMetricsReporter 接口实现（可选扩展）==========

// IncSubmitRejected 记录 Submit 被拒绝的原因。
func (r *Reporter) IncSubmitRejected(reason string) {
	if r.m == nil {
		return
	}
	r.m.incSubmitRejected(r.Database, r.InstanceID, reason)
}

// ObservePipelineFlushSize 记录一次 pipeline flush 接收到的请求数。
func (r *Reporter) ObservePipelineFlushSize(n int) {
	if r.m == nil {
		return
	}
	r.m.observePipelineFlushSize(r.Database, r.InstanceID, n)
}

// ObserveSchemaGroupsPerFlush 记录一次 flush 拆出的 schema 组数。
func (r *Reporter) ObserveSchemaGroupsPerFlush(n int) {
	if r.m == nil {
		return
	}
	r.m.observeSchemaGroups(r.Database, r.InstanceID, n)
}

// 确保实现接口
var (
	_ batchflow.MetricsReporter          = (*Reporter)(nil)
	_ batchflow.PipelineMetricsReporter  = (*Reporter)(nil)
	_ batchflow.BatchFlowMetricsReporter = (*Reporter)(nil)
)
