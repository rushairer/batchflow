# BatchFlow API Reference

This document describes the public API recommended for BatchFlow `v2.0.0-rc.2`.

The module path is:

```text
github.com/rushairer/batchflow/v2
```

The runtime API intentionally uses clean names. Do not add `V2` or `V3` to public type or function names.

## Runtime entrypoint

```go
type Flow = RuntimeEngine

func DefaultConfig(executor BatchExecutor) Config
func New(ctx context.Context, cfg Config) (*Flow, error)
```

Typical usage:

```go
executor := batchflow.NewSQLThrottledBatchExecutorWithDriver(db, batchflow.DefaultPostgreSQLDriver)

cfg := batchflow.DefaultConfig(executor)
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

`Config` separates three concerns:

- `Pipeline`: queue, flush, retry, metrics, observability, and executor-level concurrency.
- `Runtime`: sharding, routing, backpressure, memory limit, and adaptive tuning policy.
- `Executor`: SQL, Redis, COPY FROM, or custom batch sink.

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

### Shard routing

```go
type ShardRoutingPolicy uint8

const (
    ShardRoutingHash ShardRoutingPolicy = iota
    ShardRoutingRoundRobin
    ShardRoutingLeastLoaded
)

type ShardKeyFunc func(*Request) uint64
```

### Backpressure

```go
type BackpressureMode uint8

const (
    BackpressureBlock BackpressureMode = iota
    BackpressureReject
    BackpressureTimeout
)

type BackpressureConfig struct {
    Enabled       bool
    Mode          BackpressureMode
    HighWatermark int
    CheckInterval time.Duration
    Timeout       time.Duration
}
```

Backpressure protects each shard queue from unbounded growth.

### Memory limit

```go
var ErrMemoryLimitExceeded error

type MemoryLimitConfig struct {
    Enabled         bool
    MaxQueueBytes   int64
    AvgRequestBytes int64
    Mode            BackpressureMode
    CheckInterval   time.Duration
    Timeout         time.Duration
}
```

The limiter estimates queued memory as:

```text
sum(queueDepth(shard)) * AvgRequestBytes
```

It is deliberately estimation-based to avoid heap walks or expensive measurements in the submit hot path.

### Adaptive tuning

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

type TuningSignal struct {
    LatencyAvg    time.Duration
    QueueDepth    int
    ErrorRate     float64
    ThroughputRPS float64
}

type TuningRecommendation struct {
    FlushSize      uint32
    FlushInterval  time.Duration
    BackpressureHW int
    ShardCount     uint32
}

type AdaptiveTuner struct { /* unexported */ }

func NewAdaptiveTuner(cfg AdaptiveTuningConfig) *AdaptiveTuner
func (t *AdaptiveTuner) Tune(signal TuningSignal) TuningRecommendation
```

`AdaptiveTuner` is policy-only in RC2. It returns recommendations and does not mutate live pipeline settings by default.

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

```go
func DefaultPipelineConfig() PipelineConfig
func (c PipelineConfig) Validate() error
```

## Flow lifecycle

```go
func (f *Flow) Submit(ctx context.Context, request *Request) error
func (f *Flow) ErrorChan(size int) <-chan error
func (f *Flow) Close() error
func (f *Flow) Wait() error
func (f *Flow) Done() <-chan struct{}
```

Semantics:

- `Submit` only enqueues data. It may reject/block/timeout according to runtime backpressure and memory limit.
- `ErrorChan` returns asynchronous execution errors.
- `Close` is idempotent. The first call closes input and waits for final flush.
- `Wait` waits for background shutdown without closing input.
- `Done` is closed when the background runtime exits.

## Executors

```go
type BatchExecutor interface {
    ExecuteBatch(ctx context.Context, schema SchemaInterface, data []map[string]any) error
}
```

SQL executor:

```go
func NewSQLThrottledBatchExecutorWithDriver(db *sql.DB, driver SQLDriver) *ThrottledBatchExecutor
```

Redis executor:

```go
func NewRedisThrottledBatchExecutor(client *redis.Client) *ThrottledBatchExecutor
func NewRedisThrottledBatchExecutorWithDriver(client *redis.Client, driver RedisDriver) *ThrottledBatchExecutor
```

Generic executor wrapper:

```go
func NewThrottledBatchExecutor(processor BatchProcessor) *ThrottledBatchExecutor
```

COPY FROM executor:

