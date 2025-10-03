package batchflow_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rushairer/batchflow"
)

// 通过公开的重试路径间接覆盖分类逻辑：构造一个 Processor，前两次返回可重试错误，第三次成功
type retryingProcessor struct {
	attempt int
}

func (p *retryingProcessor) GenerateOperations(ctx context.Context, schema batchflow.SchemaInterface, data []map[string]any) (batchflow.Operations, error) {
	return batchflow.Operations{}, nil
}

func (p *retryingProcessor) ExecuteOperations(ctx context.Context, ops batchflow.Operations) error {
	p.attempt++
	switch p.attempt {
	case 1:
		return errors.New("timeout: i/o timeout") // 应被判为可重试
	case 2:
		return errors.New("deadlock detected") // 应被判为可重试
	default:
		return nil
	}
}

func TestDefaultRetryClassifier_ThroughExecutor(t *testing.T) {
	exec := batchflow.NewThrottledBatchExecutor(&retryingProcessor{})
	// 配一个较小的 backoff 与最大尝试次数，确保覆盖重试路径
	exec.WithRetryConfig(batchflow.RetryConfig{
		Enabled:     true,
		MaxAttempts: 3,
		BackoffBase: 1 * time.Millisecond,
		MaxBackoff:  2 * time.Millisecond,
	})

	ctx := context.Background()
	schema := batchflow.NewSQLSchema("users", batchflow.ConflictIgnoreOperationConfig, "id")
	if err := exec.ExecuteBatch(ctx, schema, []map[string]any{{"id": 1}}); err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
}
