package copy_test

import (
	"context"
	"fmt"
	"time"

	batchflow "github.com/rushairer/batchflow/v2"
)

type fakeCopyClient struct{}

func (c *fakeCopyClient) CopyFrom(ctx context.Context, table string, columns []string, rows [][]any) (int64, error) {
	return int64(len(rows)), nil
}

func ExampleCopyFromExecutor() {
	ctx := context.Background()

	copyExecutor := batchflow.NewCopyFromExecutor(&fakeCopyClient{}).
		WithTimeout(time.Second)

	cfg := batchflow.DefaultConfig(copyExecutor)
	cfg.Pipeline.BufferSize = 1000
	cfg.Pipeline.FlushSize = 100
	cfg.Pipeline.FlushInterval = 10 * time.Millisecond
	cfg.Runtime.ShardCount = 2

	flow, err := batchflow.New(ctx, cfg)
	if err != nil {
		fmt.Println(err)
		return
	}

	schema := batchflow.NewSQLSchema(
		"events",
		batchflow.ConflictIgnoreOperationConfig,
		"id", "event_name",
	)

	req := batchflow.NewRequest(schema).
		SetInt64("id", 1).
		SetString("event_name", "signup")

	if err := flow.Submit(ctx, req); err != nil {
		fmt.Println(err)
		return
	}
	if err := flow.Close(); err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println("copy ok")
	// Output:
	// copy ok
}
