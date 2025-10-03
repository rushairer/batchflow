package batchflow_test

import (
	"testing"
	"time"

	"github.com/rushairer/batchflow"
)

func TestNoopMetricsReporter_AllMethods(t *testing.T) {
	m := batchflow.NewNoopMetricsReporter()
	// 不应 panic，全部为空实现
	m.ObserveEnqueueLatency(1 * time.Millisecond)
	m.ObserveBatchAssemble(2 * time.Millisecond)
	m.ObserveExecuteDuration("t", 3, 3*time.Millisecond, "ok")
	m.ObserveBatchSize(10)
	m.IncError("t", "x")
	m.SetConcurrency(5)
	m.SetQueueLength(7)
	m.IncInflight()
	m.DecInflight()
}
