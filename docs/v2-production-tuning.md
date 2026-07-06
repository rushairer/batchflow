# BatchFlow V2 Production Tuning Guide

This guide defines the recommended defaults for the stable `github.com/rushairer/batchflow/v2` runtime.

## Recommended defaults

### General SQL batch writes

```go
cfg := batchflow.DefaultV2Config(executor)
cfg.Pipeline.BatchSize = 1000
cfg.Pipeline.FlushInterval = 50 * time.Millisecond
cfg.Pipeline.BufferSize = 10000
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
```

### Hologres / PostgreSQL COPY path

```go
cfg := batchflow.DefaultV2Config(copyExecutor)
cfg.Pipeline.BatchSize = 5000
cfg.Pipeline.FlushInterval = 20 * time.Millisecond
cfg.Pipeline.BufferSize = 50000
cfg.Runtime.ShardCount = 8
cfg.Runtime.Routing = batchflow.ShardRoutingHash
cfg.Runtime.Backpressure = batchflow.BackpressureConfig{
    Enabled: true,
    Mode: batchflow.BackpressureTimeout,
    HighWatermark: 40000,
    Timeout: 1 * time.Second,
}
cfg.Runtime.MemoryLimit = batchflow.MemoryLimitConfig{
    Enabled: true,
    MaxQueueBytes: 1 << 30,
    AvgRequestBytes: 512,
    Mode: batchflow.BackpressureTimeout,
    Timeout: 1 * time.Second,
}
```

### Low-latency online writes

```go
cfg.Pipeline.BatchSize = 100
cfg.Pipeline.FlushInterval = 10 * time.Millisecond
cfg.Pipeline.BufferSize = 5000
cfg.Runtime.ShardCount = 2
cfg.Runtime.Backpressure = batchflow.BackpressureConfig{
    Enabled: true,
    Mode: batchflow.BackpressureReject,
    HighWatermark: 4000,
}
cfg.Runtime.MemoryLimit = batchflow.MemoryLimitConfig{
    Enabled: true,
    MaxQueueBytes: 128 << 20,
    AvgRequestBytes: 512,
    Mode: batchflow.BackpressureReject,
}
```

## Shard count

Start with:

- `2` shards for low latency services.
- `4` shards for normal SQL batch inserts.
- `8` shards for COPY FROM / Hologres ingest.
- Do not exceed database connection pool capacity.

## Backpressure mode

- `BackpressureTimeout`: recommended production default.
- `BackpressureReject`: best for online APIs where callers can retry.
- `BackpressureBlock`: safe for offline jobs, but can hide downstream saturation.

## Memory limit

Use estimated queue memory instead of Go heap sampling in the hot path.

Recommended formula:

```text
MaxQueueBytes = BufferSize * ShardCount * AvgRequestBytes * 1.5
```

For wide rows, set `AvgRequestBytes` to `1024` or `2048`.

## Adaptive tuning

The `AdaptiveTuner` is intentionally policy-only. It returns recommendations but does not mutate live pipeline settings by default. Apply changes through your own control plane after observing stability windows.

Recommended adaptive config:

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

## Benchmark command

```bash
go test ./benchmark -bench=. -benchmem
```

Run also:

```bash
go test ./... -race
```

before tagging the stable v2 release.
