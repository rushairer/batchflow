package batchflow_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rushairer/batchflow"
)

// fakeProcessor simulates a BatchProcessor that fails the first failCount times with a retryable error.
type fakeProcessor struct {
	failCount  int32
	failReason string
	genCalls   int32
	execCalls  int32
}

func (p *fakeProcessor) GenerateOperations(ctx context.Context, schema batchflow.SchemaInterface, data []map[string]any) (batchflow.Operations, error) {
	atomic.AddInt32(&p.genCalls, 1)
	return batchflow.Operations{}, nil
}

func (p *fakeProcessor) ExecuteOperations(ctx context.Context, ops batchflow.Operations) error {
	atomic.AddInt32(&p.execCalls, 1)
	i := atomic.AddInt32(&p.failCount, -1)
	if i >= 0 {
		// return a retryable error by embedding the reason keyword
		return errors.New(p.failReason)
	}
	return nil
}

// fakeMetrics collects error tags and inflight counters.
type fakeMetrics struct {
	retryErrors []string
	finalErrors []string
	inflight    int32
}

func (m *fakeMetrics) ObserveEnqueueLatency(d time.Duration) {}
func (m *fakeMetrics) ObserveBatchAssemble(d time.Duration)  {}
func (m *fakeMetrics) ObserveExecuteDuration(table string, n int, d time.Duration, status string) {
}
func (m *fakeMetrics) ObserveBatchSize(n int) {}
func (m *fakeMetrics) SetConcurrency(n int)   {}
func (m *fakeMetrics) SetQueueLength(n int)   {}
func (m *fakeMetrics) IncInflight()           { atomic.AddInt32(&m.inflight, 1) }
func (m *fakeMetrics) DecInflight()           { atomic.AddInt32(&m.inflight, -1) }
func (m *fakeMetrics) IncError(table, kind string) {
	if len(kind) >= 6 && kind[:6] == "retry:" {
		m.retryErrors = append(m.retryErrors, kind)
	} else if len(kind) >= 6 && kind[:6] == "final:" {
		m.finalErrors = append(m.finalErrors, kind)
	}
}

func TestThrottledExecutor_Retry_SucceedsBeforeMaxAttempts(t *testing.T) {
	// Fail first 2 attempts with "timeout" (retryable by default classifier), succeed on 3rd.
	proc := &fakeProcessor{failCount: 2, failReason: "timeout"}
	exec := batchflow.NewThrottledBatchExecutor(proc)
	exec.WithRetryConfig(batchflow.RetryConfig{
		Enabled:     true,
		MaxAttempts: 3,
		BackoffBase: 1 * time.Millisecond,
		MaxBackoff:  3 * time.Millisecond,
	})
	metrics := &fakeMetrics{}
	exec.WithMetricsReporter(metrics)

	ctx := context.Background()
	schema := batchflow.NewSQLSchema("users", batchflow.ConflictIgnoreOperationConfig, "id", "name")
	data := []map[string]any{{"id": 1, "name": "a"}}

	if err := exec.ExecuteBatch(ctx, schema, data); err != nil {
		t.Fatalf("expected success after retries, got err: %v", err)
	}
	// must have recorded at least one retry error but no final error
	if len(metrics.retryErrors) == 0 {
		t.Fatalf("expected retry errors recorded")
	}
	if len(metrics.finalErrors) != 0 {
		t.Fatalf("did not expect final errors when eventually succeeded")
	}
	if metrics.inflight != 0 {
		t.Fatalf("inflight should be 0 after completion, got %d", metrics.inflight)
	}
}

func TestThrottledExecutor_Retry_Exhausted(t *testing.T) {
	// Always fail with "deadlock" (retryable), but attempts limited to 2 -> final error recorded.
	proc := &fakeProcessor{failCount: 100, failReason: "deadlock"}
	exec := batchflow.NewThrottledBatchExecutor(proc)
	exec.WithRetryConfig(batchflow.RetryConfig{
		Enabled:     true,
		MaxAttempts: 2,
		BackoffBase: 1 * time.Millisecond,
		MaxBackoff:  2 * time.Millisecond,
	})
	metrics := &fakeMetrics{}
	exec.WithMetricsReporter(metrics)

	ctx := context.Background()
	schema := batchflow.NewSQLSchema("orders", batchflow.ConflictIgnoreOperationConfig, "id")
	data := []map[string]any{{"id": 1}}

	if err := exec.ExecuteBatch(ctx, schema, data); err == nil {
		t.Fatalf("expected final error after exhausting retries")
	}
	if len(metrics.finalErrors) == 0 {
		t.Fatalf("expected final error recorded")
	}
}
