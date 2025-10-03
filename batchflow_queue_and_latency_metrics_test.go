package batchflow_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rushairer/batchflow"
)

type fakeQueueMetrics struct {
	enqueueCalls int32
	setQLCalls   int32
}

func (f *fakeQueueMetrics) ObserveEnqueueLatency(d time.Duration) {
	atomic.AddInt32(&f.enqueueCalls, 1)
}
func (f *fakeQueueMetrics) ObserveBatchAssemble(d time.Duration) {}
func (f *fakeQueueMetrics) ObserveExecuteDuration(table string, n int, d time.Duration, status string) {
}
func (f *fakeQueueMetrics) ObserveBatchSize(n int)      {}
func (f *fakeQueueMetrics) SetConcurrency(n int)        {}
func (f *fakeQueueMetrics) SetQueueLength(n int)        { atomic.AddInt32(&f.setQLCalls, 1) }
func (f *fakeQueueMetrics) IncInflight()                {}
func (f *fakeQueueMetrics) DecInflight()                {}
func (f *fakeQueueMetrics) IncError(table, kind string) {}

func TestBatchFlow_Submit_QueueAndLatencyMetrics(t *testing.T) {
	t.Skip("Pipeline 级 MetricsReporter 暂无对外注入入口，仅 Executor 支持；此用例暂跳过")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := batchflow.PipelineConfig{
		BufferSize:    10,
		FlushSize:     10_000,
		FlushInterval: 200 * time.Millisecond,
	}
	b, mock := batchflow.NewBatchFlowWithMock(ctx, cfg)
	m := &fakeQueueMetrics{}

	schema := batchflow.NewSQLSchema("users", batchflow.ConflictIgnoreOperationConfig, "id")
	req := batchflow.NewRequest(schema).SetInt64("id", 1)

	// 提交一次请求，触发入队延迟和队列长度埋点
	if err := b.Submit(ctx, req); err != nil {
		t.Fatalf("submit failed: %v", err)
	}

	// 稍等让后台 goroutine 有机会调用 SetQueueLength
	time.Sleep(10 * time.Millisecond)

	if atomic.LoadInt32(&m.enqueueCalls) == 0 {
		t.Fatalf("expected ObserveEnqueueLatency to be called at least once")
	}
	if atomic.LoadInt32(&m.setQLCalls) == 0 {
		t.Fatalf("expected SetQueueLength to be called at least once")
	}

	// 收尾
	_ = mock // 防止未使用告警
}
