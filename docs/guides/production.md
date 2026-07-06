# Production Guide

This guide summarizes the minimum production checklist for BatchFlow deployments on `github.com/rushairer/batchflow/v2` RC2.

## Configuration Baseline

New production code should use the clean runtime API:

```go
executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultPostgreSQLDriver).
    WithConcurrencyLimit(8).
    WithRetryConfig(batchflow.RetryConfig{
        Enabled: true,
        MaxAttempts: 3,
        BackoffBase: 20 * time.Millisecond,
        MaxBackoff: 500 * time.Millisecond,
    })

cfg := batchflow.DefaultConfig(executor)
cfg.Pipeline.BufferSize = 10000
cfg.Pipeline.FlushSize = 1000
cfg.Pipeline.FlushInterval = 50 * time.Millisecond
cfg.Pipeline.Timeout = 2 * time.Second
cfg.Pipeline.MetricsReporter = reporter
cfg.Pipeline.Observability = batchflow.ObservabilityConfig{
    Logger: logger,
    Sampler: batchflow.NewErrorAndSlowSampler(500 * time.Millisecond),
    Redactor: batchflow.DefaultRedactor(),
    SlowBatchThreshold: 500 * time.Millisecond,
}

cfg.Runtime.ShardCount = 4
cfg.Runtime.Routing = batchflow.ShardRoutingHash
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
```

Legacy constructors remain available for quick migrations, but they do not expose the full runtime controls in one config object.

## Database Connection Pools

BatchFlow does not own `*sql.DB` pool sizing. Configure it in the application:

```go
db.SetMaxOpenConns(64)
db.SetMaxIdleConns(32)
db.SetConnMaxLifetime(time.Hour)
db.SetConnMaxIdleTime(10 * time.Minute)
```

Keep executor concurrency and runtime shard count below the database pool and backend write capacity. If execute latency rises while CPU is low, inspect connection waits and database locks before increasing BatchFlow concurrency.

## COPY FROM / Hologres

For append-only PostgreSQL/Hologres ingest, use the COPY path:

```go
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
```

COPY FROM is append-only. Use SQL executors for upsert/update/replace semantics.

## Backpressure and Memory Limit

Use both controls in production:

- `BackpressureConfig`: protects per-shard queue depth.
- `MemoryLimitConfig`: protects total estimated queued memory.

Recommended modes:

- Online API: `BackpressureReject` so callers can retry quickly.
- Worker/service ingest: `BackpressureTimeout` so short downstream stalls can recover.
- Offline jobs: `BackpressureBlock` only when blocking is acceptable.

## Timeouts and Retries

- Use caller context deadlines for request lifetime.
- Use `PipelineConfig.Timeout` for backend execution limits.
- Keep retry attempts small, usually 2 or 3 total attempts.
- Retry only transient reasons such as `deadlock`, `lock_timeout`, `timeout`, `connection`, and `io`.
- Do not retry `duplicate_key`, `syntax`, validation, or context cancellation errors by default.

See [Error Classification](error-classification.md) for the full reason dictionary.

## Shutdown

Always close the flow during application shutdown:

```go
if err := flow.Close(); err != nil {
    logger.Error("batchflow close failed", "error", err)
}
```

Consume `ErrorChan` when asynchronous execution failures matter:

```go
errs := flow.ErrorChan(256)
go func() {
    for err := range errs {
        logger.Error("batchflow async error", "error", err)
    }
}()
```

The first `ErrorChan(size)` call decides the channel buffer size.

## SQL Dry Run

Before enabling PostgreSQL/MySQL update or replace flows in production, verify final SQL and conflict-key behavior:

```go
preview, err := batchflow.GenerateSQLPreview(ctx, batchflow.DefaultPostgreSQLDriver, schema, rows)
if err != nil {
    return err
}
logger.Info("sql preview",
    "table", preview.Table,
    "fingerprint", preview.Fingerprint,
    "args_count", preview.ArgsCount,
    "conflict_columns", preview.ConflictColumns,
    "update_columns", preview.UpdateColumns,
    "input_rows", preview.DedupStats.InputRows,
    "output_rows", preview.DedupStats.OutputRows,
)
```

Do not log `preview.Args` in production.

## Metrics Cardinality

Recommended labels:

- `database`
- `instance_id`
- `status`
- `backend`
- `operation`
- `stage`
- `reason`

Avoid labels containing request IDs, raw SQL, SQL args, Redis keys, HTTP paths with IDs, user IDs, emails, phone numbers, or timestamps.

Enable `table` only when schema count is small and controlled.

## Adaptive Tuning

`AdaptiveTuner` is policy-only in RC2. It returns recommendations and does not mutate live runtime settings automatically. Apply changes only through a control plane after a stability window.

```go
tuner := batchflow.NewAdaptiveTuner(cfg.Runtime.Adaptive)
recommendation := tuner.Tune(batchflow.TuningSignal{
    LatencyAvg: 50 * time.Millisecond,
    QueueDepth: 9000,
    ErrorRate: 0,
    ThroughputRPS: 100000,
})
```

## Non-SQL and DIY Backends

For HTTP, document stores, message queues, or other custom sinks, prefer:

1. Implement `BatchProcessor` to generate backend operations and execute them.
2. Implement `OperationPreviewer` to provide safe diagnostics.
3. Run through `ThrottledBatchExecutor` to reuse retry, concurrency limit, metrics, and observer logic.

See [custom examples](../../examples/custom).

## Readiness Checklist

- [ ] New production code uses `DefaultConfig(executor)` + `New(ctx, cfg)`.
- [ ] `Close()` is called on shutdown.
- [ ] `ErrorChan` is consumed or intentionally ignored.
- [ ] `MetricsReporter` is configured.
- [ ] `ObservabilityConfig` redacts sensitive fields.
- [ ] Runtime backpressure is configured.
- [ ] Runtime memory limit is configured.
- [ ] SQL update/replace flows use explicit `ConflictColumns`.
- [ ] PostgreSQL/MySQL write paths were checked with `GenerateSQLPreview`.
- [ ] Retry policy uses low-cardinality reason labels.
- [ ] Integration tests have run against the target backend.
- [ ] Optional pgx adapter is validated if COPY FROM is used.
