package batchflow_test

import (
	"context"
	"time"

	"github.com/rushairer/batchflow/v2"
)

func ExampleBatchFlow_Close() {
	ctx := context.Background()
	flow, _ := batchflow.NewBatchFlowWithMock(ctx, batchflow.PipelineConfig{
		BufferSize:    16,
		FlushSize:     100,
		FlushInterval: time.Second,
	})

	schema := batchflow.NewSQLSchema("users", batchflow.ConflictIgnoreOperationConfig, "id", "name")
	req := batchflow.NewRequest(schema).
		SetUint64("id", 1).
		SetString("name", "alice")

	if err := flow.Submit(ctx, req); err != nil {
		panic(err)
	}

	if err := flow.Close(); err != nil {
		panic(err)
	}
}

func ExampleBatchFlow_Done() {
	ctx := context.Background()
	flow, _ := batchflow.NewBatchFlowWithMock(ctx, batchflow.PipelineConfig{
		BufferSize:    16,
		FlushSize:     100,
		FlushInterval: time.Second,
	})

	stopped := make(chan struct{})
	go func() {
		<-flow.Done()
		close(stopped)
	}()

	if err := flow.Close(); err != nil {
		panic(err)
	}

	<-stopped
}
