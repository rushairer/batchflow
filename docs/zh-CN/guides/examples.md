# 使用示例

本文档是 [Examples](../../guides/examples.md) 的中文镜像摘要。示例均使用 `github.com/rushairer/batchflow/v2`。

## PostgreSQL Update

```go
schema := batchflow.NewSQLSchema(
	"users",
	batchflow.ConflictUpdateOperationConfig.
		WithConflictColumns("id").
		WithUpdateColumns("name", "email"),
	"id", "name", "email", "updated_at",
)
```

生成语义：

```sql
INSERT INTO users (...) VALUES (...)
ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, email = EXCLUDED.email
```

## PostgreSQL Replace

```go
schema := batchflow.NewSQLSchema(
	"users",
	batchflow.ConflictReplaceOperationConfig.WithConflictColumns("id"),
	"id", "name", "email", "updated_at",
)
```

PostgreSQL `ConflictReplace` 是 upsert 覆盖，冲突时更新所有非冲突列。

## MySQL Update / Replace

```go
schema := batchflow.NewSQLSchema(
	"users",
	batchflow.ConflictUpdateOperationConfig.
		WithConflictColumns("id").
		WithUpdateColumns("name", "email"),
	"id", "name", "email", "updated_at",
)
```

MySQL `ConflictUpdate` 使用 `ON DUPLICATE KEY UPDATE`；MySQL `ConflictReplace` 使用原生 `REPLACE INTO`。

## 非 SQL 合并

```go
flow := batchflow.NewRedisBatchFlow(ctx, redisClient, batchflow.PipelineConfig{
	BufferSize:    1000,
	FlushSize:     100,
	FlushInterval: 100 * time.Millisecond,
	Coalescer:     batchflow.NewKeyCoalescer(batchflow.CoalesceKeepLast, "key"),
})
defer flow.Close()
```

可用策略：

- `CoalesceKeepFirst`
- `CoalesceKeepLast`
- `CoalesceMergePresentFields`

## 可编译示例

- [HTTP batch processor](../../../examples/custom/http_processor_example_test.go)
- [Bulk write processor](../../../examples/custom/bulk_write_processor_example_test.go)
- [SQL preview](../../../examples/sql/upsert_preview_example_test.go)
- [Redis coalescing](../../../examples/redis/coalescer_example_test.go)
