# 使用示例

本文档是 [Examples](../../guides/examples.md) 的中文镜像摘要。示例均使用 `github.com/rushairer/batchflow/v2`。

## 推荐 runtime 入口

新生产代码推荐使用：

```go
executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultMySQLDriver).
	WithConcurrencyLimit(8)

cfg := batchflow.DefaultConfig(executor)
cfg.Pipeline.BufferSize = 10000
cfg.Pipeline.FlushSize = 1000
cfg.Pipeline.FlushInterval = 50 * time.Millisecond
cfg.Runtime.ShardCount = 4
cfg.Runtime.Backpressure = batchflow.BackpressureConfig{
	Enabled:       true,
	Mode:          batchflow.BackpressureTimeout,
	HighWatermark: 8000,
	Timeout:       500 * time.Millisecond,
}
cfg.Runtime.MemoryLimit = batchflow.MemoryLimitConfig{
	Enabled:         true,
	MaxQueueBytes:   512 << 20,
	AvgRequestBytes: 512,
	Mode:            batchflow.BackpressureTimeout,
	Timeout:         500 * time.Millisecond,
}

flow, err := batchflow.New(ctx, cfg)
if err != nil {
	return err
}
defer flow.Close()
```

旧的 `NewMySQLBatchFlow` / `NewRedisBatchFlow` 仍可用于简单迁移，但新生产代码建议使用 `DefaultConfig(executor)` + `New(ctx, cfg)`。

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

## PostgreSQL / Hologres COPY FROM

append-only 高吞吐写入推荐 COPY path：

```go
copyExecutor := pgxcopy.NewExecutor(pool)

cfg := batchflow.DefaultConfig(copyExecutor)
cfg.Pipeline.BufferSize = 50000
cfg.Pipeline.FlushSize = 5000
cfg.Pipeline.FlushInterval = 20 * time.Millisecond
cfg.Runtime.ShardCount = 8
cfg.Runtime.Backpressure = batchflow.BackpressureConfig{
	Enabled:       true,
	Mode:          batchflow.BackpressureTimeout,
	HighWatermark: 40000,
	Timeout:       time.Second,
}
cfg.Runtime.MemoryLimit = batchflow.MemoryLimitConfig{
	Enabled:         true,
	MaxQueueBytes:   1 << 30,
	AvgRequestBytes: 512,
	Mode:            batchflow.BackpressureTimeout,
	Timeout:         time.Second,
}

flow, err := batchflow.New(ctx, cfg)
```

COPY FROM 只支持 append-only 语义；需要 upsert/update/replace 时使用 SQL executor。

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

## Redis / 非 SQL 合并

```go
executor := batchflow.NewRedisThrottledBatchExecutor(redisClient)

cfg := batchflow.DefaultConfig(executor)
cfg.Pipeline.BufferSize = 1000
cfg.Pipeline.FlushSize = 100
cfg.Pipeline.FlushInterval = 100 * time.Millisecond
cfg.Pipeline.Coalescer = batchflow.NewKeyCoalescer(batchflow.CoalesceKeepLast, "key")

flow, err := batchflow.New(ctx, cfg)
```

可用策略：

- `CoalesceKeepFirst`
- `CoalesceKeepLast`
- `CoalesceMergePresentFields`

## 可编译示例

- [Runtime clean API](../../../examples/runtime/flow_example_test.go)
- [COPY executor](../../../examples/copy/copy_executor_example_test.go)
- [HTTP batch processor](../../../examples/custom/http_processor_example_test.go)
- [Bulk write processor](../../../examples/custom/bulk_write_processor_example_test.go)
- [SQL preview](../../../examples/sql/upsert_preview_example_test.go)
- [Redis coalescing](../../../examples/redis/coalescer_example_test.go)
