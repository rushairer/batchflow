package benchmark_test

import (
	"context"
	"testing"

	batchflow "github.com/rushairer/batchflow/v2"
)

type fakeExecutor struct{}

func (f *fakeExecutor) ExecuteBatch(ctx context.Context, schema batchflow.SchemaInterface, data []map[string]any) error {
	return nil
}

func BenchmarkCopyPath(b *testing.B) {
	ctx := context.Background()
	exec := &fakeExecutor{}
	cfg := batchflow.DefaultV2Config(exec)

	engine, _ := batchflow.NewV2BatchFlow(ctx, cfg)

	schema := batchflow.NewSQLSchema("t", batchflow.ConflictIgnoreOperationConfig, "a", "b", "c")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := batchflow.NewRequest(schema)
		r.SetInt("a", i).SetInt("b", i).SetInt("c", i)
		_ = engine.Submit(ctx, r)
	}

	_ = engine.Close()
}
