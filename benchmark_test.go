package batchflow_test

import (
	"context"
	"testing"
	"time"

	"github.com/rushairer/batchflow"
)

func BenchmarkBatchFlow_Submit(b *testing.B) {
	ctx := context.Background()
	config := batchflow.PipelineConfig{
		BufferSize:    10000,
		FlushSize:     1000,
		FlushInterval: time.Second,
	}
	batch, mock := batchflow.NewBatchFlowWithMock(ctx, config)
	b.Cleanup(func() {
		if testing.Verbose() {
			agg := mock.SnapshotResults()
			for table, m := range agg {
				b.Logf("[MockExec Summary] table=%s batches=%d rows=%d args=%d", table, m["batches"], m["rows"], m["args"])
			}
		}
	})

	schema := batchflow.NewSQLSchema("users", batchflow.ConflictIgnoreOperationConfig, "id", "name", "email")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			request := batchflow.NewRequest(schema).
				SetInt64("id", int64(i)).
				SetString("name", "User"+string(rune(i))).
				SetString("email", "user"+string(rune(i))+"@example.com")

			if err := batch.Submit(ctx, request); err != nil {
				b.Errorf("Submit failed: %v", err)
			}
			i++
		}
	})
}

func BenchmarkBatchFlow_MultiSchema(b *testing.B) {
	ctx := context.Background()
	config := batchflow.PipelineConfig{
		BufferSize:    10000,
		FlushSize:     1000,
		FlushInterval: time.Second,
	}
	batch, mock := batchflow.NewBatchFlowWithMock(ctx, config)
	b.Cleanup(func() {
		if testing.Verbose() {
			agg := mock.SnapshotResults()
			for table, m := range agg {
				b.Logf("[MockExec Summary] table=%s batches=%d rows=%d args=%d", table, m["batches"], m["rows"], m["args"])
			}
		}
	})

	userSchema := batchflow.NewSQLSchema("users", batchflow.ConflictIgnoreOperationConfig, "id", "name", "email")
	productSchema := batchflow.NewSQLSchema("products", batchflow.ConflictUpdateOperationConfig, "id", "name", "price")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%2 == 0 {
				request := batchflow.NewRequest(userSchema).
					SetInt64("id", int64(i)).
					SetString("name", "User"+string(rune(i))).
					SetString("email", "user"+string(rune(i))+"@example.com")
				if err := batch.Submit(ctx, request); err != nil {
					b.Errorf("Submit user failed: %v", err)
				}
			} else {
				request := batchflow.NewRequest(productSchema).
					SetInt64("id", int64(i)).
					SetString("name", "Product"+string(rune(i))).
					SetFloat64("price", float64(i)*10.5)
				if err := batch.Submit(ctx, request); err != nil {
					b.Errorf("Submit product failed: %v", err)
				}
			}
			i++
		}
	})
}

func BenchmarkRequest_Creation(b *testing.B) {
	schema := batchflow.NewSQLSchema("users", batchflow.ConflictIgnoreOperationConfig, "id", "name", "email", "age", "created_at")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		request := batchflow.NewRequest(schema).
			SetInt64("id", int64(i)).
			SetString("name", "User"+string(rune(i))).
			SetString("email", "user"+string(rune(i))+"@example.com").
			SetInt32("age", int32(20+i%50)).
			SetTime("created_at", time.Now())

		// 验证请求创建成功
		if request.Schema() != schema {
			b.Errorf("Schema mismatch")
		}
	}
}

func BenchmarkSQLGeneration(b *testing.B) {
	schema := batchflow.NewSQLSchema("users", batchflow.ConflictIgnoreOperationConfig, "id", "name", "email")
	data := make([]map[string]any, 100)
	for i := 0; i < 100; i++ {
		data[i] = map[string]any{
			"id":    int64(i),
			"name":  "User" + string(rune(i)),
			"email": "user" + string(rune(i)) + "@example.com",
		}
	}

	// 测试不同数据库驱动的SQL生成性能
	drivers := map[string]batchflow.SQLDriver{
		"MySQL":      &mysqlDriver{},
		"PostgreSQL": &postgresDriver{},
		"SQLite":     &sqliteDriver{},
	}

	ctx := context.Background()

	for name, driver := range drivers {
		b.Run(name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, _, err := driver.GenerateInsertSQL(ctx, schema, data)
				if err != nil {
					b.Errorf("GenerateInsertSQL failed: %v", err)
				}
			}
		})
	}
}

// 简化的驱动实现用于基准测试
type mysqlDriver struct{}

func (d *mysqlDriver) GenerateInsertSQL(ctx context.Context, schema *batchflow.SQLSchema, data []map[string]any) (string, []any, error) {
	// 简化实现
	return "INSERT IGNORE INTO users (id, name, email) VALUES (?, ?, ?)", []any{1, "test", "test@example.com"}, nil
}

type postgresDriver struct{}

func (d *postgresDriver) GenerateInsertSQL(ctx context.Context, schema *batchflow.SQLSchema, data []map[string]any) (string, []any, error) {
	return "INSERT INTO users (id, name, email) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING", []any{1, "test", "test@example.com"}, nil
}

type sqliteDriver struct{}

func (d *sqliteDriver) GenerateInsertSQL(ctx context.Context, schema *batchflow.SQLSchema, data []map[string]any) (string, []any, error) {
	return "INSERT OR IGNORE INTO users (id, name, email) VALUES (?, ?, ?)", []any{1, "test", "test@example.com"}, nil
}
