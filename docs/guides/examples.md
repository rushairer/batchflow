# Examples

This guide keeps examples aligned with compiled repository code and the `github.com/rushairer/batchflow/v2` module path.

## Recommended runtime entrypoint

New production code should use `DefaultConfig(executor)` plus `New(ctx, cfg)`.

```go
executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultMySQLDriver).
    WithConcurrencyLimit(8)

cfg := batchflow.DefaultConfig(executor)
cfg.Pipeline.BufferSize = 10000
cfg.Pipeline.FlushSize = 1000
cfg.Pipeline.FlushInterval = 50 * time.Millisecond
cfg.Runtime.ShardCount = 4
cfg.Runtime.Backpressure = batchflow.BackpressureConfig{
    Enabled: true,
    Mode: batchflow.BackpressureTimeout,
    HighWatermark: 8000,
    Timeout: 500 * time.Millisecond,
}
cfg.Runtime.MemoryLimit = batchflow.MemoryLimitConfig{
    Enabled: true,
    MaxQueueBytes: 512 << 20,
    AvgRequestBytes: 512,
    Mode: batchflow.BackpressureTimeout,
    Timeout: 500 * time.Millisecond,
}

flow, err := batchflow.New(ctx, cfg)
if err != nil {
    return err
}
defer flow.Close()
```

Legacy convenience constructors such as `NewMySQLBatchFlow` remain available for quick migrations, but they do not expose the full runtime controls in one place.

## MySQL

```go
db, err := sql.Open("mysql", dsn)
if err != nil {
    return err
}
defer db.Close()

executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultMySQLDriver).
    WithConcurrencyLimit(8)

cfg := batchflow.DefaultConfig(executor)
cfg.Pipeline.BufferSize = 5000
cfg.Pipeline.FlushSize = 200
cfg.Pipeline.FlushInterval = 100 * time.Millisecond

flow, err := batchflow.New(ctx, cfg)
if err != nil {
    return err
}
defer flow.Close()

schema := batchflow.NewSQLSchema(
    "users",
    batchflow.ConflictUpdateOperationConfig.
        WithConflictColumns("id").
        WithUpdateColumns("name", "email"),
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

## PostgreSQL Upsert Update

PostgreSQL code should declare conflict keys explicitly. `ConflictUpdate` updates non-conflict columns by default; `WithUpdateColumns` narrows the update list.

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

Generated semantics:

```sql
INSERT INTO users (...) VALUES (...)
ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, email = EXCLUDED.email
```

## PostgreSQL / Hologres COPY FROM

Use COPY FROM for append-only high-throughput ingestion.

```go
import (
    batchflow "github.com/rushairer/batchflow/v2"
    "github.com/rushairer/batchflow/adapters/pgxcopy"
)

copyExecutor := pgxcopy.NewExecutor(pool)

cfg := batchflow.DefaultConfig(copyExecutor)
cfg.Pipeline.BufferSize = 50000
cfg.Pipeline.FlushSize = 5000
cfg.Pipeline.FlushInterval = 20 * time.Millisecond
cfg.Runtime.ShardCount = 8
cfg.Runtime.Backpressure = batchflow.BackpressureConfig{
    Enabled: true,
    Mode: batchflow.BackpressureTimeout,
    HighWatermark: 40000,
    Timeout: time.Second,
}
cfg.Runtime.MemoryLimit = batchflow.MemoryLimitConfig{
    Enabled: true,
    MaxQueueBytes: 1 << 30,
    AvgRequestBytes: 512,
    Mode: batchflow.BackpressureTimeout,
    Timeout: time.Second,
}

flow, err := batchflow.New(ctx, cfg)
if err != nil {
    return err
}
defer flow.Close()
```

COPY FROM only supports append-only schemas. If the SQL schema uses update/replace conflict behavior, use the SQL executor path instead.

## PostgreSQL Upsert Replace

PostgreSQL does not have MySQL `REPLACE INTO` delete-plus-insert semantics. BatchFlow defines `ConflictReplace` as upsert overwrite: update all non-conflict columns on conflict.

```go
schema := batchflow.NewSQLSchema(
    "users",
    batchflow.ConflictReplaceOperationConfig.WithConflictColumns("id"),
    "id", "name", "email", "updated_at",
)
```

Generated semantics:

```sql
INSERT INTO users (...) VALUES (...)
ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, email = EXCLUDED.email, updated_at = EXCLUDED.updated_at
```

Duplicate conflict keys inside one batch are coalesced before SQL generation, avoiding PostgreSQL "cannot affect row a second time" failures.

## PostgreSQL Composite Conflict Keys

```go
schema := batchflow.NewSQLSchema(
    "user_profiles",
    batchflow.ConflictUpdateOperationConfig.
        WithConflictColumns("tenant_id", "user_id"),
    "tenant_id", "user_id", "display_name", "avatar_url",
)
```

Generated semantics:

```sql
ON CONFLICT (tenant_id, user_id) DO UPDATE SET display_name = EXCLUDED.display_name, avatar_url = EXCLUDED.avatar_url
```

## MySQL Update and Replace

MySQL `ConflictUpdate` uses `ON DUPLICATE KEY UPDATE`. Configure `ConflictColumns` so primary or unique key columns are not updated accidentally.

```go
schema := batchflow.NewSQLSchema(
    "users",
    batchflow.ConflictUpdateOperationConfig.
        WithConflictColumns("id").
        WithUpdateColumns("name", "email"),
    "id", "name", "email", "updated_at",
)
```

Generated semantics:

```sql
INSERT INTO users (...) VALUES (...)
ON DUPLICATE KEY UPDATE name = VALUES(name), email = VALUES(email)
```

MySQL `ConflictReplace` keeps native `REPLACE INTO` behavior:

```go
schema := batchflow.NewSQLSchema(
    "users",
    batchflow.ConflictReplaceOperationConfig.WithConflictColumns("id"),
    "id", "name", "email",
)
```

## SQL Dry Run

```go
preview, err := batchflow.GenerateSQLPreview(ctx, batchflow.DefaultPostgreSQLDriver, schema, rows)
if err != nil {
    return err
}

