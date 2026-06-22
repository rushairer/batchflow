# BatchFlow 使用示例

本文档只保留当前仓库已经验证过的推荐写法。

## MySQL

```go
db, err := sql.Open("mysql", dsn)
if err != nil {
	return err
}
defer db.Close()

flow := batchflow.NewMySQLBatchFlow(ctx, db, batchflow.PipelineConfig{
	BufferSize:       5000,
	FlushSize:        200,
	FlushInterval:    100 * time.Millisecond,
	ConcurrencyLimit: 8,
})
defer flow.Close()

schema := batchflow.NewSQLSchema(
	"users",
	batchflow.ConflictIgnoreOperationConfig,
	"id", "name", "email",
)

for i := 0; i < 1000; i++ {
	req := batchflow.NewRequest(schema).
		SetUint64("id", uint64(i)).
		SetString("name", fmt.Sprintf("user_%d", i)).
		SetString("email", fmt.Sprintf("user_%d@example.com", i))

	if err := flow.Submit(ctx, req); err != nil {
		return err
	}
}
```

## PostgreSQL

```go
db, err := sql.Open("postgres", dsn)
if err != nil {
	return err
}
defer db.Close()

flow := batchflow.NewPostgreSQLBatchFlow(ctx, db, batchflow.PipelineConfig{
	BufferSize:    2000,
	FlushSize:     200,
	FlushInterval: 100 * time.Millisecond,
})
defer flow.Close()
```

### PostgreSQL Upsert Update

PostgreSQL 推荐显式声明冲突键。`ConflictUpdate` 默认只更新非冲突列；如果只想更新部分列，使用 `WithUpdateColumns`。

```go
schema := batchflow.NewSQLSchema(
	"users",
	batchflow.ConflictUpdateOperationConfig.
		WithConflictColumns("id").
		WithUpdateColumns("name", "email"),
	"id", "name", "email", "updated_at",
)

req := batchflow.NewRequest(schema).
	SetInt64("id", 42).
	SetString("name", "alice").
	SetString("email", "alice@example.com").
	SetTime("updated_at", time.Now().UTC())

if err := flow.Submit(ctx, req); err != nil {
	return err
}
```

生成语义：

```sql
INSERT INTO users (...) VALUES (...)
ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, email = EXCLUDED.email
```

### PostgreSQL Upsert Replace

PostgreSQL 没有 MySQL `REPLACE INTO` 的 delete+insert 语义。这里的 `ConflictReplace` 采用 Hologres 风格：主键冲突时覆盖所有非冲突列。

```go
schema := batchflow.NewSQLSchema(
	"users",
	batchflow.ConflictReplaceOperationConfig.WithConflictColumns("id"),
	"id", "name", "email", "updated_at",
)
```

生成语义：

```sql
INSERT INTO users (...) VALUES (...)
ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, email = EXCLUDED.email, updated_at = EXCLUDED.updated_at
```

同一批次内如果出现重复冲突键，BatchFlow 会先在客户端合并，避免 PostgreSQL 一次 upsert 影响同一行多次。

### PostgreSQL 复合冲突键

```go
schema := batchflow.NewSQLSchema(
	"user_profiles",
	batchflow.ConflictUpdateOperationConfig.
		WithConflictColumns("tenant_id", "user_id"),
	"tenant_id", "user_id", "display_name", "avatar_url",
)
```

生成语义：

```sql
ON CONFLICT (tenant_id, user_id) DO UPDATE SET display_name = EXCLUDED.display_name, avatar_url = EXCLUDED.avatar_url
```

## MySQL Upsert Update / Replace

MySQL `ConflictUpdate` 使用 `ON DUPLICATE KEY UPDATE`，默认更新所有非冲突列；显式配置 `WithConflictColumns` 可以避免更新主键列。

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
ON DUPLICATE KEY UPDATE name = VALUES(name), email = VALUES(email)
```

MySQL `ConflictReplace` 保持数据库原生 `REPLACE INTO` 语义：

```go
schema := batchflow.NewSQLSchema(
	"users",
	batchflow.ConflictReplaceOperationConfig.WithConflictColumns("id"),
	"id", "name", "email",
)
```

```sql
REPLACE INTO users (...) VALUES (...)
```

## SQLite

```go
db, err := sql.Open("sqlite3", "./test.db")
if err != nil {
	return err
}
defer db.Close()

flow := batchflow.NewSQLiteBatchFlow(ctx, db, batchflow.PipelineConfig{
	BufferSize:    1000,
	FlushSize:     100,
	FlushInterval: 200 * time.Millisecond,
})
defer flow.Close()
```

## Redis

```go
rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
defer rdb.Close()

