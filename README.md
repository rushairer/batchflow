# BatchFlow

[![Release](https://img.shields.io/github/v/release/rushairer/batchflow?display_name=tag&include_prereleases&sort=semver)](https://github.com/rushairer/batchflow/releases)
[![Go Reference](https://pkg.go.dev/badge/github.com/rushairer/batchflow/v2.svg)](https://pkg.go.dev/github.com/rushairer/batchflow/v2)
[![Go Report Card](https://goreportcard.com/badge/github.com/rushairer/batchflow/v2)](https://goreportcard.com/report/github.com/rushairer/batchflow/v2)
[![License](https://img.shields.io/github/license/rushairer/batchflow)](https://github.com/rushairer/batchflow/blob/main/LICENSE)

BatchFlow is a Go batch-processing framework built on [go-pipeline](https://github.com/rushairer/go-pipeline). It provides one ingestion model for SQL databases, Redis, and custom batch sinks: collect records, flush asynchronously, execute with optional retry/concurrency control, and expose metrics plus safe diagnostics.

Chinese documentation: [README.zh-CN.md](README.zh-CN.md).

## Features

- Unified API for MySQL, PostgreSQL, SQLite, Redis, and custom `BatchExecutor` implementations.
- Async batching through `FlushSize` and `FlushInterval`.
- SQL upsert controls for explicit conflict keys, update columns, and in-batch duplicate-key coalescing.
- Generic `Coalescer` support for non-SQL and DIY data flows.
- Retry, timeout, concurrency limit, structured error classification, and optional observability hooks.
- Prometheus-ready metrics examples, SQL dry-run previews, and Docker integration/stress test tooling.
- Complete lifecycle controls with `Close()`, `Wait()`, and `Done()`.

## Install

```bash
go get github.com/rushairer/batchflow/v2
```

## Quick Start

```go
package main

import (
	"context"
	"database/sql"
	"log"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/rushairer/batchflow/v2"
)

func main() {
	ctx := context.Background()

	db, err := sql.Open("mysql", "user:password@tcp(localhost:3306)/testdb?parseTime=true")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	flow := batchflow.NewMySQLBatchFlow(ctx, db, batchflow.PipelineConfig{
		BufferSize:       1000,
		FlushSize:        200,
		FlushInterval:    100 * time.Millisecond,
		Timeout:          500 * time.Millisecond,
		ConcurrencyLimit: 8,
		Retry: batchflow.RetryConfig{
			Enabled:     true,
			MaxAttempts: 3,
			BackoffBase: 20 * time.Millisecond,
			MaxBackoff:  500 * time.Millisecond,
		},
	})
	defer func() {
		if err := flow.Close(); err != nil {
			log.Printf("batchflow close: %v", err)
		}
	}()

	schema := batchflow.NewSQLSchema(
		"users",
		batchflow.ConflictUpdateOperationConfig.
			WithConflictColumns("id").
			WithUpdateColumns("name", "email"),
		"id", "name", "email",
	)

	req := batchflow.NewRequest(schema).
		SetUint64("id", 1).
		SetString("name", "alice").
		SetString("email", "alice@example.com")

	if err := flow.Submit(ctx, req); err != nil {
		log.Fatal(err)
	}

	errs := flow.ErrorChan(32)
	go func() {
		for err := range errs {
			log.Printf("batchflow async error: %v", err)
		}
	}()
}
```

## Core Semantics

### Lifecycle

- `NewXxxBatchFlow(...)` starts the background pipeline immediately.
- `Submit(ctx, req)` only enqueues data. It returns an error when the caller context is canceled or the flow is already closed.
- `Close()` stops accepting new records, closes the input channel, triggers the final flush, and waits for shutdown.
- `Wait()` waits for shutdown without closing input.
- `Done()` returns a read-only channel closed when the background pipeline exits.

Always call `Close()` during application shutdown:

```go
if err := flow.Close(); err != nil {
	return err
}
```

### Batching

- The pipeline groups requests by `FlushSize` or `FlushInterval`.
- Each flush is split by `SchemaInterface`.
- Each schema group calls `BatchExecutor.ExecuteBatch(...)` once.
- `ObserveBatchSize(n)` reports the size of one schema execution batch, not the whole flush input size.

### Requests

Use typed setters when available:

```go
req := batchflow.NewRequest(schema).
	SetInt("retry_count", 3).
	SetUint64("id", 42).
	SetString("email", "alice@example.com").
	SetBool("enabled", true)
```

Other values can be set with `Set(name, value)` or `SetNull(name)`.

## SQL Update and Replace

Use explicit conflict keys for PostgreSQL/MySQL upserts:

```go
schema := batchflow.NewSQLSchema(
	"users",
	batchflow.ConflictUpdateOperationConfig.
		WithConflictColumns("tenant_id", "user_id").
		WithUpdateColumns("name", "email"),
	"tenant_id", "user_id", "name", "email", "updated_at",
)
```

Rules:

- If `ConflictColumns` is omitted, BatchFlow keeps the legacy fallback and uses the first schema column. Treat this as compatibility only.
- `ConflictUpdate` updates non-conflict columns by default, or only `UpdateColumns` when configured.
- PostgreSQL `ConflictReplace` means upsert overwrite with `ON CONFLICT (...) DO UPDATE SET ...`.
- MySQL `ConflictReplace` keeps native `REPLACE INTO` semantics.
- In-batch duplicate conflict keys are coalesced before SQL generation to avoid PostgreSQL errors such as "cannot affect row a second time".

Dry-run the final SQL before production rollout:

```go
preview, err := batchflow.GenerateSQLPreview(ctx, batchflow.DefaultPostgreSQLDriver, schema, rows)
if err != nil {
	return err
}
log.Printf("sql=%s fingerprint=%s args=%d input=%d output=%d dedup=%d",
	preview.SQL,
	preview.Fingerprint,
	preview.ArgsCount,
	preview.DedupStats.InputRows,
	preview.DedupStats.OutputRows,
	preview.DedupStats.DeduplicatedRows,
)
```

Do not log `preview.Args` in production unless the values are known to be safe.

## Non-SQL and Custom Flows

For Redis, HTTP, document stores, queues, or any custom sink, use `PipelineConfig.Coalescer` when duplicate keys should be merged before execution:

```go
flow := batchflow.NewRedisBatchFlow(ctx, redisClient, batchflow.PipelineConfig{
	BufferSize:    1000,
	FlushSize:     100,
	FlushInterval: 100 * time.Millisecond,
	Coalescer:     batchflow.NewKeyCoalescer(batchflow.CoalesceKeepLast, "key"),
})
defer flow.Close()
```

For reusable custom sinks, implement `BatchProcessor` and optionally `OperationPreviewer`, then wrap it with `NewThrottledBatchExecutor` to reuse retry, concurrency limit, metrics, and structured logging.

## Observability

BatchFlow exposes three metrics layers:

- `MetricsReporter`: executor and core execution metrics.
- `PipelineMetricsReporter`: queue wait, pipeline processing latency, dropped async errors.
- `BatchFlowMetricsReporter`: rejected submits, flush input size, schema groups per flush.

Prometheus example package:

```go
import prommetrics "github.com/rushairer/batchflow/v2/examples/metrics/prometheus"
```

Structured diagnostics can be configured with `ObservabilityConfig`:

```go
flow := batchflow.NewPostgreSQLBatchFlow(ctx, db, batchflow.PipelineConfig{
	Observability: batchflow.ObservabilityConfig{
		Logger:             logger,
		Sampler:            batchflow.NewErrorAndSlowSampler(500 * time.Millisecond),
		Redactor:           batchflow.DefaultRedactor(),
		SlowBatchThreshold: 500 * time.Millisecond,
	},
})
```

Built-in error classification recognizes structured PostgreSQL SQLSTATE, MySQL error numbers, Redis errors, context cancellation, timeouts, and connection classes. Custom backends can register classifiers with `RegisterErrorClassifier`.

## Documentation

- [Documentation index](docs/index.md)
- [API reference](docs/api/reference.md)
- [Configuration](docs/api/configuration.md)
- [Examples](docs/guides/examples.md)
- [Production guide](docs/guides/production.md)
- [Testing guide](docs/guides/testing.md)
- [Error classification](docs/guides/error-classification.md)
- [Monitoring quickstart](docs/guides/monitoring-quickstart.md)
- [Metrics specification](docs/guides/metrics-spec.md)
- [v2 migration guide](docs/development/migration-v2.md)

## Development

```bash
make fmt
make test
make lint
make docs-check
```

Docker stress tests:

```bash
make docker-postgres-test
make docker-mysql-test
make docker-redis-test
```

Generate a stress report:

```bash
./scripts/run_stress_report.sh
```

## Community and Security

- [Contributing guide](CONTRIBUTING.md)
- [Code of conduct](CODE_OF_CONDUCT.md)
- [Security policy](SECURITY.md)
