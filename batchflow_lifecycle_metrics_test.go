package batchflow_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rushairer/batchflow/v2"
)

type lifecycleMetrics struct {
	rejectedCalls    int32
	flushSizeCalls   int32
	schemaGroupCalls int32
	dequeueCalls     int32
	processCalls     int32
	lastRejectReason atomic.Value
}

func (*lifecycleMetrics) ObserveEnqueueLatency(time.Duration) {}
func (*lifecycleMetrics) ObserveBatchAssemble(time.Duration)  {}
func (*lifecycleMetrics) ObserveExecuteDuration(string, int, time.Duration, string) {
}
func (*lifecycleMetrics) ObserveBatchSize(int)                  {}
func (*lifecycleMetrics) IncError(string, string)               {}
func (*lifecycleMetrics) SetConcurrency(int)                    {}
func (*lifecycleMetrics) SetQueueLength(int)                    {}
func (*lifecycleMetrics) IncInflight()                          {}
func (*lifecycleMetrics) DecInflight()                          {}
func (m *lifecycleMetrics) ObserveDequeueLatency(time.Duration) { atomic.AddInt32(&m.dequeueCalls, 1) }
func (m *lifecycleMetrics) ObserveProcessDuration(time.Duration, string) {
	atomic.AddInt32(&m.processCalls, 1)
}
func (*lifecycleMetrics) IncDropped(string) {}
func (m *lifecycleMetrics) IncSubmitRejected(reason string) {
	atomic.AddInt32(&m.rejectedCalls, 1)
	m.lastRejectReason.Store(reason)
}

func (m *lifecycleMetrics) ObservePipelineFlushSize(int) {
	atomic.AddInt32(&m.flushSizeCalls, 1)
}

func (m *lifecycleMetrics) ObserveSchemaGroupsPerFlush(int) {
	atomic.AddInt32(&m.schemaGroupCalls, 1)
}

type okLifecycleProcessor struct{}

func (okLifecycleProcessor) GenerateOperations(ctx context.Context, schema batchflow.SchemaInterface, data []map[string]any) (batchflow.Operations, error) {
	return batchflow.Operations{}, nil
}

func (okLifecycleProcessor) ExecuteOperations(ctx context.Context, ops batchflow.Operations) error {
	return nil
}

func TestBatchFlow_CloseFlushesBufferedRequests(t *testing.T) {
	ctx := context.Background()
	cfg := batchflow.PipelineConfig{
		BufferSize:    16,
		FlushSize:     100,
		FlushInterval: time.Hour,
	}
	b, mock := batchflow.NewBatchFlowWithMock(ctx, cfg)

	schema := batchflow.NewSQLSchema("users", batchflow.ConflictIgnoreOperationConfig, "id", "name")
	req := batchflow.NewRequest(schema).SetInt64("id", 1).SetString("name", "alice")

	if err := b.Submit(ctx, req); err != nil {
		t.Fatalf("submit failed: %v", err)
	}

	if err := b.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}

	results := mock.SnapshotResults()
	if got := results["users"]["rows"]; got != 1 {
		t.Fatalf("expected final flush to persist 1 row, got %d", got)
	}
}

func TestBatchFlow_MetricsExtensions_ObserveFlushAndRejects(t *testing.T) {
	ctx := context.Background()
	reporter := &lifecycleMetrics{}
	exec := batchflow.NewThrottledBatchExecutor(okLifecycleProcessor{}).
		WithMetricsReporter(reporter)
	b := batchflow.NewBatchFlow(ctx, 16, 100, time.Hour, exec)

	schema := batchflow.NewSQLSchema("users", batchflow.ConflictIgnoreOperationConfig, "id")
	req := batchflow.NewRequest(schema).SetInt64("id", 1)

	if err := b.Submit(ctx, req); err != nil {
		t.Fatalf("submit failed: %v", err)
	}
	if err := b.Submit(ctx, nil); err == nil {
		t.Fatalf("expected nil request to be rejected")
	}
	if err := b.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}

	if atomic.LoadInt32(&reporter.dequeueCalls) == 0 {
		t.Fatalf("expected dequeue latency to be observed")
	}
	if atomic.LoadInt32(&reporter.processCalls) == 0 {
		t.Fatalf("expected pipeline process duration to be observed")
	}
	if atomic.LoadInt32(&reporter.flushSizeCalls) == 0 {
		t.Fatalf("expected pipeline flush size to be observed")
	}
	if atomic.LoadInt32(&reporter.schemaGroupCalls) == 0 {
		t.Fatalf("expected schema groups per flush to be observed")
	}
	if atomic.LoadInt32(&reporter.rejectedCalls) == 0 {
		t.Fatalf("expected rejected submit to be observed")
	}
	if got, _ := reporter.lastRejectReason.Load().(string); got != "empty_request" {
		t.Fatalf("expected empty_request reject reason, got %q", got)
	}
}

func TestBatchFlow_DoneClosesAndWaitReturnsNilAfterClose(t *testing.T) {
	ctx := context.Background()
	cfg := batchflow.PipelineConfig{
		BufferSize:    16,
		FlushSize:     100,
		FlushInterval: time.Hour,
	}
	b, _ := batchflow.NewBatchFlowWithMock(ctx, cfg)

	schema := batchflow.NewSQLSchema("users", batchflow.ConflictIgnoreOperationConfig, "id")
	if err := b.Submit(ctx, batchflow.NewRequest(schema).SetInt64("id", 1)); err != nil {
		t.Fatalf("submit failed: %v", err)
	}

	done := b.Done()
	select {
	case <-done:
		t.Fatalf("done should not be closed before shutdown")
	default:
	}

	if err := b.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("done was not closed after close")
	}

	if err := b.Wait(); err != nil {
		t.Fatalf("wait after close failed: %v", err)
	}

	if err := b.Close(); err != nil {
		t.Fatalf("second close should be idempotent, got %v", err)
	}
}

func TestBatchFlow_CloseReturnsContextCancellationAfterParentCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg := batchflow.PipelineConfig{
		BufferSize:    16,
		FlushSize:     100,
		FlushInterval: time.Hour,
	}
	b, _ := batchflow.NewBatchFlowWithMock(ctx, cfg)

	cancel()

	err := b.Close()
	if err != nil && !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "context is canceled") {
		t.Fatalf("close err=%v, want cancellation-related error", err)
	}

	select {
	case <-b.Done():
	case <-time.After(2 * time.Second):
		t.Fatalf("done was not closed after parent context cancellation")
	}
}
