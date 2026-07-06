# BatchFlow v2 Migration Guide

BatchFlow v2.0.0-rc.2 moves the framework toward a backend-neutral ingestion runtime.

The public module path remains:

```text
github.com/rushairer/batchflow/v2
```

Because v2 is still pre-GA, public API names are clean and not version-decorated.

## Constructor migration

New production code should prefer:

```go
executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultPostgreSQLDriver)

cfg := batchflow.DefaultConfig(executor)
cfg.Pipeline.BufferSize = 10000
cfg.Pipeline.FlushSize = 1000
cfg.Pipeline.FlushInterval = 50 * time.Millisecond
cfg.Runtime.ShardCount = 4

flow, err := batchflow.New(ctx, cfg)
if err != nil {
    return err
}
defer flow.Close()
```

Existing helpers such as `NewMySQLBatchFlow`, `NewPostgreSQLBatchFlow`, `NewSQLiteBatchFlow`, and `NewRedisBatchFlow` remain available for simple migrations.

## Runtime controls

v2 RC2 adds runtime-level controls that are not present in the legacy convenience constructors:

- `RuntimeConfig.ShardCount`
- `RuntimeConfig.Routing`
- `RuntimeConfig.Backpressure`
- `RuntimeConfig.MemoryLimit`
- `RuntimeConfig.Adaptive`

Recommended production migrations should explicitly enable backpressure and memory limit.

## COPY FROM migration

For PostgreSQL/Hologres append-only ingestion, use the COPY path:

```go
copyExecutor := pgxcopy.NewExecutor(pool)

cfg := batchflow.DefaultConfig(copyExecutor)
cfg.Pipeline.BufferSize = 50000
cfg.Pipeline.FlushSize = 5000
cfg.Pipeline.FlushInterval = 20 * time.Millisecond
cfg.Runtime.ShardCount = 8

flow, err := batchflow.New(ctx, cfg)
```

COPY FROM is append-only. Continue using SQL executors for upsert/update/replace.

## Batch data model

New code can use the named aliases:

```go
type Record = map[string]any
type Batch = []Record
```

Existing `[]map[string]any` implementations continue to compile because these are aliases.

## Coalescing

For non-SQL backends, configure batch-level key coalescing explicitly:

```go
cfg := batchflow.DefaultConfig(executor)
cfg.Pipeline.Coalescer = batchflow.NewKeyCoalescer(batchflow.CoalesceKeepLast, "id")
```

SQL backends should continue to use `SQLOperationConfig.WithConflictColumns(...)`. SQL conflict strategies keep database-specific semantics and preserve SQL dry-run dedup statistics.

## Request columns

`Request.Columns()` returns a defensive copy. Code that intentionally mutated the returned map must switch to `Set(...)`, `SetNull(...)`, or typed setters before submission.

## Error classification

Reusable custom backends should register structured classifiers instead of relying on string matching:

```go
unregister := batchflow.RegisterErrorClassifier(classifier)
defer unregister()
```

Custom classifiers run after built-in MySQL/PostgreSQL structured code recognition and before string fallback.
