package runtime_test

import (
	"context"
	"fmt"
	"time"

	batchflow "github.com/rushairer/batchflow/v2"
)

type captureExecutor struct{}

func (e *captureExecutor) ExecuteBatch(ctx context.Context, schema batchflow.SchemaInterface, data []map[string]any) error {
	return nil
}

func ExampleNew() {
	ctx := context.Background()

	cfg := batchflow.DefaultConfig(&captureExecutor{})
	cfg.Pipeline.BufferSize = 1000
	cfg.Pipeline.FlushSize = 100
	cfg.Pipeline.FlushInterval = 10 * time.Millisecond
	cfg.Runtime.ShardCount = 2
	cfg.Runtime.Backpressure = batchflow.BackpressureConfig{
		Enabled:       true,
		Mode:          batchflow.BackpressureTimeout,
		HighWatermark: 800,
		Timeout:       100 * time.Millisecond,
	}
	cfg.Runtime.MemoryLimit = batchflow.MemoryLimitConfig{
		Enabled:         true,
		MaxQueueBytes:   64 << 20,
		AvgRequestBytes: 512,
		Mode:            batchflow.BackpressureTimeout,
		Timeout:         100 * time.Millisecond,
	}

	flow, err := batchflow.New(ctx, cfg)
	if err != nil {
		fmt.Println(err)
		return
	}

	schema := batchflow.NewSchema("events", "id", "name")
	req := batchflow.NewRequest(schema).
		SetInt64("id", 1).
		SetString("name", "signup")

	if err := flow.Submit(ctx, req); err != nil {
		fmt.Println(err)
		return
	}
	if err := flow.Close(); err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println("ok")
	// Output:
	// ok
}
