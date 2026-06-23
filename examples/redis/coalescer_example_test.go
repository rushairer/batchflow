package redis_test

import (
	"context"
	"fmt"

	"github.com/rushairer/batchflow/v2"
)

func ExampleNewKeyCoalescer_redisKeys() {
	schema := batchflow.NewSchema("cache", "cmd", "key", "value")
	coalescer := batchflow.NewKeyCoalescer(batchflow.CoalesceKeepLast, "key")

	result, err := coalescer.Coalesce(context.Background(), schema, batchflow.Batch{
		{"cmd": "SET", "key": "user:42", "value": "old"},
		{"cmd": "SET", "key": "user:42", "value": "new"},
		{"cmd": "SET", "key": "user:43", "value": "created"},
	})
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(result.InputItems, result.OutputItems, result.DeduplicatedItems)
	fmt.Println(result.Batch[0]["key"], result.Batch[0]["value"])
	fmt.Println(result.Batch[1]["key"], result.Batch[1]["value"])
	// Output:
	// 3 2 1
	// user:42 new
	// user:43 created
}
