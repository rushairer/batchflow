package batchflow

import "time"

// MetricsConfig 指标导出配置（骨架；默认关闭采样）
type MetricsConfig struct {
	Enabled bool
}

// MetricsReporter 统一指标接口（默认 Noop 实现，避免启用前引入开销）
type MetricsReporter interface {
	// 阶段耗时
	ObserveEnqueueLatency(d time.Duration)                                      // Submit -> 入队
	ObserveBatchAssemble(d time.Duration)                                       // 攒批/组装
	ObserveExecuteDuration(table string, n int, d time.Duration, status string) // 执行

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
