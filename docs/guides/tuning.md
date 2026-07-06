# Tuning Guide

This guide documents the RC2 tuning model implemented in `github.com/rushairer/batchflow/v2`.

## Goals

- Keep production defaults predictable.
- Make runtime controls explicit: sharding, backpressure, memory limit, and adaptive policy.
- Keep adaptive tuning policy-only in RC2 to avoid runtime oscillation.
- Provide repeatable benchmark commands before final v2.0.0 tagging.

## Baseline runtime config

```go
cfg := batchflow.DefaultConfig(executor)
cfg.Pipeline.BufferSize = 10000
cfg.Pipeline.FlushSize = 1000
cfg.Pipeline.FlushInterval = 50 * time.Millisecond
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

## Pipeline knobs

### BufferSize

- Internal input channel capacity per shard.
- Larger buffers absorb bursts but increase queued memory and tail latency.
- Start with `5x` to `10x` `FlushSize`.

### FlushSize

- Number of records that triggers an immediate flush.
- Larger flushes usually improve throughput but increase per-batch latency and memory.
- Starting points:
  - low latency: `100`
  - general SQL: `1000`
  - COPY FROM / Hologres: `5000`

### FlushInterval

- Maximum wait time before flushing a partial batch.
- Starting points:
  - low latency: `10ms`
  - general SQL: `50ms`
  - COPY FROM / Hologres: `20ms`

## Runtime knobs

### ShardCount

- `1`: default simple mode.
- `2`: low-latency online services.
- `4`: general SQL writes.
- `8`: COPY FROM / Hologres ingest.

Keep shard count below downstream pool and write capacity.

### Backpressure

Use `BackpressureTimeout` for most services:

```go
cfg.Runtime.Backpressure = batchflow.BackpressureConfig{
    Enabled: true,
    Mode: batchflow.BackpressureTimeout,
    HighWatermark: 8000,
    Timeout: 500 * time.Millisecond,
}
```

Use `BackpressureReject` for online APIs where upstream callers can retry quickly.

### Memory limit

The memory limiter estimates queued memory as:

```text
sum(queueDepth(shard)) * AvgRequestBytes
```

Recommended formula:

```text
MaxQueueBytes = BufferSize * ShardCount * AvgRequestBytes * 1.5
```

Example:

```go
cfg.Runtime.MemoryLimit = batchflow.MemoryLimitConfig{
    Enabled: true,
    MaxQueueBytes: 512 << 20,
    AvgRequestBytes: 512,
    Mode: batchflow.BackpressureTimeout,
    Timeout: 500 * time.Millisecond,
}
```

## Adaptive tuning

`AdaptiveTuner` exists in RC2 and is intentionally policy-only. It returns recommendations; it does not mutate live settings automatically.

```go
tuner := batchflow.NewAdaptiveTuner(batchflow.AdaptiveTuningConfig{
    Enabled: true,
    MinFlushSize: 100,
    MaxFlushSize: 5000,
    MinFlushInterval: 10 * time.Millisecond,
    MaxFlushInterval: 200 * time.Millisecond,
    ScaleUpQueueDepth: 8000,
    ScaleDownQueueDepth: 1000,
    TargetLatency: 50 * time.Millisecond,
})

rec := tuner.Tune(batchflow.TuningSignal{
    LatencyAvg: 75 * time.Millisecond,
    QueueDepth: 9000,
    ErrorRate: 0,
    ThroughputRPS: 100000,
})
```

Apply `TuningRecommendation` through a deployment/control plane after observing a stability window. Do not continuously mutate live settings on every sample.

## Profiles

### Low latency

```go
cfg.Pipeline.BufferSize = 5000
cfg.Pipeline.FlushSize = 100
cfg.Pipeline.FlushInterval = 10 * time.Millisecond
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

### General SQL

```go
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
```

### COPY FROM / Hologres

```go
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

## Benchmark and validation

Run:

```bash
go test ./benchmark -bench=. -benchmem
```

Before release tagging, run:

```bash
go test ./...
go test ./... -race
cd adapters/pgxcopy && go test ./...
```

## Common questions

### Will adaptive tuning cause oscillation?

It can if applied too frequently. RC2 returns recommendations only. Apply changes after a stability window and keep min/max boundaries tight.

### Should I increase shard count or concurrency first?

Increase executor concurrency only until the downstream pool saturates. Increase shard count when submit-side queueing or single-shard key hot spots dominate.

### Should SQLite use sharding?

Usually no. SQLite has single-writer constraints; prefer low concurrency and small flush sizes, or move high-throughput ingest to MySQL/PostgreSQL/Redis/COPY.
