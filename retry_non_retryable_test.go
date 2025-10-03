package batchflow_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rushairer/batchflow"
)

type nonRetryProcessor struct{}

func (nonRetryProcessor) GenerateOperations(ctx context.Context, schema batchflow.SchemaInterface, data []map[string]any) (batchflow.Operations, error) {
	return batchflow.Operations{}, nil
}

func (nonRetryProcessor) ExecuteOperations(ctx context.Context, ops batchflow.Operations) error {
	// 语义上应被判定为不可重试（non_retryable）
	return errors.New("syntax error near 'VALUES'")
}

type nonRetryMetrics struct {
	retry int32
	final int32
}

func (m *nonRetryMetrics) ObserveEnqueueLatency(d time.Duration) {}
func (m *nonRetryMetrics) ObserveBatchAssemble(d time.Duration)  {}
func (m *nonRetryMetrics) ObserveExecuteDuration(table string, n int, d time.Duration, status string) {
}
func (m *nonRetryMetrics) ObserveBatchSize(n int) {}
func (m *nonRetryMetrics) SetConcurrency(n int)   {}
func (m *nonRetryMetrics) SetQueueLength(n int)   {}
func (m *nonRetryMetrics) IncInflight()           {}
func (m *nonRetryMetrics) DecInflight()           {}
func (m *nonRetryMetrics) IncError(table, kind string) {
	if len(kind) >= 6 && kind[:6] == "retry:" {
		atomic.AddInt32(&m.retry, 1)
	} else if len(kind) >= 6 && kind[:6] == "final:" {
		atomic.AddInt32(&m.final, 1)
	}
}

func TestThrottledExecutor_Retry_NonRetryableImmediateFinal(t *testing.T) {
	exec := batchflow.NewThrottledBatchExecutor(nonRetryProcessor{})
	exec.WithRetryConfig(batchflow.RetryConfig{
		Enabled:     true,
		MaxAttempts: 3, // 即便允许重试，非重试错误也应立即 final
		BackoffBase: 1 * time.Millisecond,
		MaxBackoff:  2 * time.Millisecond,
	})
	metrics := &nonRetryMetrics{}
	exec.WithMetricsReporter(metrics)

	ctx := context.Background()
	schema := batchflow.NewSQLSchema("users", batchflow.ConflictIgnoreOperationConfig, "id")
	err := exec.ExecuteBatch(ctx, schema, []map[string]any{{"id": 1}})
	if err == nil {
		t.Fatalf("expected immediate final failure for non-retryable error")
	}
	if atomic.LoadInt32(&metrics.retry) != 0 {
		t.Fatalf("unexpected retry recorded for non-retryable error")
	}
	if atomic.LoadInt32(&metrics.final) == 0 {
		t.Fatalf("expected final error recorded")
	}
}
