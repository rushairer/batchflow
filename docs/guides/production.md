# Production Guide

This guide summarizes the minimum production checklist for BatchFlow deployments.

## Configuration Baseline

Start with conservative values and tune with metrics:

```go
flow := batchflow.NewPostgreSQLBatchFlow(ctx, db, batchflow.PipelineConfig{
	BufferSize:       5000,
	FlushSize:        200,
	FlushInterval:    100 * time.Millisecond,
	Timeout:          2 * time.Second,
	ConcurrencyLimit: 8,
	Retry: batchflow.RetryConfig{
		Enabled:     true,
		MaxAttempts: 3,
		BackoffBase: 20 * time.Millisecond,
		MaxBackoff:  500 * time.Millisecond,
	},
	MetricsReporter: reporter,
	Observability: batchflow.ObservabilityConfig{
		Logger:             logger,
		Sampler:            batchflow.NewErrorAndSlowSampler(500 * time.Millisecond),
		Redactor:           batchflow.DefaultRedactor(),
		SlowBatchThreshold: 500 * time.Millisecond,
	},
})
```

## Database Connection Pools

BatchFlow does not own `*sql.DB` pool sizing. Configure it in the application:

```go
db.SetMaxOpenConns(64)
db.SetMaxIdleConns(32)
db.SetConnMaxLifetime(time.Hour)
db.SetConnMaxIdleTime(10 * time.Minute)
```

Keep `ConcurrencyLimit` below the database pool and backend write capacity. If execute latency rises while CPU is low, inspect connection waits and database locks before increasing BatchFlow concurrency.

## Timeouts and Retries

- Use caller context deadlines for request lifetime.
- Use `PipelineConfig.Timeout` for backend execution limits.
- Keep retry attempts small, usually 2 or 3 total attempts.
- Retry only transient reasons such as `deadlock`, `lock_timeout`, `timeout`, `connection`, and `io`.
- Do not retry `duplicate_key`, `syntax`, or validation errors by default.

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

## Non-SQL and DIY Backends

For HTTP, document stores, message queues, or other custom sinks, prefer:

1. Implement `BatchProcessor` to generate backend operations and execute them.
2. Implement `OperationPreviewer` to provide safe diagnostics.
3. Run through `ThrottledBatchExecutor` to reuse retry, concurrency limit, metrics, and observer logic.

See [custom examples](../../examples/custom).

## Readiness Checklist

- [ ] `Close()` is called on shutdown.
- [ ] `ErrorChan` is consumed or intentionally ignored.
- [ ] `MetricsReporter` is configured.
- [ ] `ObservabilityConfig` redacts sensitive fields.
- [ ] SQL update/replace flows use explicit `ConflictColumns`.
- [ ] PostgreSQL/MySQL write paths were checked with `GenerateSQLPreview`.
- [ ] Retry policy uses low-cardinality reason labels.
- [ ] Integration tests have run against the target backend.
