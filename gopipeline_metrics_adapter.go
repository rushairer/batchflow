package batchflow

import (
	"time"

	gopipeline "github.com/rushairer/go-pipeline/v2"
)

// pipelineMetricsAdapter 实现 go-pipeline 的 MetricsHook，
// 并将可用的事件桥接到扩展接口 PipelineMetricsReporter。
// 注意：为避免与批处理层的精确 success/fail 耗时重复统计，
// 这里对 Flush、Error 选择不进行耗时上报，仅处理 ErrorDropped。
type pipelineMetricsAdapter struct {
	reporter MetricsReporter
}

func (a pipelineMetricsAdapter) pmr() (PipelineMetricsReporter, bool) {
	if a.reporter == nil {
		return nil, false
	}
	pmr, ok := a.reporter.(PipelineMetricsReporter)
	return pmr, ok
}

// Flush 在一次 flush 完成后被调用（items: 本次批次大小；duration: 执行耗时）。
// 说明：这里不做额外上报，避免把“整次 flush 输入量”与“单 schema ExecuteBatch 大小”
// 混入同一个 ObserveBatchSize 指标中；详细 flush 维度由 BatchFlow 自身扩展指标负责。
func (a pipelineMetricsAdapter) Flush(_ int, _ time.Duration) {}

// Error 在错误成功写入错误通道时调用。
// 说明：无持续时间，且我们已有执行器/管道级失败耗时上报，因此这里不额外上报。
func (a pipelineMetricsAdapter) Error(_ error) {
	// no-op
}

// ErrorDropped 在错误通道满导致错误被丢弃时调用。
// 映射为扩展接口的丢弃计数。
func (a pipelineMetricsAdapter) ErrorDropped() {
	if pmr, ok := a.pmr(); ok && pmr != nil {
		pmr.IncDropped("error_chan_full")
	}
}

// attachPipelineMetrics 将适配器与 go-pipeline 挂接
func attachPipelineMetrics[T any](p *gopipeline.StandardPipeline[T], r MetricsReporter) {
	if p == nil || r == nil {
		return
	}
	adapter := pipelineMetricsAdapter{reporter: r}
	p.WithMetrics(adapter)
}
