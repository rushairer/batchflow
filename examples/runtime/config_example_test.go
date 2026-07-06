package runtime_test

import (
	"context"
	"fmt"
	"time"

	batchflow "github.com/rushairer/batchflow/v2"
)

type exampleExecutor struct{}

func (e exampleExecutor) ExecuteBatch(ctx context.Context, schema batchflow.SchemaInterface, data []map[string]any) error {
	return nil
}

func ExampleNew_runtimeConfig() {
	ctx := context.Background()

	cfg := batchflow.DefaultConfig(exampleExecutor{})
	cfg.Pipeline.BufferSize = 1000
	cfg.Pipeline.FlushSize = 100
	cfg.Pipeline.FlushInterval = 50 * time.Millisecond
	cfg.Runtime.ShardCount = 2
	cfg.Runtime.Routing = batchflow.ShardRoutingHash
	cfg.Runtime.Backpressure = batchflow.BackpressureConfig{
		Enabled:       true,
		Mode:          batchflow.BackpressureTimeout,
		HighWatermark: 800,
		Timeout:       500 * time.Millisecond,
	}
	cfg.Runtime.MemoryLimit = batchflow.MemoryLimitConfig{
		Enabled:         true,
		MaxQueueBytes:   128 << 20,
		AvgRequestBytes: 512,
		Mode:            batchflow.BackpressureTimeout,
		Timeout:         500 * time.Millisecond,
	}

	flow, err := batchflow.New(ctx, cfg)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer flow.Close()

	schema := batchflow.NewSchema("events", "id", "payload")
	req := batchflow.NewRequest(schema).
		SetInt64("id", 1).
		SetString("payload", "hello")

	fmt.Println(flow.Submit(ctx, req) == nil)
	// Output:
	// true
}
