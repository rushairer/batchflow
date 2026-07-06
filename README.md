# BatchFlow

[![Release](https://img.shields.io/github/v/release/rushairer/batchflow?display_name=tag&include_prereleases&sort=semver)](https://github.com/rushairer/batchflow/releases)
[![Go Reference](https://pkg.go.dev/badge/github.com/rushairer/batchflow/v2.svg)](https://pkg.go.dev/github.com/rushairer/batchflow/v2)
[![Go Report Card](https://goreportcard.com/badge/github.com/rushairer/batchflow/v2)](https://goreportcard.com/report/github.com/rushairer/batchflow/v2)
[![License](https://img.shields.io/github/license/rushairer/batchflow)](https://github.com/rushairer/batchflow/blob/main/LICENSE)

BatchFlow is a Go ingestion runtime built on [go-pipeline](https://github.com/rushairer/go-pipeline). It provides one batching model for SQL databases, Redis, PostgreSQL/Hologres COPY FROM, and custom batch sinks: enqueue records, flush asynchronously, execute through pluggable backends, apply retry/concurrency controls, and expose production diagnostics.

Chinese documentation: [README.zh-CN.md](README.zh-CN.md).

## Current RC2 API

The public module path is:

```text
github.com/rushairer/batchflow/v2
```

Because v2 has not been formally released yet, the public runtime API intentionally uses clean names instead of version-decorated names:

```go
cfg := batchflow.DefaultConfig(executor)
flow, err := batchflow.New(ctx, cfg)
```

Versioning lives in the module path and release tag, not in type or function names.

## Features

- Clean runtime API: `Config`, `RuntimeConfig`, `Flow`, `DefaultConfig`, and `New`.
- Unified executor model for SQL, Redis, COPY FROM, and custom `BatchExecutor` implementations.
- Runtime sharding with hash, round-robin, and least-loaded routing.
- Runtime backpressure with block, reject, and timeout modes.
- Estimated queue memory limiter for OOM protection.
- Adaptive tuning policy engine for flush-size and latency recommendations.
- SQL upsert controls for explicit conflict keys, update columns, and in-batch duplicate-key coalescing.
- PostgreSQL/Hologres COPY FROM fast path through `CopyFromExecutor` and optional `adapters/pgxcopy`.
- Retry, timeout, concurrency limit, structured error classification, metrics, and safe diagnostics.
- Complete lifecycle controls with `Close()`, `Wait()`, and `Done()`.

## Install

```bash
go get github.com/rushairer/batchflow/v2@v2.0.0-rc.2
```

## Quick Start: SQL runtime

```go
package main

import (
	"context"
	"database/sql"
	"log"
	"time"

	_ "github.com/go-sql-driver/mysql"
	batchflow "github.com/rushairer/batchflow/v2"
)

func main() {
	ctx := context.Background()

	db, err := sql.Open("mysql", "user:password@tcp(localhost:3306)/testdb?parseTime=true")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultMySQLDriver).
		WithConcurrencyLimit(8).
		WithRetryConfig(batchflow.RetryConfig{
			Enabled:     true,
			MaxAttempts: 3,
			BackoffBase: 20 * time.Millisecond,
			MaxBackoff:  500 * time.Millisecond,
		})

	cfg := batchflow.DefaultConfig(executor)
	cfg.Pipeline.BufferSize = 10000
	cfg.Pipeline.FlushSize = 1000
	cfg.Pipeline.FlushInterval = 50 * time.Millisecond
	cfg.Runtime.ShardCount = 4
	cfg.Runtime.Routing = batchflow.ShardRoutingHash
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
		log.Fatal(err)
	}
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
}
```

## COPY FROM fast path

For PostgreSQL/Hologres append-only ingestion, use `CopyFromExecutor`. The root module only depends on the minimal `CopyFromClient` interface. The pgx implementation lives in the optional adapter module:

```bash
go get github.com/rushairer/batchflow/adapters/pgxcopy@v2.0.0-rc.2
```

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

COPY FROM is intentionally limited to append-only schemas. Upsert/update/replace flows should use the SQL executor path.

## Core Semantics

### Lifecycle

- `New(ctx, cfg)` starts the runtime immediately.
- `Submit(ctx, req)` enqueues data and may reject/block/timeout according to runtime backpressure and memory limits.
- `Close()` stops accepting new records, closes input, triggers final flush, and waits for shutdown.
- `Wait()` waits for shutdown without closing input.
- `Done()` returns a read-only channel closed when the background runtime exits.

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

## Legacy convenience constructors

Legacy constructors remain available for simple migrations and quick tests:

```go
flow := batchflow.NewMySQLBatchFlow(ctx, db, batchflow.PipelineConfig{
	BufferSize:    1000,
	FlushSize:     200,
	FlushInterval: 100 * time.Millisecond,
})
```

For new production code, prefer `DefaultConfig(executor)` plus `New(ctx, cfg)` so runtime sharding, memory limiter, and backpressure are explicit.

## Documentation

- [Documentation index](docs/index.md)
- [API reference](docs/api/reference.md)
- [Configuration](docs/api/configuration.md)
- [Examples](docs/guides/examples.md)
- [Production guide](docs/guides/production.md)
- [Production tuning guide](docs/v2-production-tuning.md)
- [RC2 release notes](docs/releases/v2.0.0-rc.2.md)
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

Release validation:

```bash
go test ./...
go test ./... -race
go test ./benchmark -bench=. -benchmem
cd adapters/pgxcopy && go test ./...
```