log.Printf("table=%s fingerprint=%s args=%d conflict=%v update=%v input=%d output=%d dedup=%d",
    preview.Table,
    preview.Fingerprint,
    preview.ArgsCount,
    preview.ConflictColumns,
    preview.UpdateColumns,
    preview.DedupStats.InputRows,
    preview.DedupStats.OutputRows,
    preview.DedupStats.DeduplicatedRows,
)
```

## Redis

```go
executor := batchflow.NewRedisThrottledBatchExecutor(rdb)

cfg := batchflow.DefaultConfig(executor)
cfg.Pipeline.BufferSize = 5000
cfg.Pipeline.FlushSize = 500
cfg.Pipeline.FlushInterval = 50 * time.Millisecond

flow, err := batchflow.New(ctx, cfg)
if err != nil {
    return err
}
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

## Non-SQL Coalescing

Redis, HTTP, MongoDB, queue, and custom API backends can explicitly enable duplicate-key handling through `PipelineConfig.Coalescer`:

```go
cfg.Pipeline.Coalescer = batchflow.NewKeyCoalescer(batchflow.CoalesceKeepLast, "key")
```

Available strategies:

- `CoalesceKeepFirst`: keep the first record for a key.
- `CoalesceKeepLast`: keep the last record for a key.
- `CoalesceMergePresentFields`: merge fields that are present in later records.

SQL update/replace does not need `PipelineConfig.Coalescer`; SQL conflict strategies use `SQLOperationConfig.ConflictColumns` and report deduplication through `GenerateSQLPreview`.

## Custom Executor

```go
type MyExecutor struct{}

func (e *MyExecutor) ExecuteBatch(ctx context.Context, schema batchflow.SchemaInterface, data []map[string]any) error {
    return nil
}

cfg := batchflow.DefaultConfig(&MyExecutor{})
cfg.Pipeline.BufferSize = 1000
cfg.Pipeline.FlushSize = 100
cfg.Pipeline.FlushInterval = 100 * time.Millisecond

flow, err := batchflow.New(ctx, cfg)
if err != nil {
    return err
}
defer flow.Close()
```

For reusable custom backends, implement `BatchProcessor`, optionally implement `OperationPreviewer`, and wrap it with `NewThrottledBatchExecutor`.

Compiled examples:

- [HTTP batch processor](../../examples/custom/http_processor_example_test.go)
- [Bulk write processor](../../examples/custom/bulk_write_processor_example_test.go)
- [SQL preview](../../examples/sql/upsert_preview_example_test.go)
- [Redis coalescing](../../examples/redis/coalescer_example_test.go)

## Retry and Concurrency Limit

```go
executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultMySQLDriver).
    WithConcurrencyLimit(8).
    WithRetryConfig(batchflow.RetryConfig{
        Enabled: true,
        MaxAttempts: 3,
        BackoffBase: 20 * time.Millisecond,
        MaxBackoff: 500 * time.Millisecond,
    })

cfg := batchflow.DefaultConfig(executor)
```

## Metrics

```go
import prommetrics "github.com/rushairer/batchflow/v2/examples/metrics/prometheus"

metrics := prommetrics.NewMetrics(prommetrics.Options{
    Namespace: "batchflow",
    IncludeInstanceID: true,
    EnablePipelineMetrics: true,
})

if err := metrics.StartServer(2112); err != nil {
    return err
}
defer metrics.StopServer(context.Background())

reporter := prommetrics.NewReporter(metrics, "mysql", "order_writer")

cfg.Pipeline.MetricsReporter = reporter
```

## Async Errors

```go
errs := flow.ErrorChan(64)
go func() {
    for err := range errs {
        log.Printf("async batch error: %v", err)
    }
}()
```

## Shutdown

```go
if err := flow.Close(); err != nil {
    return err
}
```

Use `Wait()` when another owner closes input. Use `Done()` as a shutdown notification channel.
