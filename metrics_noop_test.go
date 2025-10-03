package batchflow_test

import (
	"testing"
	"time"

	"github.com/rushairer/batchflow"
)

// 确认 NoopMetricsReporter 的各方法可安全调用且不 panic
func TestNoopMetricsReporter_SafeCalls(t *testing.T) {
	var m batchflow.NoopMetricsReporter
	m.ObserveEnqueueLatency(5 * time.Millisecond)
	m.ObserveBatchAssemble(3 * time.Millisecond)
	m.ObserveExecuteDuration("users", 10, 7*time.Millisecond, "success")
	m.ObserveBatchSize(100)
	m.SetConcurrency(8)
	m.SetQueueLength(42)
	m.IncInflight()
	m.DecInflight()
	m.IncError("users", "retry:timeout")
	// 没有断言，只要不 panic 即可
}
