# Configuration

This page covers configuration expected in application code for `github.com/rushairer/batchflow/v2`.

The recommended RC2 entrypoint is:

```go
cfg := batchflow.DefaultConfig(executor)
flow, err := batchflow.New(ctx, cfg)
```

## Config

```go
type Config struct {
    Pipeline PipelineConfig
    Runtime  RuntimeConfig
    Executor BatchExecutor
}
```

- `Pipeline` controls queueing, flush, retry, metrics, observability, and executor-level concurrency.
- `Runtime` controls sharding, routing, backpressure, memory protection, and adaptive tuning policy.
- `Executor` performs one schema group batch write.

## PipelineConfig

```go
type PipelineConfig struct {
    BufferSize               uint32
    FlushSize                uint32
    FlushInterval            time.Duration
    MaxConcurrentFlushes     uint32
    DrainOnCancel            bool
    DrainGracePeriod         time.Duration
    FinalFlushOnCloseTimeout time.Duration
    Retry                    RetryConfig
    Timeout                  time.Duration
    MetricsReporter          MetricsReporter
    Observability            ObservabilityConfig
    ConcurrencyLimit         int
    Coalescer                Coalescer
}
```

`DefaultPipelineConfig()` provides conservative defaults. For new production code, start with `DefaultConfig(executor)` and override the fields you need.

`Coalescer` is for non-SQL backends such as Redis, HTTP, document databases, queues, or custom APIs. SQL backends use `SQLOperationConfig.ConflictColumns` for conflict-key coalescing so SQL dry-run output can report deduplication statistics.

## RuntimeConfig

```go
type RuntimeConfig struct {
    ShardCount   uint32
    Routing      ShardRoutingPolicy
    ShardKeyFunc ShardKeyFunc

    Backpressure BackpressureConfig
    MemoryLimit  MemoryLimitConfig
    Adaptive     AdaptiveTuningConfig
}
```

### ShardCount

- `1`: default and simplest mode.
- `2`: low-latency online services.
- `4`: general SQL batch writes.
- `8`: COPY FROM / Hologres ingest starting point.

Do not configure more shards than your downstream database/pool can sustain.

### Routing

```go
const (
    ShardRoutingHash ShardRoutingPolicy = iota
    ShardRoutingRoundRobin
    ShardRoutingLeastLoaded
)
```

- `ShardRoutingHash`: keeps the same conflict/routing key on the same shard.
- `ShardRoutingRoundRobin`: spreads writes evenly when key affinity is not needed.
- `ShardRoutingLeastLoaded`: picks the shard with the shortest queue.

Use `ShardKeyFunc` when the default key is not aligned with your business key.

## BackpressureConfig

```go
type BackpressureConfig struct {
    Enabled       bool
    Mode          BackpressureMode
    HighWatermark int
    CheckInterval time.Duration
    Timeout       time.Duration
}
```

Modes:

- `BackpressureTimeout`: recommended production default.
- `BackpressureReject`: best for online APIs where callers can retry.
- `BackpressureBlock`: safe for offline jobs, but can hide downstream saturation.

## MemoryLimitConfig

```go
type MemoryLimitConfig struct {
    Enabled         bool
    MaxQueueBytes   int64
    AvgRequestBytes int64
    Mode            BackpressureMode
    CheckInterval   time.Duration
    Timeout         time.Duration
}
```

Recommended formula:

```text
MaxQueueBytes = BufferSize * ShardCount * AvgRequestBytes * 1.5
```

Use `AvgRequestBytes=512` for narrow rows, `1024` or `2048` for wide rows.

## AdaptiveTuningConfig

```go
type AdaptiveTuningConfig struct {
    Enabled bool

    MinFlushSize uint32
    MaxFlushSize uint32

    MinFlushInterval time.Duration
    MaxFlushInterval time.Duration

    ScaleUpQueueDepth   int
    ScaleDownQueueDepth int

    TargetLatency time.Duration
}
```

The RC2 adaptive tuner is policy-only. It returns recommendations and does not mutate live runtime settings automatically.

Recommended baseline:

```go
cfg.Runtime.Adaptive = batchflow.AdaptiveTuningConfig{
    Enabled: true,
    MinFlushSize: 100,
    MaxFlushSize: 5000,
    MinFlushInterval: 10 * time.Millisecond,
    MaxFlushInterval: 200 * time.Millisecond,
    ScaleUpQueueDepth: 8000,
    ScaleDownQueueDepth: 1000,
    TargetLatency: 50 * time.Millisecond,
}
```

