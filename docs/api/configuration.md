# Configuration

This page covers configuration that application code is expected to use. Docker and integration-test environment variables are documented in [Integration Tests](../guides/integration-tests.md).

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

`DefaultPipelineConfig()` provides production-oriented defaults. `NewBatchFlowWithConfig(ctx, BatchFlowConfig{...})` validates configuration. Legacy constructors remain available for compatibility.

`Coalescer` is for non-SQL backends such as Redis, HTTP, document databases, queues, or custom APIs. SQL backends use `SQLOperationConfig.ConflictColumns` for conflict-key coalescing so SQL dry-run output can report deduplication statistics.

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

Database-specific semantics:

- PostgreSQL `ConflictIgnore`: `ON CONFLICT (cols...) DO NOTHING`.
- PostgreSQL `ConflictUpdate`: `ON CONFLICT (cols...) DO UPDATE SET update_col = EXCLUDED.update_col`.
- PostgreSQL `ConflictReplace`: upsert overwrite using `DO UPDATE SET` for all non-conflict columns.
- MySQL `ConflictIgnore`: `INSERT IGNORE`.
- MySQL `ConflictUpdate`: `ON DUPLICATE KEY UPDATE`, excluding conflict columns unless explicitly allowed by the configured update columns.
- MySQL `ConflictReplace`: native `REPLACE INTO`, which may behave as delete plus insert.

## Core Fields

### BufferSize

- Internal input channel capacity.
- Larger buffers absorb submit bursts, but may increase tail queue latency.
- Start with `2x` to `10x` of `FlushSize`.

### FlushSize

- Number of records that triggers an immediate flush.
- Larger batches usually increase throughput, but also increase execution latency and memory.
- Starting points: `100-500` for OLTP writes, `500-2000` for logs or bulk sync.

### FlushInterval

- Maximum wait before flushing a partial batch.
- Shorter intervals favor latency; longer intervals favor throughput.
- Starting points: `50ms-200ms` for low latency, `200ms-1s` for throughput-first jobs.

### Timeout

- Per-execution timeout for SQL/Redis/custom processors.
- Use it when slow backend execution should fail fast and feed retry classification.

### Retry

Recommended baseline:

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

### MetricsReporter

Pass a reporter through `PipelineConfig.MetricsReporter`.

Recommended starting point:

```go
import prommetrics "github.com/rushairer/batchflow/v2/examples/metrics/prometheus"
```

The Prometheus example implements `OperationMetricsReporter`, `PipelineMetricsReporter`, `BatchFlowMetricsReporter`, and SQL-specific diagnostics.

### Observability

`ObservabilityConfig` configures structured logs, sampling, and redaction:

```go
logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

flow := batchflow.NewRedisBatchFlow(ctx, redisClient, batchflow.PipelineConfig{
	BufferSize:       5000,
	FlushSize:        200,
	FlushInterval:    100 * time.Millisecond,
	MetricsReporter:  reporter,
	Observability: batchflow.ObservabilityConfig{
		Logger:             logger,
		Sampler:            batchflow.NewErrorAndSlowSampler(500 * time.Millisecond),
		Redactor:           batchflow.DefaultRedactor(),
		SlowBatchThreshold: 500 * time.Millisecond,
	},
})
```

Recommended production policy:

- Log all error events.
- Sample success events, or log only slow batches.
- Do not log raw rows, SQL args, Redis keys, HTTP bodies, emails, phone numbers, or tokens.
- Custom processors should implement `OperationPreviewer` and return backend, operation, fingerprint, and safe attributes.

### SQL Dry Run

Use `GenerateSQLPreview` to inspect final SQL before execution:

```go
preview, err := batchflow.GenerateSQLPreview(ctx, batchflow.DefaultPostgreSQLDriver, schema, rows)
if err != nil {
	var sqlErr *batchflow.SQLError
	if errors.As(err, &sqlErr) {
		log.Printf("sql generate failed: stage=%s table=%s conflict=%v update=%v args=%d cause=%v",
			sqlErr.Stage, sqlErr.Table, sqlErr.ConflictColumns, sqlErr.UpdateColumns, sqlErr.ArgsCount, sqlErr.Cause)
	}
	return err
}

log.Printf("sql dry-run: table=%s fingerprint=%s args=%d input=%d output=%d dedup=%d merged=%d sql=%s",
	preview.Table,
	preview.Fingerprint,
	preview.ArgsCount,
	preview.DedupStats.InputRows,
	preview.DedupStats.OutputRows,
	preview.DedupStats.DeduplicatedRows,
	preview.DedupStats.MergedRows,
	preview.SQL,
)
```

`preview.Args` contains raw values and may include sensitive data. Do not print it in production logs by default.

### ConcurrencyLimit

- Limits concurrent `ExecuteBatch` calls.
- It applies at executor entry, not during `Submit`.
- `<= 0` means unlimited.
- Start with `4-8` for database backends and keep it below the database connection pool capacity.

## Tuning Profiles

Low latency:

```go
batchflow.PipelineConfig{
	BufferSize:       500,
	FlushSize:        100,
	FlushInterval:    50 * time.Millisecond,
	ConcurrencyLimit: 4,
}
```

Throughput first:

```go
batchflow.PipelineConfig{
	BufferSize:       5000,
	FlushSize:        500,
	FlushInterval:    200 * time.Millisecond,
	ConcurrencyLimit: 8,
}
```

With retry and metrics:

```go
batchflow.PipelineConfig{
	BufferSize:       2000,
	FlushSize:        200,
	FlushInterval:    100 * time.Millisecond,
	ConcurrencyLimit: 8,
	MetricsReporter:  reporter,
	Retry: batchflow.RetryConfig{
		Enabled:     true,
		MaxAttempts: 3,
		BackoffBase: 20 * time.Millisecond,
		MaxBackoff:  500 * time.Millisecond,
	},
}
```

## Shutdown

- Always call `Close()` during shutdown so the last batch is flushed.
- Use `Wait()` only when another owner closes input.
- Do not rely on `FlushInterval` as the only final-drain mechanism.
