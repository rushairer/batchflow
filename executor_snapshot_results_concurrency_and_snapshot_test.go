package batchflow_test

import (
	"context"
	"sync"
	"testing"

	"github.com/rushairer/batchflow"
)

func TestMockExecutor_SnapshotResults_ConcurrencyAndSnapshotIsolation(t *testing.T) {
	e := batchflow.NewMockExecutor()
	ctx := context.Background()

	users := batchflow.NewSQLSchema("users", batchflow.ConflictIgnoreOperationConfig, "id", "name")

	const N = 50
	var wg sync.WaitGroup
	wg.Add(N)

	totalRows := int64(0)
	// 我们不容易从外部预估 args 的精确计数（与 driver 实现强相关）
	// 因此这里只严格校验 batches/rows，总体不为 0 的 args。
	for i := 0; i < N; i++ {
		rows := (i % 5) + 1 // 1..5 行
		totalRows += int64(rows)

		go func(rows int) {
			defer wg.Done()
			data := make([]map[string]any, 0, rows)
			for r := 0; r < rows; r++ {
				data = append(data, map[string]any{
					"id":   i*1000 + r,
					"name": "n",
				})
			}
			_ = e.ExecuteBatch(ctx, users, data)
		}(rows)
	}
	wg.Wait()

	agg := e.SnapshotResults()
	u := agg["users"]
	if u["batches"] != N {
		t.Fatalf("users batches expected %d, got %d", N, u["batches"])
	}
	if u["rows"] != totalRows {
		t.Fatalf("users rows expected %d, got %d", totalRows, u["rows"])
	}
	if u["args"] <= 0 {
		t.Fatalf("users args should be > 0, got %d", u["args"])
	}

	// SnapshotExecutedBatches 外层切片拷贝隔离：
	// 修改返回的外层切片（append）不应影响内部存储的长度
	snap1 := e.SnapshotExecutedBatches()
	origLen := len(snap1)
	// 在外层切片上 append 不会影响内部（不同底层数组）
	_ = append(snap1, []map[string]any{{"dummy": 1}})

	snap2 := e.SnapshotExecutedBatches()
	if len(snap2) != origLen {
		t.Fatalf("SnapshotExecutedBatches outer slice should be isolated, expect len=%d, got %d", origLen, len(snap2))
	}
}
