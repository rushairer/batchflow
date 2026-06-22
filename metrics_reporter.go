package batchflow

import "time"

// MetricsReporter 统一指标接口（默认 Noop 实现，避免启用前引入开销）
type MetricsReporter interface {
	// 阶段耗时
	ObserveEnqueueLatency(d time.Duration) // Submit -> 入队
	ObserveBatchAssemble(d time.Duration)  // 攒批/组装
	// ObserveExecuteDuration reports execution duration for a schema/table batch.
	// The first argument is historically named table in implementations; non-SQL
	// backends should pass the schema name.
	ObserveExecuteDuration(table string, n int, d time.Duration, status string)

	// 其他观测
	ObserveBatchSize(n int)
	IncError(table string, typ string)
	SetConcurrency(n int)
	SetQueueLength(n int)
	// 在途批次数（不限流也可观察执行压力）
	IncInflight()
	DecInflight()
}

var _ MetricsReporter = (*NoopMetricsReporter)(nil)

// NoopMetricsReporter 默认关闭时的无操作实现（零开销路径）
type NoopMetricsReporter struct{}

func NewNoopMetricsReporter() *NoopMetricsReporter { return &NoopMetricsReporter{} }

func (*NoopMetricsReporter) ObserveEnqueueLatency(time.Duration)                       {}
func (*NoopMetricsReporter) ObserveBatchAssemble(time.Duration)                        {}
func (*NoopMetricsReporter) ObserveExecuteDuration(string, int, time.Duration, string) {}
func (*NoopMetricsReporter) ObserveBatchSize(int)                                      {}
func (*NoopMetricsReporter) IncError(string, string)                                   {}
func (*NoopMetricsReporter) SetConcurrency(int)                                        {}
func (*NoopMetricsReporter) SetQueueLength(int)                                        {}
func (*NoopMetricsReporter) IncInflight()                                              {}
func (*NoopMetricsReporter) DecInflight()                                              {}

// PipelineMetricsReporter 是对 go-pipeline v2.2.0 WithMetrics 的可选扩展接口。
// - 若实现该接口，框架将把管道级指标事件（通过 pipeline.WithMetrics）桥接到以下方法；
// - 若未实现，则回退到现有 MetricsReporter 的近似指标或直接忽略（保持向后兼容）。
type PipelineMetricsReporter interface {
	// 出队/取用等待时长（元素在队列中的等待时间）
	// 当前由 BatchFlow 在入队成功后自采样并在 flush 开始时上报，
	// 不依赖 go-pipeline 的 MetricsHook。
	ObserveDequeueLatency(d time.Duration)
	// 管道处理总耗时（与执行器/数据库层的 ObserveExecuteDuration 区分）
	ObserveProcessDuration(d time.Duration, status string)
	// 丢弃/拒绝计数（如队列满、背压拒绝等）
	IncDropped(reason string)
}

// BatchFlowMetricsReporter 是 BatchFlow 自身流程指标的可选扩展接口。
// 这些指标不属于执行器层，也不依赖 go-pipeline 的原生 MetricsHook，
// 仅在实现方显式选择时才会上报。
type BatchFlowMetricsReporter interface {
	// IncSubmitRejected 记录 Submit 被拒绝的次数与原因。
	IncSubmitRejected(reason string)
	// ObservePipelineFlushSize 记录一次 pipeline flush 收到的请求数。
	ObservePipelineFlushSize(n int)
	// ObserveSchemaGroupsPerFlush 记录一次 flush 被拆成的 schema 组数。
	ObserveSchemaGroupsPerFlush(n int)
}

// OperationMetricsReporter is the preferred backend-neutral extension for generated
// operation diagnostics. Implementations should keep labels low-cardinality and
// never use raw payloads as labels.
type OperationMetricsReporter interface {
	ObserveOperationGenerated(preview OperationPreview)
	IncOperationError(schema string, backend string, stage string, reason string)
}