## SQLOperationConfig

SQL conflict behavior is controlled by `SQLOperationConfig`:

```go
type SQLOperationConfig struct {
    ConflictStrategy             ConflictStrategy
    ConflictColumns              []string
    UpdateColumns                []string
    DeduplicateByConflictColumns bool
}
```

Recommended configuration:

```go
config := batchflow.ConflictUpdateOperationConfig.
    WithConflictColumns("tenant_id", "user_id").
    WithUpdateColumns("name", "email")

schema := batchflow.NewSQLSchema(
    "users",
    config,
    "tenant_id", "user_id", "name", "email", "updated_at",
)
```

Fields:

- `ConflictStrategy`: `ConflictIgnore`, `ConflictUpdate`, or `ConflictReplace`.
- `ConflictColumns`: conflict key columns for PostgreSQL/SQLite `ON CONFLICT (...)` and client-side in-batch coalescing. If omitted, BatchFlow keeps the legacy fallback and uses the first schema column.
- `UpdateColumns`: only applies to `ConflictUpdate`. If omitted, BatchFlow updates all non-conflict columns.
- `DeduplicateByConflictColumns`: enabled by default. Duplicate conflict keys inside one batch are coalesced before SQL generation.

## RetryConfig

```go
Retry: batchflow.RetryConfig{
    Enabled:     true,
    MaxAttempts: 3,
    BackoffBase: 20 * time.Millisecond,
    MaxBackoff:  500 * time.Millisecond,
}
```

Notes:

- `MaxAttempts` includes the first execution.
- Built-in classifiers mark `context.Canceled` and `context.DeadlineExceeded` as non-retryable.
- Structured MySQL/PostgreSQL/Redis errors are classified before string fallback.
- Custom backends can register low-cardinality classifiers with `RegisterErrorClassifier`.

## Observability

`ObservabilityConfig` configures structured logs, sampling, and redaction:

```go
logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

cfg.Pipeline.Observability = batchflow.ObservabilityConfig{
    Logger:             logger,
    Sampler:            batchflow.NewErrorAndSlowSampler(500 * time.Millisecond),
    Redactor:           batchflow.DefaultRedactor(),
    SlowBatchThreshold: 500 * time.Millisecond,
}
```

Recommended production policy:

- Log all error events.
- Sample success events, or log only slow batches.
- Do not log raw rows, SQL args, Redis keys, HTTP bodies, emails, phone numbers, or tokens.
- Custom processors should implement `OperationPreviewer` and return backend, operation, fingerprint, and safe attributes.

## SQL Dry Run

Use `GenerateSQLPreview` to inspect final SQL before execution:

```go
preview, err := batchflow.GenerateSQLPreview(ctx, batchflow.DefaultPostgreSQLDriver, schema, rows)
if err != nil {
    return err
}

log.Printf("sql dry-run: table=%s fingerprint=%s args=%d input=%d output=%d dedup=%d",
    preview.Table,
    preview.Fingerprint,
    preview.ArgsCount,
    preview.DedupStats.InputRows,
    preview.DedupStats.OutputRows,
    preview.DedupStats.DeduplicatedRows,
)
```

`preview.Args` contains raw values and may include sensitive data. Do not print it in production logs by default.

## Tuning profiles

General SQL:

```go
cfg.Pipeline.BufferSize = 10000
cfg.Pipeline.FlushSize = 1000
cfg.Pipeline.FlushInterval = 50 * time.Millisecond
cfg.Runtime.ShardCount = 4
```

COPY FROM / Hologres:

```go
cfg.Pipeline.BufferSize = 50000
cfg.Pipeline.FlushSize = 5000
cfg.Pipeline.FlushInterval = 20 * time.Millisecond
cfg.Runtime.ShardCount = 8
```

Low latency:

```go
cfg.Pipeline.BufferSize = 5000
cfg.Pipeline.FlushSize = 100
cfg.Pipeline.FlushInterval = 10 * time.Millisecond
cfg.Runtime.ShardCount = 2
```

## Shutdown

- Always call `Close()` during shutdown so the last batch is flushed.
- Use `Wait()` only when another owner closes input.
- Do not rely on `FlushInterval` as the only final-drain mechanism.
