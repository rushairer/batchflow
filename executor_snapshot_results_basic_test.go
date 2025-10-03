package batchflow_test

import (
	"context"
	"testing"

	"github.com/rushairer/batchflow"
)

func TestMockExecutor_SnapshotResults_Basic(t *testing.T) {
	e := batchflow.NewMockExecutor()
	ctx := context.Background()

	users := batchflow.NewSQLSchema("users", batchflow.ConflictIgnoreOperationConfig, "id", "name")

	batch1 := []map[string]any{
		{"id": 1, "name": "a"},
		{"id": 2, "name": "b"},
	}
	batch2 := []map[string]any{
		{"id": 3, "name": "c"},
	}

	if err := e.ExecuteBatch(ctx, users, batch1); err != nil {
		t.Fatalf("ExecuteBatch batch1 error: %v", err)
	}
	if err := e.ExecuteBatch(ctx, users, batch2); err != nil {
		t.Fatalf("ExecuteBatch batch2 error: %v", err)
	}

	// 仍保留旧能力：ExecutedBatches 外层快照长度
	gotBatches := e.SnapshotExecutedBatches()
	if len(gotBatches) != 2 {
		t.Fatalf("expected 2 batches, got %d", len(gotBatches))
	}

	// 新能力：聚合统计
	agg := e.SnapshotResults()
	u, ok := agg["users"]
	if !ok {
		t.Fatalf("expected users in stats")
	}
	if u["batches"] != 2 {
		t.Fatalf("batches expected 2, got %d", u["batches"])
	}
	if u["rows"] != 3 {
		t.Fatalf("rows expected 3, got %d", u["rows"])
	}
	// args 数量由驱动生成，至少应为正
	if u["args"] <= 0 {
		t.Fatalf("args should be > 0, got %d", u["args"])
	}

	// 额外覆盖：空表名回退为 "_unknown_"

	unknown := batchflow.NewSQLSchema("", batchflow.ConflictIgnoreOperationConfig, []string{"id"}...)
	if err := e.ExecuteBatch(ctx, unknown, []map[string]any{{"id": 1}}); err != nil {
		t.Fatalf("ExecuteBatch unknown error: %v", err)
	}
	agg2 := e.SnapshotResults()
	if _, ok := agg2["_unknown_"]; !ok {
		t.Fatalf("expected _unknown_ key for empty table name")
	}
}
