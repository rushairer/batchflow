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
// 说明：这里不做耗时上报，避免与 batchflow.flushFunc 中基于 err 判定 success/fail 的耗时统计重复。
func (a pipelineMetricsAdapter) Flush(items int, duration time.Duration) {
	// 启用批大小上报；耗时仍由 batchflow.flushFunc defer 负责（含成功/失败区分），避免重复统计
	if a.reporter != nil && items > 0 {
		a.reporter.ObserveBatchSize(items)
	}
}

// Error 在错误成功写入错误通道时调用。
// 说明：无持续时间，且我们已有执行器/管道级失败耗时上报，因此这里不额外上报。
func (a pipelineMetricsAdapter) Error(err error) {
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
func attachPipelineMetrics(p *gopipeline.StandardPipeline[*Request], r MetricsReporter) {
	if p == nil || r == nil {
		return
	}
	adapter := pipelineMetricsAdapter{reporter: r}
	p.WithMetrics(adapter)
}
