# BatchFlow pgxcopy adapter

This adapter connects `github.com/jackc/pgx/v5/pgxpool` to BatchFlow's `CopyFromExecutor`.

## Module path

```text
github.com/rushairer/batchflow/adapters/pgxcopy
```

This is an independent Go module with its own `go.mod`.

## Install

```bash
go get github.com/rushairer/batchflow/v2@v2.0.0-rc.2
go get github.com/rushairer/batchflow/adapters/pgxcopy@v0.1.0-rc.2
```

In this repository, the adapter release is tagged as:

```text
adapters/pgxcopy/v0.1.0-rc.2
```

The root BatchFlow runtime is tagged separately as:

```text
v2.0.0-rc.2
```

## Usage

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

flow, err := batchflow.New(ctx, cfg)
```

COPY FROM is append-only. Use BatchFlow SQL executors for upsert/update/replace semantics.