flow := batchflow.NewRedisBatchFlow(ctx, rdb, batchflow.PipelineConfig{
	BufferSize:    5000,
	FlushSize:     500,
	FlushInterval: 50 * time.Millisecond,
})
defer flow.Close()

schema := batchflow.NewSchema("cache", "cmd", "key", "ttl", "value")

req := batchflow.NewRequest(schema).
	SetString("cmd", "SETEX").
	SetString("key", "user:1").
	SetInt64("ttl", 3600).
	SetString("value", `{"name":"alice"}`)

if err := flow.Submit(ctx, req); err != nil {
	return err
}
```

## 自定义执行器

```go
type MyExecutor struct{}

func (e *MyExecutor) ExecuteBatch(ctx context.Context, schema batchflow.SchemaInterface, data []map[string]any) error {
	// 自定义执行逻辑
	return nil
}

flow := batchflow.NewBatchFlow(ctx, 1000, 100, 100*time.Millisecond, &MyExecutor{})
defer flow.Close()
```

如果希望复用限流、重试、operation metrics 和结构化日志，推荐实现 `BatchProcessor`，并额外实现 `OperationPreviewer` 提供安全 dry-run 信息。

可编译示例：

- [HTTP batch processor](../../examples/custom/http_processor_example_test.go)
- [Bulk write processor](../../examples/custom/bulk_write_processor_example_test.go)

## 非 SQL 批内合并

Redis、HTTP、MongoDB 等非 SQL 后端可以通过 `PipelineConfig.Coalescer` 显式启用同批次同 key 合并：

```go
flow := batchflow.NewRedisBatchFlow(ctx, redisClient, batchflow.PipelineConfig{
	BufferSize:    1000,
	FlushSize:     100,
	FlushInterval: 100 * time.Millisecond,
	Coalescer:     batchflow.NewKeyCoalescer(batchflow.CoalesceKeepLast, "key"),
})
defer flow.Close()
```

SQL update/replace 不需要在 `PipelineConfig` 再配 coalescer；SQL 默认根据 `SQLOperationConfig.ConflictColumns` 执行 conflict-key 合并，并在 SQL dry-run 里输出 dedup 统计。

## 带重试和限流

```go
flow := batchflow.NewMySQLBatchFlow(ctx, db, batchflow.PipelineConfig{
	BufferSize:       5000,
	FlushSize:        200,
	FlushInterval:    100 * time.Millisecond,
	ConcurrencyLimit: 8,
	Retry: batchflow.RetryConfig{
		Enabled:     true,
		MaxAttempts: 3,
		BackoffBase: 20 * time.Millisecond,
		MaxBackoff:  500 * time.Millisecond,
	},
})
defer flow.Close()
```

## 指标接入

```go
import prommetrics "github.com/rushairer/batchflow/examples/metrics/prometheus"

metrics := prommetrics.NewMetrics(prommetrics.Options{
	Namespace:             "batchflow",
	IncludeInstanceID:     true,
	EnablePipelineMetrics: true,
})

if err := metrics.StartServer(2112); err != nil {
	return err
}
defer metrics.StopServer(context.Background())

reporter := prommetrics.NewReporter(metrics, "mysql", "order_writer")

flow := batchflow.NewMySQLBatchFlow(ctx, db, batchflow.PipelineConfig{
	BufferSize:      5000,
	FlushSize:       200,
	FlushInterval:   100 * time.Millisecond,
	MetricsReporter: reporter,
})
defer flow.Close()
```

## 错误消费

```go
errs := flow.ErrorChan(64)
go func() {
	for err := range errs {
		log.Printf("async batch error: %v", err)
	}
}()
```

## 生命周期收尾

```go
if err := flow.Close(); err != nil {
	return err
}
```

## Wait / Done

```go
go func() {
	<-flow.Done()
	log.Println("batchflow stopped")
}()

if err := flow.Wait(); err != nil {
	return err
}
```

适合“提交方”和“关闭方”不在同一个 goroutine 的场景。

## Request 基础类型 setter

```go
req := batchflow.NewRequest(schema).
	SetInt("retry_count", 3).
	SetUint64("id", 42).
	SetUint8("shard", 7).
	SetBool("enabled", true)
```

说明：

- 业务代码推荐始终调用 `Close()`，不要只依赖 `FlushInterval` 自然触发。
- 如果你只是等待退出而不关闭输入，请用 `Wait()`。
- `Done()` 适合做退出通知，不负责触发收尾。
- 基础整数类型优先使用对应的 `SetInt...` / `SetUint...` 便捷方法；其他类型继续用 `Set(...)` 或现有 typed setter。
