package batchflow_test

import (
	"context"
	"testing"

	"github.com/rushairer/batchflow"
)

func TestMockExecutor_SnapshotResults_MultiTables(t *testing.T) {
	e := batchflow.NewMockExecutor()
	ctx := context.Background()

	users := batchflow.NewSQLSchema("users", batchflow.ConflictIgnoreOperationConfig, "id", "name")
	orders := batchflow.NewSQLSchema("orders", batchflow.ConflictIgnoreOperationConfig, "id", "amount")

	// users 两批
	if err := e.ExecuteBatch(ctx, users, []map[string]any{
		{"id": 1, "name": "a"},
	}); err != nil {
		t.Fatalf("users batch1 error: %v", err)
	}
	if err := e.ExecuteBatch(ctx, users, []map[string]any{
		{"id": 2, "name": "b"},
		{"id": 3, "name": "c"},
	}); err != nil {
		t.Fatalf("users batch2 error: %v", err)
	}

	// orders 一批
	if err := e.ExecuteBatch(ctx, orders, []map[string]any{
		{"id": 10, "amount": 12.5},
		{"id": 11, "amount": 7.0},
		{"id": 12, "amount": 1.2},
	}); err != nil {
		t.Fatalf("orders batch error: %v", err)
	}

	agg := e.SnapshotResults()

	u := agg["users"]
	if u["batches"] != 2 {
		t.Fatalf("users batches expected 2, got %d", u["batches"])
	}
	if u["rows"] != 3 {
		t.Fatalf("users rows expected 3, got %d", u["rows"])
	}
	if u["args"] <= 0 {
		t.Fatalf("users args should be > 0, got %d", u["args"])
	}

	o := agg["orders"]
	if o["batches"] != 1 {
		t.Fatalf("orders batches expected 1, got %d", o["batches"])
	}
	if o["rows"] != 3 {
		t.Fatalf("orders rows expected 3, got %d", o["rows"])
	}
	if o["args"] <= 0 {
		t.Fatalf("orders args should be > 0, got %d", o["args"])
	}
}
