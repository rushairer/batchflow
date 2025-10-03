package batchflow_test

import (
	"context"
	"testing"
	"time"

	"github.com/rushairer/batchflow"
)

func TestBatchFlow_Submit_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	cfg := batchflow.PipelineConfig{
		BufferSize:    16,
		FlushSize:     8,
		FlushInterval: 50 * time.Millisecond,
	}
	b, _ := batchflow.NewBatchFlowWithMock(ctx, cfg)

	schema := batchflow.NewSQLSchema("users", batchflow.ConflictIgnoreOperationConfig, "id", "name")
	req := batchflow.NewRequest(schema).SetInt64("id", 1).SetString("name", "a")

	if err := b.Submit(ctx, req); err == nil {
		t.Fatalf("expected submit to fail when pipeline context already cancelled")
	}
}

func TestBatchFlow_Submit_ImmediateCtxErr(t *testing.T) {
	ctx := context.Background()
	cfg := batchflow.PipelineConfig{
		BufferSize:    16,
		FlushSize:     8,
		FlushInterval: 50 * time.Millisecond,
	}
	b, _ := batchflow.NewBatchFlowWithMock(ctx, cfg)

	schema := batchflow.NewSQLSchema("users", batchflow.ConflictIgnoreOperationConfig, "id")
	req := batchflow.NewRequest(schema).SetInt64("id", 1)

	// pass a cancelled ctx to Submit
	reqCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := b.Submit(reqCtx, req); err == nil {
		t.Fatalf("expected submit to fail with cancelled ctx")
	}
}