```go
type CopyFromClient interface {
    CopyFrom(ctx context.Context, table string, columns []string, rows [][]any) (int64, error)
}

type CopyFromExecutor struct { /* unexported */ }

func NewCopyFromExecutor(client CopyFromClient) *CopyFromExecutor
func (e *CopyFromExecutor) WithTimeout(timeout time.Duration) *CopyFromExecutor
```

COPY FROM supports append-only SQL schemas only. Use SQL executors for upsert/update/replace flows.

## Legacy convenience constructors

These constructors remain available for simple migrations and quick tests:

```go
func NewBatchFlow(ctx context.Context, bufferSize uint32, flushSize uint32, flushInterval time.Duration, executor BatchExecutor) *BatchFlow
func NewMySQLBatchFlow(ctx context.Context, db *sql.DB, config PipelineConfig) *BatchFlow
func NewPostgreSQLBatchFlow(ctx context.Context, db *sql.DB, config PipelineConfig) *BatchFlow
func NewSQLiteBatchFlow(ctx context.Context, db *sql.DB, config PipelineConfig) *BatchFlow
func NewRedisBatchFlow(ctx context.Context, db *redis.Client, config PipelineConfig) *BatchFlow
```

For new production code, prefer `DefaultConfig(executor)` plus `New(ctx, cfg)`.

## RetryConfig

```go
type RetryConfig struct {
    Enabled     bool
    MaxAttempts int
    BackoffBase time.Duration
    MaxBackoff  time.Duration
    Classifier  func(error) (retryable bool, reason string)
}
```

Notes:

- `MaxAttempts` is the total number of attempts, including the first execution.
- Built-in classification treats `context.Canceled` and `context.DeadlineExceeded` as non-retryable.
- Default reasons include `deadlock`, `lock_timeout`, `timeout`, `connection`, `io`, `duplicate_key`, `syntax`, and `non_retryable`.

## Schema

```go
type SchemaInterface interface {
    Name() string
    Columns() []string
}

type Schema struct { /* unexported */ }
type SQLSchema struct { /* unexported */ }

func NewSchema(name string, columns ...string) *Schema
func NewSQLSchema(name string, operationConfig SQLOperationConfig, columns ...string) *SQLSchema
```

SQL conflict strategy:

```go
type ConflictStrategy uint8

const (
    ConflictIgnore ConflictStrategy = iota
    ConflictReplace
    ConflictUpdate
)
```

SQL operation config:

```go
type SQLOperationConfig struct {
    ConflictStrategy             ConflictStrategy
    ConflictColumns              []string
    UpdateColumns                []string
    DeduplicateByConflictColumns bool
}

func (c SQLOperationConfig) WithConflictColumns(cols ...string) SQLOperationConfig
func (c SQLOperationConfig) WithUpdateColumns(cols ...string) SQLOperationConfig
func (c SQLOperationConfig) WithDeduplicateByConflictColumns(enabled bool) SQLOperationConfig
```

Recommended SQL code should always set explicit `ConflictColumns` for update/replace flows.

## Request

```go
type Request struct { /* unexported */ }

func NewRequest(schema SchemaInterface) *Request
func (r *Request) Schema() SchemaInterface
func (r *Request) Columns() map[string]any
func (r *Request) Validate() error

func (r *Request) SetInt(name string, value int) *Request
func (r *Request) SetInt8(name string, value int8) *Request
func (r *Request) SetInt16(name string, value int16) *Request
func (r *Request) SetInt32(name string, value int32) *Request
func (r *Request) SetInt64(name string, value int64) *Request
func (r *Request) SetUint(name string, value uint) *Request
func (r *Request) SetUint8(name string, value uint8) *Request
func (r *Request) SetUint16(name string, value uint16) *Request
func (r *Request) SetUint32(name string, value uint32) *Request
func (r *Request) SetUint64(name string, value uint64) *Request
func (r *Request) SetFloat32(name string, value float32) *Request
func (r *Request) SetFloat64(name string, value float64) *Request
func (r *Request) SetString(name string, value string) *Request
func (r *Request) SetBool(name string, value bool) *Request
func (r *Request) SetTime(name string, value time.Time) *Request
func (r *Request) SetBytes(name string, value []byte) *Request
func (r *Request) SetNull(name string) *Request
func (r *Request) Set(name string, value any) *Request
```

## SQL preview

```go
func GenerateSQLPreview(ctx context.Context, driver SQLDriver, schema *SQLSchema, data []map[string]any) (*SQLPreview, error)
```

Use SQL preview before production rollout for MySQL/PostgreSQL update or replace flows. Do not log raw args unless they are safe.
